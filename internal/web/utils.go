package web

import (
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"nodelistdb/internal/database"
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

// buildNodeFilterFromAddress creates a node filter from a full address string
func buildNodeFilterFromAddress(address string) (database.NodeFilter, error) {
	zone, net, node, err := parseNodeAddress(address)
	if err != nil {
		return database.NodeFilter{}, err
	}

	latestOnly := true
	return database.NodeFilter{
		Zone:       &zone,
		Net:        &net,
		Node:       &node,
		LatestOnly: &latestOnly,
	}, nil
}

// buildNodeFilterFromForm creates a node filter from individual form fields
func buildNodeFilterFromForm(r *http.Request) database.NodeFilter {
	latestOnly := true
	filter := database.NodeFilter{
		LatestOnly: &latestOnly,
		Limit:      100, // Default limit
	}

	if zone := r.FormValue("zone"); zone != "" {
		if z, err := strconv.Atoi(zone); err == nil {
			filter.Zone = &z
		}
	}

	if net := r.FormValue("net"); net != "" {
		if n, err := strconv.Atoi(net); err == nil {
			filter.Net = &n
		}
	}

	if node := r.FormValue("node"); node != "" {
		if n, err := strconv.Atoi(node); err == nil {
			filter.Node = &n
		}
	}

	if systemName := r.FormValue("system_name"); systemName != "" {
		filter.SystemName = &systemName
	}

	if location := r.FormValue("location"); location != "" {
		filter.Location = &location
	}

	return filter
}
