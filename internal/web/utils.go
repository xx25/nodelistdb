package web

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
)

// parseNodeAddress parses a FidoNet node address like "2:5001/100" or "1:234/56.7"
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