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
	edgeId     string
	portgroups []pgCfg
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
			"logical_switch": &schema.Schema{
				Type:     schema.TypeSet,
				Required: true,
				MinItems: 1,
				MaxItems: 10,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"id": &schema.Schema{
							Type:     schema.TypeString,
							Required: true,
						},
						"subnet": &schema.Schema{
							Type:     schema.TypeSet,
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
										Elem:     &schema.Schema{Type: schema.TypeString},
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

	log.Printf("[INFO] Creating NSX Edge DHCP")

	dhcp, err := parseAndValidateResourceData(d)

	if err != nil {
		log.Printf("[ERROR] Configuration validation failed.")
		return err
	}

	log.Printf("[INFO] Adding DHCP configuration '%#v' to Edge '%s'", dhcp, dhcp.edgeId)

	client := meta.(*govnsx.Client)

	// Get Edge details
	edge := nsxresource.NewEdge(client)

	var edgeCfg *nsxtypes.Edge
	if edgeCfg, err = getEdge(edge, dhcp.edgeId); err != nil {
		return err
	}

	// Loop through all the vnics from 0-9 of edge config.
	// If the portgroup matches, add new address group for the
	// existing vnic. Else create a new vnic for the portgroup.
	// If all the vnics are configured, return err

	for _, portgroup := range dhcp.portgroups {

		if err := configureEdgeVnic(portgroup, edgeCfg); err != nil {
			return err
		}
	}

	// Deploy appliance to true
	edgeCfg.Appliances.DeployAppliances = true

	//update edge
	if err = updateEdge(edge, edgeCfg); err != nil {
		return err
	}

	// configure dhcp with the iprange and gw
	ipPools := []nsxtypes.IPPool{}
	for _, portgroup := range dhcp.portgroups {

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

	log.Printf("[INFO] Added DHCP configuration %#v to Edge '%s'",
		dhcpConfigSpec, dhcp.edgeId)

	return resourceNsxEdgeDHCPRead(d, meta)
}

func resourceNsxEdgeDHCPRead(d *schema.ResourceData, meta interface{}) error {

	log.Printf("[INFO] Reading NSX Edge DHCP")

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

	log.Printf("[INFO] The DHCP Configuration of Edge '%s': %v", edgeId, dhcpCfg)

	return nil
}

func resourceNsxEdgeDHCPUpdate(d *schema.ResourceData, meta interface{}) error {

	log.Printf("[INFO] Updating NSX Edge DHCP")

	edgeId := d.Get("edge_id").(string)

	client := meta.(*govnsx.Client)

	// Get Edge details.
	edge := nsxresource.NewEdge(client)

	var edgeCfg *nsxtypes.Edge
	var err error
	if edgeCfg, err = getEdge(edge, edgeId); err != nil {
		return err
	}

	edgeDHCPIPPool := nsxresource.NewEdgeDHCPIPPool(client)

	if d.HasChange("portgroup") {

		oldPg, newPg := d.GetChange("portgroup")
		oldPgSet := oldPg.(*schema.Set)
		newPgSet := newPg.(*schema.Set)

		addedPgs := newPgSet.Difference(oldPgSet)
		removedPgs := oldPgSet.Difference(newPgSet)

		log.Printf("[DEBUG] added logical switches  : %#v\n", addedPgs)
		log.Printf("[DEBUG] removed logical switches : %#v\n", removedPgs)

		for _, addedPgRaw := range addedPgs.List() {
			addedPg, _ := addedPgRaw.(map[string]interface{})
			newName := addedPg["id"].(string)

			addedSubnetSet := addedPg["subnet"].(*schema.Set)

			subnetFound := false
			for _, removedPgRaw := range removedPgs.List() {
				removedPg, _ := removedPgRaw.(map[string]interface{})

				removedSubnetSet := removedPg["subnet"].(*schema.Set)

				log.Printf("[DEBUG] addedSubnetSet : %#v\n", addedSubnetSet)
				log.Printf("[DEBUG] removedSubnetSet : %#v\n", removedSubnetSet)

				if addedSubnetSet.Equal(removedSubnetSet) {

					subnetFound = true
					if removedPg["id"].(string) != newName {

						// delete vnic and add vnic with new portgroup
						log.Printf("[DEBUG] Mofifying the logical switch id to %s", newName)

						var portgroup pgCfg
						if portgroup, err = parsePortgroup(removedPg); err != nil {
							return err
						}

						deleteVnic(portgroup, edgeCfg)

						if portgroup, err = parsePortgroup(addedPg); err != nil {
							return err
						}

						if err := configureEdgeVnic(portgroup, edgeCfg); err != nil {
							return err
						}

						addedPgs.Remove(addedPg)
						removedPgs.Remove(removedPg)
					}
					break
				}
			}
			// check for subnet changes
			if subnetFound == false {
				if err := handleSubnetChange(addedPg, addedPgs, removedPgs, edgeDHCPIPPool, edgeCfg); err != nil {
					return err
				}
			}
		}

		log.Printf("[DEBUG] added logical switches after update: %#v\n", addedPgs)
		log.Printf("[DEBUG] removed logical switches after update: %#v\n", removedPgs)

		if len(removedPgs.List()) > 0 {

			removedPortgroups, err := parsePortgroups(removedPgs)
			if err != nil {
				log.Printf("[ERROR] Removed logical switches Configuration validation failed.")
				return err
			}

			// delete vnic, delete ip pool
			if err := removeVnicAndIPPool(removedPgs, removedPortgroups, edgeDHCPIPPool, edgeCfg); err != nil {
				return err
			}
		}

		if len(addedPgs.List()) > 0 {

			addedPortgroups, err := parsePortgroups(addedPgs)

			if err != nil {
				log.Printf("[ERROR] Added logical switches Configuration validation failed.")
				return err
			}

			// add vnic, add ip pool
			if err := configureEdgeVnicAndIPPool(addedPgs, addedPortgroups, edgeDHCPIPPool, edgeCfg); err != nil {
				return err
			}
		}

		// get DHCP configs and set it to Features
		edgeDHCP := nsxresource.NewEdgeDHCP(client)
		edgeDHCPConfig, err := edgeDHCP.Get(edgeId)

		if err != nil {
			log.Printf("[ERROR] Retriving Edge DHCP configuration '%s' failed with error : '%v'", edgeId, err)
			return err
		}

		log.Printf("[DEBUG] Edge '%s'  : '%s'", edgeCfg, edgeId)

		edgeCfg.Features.Dhcp = *edgeDHCPConfig
		//update edge
		if err = updateEdge(edge, edgeCfg); err != nil {
			return err
		}
	}

	return nil
}

func resourceNsxEdgeDHCPDelete(d *schema.ResourceData, meta interface{}) error {

	log.Printf("[INFO] Deleting NSX Edge DHCP")

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

	//remove the vnic details of edge as well

	edge := nsxresource.NewEdge(client)
	var edgeCfg *nsxtypes.Edge
	if edgeCfg, err = getEdge(edge, edgeId); err != nil {
		return err
	}

	dhcp, _ := parseAndValidateResourceData(d)

	for _, portgroup := range dhcp.portgroups {

		deleteVnic(portgroup, edgeCfg)
	}

	//update edge
	if err = updateEdge(edge, edgeCfg); err != nil {
		return err
	}

	return nil
}

func parseAndValidateResourceData(d *schema.ResourceData) (*dhcpCfg, error) {

	dhcp := &dhcpCfg{
		edgeId: d.Get("edge_id").(string),
	}

	var portgroupCfgs []pgCfg
	var err error
	if portgroupCfgs, err = parsePortgroups(d.Get("logical_switch")); err != nil {
		return nil, err
	}

	dhcp.portgroups = portgroupCfgs

	return dhcp, nil
}

func parsePortgroups(vL interface{}) ([]pgCfg, error) {

	portgroupCfgs := []pgCfg{}
	if pgSet, ok := vL.(*schema.Set); ok {

		for _, value := range pgSet.List() {

			var newPortgroup pgCfg
			var err error
			if newPortgroup, err = parsePortgroup(value.(map[string]interface{})); err != nil {
				return nil, err
			}

			portgroupCfgs = append(portgroupCfgs, newPortgroup)
		}
	}
	return portgroupCfgs, nil
}

func parsePortgroup(pgVal map[string]interface{}) (pgCfg, error) {

	newPortgroup := pgCfg{}

	newPortgroup.portgroupName = pgVal["id"].(string)

	var subnets []subnet
	var err error
	if subnets, err = parseSubnets(pgVal["subnet"]); err != nil {
		return newPortgroup, err
	}

	newPortgroup.subnetList = subnets

	return newPortgroup, nil
}

func parseSubnets(vL interface{}) ([]subnet, error) {

	subnetCfgs := []subnet{}
	if subnetSet, ok := vL.(*schema.Set); ok {

		for _, value := range subnetSet.List() {

			var newSubnet subnet
			var err error
			if newSubnet, err = parseSubnet(value.(map[string]interface{})); err != nil {
				return nil, err
			}

			subnetCfgs = append(subnetCfgs, newSubnet)
		}
		// validate same cidr in multiple subnets?? TODO
	}
	return subnetCfgs, nil
}

func parseSubnet(subnetVal map[string]interface{}) (subnet, error) {

	newSubnet := subnet{}
	var err error

	cidr := subnetVal["cidr"].(string)
	newSubnet.cidr = cidr

	_, ipNet, _ := net.ParseCIDR(cidr)

	newSubnet.networkAddr = ipNet.IP.String()
	newSubnet.netMask = fmt.Sprint(
		ipNet.Mask[0], ".", ipNet.Mask[1], ".", ipNet.Mask[2], ".", ipNet.Mask[3])

	gwPresent := false
	var defaultGw net.IP

	if v, ok := subnetVal["default_gw"]; ok && v != "" {
		gwPresent = true
		gw := v.(string)

		defaultGw = net.ParseIP(gw)

		// check if default gateway belongs to subnet
		if ipNet.Contains(defaultGw) {
			newSubnet.defaultGw = gw
		} else {
			return subnet{}, fmt.Errorf("Default Gateway '%s' does not belong to CIDR %s.",
				gw, cidr)
		}
	}

	if raw, ok := subnetVal["ip_pool"]; ok && raw != nil {

		ipRangeCfgs := []ipRange{}
		for _, value := range raw.([]interface{}) {

			newIPRange := ipRange{}

			if err = validateIPRange(value); err != nil {
				return newSubnet, err
			}

			rangeValue := value.(string)

			ip := strings.Split(strings.TrimSpace(rangeValue), "-")

			start := net.ParseIP(strings.TrimSpace(ip[0]))
			end := net.ParseIP(strings.TrimSpace(ip[1]))

			// check if ip range belongs to subnet
			if ipNet.Contains(start) && ipNet.Contains(end) {
				newIPRange = ipRange{start: start,
					end: end}
			} else {
				return newSubnet, fmt.Errorf("IP Range '%s' does not belong to CIDR %s.",
					rangeValue, cidr)
			}

			if gwPresent {
				// check default gw is not part of ip range
				if bytes.Compare(defaultGw, start) >= 0 && bytes.Compare(defaultGw, end) <= 0 {
					return newSubnet, fmt.Errorf("Default Gateway '%s' is part of IP Range %s.",
						defaultGw, rangeValue)
				}
			}

			ipRangeCfgs = append(ipRangeCfgs, newIPRange)
		}

		// Validate all the ip ranges for a subnet and sort the range
		var retIPRange []ipRange
		if retIPRange, err = validateAndSortIPRange(ipRangeCfgs); err != nil {
			return newSubnet, err
		}

		newSubnet.ipRangeList = retIPRange
	} else { // ip_pool not present

		// compute ip range from subnet
		var rangeVal ipRange
		if rangeVal, err = getIPRangeFromCIDR(newSubnet.cidr); err != nil {
			return newSubnet, err
		}

		newSubnet.ipRangeList = append(newSubnet.ipRangeList, rangeVal)

		// remove the gateway address from the range
		if gwPresent {
			newSubnet.ipRangeList = removeGwAddrFromIPRange(rangeVal, defaultGw)
		}
	}

	if !gwPresent {
		// compute gw from ip range
		newSubnet.defaultGw = newSubnet.ipRangeList[0].start.String()

		// move start ip to 1 ahead and assign it to ipRange
		newSubnet.ipRangeList[0].start = intToIP(ipToInt(newSubnet.ipRangeList[0].start) + 1)
	}

	newSubnet.vnicAddr = newSubnet.ipRangeList[0].start.String()
	// move start ip to 1 ahead and assign it to ipRange
	newSubnet.ipRangeList[0].start = intToIP(ipToInt(newSubnet.ipRangeList[0].start) + 1)

	return newSubnet, nil
}

func handleSubnetChange(addedPg map[string]interface{}, addedPgs, removedPgs *schema.Set,
	edgeDHCPIPPool *nsxresource.EdgeDHCPIPPool, edgeCfg *nsxtypes.Edge) error {

	log.Printf("[DEBUG] Handling subnet changes for the logical switch: %s", addedPg["id"].(string))

	addedSubnetSet := addedPg["subnet"].(*schema.Set)

	for _, removedPgRaw := range removedPgs.List() {
		removedPg, _ := removedPgRaw.(map[string]interface{})

		removedSubnetSet := removedPg["subnet"].(*schema.Set)

		// if the subnet is added or removed, the same subnet is added in
		// the other list too. eg, if we add a new subnet inside a pg,
		// the addedPg will have 2 subnets(old & new) and the removedPg
		// will have 1 subnet(old). Hence, removing the common one from both
		//addedSubnets and RemoveSubnets

		commonSubnetSet := addedSubnetSet.Intersection(removedSubnetSet)

		log.Printf("[DEBUG] common subnets : %#v\n", commonSubnetSet)

		for _, commonSubnetsRaw := range commonSubnetSet.List() {
			commonSubnet, _ := commonSubnetsRaw.(map[string]interface{})
			log.Printf("[DEBUG] subnet is same.. removing from add & remove")
			addedSubnetSet.Remove(commonSubnet)
			removedSubnetSet.Remove(commonSubnet)
		}

		if (len(addedSubnetSet.List()) > 0) && (len(removedSubnetSet.List()) > 0) {

			for _, addedSubnetsRaw := range addedSubnetSet.List() {
				addedSubnet, _ := addedSubnetsRaw.(map[string]interface{})

				for _, removedSubnetsRaw := range removedSubnetSet.List() {
					removedSubnet, _ := removedSubnetsRaw.(map[string]interface{})

					log.Printf("[DEBUG] added subnet : %#v\n", addedSubnet)
					log.Printf("[DEBUG] removed subnet : %#v\n", removedSubnet)

					if addedPg["id"] == removedPg["id"] &&
						addedSubnet["cidr"] == removedSubnet["cidr"] {

						// Modified gw
						if addedGw, ok := addedSubnet["default_gw"]; ok {
							if removedGw, ok := removedSubnet["default_gw"]; ok {

								if addedGw != removedGw {

									// delete ip_pool and add new ip_pool
									log.Printf("[DEBUG]  Modified Gateway of the logical switch '%s'", addedPg["id"])

									parseRemovedSubnet, _ := parseSubnet(removedSubnet)
									if err := deleteIPPool([]subnet{parseRemovedSubnet}, edgeDHCPIPPool, edgeCfg); err != nil {
										return err
									}
									parseAddedSubnet, _ := parseSubnet(addedSubnet)
									if err := addIPPool([]subnet{parseAddedSubnet}, edgeDHCPIPPool, edgeCfg.Id); err != nil {
										return err
									}
									break
								}
							}
						}

						log.Printf("[DEBUG] Added Subnet IP Pools : %#v", addedSubnet["ip_pool"])
						log.Printf("[DEBUG] Removed Subnet IP Pools : %#v", removedSubnet["ip_pool"])

						var addedIPPoolRaw, removedIPPoolRaw interface{}
						if v, ok := addedSubnet["ip_pool"]; ok {
							addedIPPoolRaw = v
						}
						if v, ok := removedSubnet["ip_pool"]; ok {
							removedIPPoolRaw = v
						}

						if len(addedIPPoolRaw.([]interface{})) > 0 &&
							len(removedIPPoolRaw.([]interface{})) > 0 {

							for addKey, value := range addedIPPoolRaw.([]interface{}) {
								addedIPPool, _ := value.(string)

								for removeKey, value := range removedIPPoolRaw.([]interface{}) {
									removedIPPool, _ := value.(string)

									// check and remove the common ip pools between added and removed
									if addedIPPool == removedIPPool {
										// IP Pool is same.. removing from add & remove
										log.Printf("[DEBUG] ip pool '%s' is same. Removing from addList and removeList", addedIPPool)
										addedIPPoolRaw = removeFromSlice(addedIPPoolRaw.([]interface{}), addKey)
										removedIPPoolRaw = removeFromSlice(removedIPPoolRaw.([]interface{}), removeKey)

										break
									}
								}
								log.Printf("[DEBUG] Added IP Pool after update : %#v\n", addedIPPoolRaw)
								log.Printf("[DEBUG] removed IP Pool after update : %#v\n", removedIPPoolRaw)
							}
						}

						addedSubnet["ip_pool"] = addedIPPoolRaw
						removedSubnet["ip_pool"] = removedIPPoolRaw

						if len(removedIPPoolRaw.([]interface{})) > 0 {

							// delete ip_pool
							parseRemovedSubnet, _ := parseSubnet(removedSubnet)

							if err := deleteIPPool([]subnet{parseRemovedSubnet}, edgeDHCPIPPool, edgeCfg); err != nil {
								return err
							}
						}

						if len(addedIPPoolRaw.([]interface{})) > 0 {

							// add new ip_pool
							parseAddedSubnet, _ := parseSubnet(addedSubnet)

							if err := addIPPool([]subnet{parseAddedSubnet}, edgeDHCPIPPool, edgeCfg.Id); err != nil {
								return err
							}
						}

						// Only ip Pool Changes and the same has been taken care above.
						// Hence, remove addPg and removePg from the set
						addedPgs.Remove(addedPg)
						removedPgs.Remove(removedPg)

						break
					}
				}
				// cidr will not match.. So have to add new vnic and ip pool.
				// this will be taken care by the caller of this method
			}
		}
	}
	return nil
}

func getEdge(edge *nsxresource.Edge, edgeId string) (*nsxtypes.Edge, error) {

	// Get Edge details
	edgeCfg, err := edge.Get(edgeId)

	if err != nil {
		log.Printf("[ERROR] Retriving Edge '%s' failed with error : '%v'", edgeId, err)
		return nil, err
	}

	log.Printf("[DEBUG] Edge details of '%s': '%v'", edgeId, edgeCfg)
	return edgeCfg, nil
}

func updateEdge(edge *nsxresource.Edge, edgeCfg *nsxtypes.Edge) error {

	edgeUpdateSpec := &nsxtypes.EdgeInstallSpec{
		Tenant:     edgeCfg.Tenant,
		Appliances: edgeCfg.Appliances,
		Vnics:      edgeCfg.Vnics,
		Features:   edgeCfg.Features,
	}

	err := edge.Put(edgeUpdateSpec, edgeCfg.Id)

	if err != nil {
		log.Printf("[ERROR] Updating Edge '%s' for DHCP configuration failed with error : '%v'",
			edgeCfg.Id, err)
		return err
	}

	log.Printf("[INFO] Updated Edge '%s' for DHCP configuration", edgeCfg.Id)

	return nil
}

func configureEdgeVnic(portgroup pgCfg, edgeCfg *nsxtypes.Edge) error {

	pgFound := false
	pgConfigDone := false

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
					log.Printf("[DEBUG] Adding Vnic config '%#v' to Edge %s. Adding addressgroup",
						edgeCfg.Vnics[i], edgeCfg.Id)
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
					log.Printf("[DEBUG] Adding Vnic config '%#v' to Edge %s. Configuring a new vnic",
						edgeCfg.Vnics[i], edgeCfg.Id)
				}
				break
			}
		}
	}
	// Not found any free Vnic, return err
	if !pgFound && !pgConfigDone {
		return fmt.Errorf("No vNic available to configure DHCP to the Edge '%s'", edgeCfg.Id)
	}

	return nil
}

