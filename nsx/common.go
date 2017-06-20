package nsx

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/IBM-tfproviders/govnsx"
	"github.com/IBM-tfproviders/govnsx/nsxresource"
	"net"
	"regexp"
	"strconv"
	"strings"
)

func validateCidr(v interface{}, k string) (ws []string, errors []error) {

	cidr := v.(string)

	ip, _, err := net.ParseCIDR(cidr)

	if err != nil {
		errors = append(errors, fmt.Errorf("%s: CIDR '%s' is not valid.",
			k, cidr))
		return
	}
	if allowedIP := net.ParseIP(ip.String()); allowedIP == nil {
		errors = append(errors, fmt.Errorf("%s: IP '%s' is not valid.",
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

func validateIPRange(v interface{}) error {

	ipRange := v.(string)

	match, _ := regexp.MatchString("^(\\d+).(\\d+).(\\d+).(\\d+)-(\\d+).(\\d+).(\\d+).(\\d+)$",
		strings.TrimSpace(ipRange))

	if match {
		ip := strings.Split(strings.TrimSpace(ipRange), "-")

		// Validate start ip
		startIP := net.ParseIP(strings.TrimSpace(ip[0]))
		if startIP == nil {
			return fmt.Errorf("Start IP '%s' is not valid in range '%s'.",
				ip[0], ipRange)
		}

		// Validate end ip
		endIP := net.ParseIP(strings.TrimSpace(ip[1]))
		if endIP == nil {
			return fmt.Errorf("End IP '%s' is not valid in range '%s'.",
				ip[1], ipRange)
		}

		// Validate the range of the start and end ip
		if bytes.Compare(startIP, endIP) >= 0 {
			return fmt.Errorf(
				"Start IP '%s' needs to be smaller than End IP '%s' in the range %s.",
				startIP, endIP, ipRange)
		}

	} else {
		return fmt.Errorf("IP range '%s' is not valid.",
			ipRange)
	}

	return nil
}

func validateAndSortIPRange(ipRangeCfgs []ipRange) ([]ipRange, error) {

	for i := 0; i < len(ipRangeCfgs); i++ {
		for j := i + 1; j < len(ipRangeCfgs); j++ {

			r1 := ipRangeCfgs[i]
			r2 := ipRangeCfgs[j]

			if checkIPInRange(r1, r2.start) || checkIPInRange(r1, r2.end) {

				return nil, fmt.Errorf("Overlapping IP Ranges '%s' and '%s'",
					getIPRangeString(r1), getIPRangeString(r2))
			}

			// if r1 > r2, swap
			if ipToInt(r1.start) > ipToInt(r2.end) {
				ipRangeCfgs[i] = r2
				ipRangeCfgs[j] = r1
			}
		}
	}

	return ipRangeCfgs, nil
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

func checkIPInRange(rangeVal ipRange, ip net.IP) bool {

	if (bytes.Compare(ip, rangeVal.start) >= 0) &&
		(bytes.Compare(ip, rangeVal.end) <= 0) {
		return true
	}
	return false
}

func getIPRangeString(r ipRange) string {

	return fmt.Sprintf(r.start.String() + "-" + r.end.String())
}

func isIPInCIDR(cidr string, ip string) bool {

	_, ipNet, _ := net.ParseCIDR(cidr)

	netIP := net.ParseIP(ip)

	if ipNet.Contains(netIP) {
		return true
	}
	return false
}

func getIPRangeFromCIDR(cidr string) (ipRange, error) {

	_, ipNet, _ := net.ParseCIDR(cidr)

	elements := strings.Split(ipNet.String(), "/")

	ip := ipToInt(ipNet.IP)

	bits, _ := strconv.ParseUint(elements[1], 10, 64)

	var mask int64
	mask = ^(0xffffffff >> bits)

	network := int64(ip) & mask
	broadcast := network + ^mask

	rangeVal := ipRange{}
	if bits > 30 {
		return rangeVal, fmt.Errorf("CIDR '%s' is not valid to configure IP Ranges.",
			cidr)
	} else {
		startIP := network + 1
		endIP := broadcast - 1

		//rangeVal = ipRange{intToIP(uint32(startIP)), intToIP(uint32(endIP))}
		rangeVal.start = intToIP(uint32(startIP))
		rangeVal.end = intToIP(uint32(endIP))
	}

	return rangeVal, nil
}

func removeGwAddrFromIPRange(rangeVal ipRange, gwIP net.IP) []ipRange {

	retVal := []ipRange{}

	if rangeVal.start.String() == gwIP.String() {
		rangeVal.start = intToIP(ipToInt(rangeVal.start) + 1)
		retVal = append(retVal, rangeVal)
	} else if rangeVal.end.String() == gwIP.String() {
		rangeVal.end = intToIP(ipToInt(rangeVal.end) - 1)
		retVal = append(retVal, rangeVal)
	} else {
		retVal = append(retVal, ipRange{rangeVal.start, intToIP(ipToInt(gwIP) - 1)})
		retVal = append(retVal, ipRange{intToIP(ipToInt(gwIP) + 1), rangeVal.end})
	}

	return retVal
}

func getEdgeType(edgeId string, meta interface{}) (string, error) {

	client := meta.(*govnsx.Client)
	edge := nsxresource.NewEdge(client)

	retEdge, err := edge.Get(edgeId)
	if err != nil {
		return "", err
	}

	edgeType := retEdge.Type

	return edgeType, nil
}
