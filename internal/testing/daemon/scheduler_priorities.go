package daemon

import (
	"context"
	"math"

	"github.com/nodelistdb/internal/testing/models"
)

// calculatePriority determines the priority of a node for testing
func (s *Scheduler) calculatePriority(node *models.Node) int {
	priority := 50

	if node.HasProtocol("IBN") {
		priority += s.priorityBoost
	}
	if node.HasProtocol("ITN") {
		priority += s.priorityBoost / 2
	}
	if len(node.InternetHostnames) > 0 {
		priority += s.priorityBoost
	}

	// Check for internet protocols
	for _, protocol := range node.InternetProtocols {
		switch protocol {
		case "IBN":
			priority += s.priorityBoost
		case "IFC":
			priority += s.priorityBoost
		}
	}

	if priority > 100 {
		priority = 100
	}

	return priority
}

// calculateBackoffLevel determines the backoff level based on consecutive failures
func (s *Scheduler) calculateBackoffLevel(consecutiveFails int) int {
	if consecutiveFails <= 0 {
		return 0
	}

	level := int(math.Log2(float64(consecutiveFails))) + 1
	if level > s.maxBackoffLevel {
		level = s.maxBackoffLevel
	}

	return level
}

// getConsecutiveFailCount retrieves the number of consecutive failures from history
func (s *Scheduler) getConsecutiveFailCount(ctx context.Context, node *models.Node) int {
	results, err := s.storage.GetNodeTestHistory(ctx, node.Zone, node.Net, node.Node, 50)
	if err != nil || len(results) == 0 {
		return 0
	}

	count := 0
	for _, result := range results {
		if !result.IsOperational {
			count++
		} else {
			break
		}
	}

	return count
}
