package nsx

import (
	"bytes"
	"fmt"
	"log"
	"net"
	"strings"

	"github.com/IBM-tfproviders/govnsx"
	"github.com/IBM-tfproviders/govnsx/nsxresource"
	"github.com/IBM-tfproviders/govnsx/nsxtypes"
	"github.com/hashicorp/terraform/helper/schema"
)

const (
	DHCPResourceIdPrefix = "dhcp-"
)

type ipRange struct {
	start net.IP
	end   net.IP
}

type subnet struct {
	cidr        string
	defaultGw   string
	networkAddr string // network address eg: 10.10.10.0
	netMask     string // netmask eg: 255.255.255.0
	vnicAddr    string
	ipRangeList []ipRange
}

type pgCfg struct {
	portgroupName string
	subnetList    []subnet
}

type dhcpCfg struct {
	edgeId    string
	portgroup []pgCfg
}

func resourceNsxEdgeDHCP() *schema.Resource {
	return &schema.Resource{
		Create: resourceNsxEdgeDHCPCreate,
		Read:   resourceNsxEdgeDHCPRead,
		Update: resourceNsxEdgeDHCPUpdate,
		Delete: resourceNsxEdgeDHCPDelete,

		Schema: map[string]*schema.Schema{
			"edge_id": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"portgroup": &schema.Schema{
				Type:     schema.TypeList,
				Required: true,
				MinItems: 1,
				MaxItems: 10,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"name": &schema.Schema{
							Type:     schema.TypeString,
							Required: true,
						},
						"subnet": &schema.Schema{
							Type:     schema.TypeList,
							Required: true,
							MinItems: 1,
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"cidr": &schema.Schema{
										Type:         schema.TypeString,
										Required:     true,
										ValidateFunc: validateCidr,
									},
									"default_gw": &schema.Schema{
										Type:         schema.TypeString,
										Optional:     true,
										ValidateFunc: validateIP,
									},
									"ip_pool": &schema.Schema{
										Type:     schema.TypeList,
										Optional: true,
										MinItems: 1,
										Elem: &schema.Resource{
											Schema: map[string]*schema.Schema{
												"ip_range": &schema.Schema{
													Type:         schema.TypeString,
													Optional:     true,
													ValidateFunc: validateIPRange,
												},
												"pool_id": &schema.Schema{
													Type:     schema.TypeString,
													Computed: true,
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func resourceNsxEdgeDHCPCreate(d *schema.ResourceData, meta interface{}) error {

	dhcp, err := parseAndValidateResourceData(d)

	if err != nil {
		log.Printf("[ERROR] Configuration validation failed.")
		return err
	}

	log.Printf("[INFO] Adding DHCP configuration '%#v' to Edge '%s'", dhcp, dhcp.edgeId)

	client := meta.(*govnsx.Client)

	// Get Edge details.
	edge := nsxresource.NewEdge(client)
	edgeCfg, err := edge.Get(dhcp.edgeId)

	if err != nil {
		log.Printf("[ERROR] Retriving Edge '%s' failed with error : '%v'", dhcp.edgeId, err)
		return err
	}

	/* Loop through all the vnics from 0-9 of edge config.
	   If the portgroup matches, add new address group for the
	   existing vnic. Else create a new vnic for the portgroup.
	   If all the vnics are configured, return err */

	pgFound := false
	pgConfigDone := false
	for _, portgroup := range dhcp.portgroup {

		for i, vnic := range edgeCfg.Vnics {

			if vnic.PortgroupId == portgroup.portgroupName {

				pgFound = true
				addrGroupFound := false
				for _, subnetCfg := range portgroup.subnetList {

					// If the subnet is already configured with vnic, ignore
					for _, addrGroupCfg := range vnic.AddressGroups {

						if isIPInCIDR(subnetCfg.cidr, addrGroupCfg.PrimaryAddress) {
							addrGroupFound = true
							break
						}
					}

					if !addrGroupFound {
						addrGroup := nsxtypes.AddressGroup{}
						addrGroup.PrimaryAddress = subnetCfg.vnicAddr
						addrGroup.SubnetMask = subnetCfg.netMask

						edgeCfg.Vnics[i].AddressGroups = append(
							edgeCfg.Vnics[i].AddressGroups, addrGroup)
					}
				}
				break
			}
		}
		if !pgFound {
			// configure a new vnic
			for i, vnic := range edgeCfg.Vnics {
				// check IsConnected. If it is false, configure that vnic
				if vnic.IsConnected == false {

					pgConfigDone = true
					edgeCfg.Vnics[i].PortgroupId = portgroup.portgroupName
					edgeCfg.Vnics[i].IsConnected = true

					for _, subnetCfg := range portgroup.subnetList {

						addrGroup := nsxtypes.AddressGroup{}
						addrGroup.PrimaryAddress = subnetCfg.vnicAddr
						addrGroup.SubnetMask = subnetCfg.netMask

						edgeCfg.Vnics[i].AddressGroups = append(
							edgeCfg.Vnics[i].AddressGroups, addrGroup)
					}
					break
				}
			}
		}
	}
	// Not found any free Vnic, return err
	if !pgFound && !pgConfigDone {
		return fmt.Errorf("No vNic available to configure DHCP to the Edge '%s'", dhcp.edgeId)
	}

	// Deploy appliance to true
	edgeCfg.Appliances.DeployAppliances = true

	edgeUpdateSpec := &nsxtypes.EdgeInstallSpec{
		Tenant:     edgeCfg.Tenant,
		Appliances: edgeCfg.Appliances,
		Vnics:      edgeCfg.Vnics,
	}

	err = edge.Put(edgeUpdateSpec, dhcp.edgeId)

	if err != nil {
		log.Printf("[ERROR] Updating Edge '%s' for DHCP configuration failed with error : '%v'",
			dhcp.edgeId, err)
		return err
	}

	log.Printf("[INFO] Updated Edge '%s' for DHCP configuration", dhcp.edgeId)

	// configure dhcp with the iprange and gw
	ipPools := []nsxtypes.IPPool{}
	for _, portgroup := range dhcp.portgroup {

		for _, subnetCfg := range portgroup.subnetList {

			ipPool := nsxtypes.IPPool{}

			for _, ipRangeVal := range subnetCfg.ipRangeList {
				ipPool.IPRange = getIPRangeString(ipRangeVal)
				ipPool.DefaultGw = subnetCfg.defaultGw
				ipPool.SubnetMask = subnetCfg.netMask
				ipPools = append(ipPools, ipPool)
			}
		}
	}

	dhcpConfigSpec := &nsxtypes.ConfigDHCPServiceSpec{
		IPPools: ipPools,
	}

	edgeDHCP := nsxresource.NewEdgeDHCP(client)
	err = edgeDHCP.Put(dhcpConfigSpec, dhcp.edgeId)

	if err != nil {
		log.Printf("[ERROR] Adding DHCP configuration to Edge '%s' failed with error : '%v'",
			dhcp.edgeId, err)
		return err
	}

	d.SetId(fmt.Sprintf(DHCPResourceIdPrefix + dhcp.edgeId))

	log.Printf("[INFO] Added  DHCP configuration to Edge '%s'", dhcp.edgeId)

	return resourceNsxEdgeDHCPRead(d, meta)
}

func resourceNsxEdgeDHCPRead(d *schema.ResourceData, meta interface{}) error {

	client := meta.(*govnsx.Client)

	edgeId := d.Get("edge_id").(string)

	// Get DHCP Config of edge
	edgeDHCP := nsxresource.NewEdgeDHCP(client)

	dhcpCfg, err := edgeDHCP.Get(edgeId)

	if err != nil {
		log.Printf("[ERROR] Retriving Edge '%s' failed with error : '%v'", edgeId, err)
		d.SetId("")
		return err
	}

	// set pool_id
	//d.Set()

	log.Printf("[INFO] The DHCP Configuration of Edge '%s': %v", edgeId, dhcpCfg)

	return nil
}

func resourceNsxEdgeDHCPUpdate(d *schema.ResourceData, meta interface{}) error {

	log.Printf("[INFO] Updating Nsx Edge DHCP")
	log.Printf("[WARN] Yet to be implemented")
	return nil
}

func resourceNsxEdgeDHCPDelete(d *schema.ResourceData, meta interface{}) error {

	client := meta.(*govnsx.Client)
	edgeDHCP := nsxresource.NewEdgeDHCP(client)

	edgeId := d.Get("edge_id").(string)

	log.Printf("[INFO] Deleting DHCP configuration from Edge: '%s'", edgeId)
	err := edgeDHCP.Delete(edgeId)
	if err != nil {
		log.Printf("[ERROR] Deleting DHCP configuration from Edge '%s' failed with error : '%v'",
			edgeId, err)
		return err
	}

	//Should we remove the vnic details of edge as well? TODO
	return nil
}

func parseAndValidateResourceData(d *schema.ResourceData) (*dhcpCfg, error) {

	dhcp := &dhcpCfg{
		edgeId: d.Get("edge_id").(string),
	}

	portgroupCfgs := []pgCfg{}
	vL := d.Get("portgroup")

	for _, value := range vL.([]interface{}) {

		newPortgroup := pgCfg{}

		portgroup := value.(map[string]interface{})

		newPortgroup.portgroupName = portgroup["name"].(string)

		vL = portgroup["subnet"]

		subnetCfgs := []subnet{}
		for _, value := range vL.([]interface{}) {

			newSubnet := subnet{}

			subnet := value.(map[string]interface{})

			cidr := subnet["cidr"].(string)
			newSubnet.cidr = cidr

			_, ipNet, _ := net.ParseCIDR(cidr)

			newSubnet.networkAddr = ipNet.IP.String()
			newSubnet.netMask = fmt.Sprint(
				ipNet.Mask[0], ".", ipNet.Mask[1], ".", ipNet.Mask[2], ".", ipNet.Mask[3])

			ipRangePresent := false
			gwPresent := false
			var defaultGw net.IP

			if v, ok := subnet["default_gw"]; ok {
				gwPresent = true
				gw := v.(string)

				defaultGw = net.ParseIP(gw)

				// check if default gateway belongs to subnet
				if ipNet.Contains(defaultGw) {
					newSubnet.defaultGw = gw
				} else {
					return nil, fmt.Errorf("Default Gateway '%s' does not belong to CIDR %s.",
						gw, cidr)
				}
			}

			if raw, ok := subnet["ip_pool"]; ok {

				ipRangePresent = true

				ipRangeCfgs := []ipRange{}
				for _, value := range raw.([]interface{}) {

					newIPRange := ipRange{}

					ipPool := value.(map[string]interface{})

					rangeValue := ipPool["ip_range"].(string)

					ip := strings.Split(strings.TrimSpace(rangeValue), "-")

					start := net.ParseIP(strings.TrimSpace(ip[0]))
					end := net.ParseIP(strings.TrimSpace(ip[1]))

					// check if ip range belongs to subnet
					if ipNet.Contains(start) && ipNet.Contains(end) {
						newIPRange = ipRange{start: start,
							end: end}
					} else {
						return nil, fmt.Errorf("IP Range '%s' does not belong to CIDR %s.",
							rangeValue, cidr)
					}

					if gwPresent {
						// check default gw is not part of ip range
						if bytes.Compare(defaultGw, start) >= 0 && bytes.Compare(defaultGw, end) <= 0 {
							return nil, fmt.Errorf("Default Gateway '%s' is part of IP Range %s.",
								defaultGw, rangeValue)
						}
					}

					ipRangeCfgs = append(ipRangeCfgs, newIPRange)
				}

				// Validate all the ip ranges for a subnet like overlapping, etc
				// and sort the range
				retIPRange, err := validateAndSortIPRange(ipRangeCfgs)
				if err != nil {
					return nil, err
				}

				newSubnet.ipRangeList = retIPRange
			}

			if ipRangePresent && gwPresent {

				newSubnet.vnicAddr = newSubnet.ipRangeList[0].start.String()
				// move start ip to 1 ahead and assign it to ipRange
				newSubnet.ipRangeList[0].start = intToIP(ipToInt(newSubnet.ipRangeList[0].start) + 1)
			} else if gwPresent {
				// compute ip range from subnet
				rangeVal, err := getIPRangeFromCIDR(newSubnet.cidr)

				if err != nil {
					return nil, err
				}
				//remove the gateway address from the range
				rangeValCfgs := removeGwAddrFromRange(rangeVal, defaultGw)

				newSubnet.vnicAddr = rangeValCfgs[0].start.String()
				// move start to 1 ip ahead and assign to ipRange
				rangeVal.start = intToIP(ipToInt(rangeValCfgs[0].start) + 1)
				newSubnet.ipRangeList = rangeValCfgs
			} else if ipRangePresent {
				// compute gw from ip range
				newSubnet.defaultGw = newSubnet.ipRangeList[0].start.String()
				vnicIP := intToIP(ipToInt(newSubnet.ipRangeList[0].start) + 1)
				newSubnet.vnicAddr = vnicIP.String()
				// move start ip to 2 ahead and assign it to ipRange
				newSubnet.ipRangeList[0].start = intToIP(ipToInt(newSubnet.ipRangeList[0].start) + 2)
			} else {
				// compute ip range from subnet
				rangeVal, err := getIPRangeFromCIDR(newSubnet.cidr)

				if err != nil {
					return nil, err
				}
				// compute gw from ip_range
				newSubnet.defaultGw = rangeVal.start.String()

				vnicIP := intToIP(ipToInt(rangeVal.start) + 1)
				newSubnet.vnicAddr = vnicIP.String()
				rangeVal.start = intToIP(ipToInt(rangeVal.start) + 2)
				newSubnet.ipRangeList = append(newSubnet.ipRangeList, rangeVal)
			}

			subnetCfgs = append(subnetCfgs, newSubnet)
		}
		newPortgroup.subnetList = subnetCfgs
		portgroupCfgs = append(portgroupCfgs, newPortgroup)
	}
	dhcp.portgroup = portgroupCfgs

	return dhcp, nil
}
