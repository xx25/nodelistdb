// Package main provides API-driven PSTN node fetching and time-aware scheduling
// for the modem-test CLI tool.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	modemPkg "github.com/nodelistdb/internal/modem"
	"github.com/nodelistdb/internal/testing/timeavail"
)

// NodeTarget represents a PSTN node fetched from the NodelistDB API.
type NodeTarget struct {
	Phone      string   // Normalized phone number (for dialing)
	PhoneRaw   string   // Original format from nodelist
	Zone       int      // FidoNet zone
	Net        int      // FidoNet net
	Node       int      // FidoNet node
	SystemName string   // BBS name
	SysopName  string   // Sysop name
	Location   string   // Node location
	IsCM       bool     // Continuous Mail (24/7)
	Flags      []string // General flags (CM, XA, TAN, etc.)
	ModemFlags []string // Modem capability flags (V34, V32B, etc.)
	MaxSpeed   uint32   // Maximum baud rate
}

// Address returns the FidoNet address string.
func (n *NodeTarget) Address() string {
	return fmt.Sprintf("%d:%d/%d", n.Zone, n.Net, n.Node)
}

// apiResponse matches the JSON response from GET /api/nodes/pstn.
type apiResponse struct {
	Nodes []apiNode `json:"nodes"`
	Count int       `json:"count"`
}

type apiNode struct {
	Zone            int      `json:"zone"`
	Net             int      `json:"net"`
	Node            int      `json:"node"`
	SystemName      string   `json:"system_name"`
	SysopName       string   `json:"sysop_name"`
	Location        string   `json:"location"`
	Phone           string   `json:"phone"`
	PhoneNormalized string   `json:"phone_normalized"`
	IsCM            bool     `json:"is_cm"`
	Flags           []string `json:"flags"`
	ModemFlags      []string `json:"modem_flags"`
	MaxSpeed        uint32   `json:"max_speed"`
	IsPSTNDead      bool     `json:"is_pstn_dead"`
}

// FetchPSTNNodes fetches PSTN nodes from the NodelistDB API.
func FetchPSTNNodes(apiURL string, timeout time.Duration, log *TestLogger) ([]NodeTarget, error) {
	nodes, _, err := FetchPSTNNodesWithCount(apiURL, timeout, log)
	return nodes, err
}

// FetchPSTNNodesWithCount fetches PSTN nodes and returns total count for truncation detection.
func FetchPSTNNodesWithCount(apiURL string, timeout time.Duration, log *TestLogger) ([]NodeTarget, int, error) {
	url := strings.TrimRight(apiURL, "/") + "/api/nodes/pstn?limit=10000"

	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(url)
	if err != nil {
		return nil, 0, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, 0, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	var apiResp apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, 0, fmt.Errorf("failed to parse API response: %w", err)
	}

	nodes := make([]NodeTarget, 0, len(apiResp.Nodes))
	deadSkipped := 0
	for _, n := range apiResp.Nodes {
		if n.IsPSTNDead {
			deadSkipped++
			continue
		}

		phone := n.PhoneNormalized
		if phone == "" {
			phone = modemPkg.NormalizePhone(n.Phone)
		}
		if phone == "" {
			continue // Skip nodes with unparseable phones
		}

		nodes = append(nodes, NodeTarget{
			Phone:      phone,
			PhoneRaw:   n.Phone,
			Zone:       n.Zone,
			Net:        n.Net,
			Node:       n.Node,
			SystemName: n.SystemName,
			SysopName:  n.SysopName,
			Location:   n.Location,
			IsCM:       n.IsCM,
			Flags:      n.Flags,
			ModemFlags: n.ModemFlags,
			MaxSpeed:   n.MaxSpeed,
		})
	}

	if deadSkipped > 0 {
		log.Info("Skipped %d PSTN-dead nodes", deadSkipped)
	}

	// Return len(apiResp.Nodes) as the "fetched from API" count so callers can
	// detect actual API-level truncation (limit hit). Using len(nodes) would
	// false-alarm because dead/unparseable nodes are filtered locally.
	return nodes, len(apiResp.Nodes), nil
}

// recentSuccessResponse matches the JSON response from GET /api/nodes/pstn/recent-success.
type recentSuccessResponse struct {
	Phones []string `json:"phones"`
	Count  int      `json:"count"`
}

