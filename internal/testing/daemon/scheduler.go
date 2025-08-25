package daemon

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"sync"
	"time"

	"github.com/nodelistdb/internal/testing/models"
	"github.com/nodelistdb/internal/testing/storage"
)

type ScheduleStrategy int

const (
	StrategyRegular ScheduleStrategy = iota
	StrategyAdaptive
	StrategyPriority
)

type NodeSchedule struct {
	Node             *models.Node
	LastTestTime     time.Time
	LastTestSuccess  bool
	ConsecutiveFails int
	NextTestTime     time.Time
	Priority         int
	BackoffLevel     int
}

type Scheduler struct {
	mu sync.RWMutex

	baseInterval      time.Duration
	minInterval       time.Duration
	maxInterval       time.Duration
	failureMultiplier float64
	maxBackoffLevel   int
	strategy          ScheduleStrategy
	staleTestThreshold time.Duration  // Consider test stale after this duration
	failedRetryInterval time.Duration // Retry failed nodes after this duration

	schedules map[string]*NodeSchedule
	storage   storage.Storage

	jitterPercent float64
	priorityBoost int
}

type SchedulerConfig struct {
	BaseInterval      time.Duration
	MinInterval       time.Duration
	MaxInterval       time.Duration
	FailureMultiplier float64
	MaxBackoffLevel   int
	Strategy          ScheduleStrategy
	JitterPercent     float64
	PriorityBoost     int
	StaleTestThreshold time.Duration
	FailedRetryInterval time.Duration
}

func NewScheduler(cfg SchedulerConfig, storage storage.Storage) *Scheduler {
	if cfg.BaseInterval == 0 {
		cfg.BaseInterval = 1 * time.Hour
	}
	if cfg.MinInterval == 0 {
		cfg.MinInterval = 5 * time.Minute
	}
	if cfg.MaxInterval == 0 {
		cfg.MaxInterval = 24 * time.Hour
	}
	if cfg.FailureMultiplier == 0 {
		cfg.FailureMultiplier = 0.5
	}
	if cfg.MaxBackoffLevel == 0 {
		cfg.MaxBackoffLevel = 5
	}
	if cfg.JitterPercent == 0 {
		cfg.JitterPercent = 0.1
	}
	if cfg.PriorityBoost == 0 {
		cfg.PriorityBoost = 10
	}
	if cfg.StaleTestThreshold == 0 {
		cfg.StaleTestThreshold = cfg.BaseInterval
	}
	if cfg.FailedRetryInterval == 0 {
		cfg.FailedRetryInterval = 24 * time.Hour
	}

	return &Scheduler{
		baseInterval:      cfg.BaseInterval,
		minInterval:       cfg.MinInterval,
		maxInterval:       cfg.MaxInterval,
		failureMultiplier: cfg.FailureMultiplier,
		maxBackoffLevel:   cfg.MaxBackoffLevel,
		strategy:          cfg.Strategy,
		staleTestThreshold: cfg.StaleTestThreshold,
		failedRetryInterval: cfg.FailedRetryInterval,
		schedules:         make(map[string]*NodeSchedule),
		storage:           storage,
		jitterPercent:     cfg.JitterPercent,
		priorityBoost:     cfg.PriorityBoost,
	}
}

func (s *Scheduler) InitializeSchedules(ctx context.Context, nodes []*models.Node) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	fmt.Printf("DEBUG InitializeSchedules: Processing %d nodes\n", len(nodes))
	nodesWithHistory := 0
	nodesWithoutHistory := 0
	failedQueries := 0

	for i, node := range nodes {
		key := s.nodeKey(node)
		
		// Get last test result for this node
		history, err := s.storage.GetNodeTestHistory(ctx, node.Zone, node.Net, node.Node, 1)
		var lastResult *models.TestResult
		if err == nil && len(history) > 0 {
			lastResult = history[0]
			nodesWithHistory++
		} else if err == nil && len(history) == 0 {
			nodesWithoutHistory++
		} else {
			failedQueries++
		}
		
		// Log progress every 100 nodes
		if (i+1) % 100 == 0 {
			fmt.Printf("DEBUG InitializeSchedules: Processed %d/%d nodes (with_history=%d, without=%d, failed=%d)\n", 
				i+1, len(nodes), nodesWithHistory, nodesWithoutHistory, failedQueries)
		}
		
		schedule := &NodeSchedule{
			Node:             node,
			LastTestTime:     time.Time{},
			LastTestSuccess:  false,
			ConsecutiveFails: 0,
			Priority:         s.calculatePriority(node),
			BackoffLevel:     0,
		}

		if err == nil && lastResult != nil {
			schedule.LastTestTime = lastResult.TestTime
			schedule.LastTestSuccess = lastResult.IsOperational
			
			if !lastResult.IsOperational {
				schedule.ConsecutiveFails = s.getConsecutiveFailCount(ctx, node)
				schedule.BackoffLevel = s.calculateBackoffLevel(schedule.ConsecutiveFails)
			}
		} else if err != nil {
			// Log error retrieving test history
			fmt.Printf("DEBUG InitializeSchedules: Failed to get test history for %s: %v\n", key, err)
		}

		schedule.NextTestTime = s.calculateNextTestTime(schedule)
		s.schedules[key] = schedule
	}

	fmt.Printf("DEBUG InitializeSchedules: Final stats - nodes_with_history=%d, without_history=%d, failed_queries=%d\n",
		nodesWithHistory, nodesWithoutHistory, failedQueries)

	return nil
}

