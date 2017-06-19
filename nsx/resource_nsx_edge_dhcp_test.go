package nsx

import (
	"fmt"
	"log"
	"net"
	"os"
	"reflect"
	"strings"
	"testing"
	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/terraform"
	"github.com/IBM-tfproviders/govnsx"
	"github.com/IBM-tfproviders/govnsx/nsxresource"
	//"github.com/IBM-tfproviders/govnsx/nsxtypes"
)

var (
	lsId   = os.Getenv("NSX_LOGICAL_SWITCH_ID")
	edgeId = os.Getenv("NSX_EDGE_ID")
)

const (

    cidr1 = "1.2.3.0/24"
    defaultGw1 = "1.2.3.1"
    ipRange1 = "1.2.3.2-1.2.3.50"

    cidr2 = "4.3.2.0/24"
    defaultGw2 = "4.3.2.1"
    ipRange2 = "4.3.2.2-4.3.2.50"

	testAccCheckEdgeDhcpConf_min = `
resource "nsxv_edge_dhcp" "%s" {
    edge_id = "%s"

    logical_switch {
        id = "%s",

        subnet {
            cidr = "%s"
        }
    }
}
`
	testAccCheckEdgeDhcpConf_full = `
resource "nsxv_edge_dhcp" "%s" {
    edge_id = "%s"

    logical_switch {
        id = "%s"
            
         subnet {
            cidr = "%s"
            default_gw = "%s"
            ip_pool = ["%s"]
        }
    }
}
`

	testAccCheckEdgeDhcpConf_2subnets = `
resource "nsxv_edge_dhcp" "%s" {
    edge_id = "%s"

    logical_switch {
        id = "%s"

        subnet {
            cidr = "%s"
            default_gw = "%s"
            ip_pool = ["%s"]
        }

        subnet {
            cidr = "%s"
            default_gw = "%s"
            ip_pool = ["%s"]
        }
    }
}
`
)


func TestAccNsxEdgeDHCP_ValidatorFunc(t *testing.T) {
	var validatorCases = []attributeValueValidationTestSpec{
		{name: "cidr", validatorFn: validateCidr,
			values: []attributeProperty{
				{value: "255.355.0.0/24", expErr: "is not valid"},
				{value: "1.2.3.0/33", expErr: "is not valid"},
				{value: "1.2.3.0/0", expErr: "is not valid"},
				{value: "1.2.3.4/32", successCase: true},
				{value: "1.2.3.0/24", successCase: true},
				{value: "8193", expErr: "is not valid"},
			},
		},
		{name: "default_gw", validatorFn: validateIP,
			values: []attributeProperty{
				{value: "4095", expErr: "is not valid"},
				{value: "1.2.355.1", expErr: "is not valid"},
				{value: "1.2.3.1", successCase: true},
				{value: "swqfrewq", expErr: "is not valid"},
			},
		},
	}

	verifySchemaValidationFunctions(t, validatorCases)
}

type ipRangeData struct {
	v        interface{}
	expected string
}

func TestAccNsxEdgeDHCP_ValidateIPRange(t *testing.T) {
	testData := []ipRangeData{
		{"1.2.3.4-1.2.3.50", ""},
		{"1.2.3.4", "is not valid"},
		{"1234", "is not valid"},
		{"asdfsdgas", "is not valid"},
		{"1.2.355.5-1.2.3.60", "is not valid"},
		{"1.2.3.5-1.355.3.60", "is not valid"},
		{"1.2.3.50-1.2.3.20", "needs to be smaller than"},
	}

	for _, data := range testData {

		log.Printf("Validating IP Range '%s'", data.v)
		err := validateIPRange(data.v)

		if data.expected == "" && err != nil {
			t.Fatalf("ValidationFailed: IP Range '%v' is not VALID.", data.v.(string))
		} else if err != nil {
			ok := strings.Contains(err.Error(), data.expected)
			if !ok {
				t.Fatalf("ValidationFailed: Expected ERROR '%v' is not found.", data.expected)
			}
		}
	}
}

type ipRangeSortData struct {
	v              []ipRange
	expectedRetVal []ipRange
	expected       string
}

func TestAccNsxEdgeDHCP_ValidateAndSortIPRange(t *testing.T) {

	ipRange1 := ipRange{net.ParseIP("1.2.3.4"), net.ParseIP("1.2.3.40")}
	ipRange2 := ipRange{net.ParseIP("1.2.3.50"), net.ParseIP("1.2.3.70")}
	ipRange3 := ipRange{net.ParseIP("1.2.3.80"), net.ParseIP("1.2.3.100")}
	ipRange4 := ipRange{net.ParseIP("1.2.3.30"), net.ParseIP("1.2.3.60")}
	testData := []ipRangeSortData{
		{[]ipRange{ipRange1, ipRange2, ipRange3}, []ipRange{ipRange1, ipRange2, ipRange3}, ""},
		{[]ipRange{ipRange2, ipRange1, ipRange3}, []ipRange{ipRange1, ipRange2, ipRange3}, ""},
		{[]ipRange{ipRange3, ipRange2, ipRange1}, []ipRange{ipRange1, ipRange2, ipRange3}, ""},
		{[]ipRange{ipRange1, ipRange4}, []ipRange{}, "Overlapping IP Ranges"},
		{[]ipRange{ipRange1, ipRange4, ipRange3}, []ipRange{}, "Overlapping IP Ranges"},
	}

	log.Printf("Sorting IP Ranges")
	for _, data := range testData {

		_, err := validateAndSortIPRange(data.v)

		if data.expected == "" && err != nil {
			t.Fatalf("ValidationFailed: attribute value '%v' is not VALID.", data.v)
		} else if err != nil {
			ok := strings.Contains(err.Error(), data.expected)
			if !ok {
				t.Fatalf("ValidationFailed: Expected ERROR '%v' is not found.", data.expected)
			}
		}
	}
}

