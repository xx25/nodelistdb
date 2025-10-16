package daemon

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/nodelistdb/internal/testing/logging"
	"github.com/nodelistdb/internal/testing/models"
	"github.com/nodelistdb/internal/testing/storage"
	"github.com/nodelistdb/internal/testing/timeavail"
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
	TestReason       string // Reason for current/next test: "stale", "new", "config_changed", "scheduled", "failed_retry"
}

type Scheduler struct {
	mu sync.RWMutex

	baseInterval        time.Duration
	minInterval         time.Duration
	maxInterval         time.Duration
	failureMultiplier   float64
	maxBackoffLevel     int
	strategy            ScheduleStrategy
	staleTestThreshold  time.Duration // Consider test stale after this duration
	failedRetryInterval time.Duration // Retry failed nodes after this duration

	schedules map[string]*NodeSchedule
	storage   storage.Storage

	jitterPercent float64
	priorityBoost int
}

type SchedulerConfig struct {
	BaseInterval        time.Duration
	MinInterval         time.Duration
	MaxInterval         time.Duration
	FailureMultiplier   float64
	MaxBackoffLevel     int
	Strategy            ScheduleStrategy
	JitterPercent       float64
	PriorityBoost       int
	StaleTestThreshold  time.Duration
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
		baseInterval:        cfg.BaseInterval,
		minInterval:         cfg.MinInterval,
		maxInterval:         cfg.MaxInterval,
		failureMultiplier:   cfg.FailureMultiplier,
		maxBackoffLevel:     cfg.MaxBackoffLevel,
		strategy:            cfg.Strategy,
		staleTestThreshold:  cfg.StaleTestThreshold,
		failedRetryInterval: cfg.FailedRetryInterval,
		schedules:           make(map[string]*NodeSchedule),
		storage:             storage,
		jitterPercent:       cfg.JitterPercent,
		priorityBoost:       cfg.PriorityBoost,
	}
}

// timeNow returns the current time (extracted for testability)
func (s *Scheduler) timeNow() time.Time {
	return time.Now()
}

func (s *Scheduler) InitializeSchedules(ctx context.Context, nodes []*models.Node) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	logging.Debugf("InitializeSchedules: Processing %d nodes", len(nodes))
	nodesWithHistory := 0
	nodesWithoutHistory := 0
	failedQueries := 0

	for i, node := range nodes {
		key := s.nodeKey(node)

		// Parse time availability if flags are present
		if len(node.Flags) > 0 && node.Availability == nil {
			phoneNumber := "" // TODO: Get phone number from database if available
			availability, err := timeavail.ParseAvailability(node.Flags, node.Zone, phoneNumber)
			if err != nil {
				logging.Debugf("Failed to parse availability for %s: %v", key, err)
			} else {
				node.Availability = availability
			}
		}

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
		if (i+1)%100 == 0 {
			logging.Debugf("InitializeSchedules: Processed %d/%d nodes (with_history=%d, without=%d, failed=%d)",
				i+1, len(nodes), nodesWithHistory, nodesWithoutHistory, failedQueries)
		}

		schedule := &NodeSchedule{
			Node:             node,
			LastTestTime:     time.Time{},
			LastTestSuccess:  false,
			ConsecutiveFails: 0,
			Priority:         s.calculatePriority(node),
			BackoffLevel:     0,
			TestReason:       "new", // Default to "new" until we check history
		}

		if err == nil && lastResult != nil {
			schedule.LastTestTime = lastResult.TestTime
			schedule.LastTestSuccess = lastResult.IsOperational

			// Determine test reason based on history
			timeSinceLastTest := time.Since(lastResult.TestTime)
			if timeSinceLastTest > s.staleTestThreshold {
				schedule.TestReason = "stale"
			} else if !lastResult.IsOperational {
				schedule.TestReason = "failed_retry"
				schedule.ConsecutiveFails = s.getConsecutiveFailCount(ctx, node)
				schedule.BackoffLevel = s.calculateBackoffLevel(schedule.ConsecutiveFails)
			} else {
				schedule.TestReason = "scheduled"
			}
		} else if err != nil {
			// Log error retrieving test history
			logging.Debugf("InitializeSchedules: Failed to get test history for %s: %v", key, err)
		}

		schedule.NextTestTime = s.calculateNextTestTime(schedule)
		s.schedules[key] = schedule
	}

	logging.Debugf("InitializeSchedules: Final stats - nodes_with_history=%d, without_history=%d, failed_queries=%d",
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
	nodesWithChangedConfig := 0

	for _, node := range nodes {
		key := s.nodeKey(node)
		currentNodeKeys[key] = true

		// Parse time availability if flags are present
		if len(node.Flags) > 0 && node.Availability == nil {
			phoneNumber := "" // TODO: Get phone number from database if available
			availability, err := timeavail.ParseAvailability(node.Flags, node.Zone, phoneNumber)
			if err != nil {
				logging.Debugf("Failed to parse availability for %s: %v", key, err)
			} else {
				node.Availability = availability
			}
		}

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
				NextTestTime:     s.timeNow(), // Test new nodes immediately
				TestReason:       "new",
			}
			s.schedules[key] = schedule
			logging.Debugf("New node %s added to scheduler, will test immediately", key)
		} else {
			// Check if internet configuration has changed
			configChanged := s.hasInternetConfigChanged(existingSchedule.Node, node)

			// Update existing node data (hostname, protocols might have changed)
			existingSchedule.Node = node
			existingSchedule.Priority = s.calculatePriority(node)

			// If internet configuration changed, schedule immediate retest
			if configChanged {
				existingSchedule.NextTestTime = s.timeNow()
				existingSchedule.BackoffLevel = 0 // Reset backoff since config changed
				existingSchedule.TestReason = "config_changed"
				nodesWithChangedConfig++

				logging.Infof("Internet config changed for %s, scheduling immediate retest", key)
			}
		}
	}

	// Remove schedules for nodes that no longer exist in database
	for key := range s.schedules {
		if !currentNodeKeys[key] {
			delete(s.schedules, key)
		}
	}

	if nodesWithChangedConfig > 0 {
		logging.Infof("Found %d nodes with changed internet configuration, scheduled for immediate retest", nodesWithChangedConfig)
	}

	return nil
}