// FetchRecentSuccessPhones fetches phone numbers that were successfully tested via modem
// within the specified number of days. Returns a set for O(1) lookup.
func FetchRecentSuccessPhones(apiURL string, days int, timeout time.Duration) (map[string]bool, error) {
	url := fmt.Sprintf("%s/api/nodes/pstn/recent-success?days=%d",
		strings.TrimRight(apiURL, "/"), days)

	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	var apiResp recentSuccessResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse API response: %w", err)
	}

	phoneSet := make(map[string]bool, len(apiResp.Phones))
	for _, p := range apiResp.Phones {
		// Normalize: strip leading + so lookups match CLI's stripped phones
		phoneSet[strings.TrimPrefix(p, "+")] = true
	}

	return phoneSet, nil
}

// ParseNodeAddress parses "zone:net/node" format, returns error for points.
func ParseNodeAddress(addr string) (zone, net, node int, err error) {
	// Check for point address (not supported)
	if strings.Contains(addr, ".") {
		return 0, 0, 0, fmt.Errorf("point addresses are not supported")
	}

	// Parse zone:net/node
	parts := strings.Split(addr, ":")
	if len(parts) != 2 {
		return 0, 0, 0, fmt.Errorf("invalid format: expected zone:net/node")
	}

	_, err = fmt.Sscanf(parts[0], "%d", &zone)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid zone: %w", err)
	}

	netNode := strings.Split(parts[1], "/")
	if len(netNode) != 2 {
		return 0, 0, 0, fmt.Errorf("invalid format: expected net/node after zone")
	}

	_, err = fmt.Sscanf(netNode[0], "%d", &net)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid net: %w", err)
	}

	_, err = fmt.Sscanf(netNode[1], "%d", &node)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid node: %w", err)
	}

	return zone, net, node, nil
}

// singleNodeResponse matches the JSON response from GET /api/nodes/{zone}/{net}/{node}.
type singleNodeResponse struct {
	Zone            int      `json:"zone"`
	Net             int      `json:"net"`
	Node            int      `json:"node"`
	SystemName      string   `json:"system_name"`
	SysopName       string   `json:"sysop_name"`
	Location        string   `json:"location"`
	Phone           string   `json:"phone"`
	PhoneNormalized string   `json:"phone_normalized"`
	IsCM            bool     `json:"is_cm"`
	Flags           []string `json:"flags"`
	ModemFlags      []string `json:"modem_flags"`
	MaxSpeed        uint32   `json:"max_speed"`
}

// FetchNodeByAddress fetches a single node from the API by address.
func FetchNodeByAddress(apiURL string, zone, net, node int, timeout time.Duration) (*NodeTarget, error) {
	url := fmt.Sprintf("%s/api/nodes/%d/%d/%d", strings.TrimRight(apiURL, "/"), zone, net, node)

	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("node %d:%d/%d not found", zone, net, node)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	var n singleNodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&n); err != nil {
		return nil, fmt.Errorf("failed to parse API response: %w", err)
	}

	phone := n.PhoneNormalized
	if phone == "" {
		phone = modemPkg.NormalizePhone(n.Phone)
	}

	return &NodeTarget{
		Phone:      phone,
		PhoneRaw:   n.Phone,
		Zone:       n.Zone,
		Net:        n.Net,
		Node:       n.Node,
		SystemName: n.SystemName,
		SysopName:  n.SysopName,
		Location:   n.Location,
		IsCM:       n.IsCM,
		Flags:      n.Flags,
		ModemFlags: n.ModemFlags,
		MaxSpeed:   n.MaxSpeed,
	}, nil
}

// FilterExceptPrefixes returns nodes NOT matching any exception prefix.
func FilterExceptPrefixes(nodes []NodeTarget, exceptPrefixes []string) []NodeTarget {
	if len(exceptPrefixes) == 0 {
		return nodes
	}

	// Normalize exception prefixes
	normalizedExcept := make([]string, 0, len(exceptPrefixes))
	for _, p := range exceptPrefixes {
		normalized := modemPkg.NormalizePrefix(strings.TrimSpace(p))
		if normalized != "" {
			normalizedExcept = append(normalizedExcept, normalized)
		}
	}
	if len(normalizedExcept) == 0 {
		return nodes
	}

	var filtered []NodeTarget
	for _, n := range nodes {
		excluded := false
		for _, prefix := range normalizedExcept {
			if modemPkg.HasPhonePrefix(n.Phone, prefix) {
				excluded = true
				break
			}
		}
		if !excluded {
			filtered = append(filtered, n)
		}
	}
	return filtered
}

// FilterByPrefix filters nodes whose normalized phone starts with the given prefix.
func FilterByPrefix(nodes []NodeTarget, prefix string) []NodeTarget {
	normalizedPrefix := modemPkg.NormalizePrefix(prefix)
	if normalizedPrefix == "" {
		return nodes // Empty prefix = no filtering
	}

	var filtered []NodeTarget
	for _, n := range nodes {
		if modemPkg.HasPhonePrefix(n.Phone, normalizedPrefix) {
			filtered = append(filtered, n)
		}
	}
	return filtered
}