func deleteVnic(portgroup pgCfg, edgeCfg *nsxtypes.Edge) {

	pgFound := false

	for i, vnic := range edgeCfg.Vnics {

		if vnic.PortgroupId == portgroup.portgroupName {

			pgFound = true
			for _, subnetCfg := range portgroup.subnetList {

				for j, addrGroupCfg := range vnic.AddressGroups {

					if isIPInCIDR(subnetCfg.cidr, addrGroupCfg.PrimaryAddress) {

						edgeCfg.Vnics[i].AddressGroups[j] = edgeCfg.Vnics[i].AddressGroups[len(edgeCfg.Vnics[i].AddressGroups)-1]

						edgeCfg.Vnics[i].AddressGroups = edgeCfg.Vnics[i].AddressGroups[:len(
							edgeCfg.Vnics[i].AddressGroups)-1]
						break
					}
				}
			}

			if len(edgeCfg.Vnics[i].AddressGroups) <= 0 {

				edgeCfg.Vnics[i].PortgroupId = ""
				edgeCfg.Vnics[i].IsConnected = false
				edgeCfg.Vnics[i].AddressGroups = []nsxtypes.AddressGroup{}
			}
			break
		}
	}

	// Not found any configured Vnic,
	if !pgFound {
		log.Printf("[INFO] No vNic is configured for the logical switch '%s' to remove from the Edge '%s'", edgeCfg.Id)
	}
}

