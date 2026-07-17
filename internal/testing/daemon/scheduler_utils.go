package daemon

import (
	"context"
	"time"

	"github.com/nodelistdb/internal/testing/models"
)

// nodeKey generates a unique key for a node. The key is domain-qualified
// ("zone:net/node@domain"): the same 3D address may exist in several FTN
// networks and each gets its own schedule entry.
func (s *Scheduler) nodeKey(node *models.Node) string {
	return node.Key()
}

// GetScheduleStatus returns statistics about the current schedule state
func (s *Scheduler) GetScheduleStatus() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := s.timeNow() // Use timeNow() for testability
	totalNodes := len(s.schedules)
	readyNodes := 0
	failingNodes := 0
	pendingFirstTest := 0
	avgBackoffLevel := 0.0

	for _, schedule := range s.schedules {
		if schedule.NextTestTime.Before(now) || schedule.NextTestTime.Equal(now) {
			readyNodes++
		}
		// Check if node has never been tested
		if schedule.LastTestTime.IsZero() {
			pendingFirstTest++
		} else if !schedule.LastTestSuccess {
			// Only count as failing if it has been tested and failed
			failingNodes++
			avgBackoffLevel += float64(schedule.BackoffLevel)
		}
	}

	if failingNodes > 0 {
		avgBackoffLevel /= float64(failingNodes)
	}

	return map[string]interface{}{
		"total_nodes":        totalNodes,
		"ready_for_test":     readyNodes,
		"failing_nodes":      failingNodes,
		"pending_first_test": pendingFirstTest,
		"avg_backoff_level":  avgBackoffLevel,
		"strategy":           s.strategy.String(),
	}
}

// ResetNodeSchedule resets the schedule for a specific node. It builds the
// same domain-qualified key as nodeKey(); an empty domain means fidonet.
func (s *Scheduler) ResetNodeSchedule(ctx context.Context, zone, net, node uint16, domain string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := (&models.Node{Zone: int(zone), Net: int(net), Node: int(node), Domain: domain}).Key()
	if schedule, exists := s.schedules[key]; exists {
		schedule.ConsecutiveFails = 0
		schedule.BackoffLevel = 0
		schedule.NextTestTime = s.timeNow() // Use timeNow() for testability
	}
}

// SchedulesFor3D returns the scheduled nodes matching a 3D address across all
// FTN networks. Used by AKA-derivation to find the same physical host's
// entries in other networks.
func (s *Scheduler) SchedulesFor3D(zone, net, node int) []*models.Node {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*models.Node
	for _, schedule := range s.schedules {
		n := schedule.Node
		if n.Zone == zone && n.Net == net && n.Node == node {
			result = append(result, n)
		}
	}
	return result
}

// AllScheduledNodes returns a snapshot of all scheduled nodes. Used to seed
// the AKA equivalence index at startup.
func (s *Scheduler) AllScheduledNodes() []*models.Node {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*models.Node, 0, len(s.schedules))
	for _, schedule := range s.schedules {
		result = append(result, schedule.Node)
	}
	return result
}

// LastTestTime returns the recorded last test time for a node key.
func (s *Scheduler) LastTestTime(key string) (t time.Time, ok bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if schedule, exists := s.schedules[key]; exists {
		return schedule.LastTestTime, true
	}
	return time.Time{}, false
}

// MarkDerivedResult updates a node's schedule after its state was covered by
// an AKA-derived result: the node counts as freshly tested and its next direct
// test moves one interval out.
func (s *Scheduler) MarkDerivedResult(node *models.Node, result *models.TestResult) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := s.nodeKey(node)
	schedule, exists := s.schedules[key]
	if !exists {
		schedule = &NodeSchedule{
			Node:     node,
			Priority: s.calculatePriority(node),
		}
		s.schedules[key] = schedule
	}

	schedule.LastTestTime = result.TestTime
	schedule.LastTestSuccess = result.IsOperational
	schedule.TestReason = "aka_derived"

	if result.IsOperational {
		schedule.ConsecutiveFails = 0
		schedule.BackoffLevel = 0
	} else {
		schedule.ConsecutiveFails++
		schedule.BackoffLevel = s.calculateBackoffLevel(schedule.ConsecutiveFails)
	}

	schedule.NextTestTime = s.calculateNextTestTime(schedule)
}

// String returns the string representation of a ScheduleStrategy
func (st ScheduleStrategy) String() string {
	switch st {
	case StrategyRegular:
		return "regular"
	case StrategyAdaptive:
		return "adaptive"
	case StrategyPriority:
		return "priority"
	default:
		return "unknown"
	}
}

// stringSlicesEqual compares two string slices for equality
func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i, v := range a {
		if v != b[i] {
			return false
		}
	}
	return true
}

// hasInternetConfigChanged checks if the internet configuration has changed between old and new node
func (s *Scheduler) hasInternetConfigChanged(oldNode, newNode *models.Node) bool {
	// Check if inet status changed
	if oldNode.HasInet != newNode.HasInet {
		return true
	}

	// Check if hostnames changed
	if !stringSlicesEqual(oldNode.InternetHostnames, newNode.InternetHostnames) {
		return true
	}

	// Check if protocols changed
	if !stringSlicesEqual(oldNode.InternetProtocols, newNode.InternetProtocols) {
		return true
	}

	// Check if protocol ports changed (if available)
	if oldNode.ProtocolPorts != nil && newNode.ProtocolPorts != nil {
		if len(oldNode.ProtocolPorts) != len(newNode.ProtocolPorts) {
			return true
		}
		for proto, oldPort := range oldNode.ProtocolPorts {
			newPort, exists := newNode.ProtocolPorts[proto]
			if !exists || oldPort != newPort {
				return true
			}
		}
	} else if (oldNode.ProtocolPorts == nil) != (newNode.ProtocolPorts == nil) {
		// One has ports, the other doesn't
		return true
	}

	return false
}
