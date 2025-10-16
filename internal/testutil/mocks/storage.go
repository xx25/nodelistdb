package mocks

import (
	"fmt"
	"sync"
	"time"

	"github.com/nodelistdb/internal/database"
)

// MockStorage is a mock implementation for storage operations
type MockStorage struct {
	mu sync.RWMutex

	// Mock data
	Nodes       []database.Node
	Stats       *database.NetworkStats
	NodeHistory []database.Node

	// Behavior configuration
	ShouldFailGetNodes       bool
	ShouldFailInsertNodes    bool
	ShouldFailGetNodeHistory bool
	ShouldFailCountNodes     bool
	ShouldFailGetStats       bool

	// Call tracking
	GetNodesCalled       int
	InsertNodesCalled    int
	GetNodeHistoryCalled int
	CountNodesCalled     int
	GetStatsCalled       int
}

// NewMockStorage creates a new mock storage
func NewMockStorage() *MockStorage {
	return &MockStorage{
		Nodes: []database.Node{},
		Stats: &database.NetworkStats{
			TotalNodes:  1000,
			ActiveNodes: 950,
			Date:        time.Now(),
		},
		NodeHistory: []database.Node{},
	}
}

// GetNodes mocks getting nodes
func (m *MockStorage) GetNodes(filter database.NodeFilter) ([]database.Node, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.GetNodesCalled++

	if m.ShouldFailGetNodes {
		return nil, fmt.Errorf("mock get nodes error")
	}

	// Simple filtering
	var result []database.Node
	for _, node := range m.Nodes {
		if filter.Zone != nil && node.Zone != *filter.Zone {
			continue
		}
		if filter.Net != nil && node.Net != *filter.Net {
			continue
		}
		if filter.Node != nil && node.Node != *filter.Node {
			continue
		}
		result = append(result, node)
	}

	// Apply limit
	if filter.Limit > 0 && len(result) > filter.Limit {
		result = result[:filter.Limit]
	}

	return result, nil
}

// InsertNodes mocks inserting nodes
func (m *MockStorage) InsertNodes(nodes []database.Node) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.InsertNodesCalled++

	if m.ShouldFailInsertNodes {
		return fmt.Errorf("mock insert nodes error")
	}

	m.Nodes = append(m.Nodes, nodes...)
	return nil
}

// GetNodeHistory mocks getting node history
func (m *MockStorage) GetNodeHistory(zone, net, node int) ([]database.Node, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.GetNodeHistoryCalled++

	if m.ShouldFailGetNodeHistory {
		return nil, fmt.Errorf("mock get node history error")
	}

	return m.NodeHistory, nil
}

// CountNodes mocks counting nodes
func (m *MockStorage) CountNodes(date time.Time) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.CountNodesCalled++

	if m.ShouldFailCountNodes {
		return 0, fmt.Errorf("mock count nodes error")
	}

	return len(m.Nodes), nil
}

// GetStats mocks getting statistics
func (m *MockStorage) GetStats(date time.Time) (*database.NetworkStats, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.GetStatsCalled++

	if m.ShouldFailGetStats {
		return nil, fmt.Errorf("mock get stats error")
	}

	return m.Stats, nil
}

// AddNode is a helper to add a node to the mock storage
func (m *MockStorage) AddNode(node database.Node) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Nodes = append(m.Nodes, node)
}

// AddNodes is a helper to add multiple nodes
func (m *MockStorage) AddNodes(nodes ...database.Node) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Nodes = append(m.Nodes, nodes...)
}

// SetNodes replaces all nodes
func (m *MockStorage) SetNodes(nodes []database.Node) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Nodes = nodes
}

// SetStats sets the stats
func (m *MockStorage) SetStats(stats *database.NetworkStats) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Stats = stats
}

// SetNodeHistory sets the node history
func (m *MockStorage) SetNodeHistory(history []database.Node) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.NodeHistory = history
}

// Reset resets all call tracking
func (m *MockStorage) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.GetNodesCalled = 0
	m.InsertNodesCalled = 0
	m.GetNodeHistoryCalled = 0
	m.CountNodesCalled = 0
	m.GetStatsCalled = 0

	m.ShouldFailGetNodes = false
	m.ShouldFailInsertNodes = false
	m.ShouldFailGetNodeHistory = false
	m.ShouldFailCountNodes = false
	m.ShouldFailGetStats = false

	m.Nodes = []database.Node{}
	m.NodeHistory = []database.Node{}
}

// Clear clears all stored data
func (m *MockStorage) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.Nodes = []database.Node{}
	m.NodeHistory = []database.Node{}
}