// BuildNodeLookupByPhone creates a map from normalized phone to NodeTarget.
// If multiple nodes share a phone, the first one wins (they share the same line).
func BuildNodeLookupByPhone(nodes []NodeTarget) map[string]*NodeTarget {
	lookup := make(map[string]*NodeTarget, len(nodes))
	for i := range nodes {
		if _, exists := lookup[nodes[i].Phone]; !exists {
			lookup[nodes[i].Phone] = &nodes[i]
		}
	}
	return lookup
}

// UniquePhones returns deduplicated phone numbers preserving order.
func UniquePhones(nodes []NodeTarget) []string {
	seen := make(map[string]bool, len(nodes))
	var phones []string
	for _, n := range nodes {
		if !seen[n.Phone] {
			seen[n.Phone] = true
			phones = append(phones, n.Phone)
		}
	}
	return phones
}

// GetNodeAvailability parses time availability from a node's flags.
// Returns nil on error (caller should treat as callable).
func GetNodeAvailability(node *NodeTarget) *timeavail.NodeAvailability {
	avail, err := timeavail.ParseAvailability(node.Flags, node.Zone, node.Phone)
	if err != nil {
		return nil
	}
	return avail
}

// nodeCallSchedule pairs a node with its computed call schedule for sorting.
type nodeCallSchedule struct {
	node     NodeTarget
	schedule *timeavail.CallSchedule
	avail    *timeavail.NodeAvailability // nil = always callable
}

// waitForCallWindow blocks until the target time or context cancellation,
// logging periodic reminders so the user knows the process is still alive.
func waitForCallWindow(ctx context.Context, target time.Time, nodeAddr, nodeName string, log *TestLogger) {
	const reminderInterval = 5 * time.Minute
	for {
		remaining := time.Until(target)
		if remaining <= 0 {
			return
		}
		// Use the shorter of remaining time or reminder interval
		waitDur := remaining
		if waitDur > reminderInterval {
			waitDur = reminderInterval
		}
		select {
		case <-time.After(waitDur):
			if time.Until(target) > 0 {
				log.Info("Node %s (%s): still waiting for call window at %s UTC (%v remaining, now %s UTC)",
					nodeAddr, nodeName, target.Format("15:04"),
					time.Until(target).Round(time.Second), time.Now().UTC().Format("15:04"))
			}
		case <-ctx.Done():
			return
		}
	}
}