type ipAndCidrData struct {
	v1       string
	v2       string
	expected bool
}

func TestAccNsxEdgeDHCP_IsIPInCidr(t *testing.T) {

	testData := []ipAndCidrData{
		{"1.2.3.0/24", "1.2.3.5", true},
		{"1.2.3.0/24", "1.2.5.5", false},
	}

	for _, data := range testData {

		log.Printf("Sorting IP Ranges")
		retVal := isIPInCIDR(data.v1, data.v2)

		if data.expected != retVal {
			t.Fatalf("ValidationFailed: IP '%s' is not in CIDR %s.", data.v2, data.v1)
		}
	}
}

type subnetData struct {
	v              map[string]interface{}
	expectedSubnet subnet
	expectedErr    string
}

func TestAccNsxEdgeDHCP_ParseSubnet(t *testing.T) {

	subnet1 := createSubnetTestData("1.2.3.0/24", "1.2.3.1", []string{"1.2.3.5-1.2.3.50"})
	subnet2 := createSubnetTestData("1.2.3.0/24", "", []string{"1.2.3.5-1.2.3.50"})
	subnet3 := createSubnetTestData("1.2.3.0/24", "1.2.3.1", []string{})
	subnet4 := createSubnetTestData("1.2.3.0/24", "", []string{})
	subnet5 := createSubnetTestData("1.2.3.0/24", "1.2.3.10", []string{"1.2.3.5-1.2.3.50"})
	subnet6 := createSubnetTestData("1.2.3.0/24", "11.22.33.10", []string{"1.2.3.5-1.2.3.50"})
	subnet7 := createSubnetTestData("1.2.3.0/24", "1.2.3.1", []string{"11.22.33.5-11.22.33.50"})
	subnet8 := createSubnetTestData("1.2.3.0/24", "1.2.3.1", []string{"1.2.3.5-1.2.3.50",
		"1.2.3.25-1.2.3.35"})

	expectedSubnet1 := subnet{defaultGw: "1.2.3.1", vnicAddr: "1.2.3.5", netMask: "255.255.255.0",
		ipRangeList: []ipRange{ipRange{net.ParseIP("1.2.3.6"), net.ParseIP("1.2.3.50")}}}

	expectedSubnet2 := subnet{defaultGw: "1.2.3.5", vnicAddr: "1.2.3.6", netMask: "255.255.255.0",
		ipRangeList: []ipRange{ipRange{net.ParseIP("1.2.3.7"), net.ParseIP("1.2.3.50")}}}

	expectedSubnet3 := subnet{defaultGw: "1.2.3.1", vnicAddr: "1.2.3.2", netMask: "255.255.255.0",
		ipRangeList: []ipRange{ipRange{net.ParseIP("1.2.3.3"), net.ParseIP("1.2.3.254")}}}

	testData := []subnetData{
		{subnet1, expectedSubnet1, ""},
		{subnet2, expectedSubnet2, ""},
		{subnet3, expectedSubnet3, ""},
		{subnet4, expectedSubnet3, ""},
		{subnet5, subnet{}, "is part of IP Range"},
		{subnet6, subnet{}, "does not belong to CIDR"},
		{subnet7, subnet{}, "does not belong to CIDR"},
		{subnet8, subnet{}, "Overlapping IP Ranges"},
	}

	for _, data := range testData {

		log.Printf("Parsing Subnet %#v", (data.v))

		retSubnet, err := parseSubnet(data.v)

		if data.expectedErr == "" && err != nil {
			t.Fatalf("Parsing subnet failed with error %s:", err)
		} else if err != nil {
			ok := strings.Contains(err.Error(), data.expectedErr)
			if !ok {
				t.Fatalf("Parsing subnet failed with error: Expected ERROR '%v' is not found.", data.expectedErr)
			}
		}

		if err == nil && !validateRetSubnet(retSubnet, data.expectedSubnet) {
			t.Fatalf("Parsing subnet failed : Expected value '%v' is not found.", data.expectedSubnet)
		}
	}
}

