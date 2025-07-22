package web

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
)

// parseNodeAddress parses a FidoNet node address string into its components
func parseNodeAddress(address string) (zone, net, node int, err error) {
	// Remove any whitespace
	address = strings.TrimSpace(address)
	
	// Regular expression to match FidoNet address format: zone:net/node[.point]
	// We only care about zone:net/node for this search
	re := regexp.MustCompile(`^(\d+):(\d+)/(\d+)(?:\.(\d+))?$`)
	matches := re.FindStringSubmatch(address)
	
	if len(matches) < 4 {
		return 0, 0, 0, errors.New("invalid node address format")
	}
	
	zone, err = strconv.Atoi(matches[1])
	if err != nil {
		return 0, 0, 0, err
	}
	
	net, err = strconv.Atoi(matches[2])
	if err != nil {
		return 0, 0, 0, err
	}
	
	node, err = strconv.Atoi(matches[3])
	if err != nil {
		return 0, 0, 0, err
	}
	
	return zone, net, node, nil
}

// getZoneDescription returns the description for a FidoNet zone
func getZoneDescription(zone int) string {
	switch zone {
	case 1:
		return "North America (USA and Canada)"
	case 2:
		return "Europe, Former Soviet Union, and Israel"
	case 3:
		return "Australasia (includes former Zone 6 nodes)"
	case 4:
		return "Latin America (except Puerto Rico)"
	case 5:
		return "Africa"
	case 6:
		return "Asia (removed July 2007, nodes moved to Zone 3)"
	default:
		return "Unknown Zone"
	}
}