package nsx

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"regexp"
	"strconv"
	"strings"

	"github.com/IBM-tfproviders/govnsx"
	"github.com/IBM-tfproviders/govnsx/nsxresource"
	"github.com/IBM-tfproviders/govnsx/nsxtypes"
	"github.com/hashicorp/terraform/helper/schema"
)

type ipRange struct {
	startIP string
	endIP   string
}

type dhcpCfg struct {
	netAddr   string // network address
	netMask   string // netmask eg: 255.255.255.0
	vnicAddr  string
	defaultGw string
	ipRange
}

type edgeSGW struct {
	edgeName       string
	description    string
	edgeId         string
	version        string
	datacenter     string
	tenantId       string
	resourcePoolId string
	datastoreId    string
	folder         string
	dhcp           map[string][]dhcpCfg
}

func resourceNsxEdgeSGW() *schema.Resource {
	return &schema.Resource{
		Create: resourceNsxEdgeSGWCreate,
		Read:   resourceNsxEdgeSGWRead,
		Update: resourceNsxEdgeSGWUpdate,
		Delete: resourceNsxEdgeSGWDelete,

		Schema: map[string]*schema.Schema{
			"edge_id": &schema.Schema{
				Type:     schema.TypeString,
				Computed: true,
			},
			"version": &schema.Schema{
				Type:     schema.TypeInt,
				Computed: true,
			},
			"datacenter": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: false,
			},
			"tenant_id": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				Default:  "Terraform Provider",
				ForceNew: false,
			},
			"resource_pool_id": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: false,
			},
			"datastore_id": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: false,
			},
			"folder": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: false,
			},
			"edge_name": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: false,
				Default:  "",
			},
			"description": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				Default:  "Created by Terraform",
			},
			"dhcp": &schema.Schema{
				Type:     schema.TypeSet,
				Required: true,
				ForceNew: false,
				MinItems: 1,
				MaxItems: 10, // a vnic can have more than 1 addr group.
				// Hence, if the network is same we can configure the same
				// vnic with the new addr group.  so MaxItem can't be set
				//
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"cidr": &schema.Schema{
							Type:         schema.TypeString,
							Required:     true,
							ForceNew:     false,
							ValidateFunc: validateCidr,
						},
						"default_gw": &schema.Schema{
							Type:         schema.TypeString,
							Optional:     true,
							ForceNew:     false,
							ValidateFunc: validateIP,
						},
						"ip_range": &schema.Schema{
							Type:         schema.TypeString,
							Optional:     true,
							ForceNew:     false,
							ValidateFunc: validateIPRange,
						},
						"portgroup": &schema.Schema{
							Type:     schema.TypeString,
							Required: true,
							ForceNew: false,
						},
					},
				},
			},
		},
	}
}

func resourceNsxEdgeSGWCreate(d *schema.ResourceData, meta interface{}) error {

	sgw, err := NewNsxEdgeSGW(d)

	if err != nil {
		return err
	}

	log.Printf("[INFO] Creating NSX Edge: %#v", sgw)

	client := meta.(*govnsx.Client)

	edge := nsxresource.NewEdge(client) // TODO: what is common, newcommon. etc ?

	vnics := []nsxtypes.Vnic{}
	ipPools := []nsxtypes.IPPool{}
	for portgroup, dhcpCfgs := range sgw.dhcp {

		index := 0
		/* if portgroup is same for multiple dhcp configs,  add 2 address
		groups for the same vnic.. dont create new vnic */
		addrGroups := []nsxtypes.AddressGroup{}
		for _, dhcp := range dhcpCfgs {

			addrGroup := nsxtypes.AddressGroup{}
			addrGroup.PrimaryAddress = dhcp.vnicAddr
			addrGroup.SubnetMask = dhcp.netMask
			addrGroups = append(addrGroups, addrGroup)

			ipPool := nsxtypes.IPPool{}
			ipPool.IPRange = fmt.Sprintf(dhcp.ipRange.startIP + "-" + dhcp.ipRange.endIP)
			ipPool.DefaultGw = dhcp.defaultGw
			ipPool.SubnetMask = dhcp.netMask
			ipPools = append(ipPools, ipPool)
		}

		vnic := nsxtypes.Vnic{}
		vnic.Index = strconv.Itoa(index)
		vnic.PortgroupId = portgroup
		vnic.AddressGroups = addrGroups
		vnic.IsConnected = true
		vnics = append(vnics, vnic)
		index = index + 1
	}

	var appliances = []nsxtypes.Appliance{nsxtypes.Appliance{
		ResourcePoolId: sgw.resourcePoolId,
		DatastoreId:    sgw.datastoreId,
	}}

	edgeInstallSpec := &nsxtypes.EdgeSGWInstallSpec{
		Name:           sgw.edgeName,
		Description:    sgw.description,
		Tenant:         sgw.tenantId,
		AppliancesList: appliances,
		Vnics:          vnics,
	}

	resp, err := edge.Post(edgeInstallSpec)

	if err != nil {
		log.Printf("[Error] edge.Post() returned error : %v", err)
		return err
	}

	log.Printf("[INFO] Created NSX Edge: %s", resp.EdgeId)

	// configure dhcp with the iprange and gw
	var logInfo = nsxtypes.LoggingInfo{Enable: false, LogLevel: "info"}
	dhcpSpec := &nsxtypes.ConfigDHCPServiceSpec{
		IPPools: ipPools,
		Logging: logInfo,
	}

	edgeDhcp := nsxresource.NewEdgeDhcp(client) // TODO: what is common, newcommon. etc ?
	err = edgeDhcp.Put(dhcpSpec, resp.EdgeId)

	if err != nil {
		log.Printf("[Error] edgeDhcp.Put() returned error : %v", err)
		return err
	}

	log.Printf("[INFO] Configured DHCP for NSX Edge: %s", resp.EdgeId)

	d.SetId(resp.EdgeId)
	err = d.Set("edge_id", resp.EdgeId)
	if err != nil {
		return fmt.Errorf("Invalid Edge id to set: %#v", resp.EdgeId)
	}
	return nil
}