func configureEdgeVnicAndIPPool(addedPgs *schema.Set, portgroups []pgCfg, edgeDHCPIPPool *nsxresource.EdgeDHCPIPPool,
	edgeCfg *nsxtypes.Edge) error {

	for _, addedPgRaw := range addedPgs.List() {
		addedPg, _ := addedPgRaw.(map[string]interface{})

		for _, portgroup := range portgroups {

			if portgroup.portgroupName == addedPg["id"].(string) {

				if err := configureEdgeVnic(portgroup, edgeCfg); err != nil {
					return err
				}

				// add ip_pool
				if err := addIPPool(portgroup.subnetList, edgeDHCPIPPool, edgeCfg.Id); err != nil {
					return err
				}
				break
			}
		}
	}
	return nil
}

func removeVnicAndIPPool(removedPgs *schema.Set, portgroups []pgCfg, edgeDHCPIPPool *nsxresource.EdgeDHCPIPPool,
	edgeCfg *nsxtypes.Edge) error {

	for _, removedPgRaw := range removedPgs.List() {
		removedPg, _ := removedPgRaw.(map[string]interface{})

		for _, portgroup := range portgroups {

			if len(portgroup.subnetList) > 0 {

				if portgroup.portgroupName == removedPg["id"].(string) {

					deleteVnic(portgroup, edgeCfg)

					// delete ip_pool

					if err := deleteIPPool(portgroup.subnetList, edgeDHCPIPPool, edgeCfg); err != nil {
						return err
					}
					break
				}
			}
		}
	}
	return nil
}