func (s *Scheduler) GetNodesForTesting(ctx context.Context, maxNodes int) []*models.Node {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.timeNow()
	var readyNodes []*NodeSchedule
	var allFutureNodes []*NodeSchedule
	var notInCallWindow []*NodeSchedule
	staleNodes := 0
	skippedForTimeWindow := 0

	for _, schedule := range s.schedules {
		// Check if test is stale (hasn't been tested in staleTestThreshold duration)
		isStale := !schedule.LastTestTime.IsZero() && now.Sub(schedule.LastTestTime) > s.staleTestThreshold

		// Check if node is ready based on schedule OR if test is stale
		if schedule.NextTestTime.Before(now) || schedule.NextTestTime.Equal(now) || isStale {
			// Check time availability if node has it configured
			if schedule.Node.Availability != nil && !schedule.Node.Availability.IsCallableNow(now) {
				// Node is ready but outside call window
				schedule.TestReason = "outside_call_window"
				notInCallWindow = append(notInCallWindow, schedule)
				skippedForTimeWindow++
				continue
			}

			// Update test reason based on current state
			if isStale {
				schedule.TestReason = "stale"
				staleNodes++
			} else if schedule.TestReason == "" || schedule.TestReason == "scheduled" {
				// Update reason if not already set by RefreshNodes
				if schedule.ConsecutiveFails > 0 {
					schedule.TestReason = "failed_retry"
				} else {
					schedule.TestReason = "scheduled"
				}
			}

			readyNodes = append(readyNodes, schedule)
		} else {
			// Collect ALL future nodes
			allFutureNodes = append(allFutureNodes, schedule)
		}
	}

	// Sort future nodes by NextTestTime to show which will be tested soonest
	sort.Slice(allFutureNodes, func(i, j int) bool {
		return allFutureNodes[i].NextTestTime.Before(allFutureNodes[j].NextTestTime)
	})

	logging.Debugf("GetNodesForTesting: now=%v, ready=%d (stale=%d), future=%d, outside_call_window=%d, total=%d, staleThreshold=%v",
		now, len(readyNodes), staleNodes, len(allFutureNodes), skippedForTimeWindow, len(s.schedules), s.staleTestThreshold)

	// Log nodes skipped due to time windows
	if skippedForTimeWindow > 0 {
		logging.Infof("Skipped %d nodes outside their call windows", skippedForTimeWindow)
		if len(notInCallWindow) > 0 {
			showCount := 5
			if len(notInCallWindow) < showCount {
				showCount = len(notInCallWindow)
			}
			for i := 0; i < showCount; i++ {
				node := notInCallWindow[i]
				nodeAddr := fmt.Sprintf("%d:%d/%d", node.Node.Zone, node.Node.Net, node.Node.Node)
				availStr := "no availability info"
				if node.Node.Availability != nil {
					availStr = timeavail.FormatAvailability(node.Node.Availability)
				}
				logging.Debugf("  Skipped %s: %s", nodeAddr, availStr)
			}
		}
	}

	// Show the next 15 nodes that will be tested
	if len(allFutureNodes) > 0 {
		logging.Debug("GetNodesForTesting: Next nodes to be tested (sorted by time):")
		showCount := 15
		if len(allFutureNodes) < showCount {
			showCount = len(allFutureNodes)
		}
		for i := 0; i < showCount; i++ {
			node := allFutureNodes[i]
			nodeAddr := fmt.Sprintf("%d:%d/%d", node.Node.Zone, node.Node.Net, node.Node.Node)
			logging.Debugf("  [%d] %s -> %v (in %v)", i, nodeAddr, node.NextTestTime, node.NextTestTime.Sub(now))
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
		// Copy the node and include the test reason
		node := readyNodes[i].Node
		node.TestReason = readyNodes[i].TestReason
		result[i] = node
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