// RefreshNodes updates the scheduler with fresh node data from the database
// This should be called periodically to pick up changes made by the parser
func (s *Scheduler) RefreshNodes(ctx context.Context) error {
	// Get fresh nodes from database
	nodes, err := s.storage.GetNodesWithInternet(ctx, 0)
	if err != nil {
		return fmt.Errorf("failed to get nodes from database: %w", err)
	}
	
	s.mu.Lock()
	defer s.mu.Unlock()
	
	// Track which nodes are still in database
	currentNodeKeys := make(map[string]bool)
	
	for _, node := range nodes {
		key := s.nodeKey(node)
		currentNodeKeys[key] = true
		
		// Check if this is a new node or existing one
		existingSchedule, exists := s.schedules[key]
		if !exists {
			// New node - initialize schedule for it
			schedule := &NodeSchedule{
				Node:             node,
				LastTestTime:     time.Time{},
				LastTestSuccess:  false,
				ConsecutiveFails: 0,
				Priority:         s.calculatePriority(node),
				BackoffLevel:     0,
				NextTestTime:     time.Now(), // Test new nodes immediately
			}
			s.schedules[key] = schedule
		} else {
			// Update existing node data (hostname, protocols might have changed)
			existingSchedule.Node = node
			existingSchedule.Priority = s.calculatePriority(node)
		}
	}
	
	// Remove schedules for nodes that no longer exist in database
	for key := range s.schedules {
		if !currentNodeKeys[key] {
			delete(s.schedules, key)
		}
	}
	
	return nil
}

func (s *Scheduler) GetNodesForTesting(ctx context.Context, maxNodes int) []*models.Node {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := time.Now()
	var readyNodes []*NodeSchedule
	futureNodes := 0
	staleNodes := 0
	sampleFutureTimes := []time.Time{}

	for _, schedule := range s.schedules {
		// Check if test is stale (hasn't been tested in staleTestThreshold duration)
		isStale := !schedule.LastTestTime.IsZero() && now.Sub(schedule.LastTestTime) > s.staleTestThreshold
		
		// Check if node is ready based on schedule OR if test is stale
		if schedule.NextTestTime.Before(now) || schedule.NextTestTime.Equal(now) || isStale {
			readyNodes = append(readyNodes, schedule)
			if isStale {
				staleNodes++
			}
		} else {
			futureNodes++
			// Collect sample of future times for debugging
			if len(sampleFutureTimes) < 5 {
				sampleFutureTimes = append(sampleFutureTimes, schedule.NextTestTime)
			}
		}
	}

	fmt.Printf("DEBUG GetNodesForTesting: now=%v, ready=%d (stale=%d), future=%d, total=%d, staleThreshold=%v\n", 
		now, len(readyNodes), staleNodes, futureNodes, len(s.schedules), s.staleTestThreshold)
	
	if len(sampleFutureTimes) > 0 {
		fmt.Printf("DEBUG GetNodesForTesting: Sample future NextTestTimes:\n")
		for i, t := range sampleFutureTimes {
			fmt.Printf("  [%d] %v (in %v)\n", i, t, t.Sub(now))
		}
	}

	if s.strategy == StrategyPriority {
		s.sortByPriority(readyNodes)
	} else if s.strategy == StrategyAdaptive {
		s.sortByAdaptive(readyNodes, now)
	}

	resultCount := len(readyNodes)
	if maxNodes > 0 && resultCount > maxNodes {
		resultCount = maxNodes
	}

	result := make([]*models.Node, resultCount)
	for i := 0; i < resultCount; i++ {
		result[i] = readyNodes[i].Node
	}

	return result
}

func (s *Scheduler) UpdateTestResult(ctx context.Context, node *models.Node, result *models.TestResult) {
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

	if result.IsOperational {
		schedule.ConsecutiveFails = 0
		schedule.BackoffLevel = 0
	} else {
		schedule.ConsecutiveFails++
		schedule.BackoffLevel = s.calculateBackoffLevel(schedule.ConsecutiveFails)
	}

	schedule.NextTestTime = s.calculateNextTestTime(schedule)
}

