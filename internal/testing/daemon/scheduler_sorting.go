package daemon

import (
	"sort"
	"time"
)

// sortByPriority sorts nodes by priority in descending order
// Using sort.Slice (O(n log n)) instead of bubble sort (O(n²)) for better performance
func (s *Scheduler) sortByPriority(nodes []*NodeSchedule) {
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].Priority > nodes[j].Priority
	})
}

// sortByAdaptive sorts nodes by adaptive score in descending order
// Using sort.Slice (O(n log n)) instead of bubble sort (O(n²)) for better performance
func (s *Scheduler) sortByAdaptive(nodes []*NodeSchedule, now time.Time) {
	sort.Slice(nodes, func(i, j int) bool {
		score1 := s.calculateAdaptiveScore(nodes[i], now)
		score2 := s.calculateAdaptiveScore(nodes[j], now)
		return score1 > score2
	})
}

// calculateAdaptiveScore calculates a score for adaptive scheduling
func (s *Scheduler) calculateAdaptiveScore(schedule *NodeSchedule, now time.Time) float64 {
	timeSinceLastTest := now.Sub(schedule.LastTestTime).Hours()

	score := float64(schedule.Priority)

	if !schedule.LastTestSuccess {
		score += 20.0

		if schedule.ConsecutiveFails <= 3 {
			score += 30.0
		} else if schedule.ConsecutiveFails <= 10 {
			score += 20.0
		} else {
			score -= float64(schedule.ConsecutiveFails)
		}
	}

	score += timeSinceLastTest * 0.5

	if now.After(schedule.NextTestTime) {
		overdue := now.Sub(schedule.NextTestTime).Hours()
		score += overdue * 2
	}

	return score
}
