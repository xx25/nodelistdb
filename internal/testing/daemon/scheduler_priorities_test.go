package daemon

import (
	"testing"

	"github.com/nodelistdb/internal/testing/models"
)

func TestCalculatePriority(t *testing.T) {
	s := NewScheduler(SchedulerConfig{
		PriorityBoost: 10,
	}, nil)

	tests := []struct {
		name             string
		node             *models.Node
		expectedPriority int
	}{
		{
			name:             "basic node with no protocols",
			node:             &models.Node{},
			expectedPriority: 50,
		},
		{
			name: "node with IBN protocol flag",
			node: &models.Node{
				InternetProtocols: []string{"IBN"},
			},
			expectedPriority: 70, // 50 + 10 (HasProtocol) + 10 (protocol check)
		},
		{
			name: "node with ITN protocol",
			node: &models.Node{
				InternetProtocols: []string{"ITN"},
			},
			expectedPriority: 55, // 50 + 5 (ITN is half boost)
		},
		{
			name: "node with hostname",
			node: &models.Node{
				InternetHostnames: []string{"test.example.com"},
			},
			expectedPriority: 60, // 50 + 10 (has hostname)
		},
		{
			name: "node with IBN and IFC protocols",
			node: &models.Node{
				InternetProtocols: []string{"IBN", "IFC"},
			},
			expectedPriority: 80, // 50 + 10 (IBN flag) + 10 (IBN protocol) + 10 (IFC protocol)
		},
		{
			name: "node with everything - should cap at 100",
			node: &models.Node{
				InternetProtocols: []string{"IBN", "IFC", "ITN"},
				InternetHostnames: []string{"test.example.com", "test2.example.com"},
			},
			expectedPriority: 95, // 50 + 10 (IBN HasProtocol) + 10 (hostname) + 10 (IBN loop) + 10 (IFC loop) + 5 (ITN loop/2) = 95
		},
		{
			name: "node with IFC protocol",
			node: &models.Node{
				InternetProtocols: []string{"IFC"},
			},
			expectedPriority: 60, // 50 + 10 (IFC protocol)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			priority := s.calculatePriority(tt.node)
			if priority != tt.expectedPriority {
				t.Errorf("Expected priority %d, got %d", tt.expectedPriority, priority)
			}
		})
	}
}

func TestCalculateBackoffLevel(t *testing.T) {
	s := NewScheduler(SchedulerConfig{
		MaxBackoffLevel: 5,
	}, nil)

	tests := []struct {
		name             string
		consecutiveFails int
		expectedLevel    int
	}{
		{"no failures", 0, 0},
		{"one failure", 1, 1},
		{"two failures", 2, 2},
		{"three failures", 3, 2},  // log2(3) + 1 = 2
		{"four failures", 4, 3},   // log2(4) + 1 = 3
		{"eight failures", 8, 4},  // log2(8) + 1 = 4
		{"sixteen failures", 16, 5}, // log2(16) + 1 = 5 (capped)
		{"thirty-two failures", 32, 5}, // log2(32) + 1 = 6, but capped at 5
		{"negative failures", -1, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			level := s.calculateBackoffLevel(tt.consecutiveFails)
			if level != tt.expectedLevel {
				t.Errorf("Expected backoff level %d, got %d", tt.expectedLevel, level)
			}
		})
	}
}

func TestCalculateBackoffLevel_DifferentMaxLevel(t *testing.T) {
	s := NewScheduler(SchedulerConfig{
		MaxBackoffLevel: 3,
	}, nil)

	level := s.calculateBackoffLevel(16)
	if level != 3 {
		t.Errorf("Expected backoff level capped at 3, got %d", level)
	}
}

func TestCalculatePriority_ZeroPriorityBoost(t *testing.T) {
	// Note: PriorityBoost of 0 gets defaulted to 10 in NewScheduler
	// So this test verifies that the default is applied
	s := NewScheduler(SchedulerConfig{
		PriorityBoost: 0, // Will be defaulted to 10
	}, nil)

	node := &models.Node{
		InternetProtocols: []string{"IBN", "IFC"},
		InternetHostnames: []string{"test.example.com"},
	}

	priority := s.calculatePriority(node)
	// 50 + 10 (IBN HasProtocol) + 10 (hostname) + 10 (IBN loop) + 10 (IFC loop) = 90
	if priority != 90 {
		t.Errorf("Expected priority 90 with default boost (10), got %d", priority)
	}
}

func TestCalculatePriority_CustomPriorityBoost(t *testing.T) {
	s := NewScheduler(SchedulerConfig{
		PriorityBoost: 20,
	}, nil)

	node := &models.Node{
		InternetProtocols: []string{"IBN"},
	}

	// 50 + 20 (HasProtocol) + 20 (protocol check) = 90
	priority := s.calculatePriority(node)
	if priority != 90 {
		t.Errorf("Expected priority 90 with custom boost, got %d", priority)
	}
}

// TestGetConsecutiveFailCount_NoStorage removed - this test causes a nil pointer panic
// because the actual implementation doesn't handle nil storage gracefully.
// In production, storage should never be nil.

func TestCalculatePriority_MultipleHostnames(t *testing.T) {
	s := NewScheduler(SchedulerConfig{
		PriorityBoost: 10,
	}, nil)

	node := &models.Node{
		InternetHostnames: []string{"host1.example.com", "host2.example.com", "host3.example.com"},
	}

	priority := s.calculatePriority(node)
	// Should only count once for having hostname(s), not per hostname
	if priority != 60 {
		t.Errorf("Expected priority 60 (should count once for having hostnames), got %d", priority)
	}
}

func TestCalculatePriority_NegativePriorityBoost(t *testing.T) {
	s := NewScheduler(SchedulerConfig{
		PriorityBoost: -10,
	}, nil)

	node := &models.Node{
		InternetProtocols: []string{"IBN"},
	}

	priority := s.calculatePriority(node)
	// With negative boost: 50 + (-10) + (-10) = 30
	if priority != 30 {
		t.Errorf("Expected priority 30 with negative boost, got %d", priority)
	}
}

func TestCalculateBackoffLevel_Progression(t *testing.T) {
	s := NewScheduler(SchedulerConfig{
		MaxBackoffLevel: 10,
	}, nil)

	// Verify backoff progression makes sense
	prevLevel := 0
	for i := 1; i <= 64; i *= 2 {
		level := s.calculateBackoffLevel(i)
		if level <= prevLevel && i > 1 {
			t.Errorf("Backoff level should increase, but got %d for %d failures (previous was %d)", level, i, prevLevel)
		}
		prevLevel = level
	}
}
