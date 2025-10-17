package daemon

import (
	"testing"
	"time"

	"github.com/nodelistdb/internal/testing/models"
)

func TestSortByPriority(t *testing.T) {
	s := NewScheduler(SchedulerConfig{}, nil)

	nodes := []*NodeSchedule{
		{Node: &models.Node{Zone: 1, Net: 1, Node: 1}, Priority: 50},
		{Node: &models.Node{Zone: 1, Net: 1, Node: 2}, Priority: 80},
		{Node: &models.Node{Zone: 1, Net: 1, Node: 3}, Priority: 30},
		{Node: &models.Node{Zone: 1, Net: 1, Node: 4}, Priority: 90},
		{Node: &models.Node{Zone: 1, Net: 1, Node: 5}, Priority: 60},
	}

	s.sortByPriority(nodes)

	// Should be sorted in descending order
	expectedOrder := []int{90, 80, 60, 50, 30}
	for i, expected := range expectedOrder {
		if nodes[i].Priority != expected {
			t.Errorf("Expected priority %d at position %d, got %d", expected, i, nodes[i].Priority)
		}
	}
}

func TestSortByPriority_EqualPriorities(t *testing.T) {
	s := NewScheduler(SchedulerConfig{}, nil)

	nodes := []*NodeSchedule{
		{Node: &models.Node{Zone: 1, Net: 1, Node: 1}, Priority: 50},
		{Node: &models.Node{Zone: 1, Net: 1, Node: 2}, Priority: 50},
		{Node: &models.Node{Zone: 1, Net: 1, Node: 3}, Priority: 50},
	}

	// Should not panic with equal priorities
	s.sortByPriority(nodes)

	// All should still be 50
	for i, node := range nodes {
		if node.Priority != 50 {
			t.Errorf("Expected priority 50 at position %d, got %d", i, node.Priority)
		}
	}
}

func TestSortByPriority_EmptySlice(t *testing.T) {
	s := NewScheduler(SchedulerConfig{}, nil)

	nodes := []*NodeSchedule{}

	// Should not panic with empty slice
	s.sortByPriority(nodes)

	if len(nodes) != 0 {
		t.Error("Expected empty slice to remain empty")
	}
}

func TestSortByPriority_SingleElement(t *testing.T) {
	s := NewScheduler(SchedulerConfig{}, nil)

	nodes := []*NodeSchedule{
		{Node: &models.Node{Zone: 1, Net: 1, Node: 1}, Priority: 75},
	}

	s.sortByPriority(nodes)

	if len(nodes) != 1 || nodes[0].Priority != 75 {
		t.Error("Single element slice should remain unchanged")
	}
}

func TestCalculateAdaptiveScore_BasicPriority(t *testing.T) {
	s := NewScheduler(SchedulerConfig{}, nil)
	now := time.Now()

	schedule := &NodeSchedule{
		Node:            &models.Node{},
		Priority:        50,
		LastTestTime:    now.Add(-1 * time.Hour),
		LastTestSuccess: true,
		NextTestTime:    now.Add(1 * time.Hour),
	}

	score := s.calculateAdaptiveScore(schedule, now)

	// Base score = Priority (50) + time since last test (0.5 per hour)
	// 50 + (1 * 0.5) = 50.5
	if score < 50.0 || score > 51.0 {
		t.Errorf("Expected score around 50.5, got %f", score)
	}
}

func TestCalculateAdaptiveScore_FailedNode(t *testing.T) {
	s := NewScheduler(SchedulerConfig{}, nil)
	now := time.Now()

	schedule := &NodeSchedule{
		Node:             &models.Node{},
		Priority:         50,
		LastTestTime:     now.Add(-1 * time.Hour),
		LastTestSuccess:  false,
		ConsecutiveFails: 2,
		NextTestTime:     now.Add(1 * time.Hour),
	}

	score := s.calculateAdaptiveScore(schedule, now)

	// Base: 50 + 20 (failed) + 30 (<=3 fails) + 0.5 (time) = 100.5
	if score < 100.0 || score > 101.0 {
		t.Errorf("Expected score around 100.5 for recently failed node, got %f", score)
	}
}

