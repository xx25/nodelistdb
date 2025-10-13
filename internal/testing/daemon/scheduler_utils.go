package daemon

import (
	"context"
	"fmt"

	"github.com/nodelistdb/internal/testing/models"
)

// nodeKey generates a unique key for a node
func (s *Scheduler) nodeKey(node *models.Node) string {
	return fmt.Sprintf("%d:%d/%d", node.Zone, node.Net, node.Node)
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

// ResetNodeSchedule resets the schedule for a specific node
func (s *Scheduler) ResetNodeSchedule(ctx context.Context, zone, net, node uint16) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := fmt.Sprintf("%d:%d/%d", zone, net, node)
	if schedule, exists := s.schedules[key]; exists {
		schedule.ConsecutiveFails = 0
		schedule.BackoffLevel = 0
		schedule.NextTestTime = s.timeNow() // Use timeNow() for testability
	}
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
