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
	// Check if historical search is requested
	includeHistorical := r.FormValue("include_historical") == "1"
	latestOnly := !includeHistorical
	
	filter := database.NodeFilter{
		LatestOnly: &latestOnly,
		Limit:      25, // Default limit
	}
	
	// Track if we have any specific constraints to prevent overly broad searches
	hasSpecificConstraint := false

	if zone := r.FormValue("zone"); zone != "" {
		if z, err := strconv.Atoi(zone); err == nil {
			filter.Zone = &z
			hasSpecificConstraint = true
		}
	}

	if net := r.FormValue("net"); net != "" {
		if n, err := strconv.Atoi(net); err == nil {
			filter.Net = &n
			hasSpecificConstraint = true
		}
	}

	if node := r.FormValue("node"); node != "" {
		if n, err := strconv.Atoi(node); err == nil {
			filter.Node = &n
			// Only consider node alone as sufficient constraint if zone or net is also specified
			// This prevents resource-intensive queries like searching for node=0 across all zones/nets
			if filter.Zone != nil || filter.Net != nil {
				hasSpecificConstraint = true
			}
		}
	}

	if systemName := r.FormValue("system_name"); systemName != "" {
		// Prevent memory exhaustion from very short search strings
		if len(strings.TrimSpace(systemName)) >= 2 {
			filter.SystemName = &systemName
			hasSpecificConstraint = true
		}
	}

	if location := r.FormValue("location"); location != "" {
		// Prevent memory exhaustion from very short search strings
		if len(strings.TrimSpace(location)) >= 2 {
			filter.Location = &location
			hasSpecificConstraint = true
		}
	}

	// Return empty filter for resource-intensive searches to prevent OOM
	if !hasSpecificConstraint {
		return database.NodeFilter{LatestOnly: &latestOnly, Limit: 0} // Limit 0 = no results
	}

	return filter
}