func TestCalculateAdaptiveScore_ManyFailures(t *testing.T) {
	s := NewScheduler(SchedulerConfig{}, nil)
	now := time.Now()

	schedule := &NodeSchedule{
		Node:             &models.Node{},
		Priority:         50,
		LastTestTime:     now.Add(-1 * time.Hour),
		LastTestSuccess:  false,
		ConsecutiveFails: 5,
		NextTestTime:     now.Add(1 * time.Hour),
	}

	score := s.calculateAdaptiveScore(schedule, now)

	// Base: 50 + 20 (failed) + 20 (4-10 fails) + 0.5 (time) = 90.5
	if score < 90.0 || score > 91.0 {
		t.Errorf("Expected score around 90.5 for moderately failed node, got %f", score)
	}
}

func TestCalculateAdaptiveScore_ExtensiveFailures(t *testing.T) {
	s := NewScheduler(SchedulerConfig{}, nil)
	now := time.Now()

	schedule := &NodeSchedule{
		Node:             &models.Node{},
		Priority:         50,
		LastTestTime:     now.Add(-1 * time.Hour),
		LastTestSuccess:  false,
		ConsecutiveFails: 20,
		NextTestTime:     now.Add(1 * time.Hour),
	}

	score := s.calculateAdaptiveScore(schedule, now)

	// Base: 50 + 20 (failed) - 20 (many fails) + 0.5 (time) = 50.5
	if score < 50.0 || score > 51.0 {
		t.Errorf("Expected score around 50.5 for extensively failed node, got %f", score)
	}
}

func TestCalculateAdaptiveScore_Overdue(t *testing.T) {
	s := NewScheduler(SchedulerConfig{}, nil)
	now := time.Now()

	schedule := &NodeSchedule{
		Node:            &models.Node{},
		Priority:        50,
		LastTestTime:    now.Add(-10 * time.Hour),
		LastTestSuccess: true,
		NextTestTime:    now.Add(-2 * time.Hour), // 2 hours overdue
	}

	score := s.calculateAdaptiveScore(schedule, now)

	// Base: 50 + (10 * 0.5) + (2 * 2) = 50 + 5 + 4 = 59
	if score < 58.0 || score > 60.0 {
		t.Errorf("Expected score around 59 for overdue node, got %f", score)
	}
}

func TestCalculateAdaptiveScore_LongTimeSinceTest(t *testing.T) {
	s := NewScheduler(SchedulerConfig{}, nil)
	now := time.Now()

	schedule := &NodeSchedule{
		Node:            &models.Node{},
		Priority:        50,
		LastTestTime:    now.Add(-100 * time.Hour),
		LastTestSuccess: true,
		NextTestTime:    now.Add(1 * time.Hour),
	}

	score := s.calculateAdaptiveScore(schedule, now)

	// Base: 50 + (100 * 0.5) = 100
	if score < 99.0 || score > 101.0 {
		t.Errorf("Expected score around 100 for long-untested node, got %f", score)
	}
}

func TestSortByAdaptive(t *testing.T) {
	s := NewScheduler(SchedulerConfig{}, nil)
	now := time.Now()

	nodes := []*NodeSchedule{
		{
			Node:            &models.Node{Zone: 1, Net: 1, Node: 1},
			Priority:        50,
			LastTestTime:    now.Add(-1 * time.Hour),
			LastTestSuccess: true,
			NextTestTime:    now.Add(1 * time.Hour),
		},
		{
			Node:             &models.Node{Zone: 1, Net: 1, Node: 2},
			Priority:         50,
			LastTestTime:     now.Add(-1 * time.Hour),
			LastTestSuccess:  false,
			ConsecutiveFails: 2,
			NextTestTime:     now.Add(-1 * time.Hour), // Overdue
		},
		{
			Node:            &models.Node{Zone: 1, Net: 1, Node: 3},
			Priority:        90,
			LastTestTime:    now.Add(-1 * time.Hour),
			LastTestSuccess: true,
			NextTestTime:    now.Add(1 * time.Hour),
		},
	}

	s.sortByAdaptive(nodes, now)

	// Node 2 (failed + overdue) should have highest score
	// Node 3 (high priority) should be second
	// Node 1 (normal) should be last
	if nodes[0].Node.Node != 2 {
		t.Errorf("Expected node 2 first (highest adaptive score), got node %d", nodes[0].Node.Node)
	}
}