// ScheduleNodes returns a channel that emits phoneJobs in time-aware order.
// CM and currently-callable nodes are emitted first, then deferred nodes are
// emitted when their call window opens. When operators are configured, each job
// includes the full operator list for failover (rather than emitting separate jobs).
// The goroutine exits when all nodes have been emitted or ctx is cancelled.
func ScheduleNodes(ctx context.Context, nodes []NodeTarget, operatorsForPhone func(string) []OperatorConfig, log *TestLogger) <-chan phoneJob {
	jobs := make(chan phoneJob, 100)

	go func() {
		defer close(jobs)

		now := time.Now().UTC()
		sched := timeavail.NewScheduler(now)

		// Compute schedules and sort: callable first, then by next call time
		var scheduled []nodeCallSchedule
		for _, n := range nodes {
			avail := GetNodeAvailability(&n)
			var cs *timeavail.CallSchedule
			if avail != nil {
				cs = sched.GetNextCallTime(avail)
			} else {
				// No availability info = always callable
				cs = &timeavail.CallSchedule{IsCallable: true, NextCall: now, Reason: "No time restrictions"}
			}
			scheduled = append(scheduled, nodeCallSchedule{node: n, schedule: cs, avail: avail})
		}

		sort.SliceStable(scheduled, func(i, j int) bool {
			si, sj := scheduled[i].schedule, scheduled[j].schedule
			// Callable before not-callable
			if si.IsCallable != sj.IsCallable {
				return si.IsCallable
			}
			// Both not callable: earlier window first
			if !si.IsCallable && !sj.IsCallable {
				if !si.NextCall.IsZero() && !sj.NextCall.IsZero() {
					return si.NextCall.Before(sj.NextCall)
				}
			}
			return false
		})

		testNum := 0
		for _, entry := range scheduled {
			n := entry.node
			cs := entry.schedule
			avail := entry.avail

			// Wait for call window if not currently callable
			if !cs.IsCallable {
				if cs.NextCall.IsZero() {
					log.Warn("Node %s (%s): no call window found, calling anyway", n.Address(), n.SystemName)
				} else {
					waitDur := time.Until(cs.NextCall)
					if waitDur > 0 {
						log.Info("Node %s (%s): waiting %v until call window at %s UTC (now %s UTC)",
							n.Address(), n.SystemName, waitDur.Round(time.Second), cs.NextCall.Format("15:04"), time.Now().UTC().Format("15:04"))
						waitForCallWindow(ctx, cs.NextCall, n.Address(), n.SystemName, log)
						if ctx.Err() != nil {
							return
						}
					}
				}
			}

			// Recheck availability with CURRENT time (queue delay may have closed the window)
			if avail != nil && !avail.IsCallableNow(time.Now().UTC()) {
				recheckSched := timeavail.NewScheduler(time.Now().UTC())
				recheckCS := recheckSched.GetNextCallTime(avail)
				if !recheckCS.NextCall.IsZero() {
					waitDur := time.Until(recheckCS.NextCall)
					if waitDur > 0 {
						log.Info("Node %s (%s): window closed during queue delay, waiting %v until %s UTC (now %s UTC)",
							n.Address(), n.SystemName, waitDur.Round(time.Second), recheckCS.NextCall.Format("15:04"), time.Now().UTC().Format("15:04"))
						waitForCallWindow(ctx, recheckCS.NextCall, n.Address(), n.SystemName, log)
						if ctx.Err() != nil {
							return
						}
					}
				}
			}

			// Emit ONE job per node with operator list for failover
			testNum++
			var ops []OperatorConfig
			if operatorsForPhone != nil {
				ops = operatorsForPhone(n.Phone)
			}
			nodeCopy := n // copy for pointer stability
			job := phoneJob{
				phone:            n.Phone,
				operators:        ops,
				testNum:          testNum,
				nodeAddress:      n.Address(),
				nodeSystemName:   strings.ReplaceAll(n.SystemName, "_", " "),
				nodeLocation:     strings.ReplaceAll(n.Location, "_", " "),
				nodeSysop:        strings.ReplaceAll(n.SysopName, "_", " "),
				nodeTarget:       &nodeCopy,
				nodeAvailability: avail,
			}

			select {
			case jobs <- job:
			case <-ctx.Done():
				return
			}
		}
	}()

	return jobs
}

// handlePSTNDeadCommand handles the -mark-dead and -unmark-dead CLI commands.
// Loads config for API URL/key, calls the API, and exits.
func handlePSTNDeadCommand(markAddr, unmarkAddr, reason, configPath string) {
	// Load config to get API credentials
	var cfg *Config
	var err error
	if configPath != "" {
		cfg, err = LoadConfig(configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: Failed to load config: %v\n", err)
			os.Exit(1)
		}
	} else if discovered := DiscoverConfigFile(); discovered != "" {
		cfg, err = LoadConfig(discovered)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: Failed to load config %s: %v\n", discovered, err)
			os.Exit(1)
		}
	} else {
		cfg = DefaultConfig()
	}

	if cfg.NodelistDB.URL == "" {
		fmt.Fprintf(os.Stderr, "ERROR: nodelistdb.url is required in config for PSTN dead management\n")
		os.Exit(1)
	}
	if cfg.NodelistDB.APIKey == "" {
		fmt.Fprintf(os.Stderr, "ERROR: nodelistdb.api_key is required in config for PSTN dead management\n")
		os.Exit(1)
	}

	// Determine operation
	addr := markAddr
	method := "POST"
	action := "Marking"
	if addr == "" {
		addr = unmarkAddr
		method = "DELETE"
		action = "Unmarking"
	}

	zone, net, node, err := ParseNodeAddress(addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: Invalid node address %q: %v\n", addr, err)
		os.Exit(1)
	}

	// Build request body
	reqBody := struct {
		Zone   int    `json:"zone"`
		Net    int    `json:"net"`
		Node   int    `json:"node"`
		Reason string `json:"reason,omitempty"`
	}{
		Zone:   zone,
		Net:    net,
		Node:   node,
		Reason: reason,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: Failed to encode request: %v\n", err)
		os.Exit(1)
	}

	// Make API call
	apiURL := strings.TrimRight(cfg.NodelistDB.URL, "/") + "/api/modem/pstn-dead"
	req, err := http.NewRequestWithContext(context.Background(), method, apiURL, bytes.NewReader(body))
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: Failed to create request: %v\n", err)
		os.Exit(1)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.NodelistDB.APIKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: API request failed: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "ERROR: API returned status %d: %s\n", resp.StatusCode, string(respBody))
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "%s node %d:%d/%d as PSTN dead: OK\n", action, zone, net, node)
}
