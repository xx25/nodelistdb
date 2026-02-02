// Package main provides API-driven PSTN node fetching and time-aware scheduling
// for the modem-test CLI tool.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
}

// FetchPSTNNodes fetches PSTN nodes from the NodelistDB API.
func FetchPSTNNodes(apiURL string, timeout time.Duration) ([]NodeTarget, error) {
	url := strings.TrimRight(apiURL, "/") + "/api/nodes/pstn?limit=10000"

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

	var apiResp apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse API response: %w", err)
	}

	nodes := make([]NodeTarget, 0, len(apiResp.Nodes))
	for _, n := range apiResp.Nodes {
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

	return nodes, nil
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
}

// ScheduleNodes returns a channel that emits phoneJobs in time-aware order.
// CM and currently-callable nodes are emitted first, then deferred nodes are
// emitted when their call window opens. Each node×operator combination is
// emitted callsPerOperator times. The goroutine exits when all nodes
// have been emitted or ctx is cancelled.
func ScheduleNodes(ctx context.Context, nodes []NodeTarget, operators []OperatorConfig, callsPerOperator int, log *TestLogger) <-chan phoneJob {
	jobs := make(chan phoneJob, 100)

	go func() {
		defer close(jobs)

		if len(operators) == 0 {
			operators = []OperatorConfig{{Name: "", Prefix: ""}}
		}
		if callsPerOperator <= 0 {
			callsPerOperator = 1
		}

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
			scheduled = append(scheduled, nodeCallSchedule{node: n, schedule: cs})
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

			// Wait for call window if not currently callable
			if !cs.IsCallable {
				if cs.NextCall.IsZero() {
					log.Warn("Node %s (%s): no call window found, calling anyway", n.Address(), n.SystemName)
				} else {
					waitDur := time.Until(cs.NextCall)
					if waitDur > 0 {
						log.Info("Node %s (%s): waiting %v until call window at %s UTC",
							n.Address(), n.SystemName, waitDur.Round(time.Second), cs.NextCall.Format("15:04"))
						select {
						case <-time.After(waitDur):
						case <-ctx.Done():
							return
						}
					}
				}
			}

			// Emit callsPerOperator jobs per operator for this node
			for _, op := range operators {
				for repeat := 0; repeat < callsPerOperator; repeat++ {
					testNum++
					job := phoneJob{
						phone:          n.Phone,
						operatorName:   op.Name,
						operatorPrefix: op.Prefix,
						testNum:        testNum,
						nodeAddress:    n.Address(),
						nodeSystemName: strings.ReplaceAll(n.SystemName, "_", " "),
						nodeSysop:      strings.ReplaceAll(n.SysopName, "_", " "),
					}

					select {
					case jobs <- job:
					case <-ctx.Done():
						return
					}
				}
			}
		}
	}()

	return jobs
}
