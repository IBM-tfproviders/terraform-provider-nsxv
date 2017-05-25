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
	edgeId      string
	portgroup   string
	networkAddr string // network address eg: 10.10.10.0
	netMask     string // netmask eg: 255.255.255.0
	vnicAddr    string
	defaultGw   string
	ipRange
}

func resourceNsxEdgeDHCP() *schema.Resource {
	return &schema.Resource{
		Create: resourceNsxEdgeDHCPCreate,
		Read:   resourceNsxEdgeDHCPRead,
		Update: resourceNsxEdgeDHCPUpdate,
		Delete: resourceNsxEdgeDHCPDelete,

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
			"edge_id": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: false,
			},
		},
	}
}

func resourceNsxEdgeDHCPCreate(d *schema.ResourceData, meta interface{}) error {

	dhcp, err := NewNsxEdgeDHCP(d)

	if err != nil {
		return err
	}

	log.Printf("[INFO] Creating NSX Edge DHCP: %#v", dhcp)

	client := meta.(*govnsx.Client)

	/* Get Edge configuration. Loop through all the vnics from 1-9.
	   If the portgroup matches, add new address group for the existing vnic.
	   else create a new vnic for the portgroup.
	   If all the vnics are configured, return err */

	edge := nsxresource.NewEdge(client)

	edgeCfg, err := edge.Get(dhcp.edgeId)

	if err != nil {
		log.Printf("[Error] edge.Get() returned error : %v", err)
		return err
	}

	// Update vnic configuration of Edge
	vnics := []nsxtypes.Vnic{}
	addrGroups := []nsxtypes.AddressGroup{}

	addrGroup := nsxtypes.AddressGroup{}
	addrGroup.PrimaryAddress = dhcp.vnicAddr
	addrGroup.SubnetMask = dhcp.netMask
	addrGroups = append(addrGroups, addrGroup)

	vnic := nsxtypes.Vnic{}
	vnic.Index = strconv.Itoa(1)
	vnic.PortgroupId = dhcp.portgroup
	vnic.AddressGroups = addrGroups
	vnic.IsConnected = true

	vnics = append(vnics, vnic)

	edgeUpdateSpec := &nsxtypes.EdgeSGWInstallSpec{
		Tenant:         edgeCfg.Tenant,
		AppliancesList: edgeCfg.AppliancesList,
		Vnics:          vnics,
	}

	err = edge.Put(edgeUpdateSpec, dhcp.edgeId)

	if err != nil {
		log.Printf("[Error] edge.Put() returned error : %v", err)
		return err
	}

	log.Printf("[INFO] Updated NSX Edge for DHCP configuration: %s", dhcp.edgeId)

	// configure dhcp with the iprange and gw
	ipPools := []nsxtypes.IPPool{}
	ipPool := nsxtypes.IPPool{}
	ipPool.IPRange = fmt.Sprintf(dhcp.ipRange.startIP + "-" + dhcp.ipRange.endIP)
	ipPool.DefaultGw = dhcp.defaultGw
	ipPool.SubnetMask = dhcp.netMask
	ipPools = append(ipPools, ipPool)

	dhcpConfigSpec := &nsxtypes.ConfigDHCPServiceSpec{
		IPPools: ipPools,
	}

	edgeDHCP := nsxresource.NewEdgeDHCP(client)
	err = edgeDHCP.Put(dhcpConfigSpec, dhcp.edgeId)

	if err != nil {
		log.Printf("[Error] edgeDHCP.Put() returned error : %v", err)
		return err
	}

	d.SetId(dhcp.edgeId)
	log.Printf("[INFO] Configured DHCP for NSX Edge: %s", dhcp.edgeId)

	return nil
}

func resourceNsxEdgeDHCPRead(d *schema.ResourceData, meta interface{}) error {

	log.Printf("[INFO] Reading Nsx Edge DHCP")
	log.Printf("[WARN] Yet to be implemented")
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

	edgeId := d.Id()

	log.Printf("[INFO] Deleting NSX Edge: %s", edgeId)
	err := edgeDHCP.Delete(edgeId)
	if err != nil {
		log.Printf("[Error] edgeDHCP.Delete() returned error : %v", err)
		return err
	}
	return nil
}

func NewNsxEdgeDHCP(d *schema.ResourceData) (*dhcpCfg, error) {

	dhcp := &dhcpCfg{
		edgeId:    d.Get("edge_id").(string),
		portgroup: d.Get("portgroup").(string),
	}

	cidr := d.Get("cidr").(string)
	_, ipNet, _ := net.ParseCIDR(cidr)

	dhcp.networkAddr = ipNet.IP.String()
	dhcp.netMask = fmt.Sprint(
		ipNet.Mask[0], ".", ipNet.Mask[1], ".", ipNet.Mask[2], ".", ipNet.Mask[3])

	ipRangePresent := false
	gwPresent := false
	var start, end, defaultGw net.IP

	var ipRangeCfg string
	if v, ok := d.GetOk("ip_range"); ok {
		ipRangePresent = true
		ipRangeCfg = v.(string)

		ip := strings.Split(strings.TrimSpace(ipRangeCfg), "-")

		start = net.ParseIP(strings.TrimSpace(ip[0]))
		end = net.ParseIP(strings.TrimSpace(ip[1]))

		// check if ip range belongs to subnet
		if ipNet.Contains(start) && ipNet.Contains(end) {
			dhcp.vnicAddr = start.String()
			startIP := intToIP(ipToInt(start) + 1)
			// move start IP to 1 IP ahead and assign to ipRange
			dhcp.ipRange = ipRange{startIP: startIP.String(),
				endIP: end.String()}
		} else {
			return nil, fmt.Errorf("IP Range '%s' does not belong to CIDR %s.",
				ipRangeCfg, cidr)
		}
	}

	if v, ok := d.GetOk("default_gw"); ok {
		gwPresent = true
		gw := v.(string)

		defaultGw = net.ParseIP(gw)

		// check if default gateway belongs to subnet
		if ipNet.Contains(defaultGw) {
			dhcp.defaultGw = gw
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
		//dhcp.vnicAddr = start
		// move start to 1 ip ahead and assign to ipRange TODO
		//dhcp.ipRange = ipRange{startIP: start, endIP: end}
	} else if ipRangePresent {
		// compute gw from ip range

	} else {
		// compute gw from subnet
		// compute ip range from subnet
	}

	return dhcp, nil
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