func addIPPool(subnets []subnet, edgeDHCPIPPool *nsxresource.EdgeDHCPIPPool, edgeId string) error {

	for _, subnet := range subnets {

		for _, ipRangeVal := range subnet.ipRangeList {

			ipPoolSpec := &nsxtypes.IPPool{}

			ipPoolSpec.IPRange = getIPRangeString(ipRangeVal)
			ipPoolSpec.DefaultGw = subnet.defaultGw
			ipPoolSpec.SubnetMask = subnet.netMask

			log.Printf("[INFO] Adding DHCP IP Pool '%v' to Edge '%s'",
				ipPoolSpec, edgeId)

			_, err := edgeDHCPIPPool.Post(ipPoolSpec, edgeId)

			if err != nil {
				log.Printf("[ERROR] Adding DHCP IP Pool to Edge '%s' failed with error : '%v'",
					edgeId, err)
				return err
			}
		}
	}

	return nil
}

func deleteIPPool(subnets []subnet, edgeDHCPIPPool *nsxresource.EdgeDHCPIPPool, edgeCfg *nsxtypes.Edge) error {

	for _, subnet := range subnets {

		for _, value := range subnet.ipRangeList {

			for _, dhcpIPPool := range edgeCfg.Features.Dhcp.IPPools {

				ipRangeVal := getIPRangeString(value)

				if ipRangeVal == dhcpIPPool.IPRange {

					log.Printf("[INFO] Deleting DHCP IP Pool '%s' from Edge '%s'",
						ipRangeVal, edgeCfg.Id)

					err := edgeDHCPIPPool.Delete(edgeCfg.Id, dhcpIPPool.PoolId)

					if err != nil {
						log.Printf("[ERROR] Deleting DHCP IP Pool from Edge '%s' failed with error : '%v'",
							edgeCfg.Id, err)

						return err
					}
					break
				}
			}
		}
	}
	return nil
}

func removeFromSlice(s []interface{}, i int) []interface{} {
	s[i] = s[len(s)-1]
	return s[:len(s)-1]
}