func TestSortByAdaptive_AllSuccessful(t *testing.T) {
	s := NewScheduler(SchedulerConfig{}, nil)
	now := time.Now()

	nodes := []*NodeSchedule{
		{
			Node:            &models.Node{Zone: 1, Net: 1, Node: 1},
			Priority:        30,
			LastTestTime:    now.Add(-1 * time.Hour),
			LastTestSuccess: true,
			NextTestTime:    now.Add(1 * time.Hour),
		},
		{
			Node:            &models.Node{Zone: 1, Net: 1, Node: 2},
			Priority:        60,
			LastTestTime:    now.Add(-1 * time.Hour),
			LastTestSuccess: true,
			NextTestTime:    now.Add(1 * time.Hour),
		},
		{
			Node:            &models.Node{Zone: 1, Net: 1, Node: 3},
			Priority:        90,
			LastTestTime:    now.Add(-1 * time.Hour),
			LastTestSuccess: true,
			NextTestTime:    now.Add(1 * time.Hour),
		},
	}

	s.sortByAdaptive(nodes, now)

	// Should sort by priority when all else is equal
	// Node 3 (90) > Node 2 (60) > Node 1 (30)
	if nodes[0].Node.Node != 3 || nodes[1].Node.Node != 2 || nodes[2].Node.Node != 1 {
		t.Errorf("Expected nodes sorted by priority: 3,2,1, got %d,%d,%d",
			nodes[0].Node.Node, nodes[1].Node.Node, nodes[2].Node.Node)
	}
}

func TestCalculateAdaptiveScore_ZeroTime(t *testing.T) {
	s := NewScheduler(SchedulerConfig{}, nil)
	now := time.Now()

	schedule := &NodeSchedule{
		Node:            &models.Node{},
		Priority:        50,
		LastTestTime:    time.Time{}, // Zero time
		LastTestSuccess: false,
		ConsecutiveFails: 1,
		NextTestTime:    now.Add(1 * time.Hour),
	}

	score := s.calculateAdaptiveScore(schedule, now)

	// Should handle zero time without panicking
	// Score will be very high due to huge time difference
	if score < 0 {
		t.Errorf("Expected positive score, got %f", score)
	}
}

func TestSortByAdaptive_EmptySlice(t *testing.T) {
	s := NewScheduler(SchedulerConfig{}, nil)
	now := time.Now()

	nodes := []*NodeSchedule{}

	// Should not panic with empty slice
	s.sortByAdaptive(nodes, now)

	if len(nodes) != 0 {
		t.Error("Expected empty slice to remain empty")
	}
}

func TestCalculateAdaptiveScore_ConsecutiveFails_Boundaries(t *testing.T) {
	s := NewScheduler(SchedulerConfig{}, nil)
	now := time.Now()

	tests := []struct {
		name             string
		consecutiveFails int
		expectedBonus    float64 // Approximate expected fail bonus
	}{
		{"no failures", 0, 0},
		{"one failure", 1, 50},   // 20 + 30
		{"three failures", 3, 50}, // 20 + 30
		{"four failures", 4, 40},  // 20 + 20
		{"ten failures", 10, 40},  // 20 + 20
		{"eleven failures", 11, 9}, // 20 - 11
		{"twenty failures", 20, 0}, // 20 - 20
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schedule := &NodeSchedule{
				Node:             &models.Node{},
				Priority:         0, // Zero to isolate fail bonus
				LastTestTime:     now,
				LastTestSuccess:  tt.consecutiveFails > 0,
				ConsecutiveFails: tt.consecutiveFails,
				NextTestTime:     now.Add(1 * time.Hour),
			}

			if tt.consecutiveFails > 0 {
				schedule.LastTestSuccess = false
			}

			score := s.calculateAdaptiveScore(schedule, now)

			// For failed nodes, check the bonus is approximately correct
			if tt.consecutiveFails > 0 {
				if score < tt.expectedBonus-5 || score > tt.expectedBonus+5 {
					t.Errorf("Expected score around %f, got %f", tt.expectedBonus, score)
				}
			}
		})
	}
}
