package daemon

import (
	"math/rand"
	"time"
)

// calculateNextTestTime determines when a node should be tested next
func (s *Scheduler) calculateNextTestTime(schedule *NodeSchedule) time.Time {
	if schedule.LastTestTime.IsZero() {
		// Node has no test history - schedule immediately with small jitter to prevent thundering herd
		// Add 0-5 minutes of jitter
		jitter := time.Duration(rand.Float64() * float64(5*time.Minute))
		nextTime := time.Now().Add(jitter)
		return nextTime
	}

	var interval time.Duration

	switch s.strategy {
	case StrategyAdaptive:
		interval = s.calculateAdaptiveInterval(schedule)
	case StrategyPriority:
		interval = s.calculatePriorityInterval(schedule)
	default:
		interval = s.calculateRegularInterval(schedule)
	}

	interval = s.addJitter(interval)

	if interval < s.minInterval {
		interval = s.minInterval
	} else if interval > s.maxInterval {
		interval = s.maxInterval
	}

	nextTime := schedule.LastTestTime.Add(interval)

	// If the calculated next time is in the past (e.g., daemon was down),
	// calculate how many intervals have passed and schedule for the next one
	now := time.Now()
	if nextTime.Before(now) {
		// Calculate how many intervals have elapsed since last test
		timeSinceLastTest := now.Sub(schedule.LastTestTime)
		intervalsElapsed := int(timeSinceLastTest / interval)

		// Schedule for the next interval boundary
		nextTime = schedule.LastTestTime.Add(time.Duration(intervalsElapsed+1) * interval)

		// Add some jitter to avoid thundering herd after restart
		jitter := time.Duration(rand.Float64() * float64(time.Hour))
		nextTime = nextTime.Add(jitter)
	}

	return nextTime
}

// calculateRegularInterval returns the base interval for regular strategy
func (s *Scheduler) calculateRegularInterval(schedule *NodeSchedule) time.Duration {
	if schedule.LastTestSuccess {
		return s.baseInterval
	}

	// For failed nodes, use the configured failed retry interval
	return s.failedRetryInterval
}

// calculateAdaptiveInterval returns the interval for adaptive strategy
func (s *Scheduler) calculateAdaptiveInterval(schedule *NodeSchedule) time.Duration {
	// For both successful and failed nodes, use the regular interval
	// which already handles the distinction:
	// - Successful nodes: baseInterval (72h)
	// - Failed nodes: failedRetryInterval (24h)
	return s.calculateRegularInterval(schedule)
}

// calculatePriorityInterval returns the interval adjusted by priority
func (s *Scheduler) calculatePriorityInterval(schedule *NodeSchedule) time.Duration {
	baseInterval := s.calculateRegularInterval(schedule)

	priorityFactor := 1.0 - (float64(schedule.Priority) / 100.0 * 0.5)
	if priorityFactor < 0.5 {
		priorityFactor = 0.5
	}

	return time.Duration(float64(baseInterval) * priorityFactor)
}

// addJitter adds random jitter to an interval to prevent thundering herd
func (s *Scheduler) addJitter(interval time.Duration) time.Duration {
	if s.jitterPercent <= 0 {
		return interval
	}

	jitter := float64(interval) * s.jitterPercent
	jitterRange := int64(jitter * 2)
	if jitterRange > 0 {
		randomJitter := rand.Int63n(jitterRange) - int64(jitter)
		return interval + time.Duration(randomJitter)
	}

	return interval
}