func resourceNsxEdgeSGWRead(d *schema.ResourceData, meta interface{}) error {
	// get edgeId d.getId

	// issue Get edge

	log.Printf("[INFO] Reading Nsx Edge SGW")
	log.Printf("[WARN] Yet to be implemented")
	return nil
}

func resourceNsxEdgeSGWUpdate(d *schema.ResourceData, meta interface{}) error {

	// check if dhcp added Update edge vnic and configure dhcp for the new ip range

	// dhcp param update TODO

	log.Printf("[INFO] Updating Nsx Edge SGW")
	log.Printf("[WARN] Yet to be implemented")
	return nil
}

func resourceNsxEdgeSGWDelete(d *schema.ResourceData, meta interface{}) error {

	client := meta.(*govnsx.Client)
	edge := nsxresource.NewEdge(client)

	var edgeId string

	if v, ok := d.GetOk("edge_id"); ok {
		edgeId = v.(string)
	}

	log.Printf("[INFO] Deleting NSX Edge: %s", edgeId)
	err := edge.Delete(edgeId)
	if err != nil {
		log.Printf("[Error] edge.Delete() returned error : %v", err)
		return err
	}
	return nil
}

func NewNsxEdgeSGW(d *schema.ResourceData) (*edgeSGW, error) {

	sgw := &edgeSGW{
		datacenter:     d.Get("datacenter").(string),
		resourcePoolId: d.Get("resource_pool_id").(string),
		datastoreId:    d.Get("datastore_id").(string),
	}

	if v, ok := d.GetOk("tenant_id"); ok {
		sgw.tenantId = v.(string)
	}

	if v, ok := d.GetOk("folder"); ok {
		sgw.folder = v.(string)
	}

	if v, ok := d.GetOk("description"); ok {
		sgw.description = v.(string)
	}

	if v, ok := d.GetOk("edge_name"); ok {
		sgw.edgeName = v.(string)
	}

	vL := d.Get("dhcp")
	if dhcpSet, ok := vL.(*schema.Set); ok {

		dhcpCfgs := make(map[string][]dhcpCfg)
		for _, value := range dhcpSet.List() {

			newDhcp := dhcpCfg{}
			ipRangePresent := false
			gwPresent := false
			var start, end, defaultGw net.IP

			dhcp := value.(map[string]interface{})

			cidr := dhcp["cidr"].(string)
			_, ipNet, _ := net.ParseCIDR(cidr)

			newDhcp.netAddr = ipNet.IP.String()
			newDhcp.netMask = fmt.Sprint(
				ipNet.Mask[0], ".", ipNet.Mask[1], ".", ipNet.Mask[2], ".", ipNet.Mask[3])

			ipRangeCfg, ok := dhcp["ip_range"].(string)
			if ok {
				ipRangePresent = true

				ip := strings.Split(strings.TrimSpace(ipRangeCfg), "-")

				start = net.ParseIP(strings.TrimSpace(ip[0]))
				end = net.ParseIP(strings.TrimSpace(ip[1]))

				// check if ip range belongs to subnet
				if ipNet.Contains(start) && ipNet.Contains(end) {
					newDhcp.vnicAddr = start.String()
					startIP := intToIP(ipToInt(start) + 1)
					// move start IP to 1 IP ahead and assign to ipRange
					newDhcp.ipRange = ipRange{startIP: startIP.String(),
						endIP: end.String()}
				} else {
					return nil, fmt.Errorf("IP Range '%s' does not belong to CIDR %s.",
						ipRangeCfg, cidr)
				}
			}

			if gw, ok := dhcp["default_gw"].(string); ok {
				gwPresent = true

				defaultGw = net.ParseIP(gw)

				// check if default gateway belongs to subnet
				if ipNet.Contains(defaultGw) {
					newDhcp.defaultGw = gw
				} else {
					return nil, fmt.Errorf("Default Gateway '%s' does not belong to CIDR %s.",
						gw, cidr)
				}
			}

			if ipRangePresent && gwPresent {
				// check default gw is not part of ip range
				if bytes.Compare(defaultGw, start) >= 0 && bytes.Compare(defaultGw, end) <= 0 {
					return nil, fmt.Errorf("Default Gateway '%s' is part of IP Range %s.",
						defaultGw, ipRangeCfg)
				}
			} else if gwPresent {
				// compute ip range from subnet
				//newDhcp.vnicAddr = start
				// move start to 1 ip ahead and assign to ipRange TODO
				//newDhcp.ipRange = ipRange{startIP: start, endIP: end}

			} else if ipRangePresent {
				// compute gw from ip range

			} else {
				// compute gw from subnet
				// compute ip range from subnet
			}

			portgroup := dhcp["portgroup"].(string)
			dhcpCfgs[portgroup] = append(dhcpCfgs[portgroup], newDhcp)
		}
		sgw.dhcp = dhcpCfgs
	}
	return sgw, nil
}