func (s *Scheduler) calculateNextTestTime(schedule *NodeSchedule) time.Time {
	if schedule.LastTestTime.IsZero() {
		// Node has no test history - schedule randomly within base interval
		randomDelay := s.addJitter(time.Duration(rand.Float64() * float64(s.baseInterval)))
		nextTime := time.Now().Add(randomDelay)
		// Debug logging disabled
		// if schedule.Node.Zone == 1 && schedule.Node.Net < 110 {
		// 	fmt.Printf("DEBUG calculateNextTestTime: Node %s has no history, scheduling for %v (in %v)\n", 
		// 		s.nodeKey(schedule.Node), nextTime, randomDelay)
		// }
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
	
	// Debug logging disabled
	// if schedule.Node.Zone == 1 && schedule.Node.Net < 110 {
	// 	fmt.Printf("DEBUG calculateNextTestTime: Node %s last_test=%v, interval=%v, next=%v\n",
	// 		s.nodeKey(schedule.Node), schedule.LastTestTime, interval, nextTime)
	// }
	
	return nextTime
}

func (s *Scheduler) calculateRegularInterval(schedule *NodeSchedule) time.Duration {
	if schedule.LastTestSuccess {
		return s.baseInterval
	}

	// For failed nodes, use the configured failed retry interval
	return s.failedRetryInterval
}

func (s *Scheduler) calculateAdaptiveInterval(schedule *NodeSchedule) time.Duration {
	baseInterval := s.calculateRegularInterval(schedule)

	if !schedule.LastTestSuccess {
		if schedule.ConsecutiveFails <= 3 {
			return time.Duration(float64(baseInterval) * 0.3)
		} else if schedule.ConsecutiveFails <= 10 {
			return time.Duration(float64(baseInterval) * 0.5)
		} else if schedule.ConsecutiveFails <= 20 {
			return baseInterval
		} else {
			return time.Duration(float64(baseInterval) * 2.0)
		}
	}

	return baseInterval
}

func (s *Scheduler) calculatePriorityInterval(schedule *NodeSchedule) time.Duration {
	baseInterval := s.calculateRegularInterval(schedule)
	
	priorityFactor := 1.0 - (float64(schedule.Priority) / 100.0 * 0.5)
	if priorityFactor < 0.5 {
		priorityFactor = 0.5
	}

	return time.Duration(float64(baseInterval) * priorityFactor)
}

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

func (s *Scheduler) sortByPriority(nodes []*NodeSchedule) {
	for i := 0; i < len(nodes)-1; i++ {
		for j := 0; j < len(nodes)-i-1; j++ {
			if nodes[j].Priority < nodes[j+1].Priority {
				nodes[j], nodes[j+1] = nodes[j+1], nodes[j]
			}
		}
	}
}

func (s *Scheduler) sortByAdaptive(nodes []*NodeSchedule, now time.Time) {
	for i := 0; i < len(nodes)-1; i++ {
		for j := 0; j < len(nodes)-i-1; j++ {
			score1 := s.calculateAdaptiveScore(nodes[j], now)
			score2 := s.calculateAdaptiveScore(nodes[j+1], now)
			if score1 < score2 {
				nodes[j], nodes[j+1] = nodes[j+1], nodes[j]
			}
		}
	}
}

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

func (s *Scheduler) nodeKey(node *models.Node) string {
	return fmt.Sprintf("%d:%d/%d", node.Zone, node.Net, node.Node)
}

func (s *Scheduler) GetScheduleStatus() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := time.Now()
	totalNodes := len(s.schedules)
	readyNodes := 0
	failingNodes := 0
	avgBackoffLevel := 0.0
	
	var nextTestTimes []time.Time

	for _, schedule := range s.schedules {
		if schedule.NextTestTime.Before(now) || schedule.NextTestTime.Equal(now) {
			readyNodes++
		}
		if !schedule.LastTestSuccess {
			failingNodes++
			avgBackoffLevel += float64(schedule.BackoffLevel)
		}
		nextTestTimes = append(nextTestTimes, schedule.NextTestTime)
	}

	if failingNodes > 0 {
		avgBackoffLevel /= float64(failingNodes)
	}

	return map[string]interface{}{
		"total_nodes":       totalNodes,
		"ready_for_test":    readyNodes,
		"failing_nodes":     failingNodes,
		"avg_backoff_level": avgBackoffLevel,
		"strategy":          s.strategy.String(),
	}
}

func (s *Scheduler) ResetNodeSchedule(ctx context.Context, zone, net, node uint16) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := fmt.Sprintf("%d:%d/%d", zone, net, node)
	if schedule, exists := s.schedules[key]; exists {
		schedule.ConsecutiveFails = 0
		schedule.BackoffLevel = 0
		schedule.NextTestTime = time.Now()
	}
}

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