func TestAccNsxEdgeDHCP_Create(t *testing.T) {

    dhcpName := "TFT_DEFAULT"
    resourceName := "nsxv_edge_dhcp." + dhcpName

	config := fmt.Sprintf(testAccCheckEdgeDhcpConf_min, dhcpName, edgeId, lsId, cidr1)
	log.Printf("[DEBUG] template config= %s", config)

	resource.Test(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheckEdgeDHCP(t) },
		Providers:    testAccProviders,
		CheckDestroy: testAccCheckEdgeDHCPDestroy,
		Steps: []resource.TestStep{
			resource.TestStep{
				Config: config,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(
						resourceName, "edge_id", edgeId),
				),
			},
		},
	})
}

func TestAccNsxEdgeDHCP_UpdateAddSubnet(t *testing.T) {

    dhcpName := "TFT_DEFAULT"
    resourceName := "nsxv_edge_dhcp." + dhcpName

	config := fmt.Sprintf(testAccCheckEdgeDhcpConf_full, dhcpName, edgeId, lsId, cidr1, 
            defaultGw1, ipRange1)
	log.Printf("[DEBUG] template config= %s", config)

	configUpdate := fmt.Sprintf(testAccCheckEdgeDhcpConf_2subnets, dhcpName, edgeId, lsId, cidr1, 
            defaultGw1, ipRange1, cidr2, defaultGw2, ipRange2)
	log.Printf("[DEBUG] template configUpdate= %s", configUpdate)

	resource.Test(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheckEdgeDHCP(t) },
		Providers:    testAccProviders,
		CheckDestroy: testAccCheckEdgeDHCPDestroy,
		Steps: []resource.TestStep{
			resource.TestStep{
				Config: config,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(
						resourceName, "edge_id", edgeId),
				),
			},
            resource.TestStep{
				Config: configUpdate,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(
						resourceName, "edge_id", edgeId),
				),
			},
		},
	})
}

func testAccCheckEdgeDHCPDestroy(s *terraform.State) error {

	client := testAccProvider.Meta().(*govnsx.Client)

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "nsxv_edge_dhcp" {
			continue
		}

       // get DHCP configs and verify
    	edgeDHCP := nsxresource.NewEdgeDHCP(client)
	    edgeDHCPConfig, err := edgeDHCP.Get(edgeId)

   	    if err != nil {
        	return fmt.Errorf("Getting DHCP Config for Edge %s failed with error: %v", edgeId, err)
   	    } else {
    	    if edgeDHCPConfig != nil {

                for _, dhcpIPPool := range edgeDHCPConfig.IPPools {
                    
                    ipRangeVal := strings.Split(dhcpIPPool.IPRange, "-")
			        if isIPInCIDR(cidr1, ipRangeVal[0]) || isIPInCIDR(cidr1, ipRangeVal[1]) || 
                        isIPInCIDR(cidr2, ipRangeVal[0]) || isIPInCIDR(cidr2, ipRangeVal[1]) {
                 
                        return fmt.Errorf("DHCP Config for Edge %s still exists", edgeId)
                    }
                }
        	} else {
		        log.Printf("DHCP Config for Edge %s already deleted.", edgeId)
        	   	return nil
	       	}
    	}
    }

	return nil
}

func createSubnetTestData(cidr string, gwIP string, ipPool []string) map[string]interface{} {

	subnetMap := make(map[string]interface{})

	subnetItemsMap := make(map[string]interface{})
	subnetItemsMap["cidr"] = cidr
	subnetItemsMap["default_gw"] = gwIP

	if len(ipPool) > 0 {
		ipPoolInterface := make([]interface{}, len(ipPool))
		for i, value := range ipPool {
			ipPoolInterface[i] = value
		}

		subnetItemsMap["ip_pool"] = ipPoolInterface
	}

	subnetMap = subnetItemsMap

	return subnetMap
}

func validateRetSubnet(retSubnet, expectedSubnet subnet) bool {

	emptySubnet := subnet{}

	if (reflect.DeepEqual(expectedSubnet, emptySubnet) &&
		!(reflect.DeepEqual(retSubnet, emptySubnet))) ||
		(!(reflect.DeepEqual(expectedSubnet, emptySubnet)) &&
			reflect.DeepEqual(retSubnet, emptySubnet)) {
		return false
	}

	if retSubnet.defaultGw != expectedSubnet.defaultGw ||
		retSubnet.vnicAddr != expectedSubnet.vnicAddr ||
		retSubnet.netMask != expectedSubnet.netMask {

		return false
	}

	for _, retIPRange := range retSubnet.ipRangeList {

		for _, expectedIPRange := range expectedSubnet.ipRangeList {

			if retIPRange.start.String() != expectedIPRange.start.String() &&
				retIPRange.end.String() != expectedIPRange.end.String() {
				log.Printf("in ip range")
				return false
			}
		}
	}

	return true
}

func testAccPreCheckEdgeDHCP(t *testing.T) {

	var envList = []string{"NSX_EDGE_ID", "NSX_LOGICAL_SWITCH_ID"}

	testAccPreCheck(t)

	for _, env := range envList {
		if v := os.Getenv(env); v == "" {
			t.Fatal(env + " must be set for acceptance tests")
		}
	}
}