func validateCidr(v interface{}, k string) (ws []string, errors []error) {

	cidr := v.(string)

	ip, _, err := net.ParseCIDR(cidr)

	if err != nil {
		errors = append(errors, fmt.Errorf("%s: CIDR '%s' is not vlaid.",
			k, cidr))
		return
	}
	if allowedIP := net.ParseIP(ip.String()); allowedIP == nil {
		errors = append(errors, fmt.Errorf("%s: IP '%s' is not vlaid.",
			k, ip))
	}

	return
}

func validateIP(v interface{}, k string) (ws []string, errors []error) {

	ip := v.(string)

	if allowedIP := net.ParseIP(ip); allowedIP == nil {
		errors = append(errors, fmt.Errorf(
			"%s: IP '%d' is not valid.", k, ip))
	}
	return
}

func validateIPRange(v interface{}, k string) (ws []string, errors []error) {

	ipRange := v.(string)

	match, _ := regexp.MatchString("^(\\d+).(\\d+).(\\d+).(\\d+)-(\\d+).(\\d+).(\\d+).(\\d+)$",
		strings.TrimSpace(ipRange))

	if match {
		ip := strings.Split(strings.TrimSpace(ipRange), "-")

		// Validate start ip
		startIP := net.ParseIP(strings.TrimSpace(ip[0]))
		if startIP == nil {
			errors = append(errors, fmt.Errorf("%s: Start IP '%s' is not valid in range '%s'.",
				k, ip[0], ipRange))
			return
		}

		// Validate end ip
		endIP := net.ParseIP(strings.TrimSpace(ip[1]))
		if endIP == nil {
			errors = append(errors, fmt.Errorf("%s: End IP '%s' is not valid in range '%s'.",
				k, ip[1], ipRange))
			return
		}

		// Validate the range of the start and end ip
		if bytes.Compare(startIP, endIP) >= 0 {
			errors = append(errors, fmt.Errorf(
				"%s: Start IP '%s' is greater than End IP '%s' in the range %s.",
				k, startIP, endIP, ipRange))
		}

	} else {
		errors = append(errors, fmt.Errorf("%s: IP range '%s' is not vlaid.",
			k, ipRange))
	}

	return
}

func ipToInt(ip net.IP) uint32 {
	if len(ip) == 16 {
		return binary.BigEndian.Uint32(ip[12:16])
	}
	return binary.BigEndian.Uint32(ip)
}

func intToIP(nn uint32) net.IP {
	ip := make(net.IP, 4)
	binary.BigEndian.PutUint32(ip, nn)
	return ip
}

func checkIPInRange(start, end, ip net.IP) bool {
	if bytes.Compare(ip, start) >= 0 && bytes.Compare(ip, end) <= 0 {
		return true
	}
	return false
}
