package storage

import (
	"context"
	
	"github.com/nodelistdb/internal/testing/models"
)

// Storage defines the interface for database operations
type Storage interface {
	// Node reading operations
	GetNodesWithInternet(ctx context.Context, limit int) ([]*models.Node, error)
	GetNodesByZone(ctx context.Context, zone int) ([]*models.Node, error)
	GetNodesByProtocol(ctx context.Context, protocol string, limit int) ([]*models.Node, error)
	GetStatistics(ctx context.Context) (map[string]int, error)
	
	// Test result storage operations
	StoreTestResult(ctx context.Context, result *models.TestResult) error
	StoreTestResults(ctx context.Context, results []*models.TestResult) error
	StoreDailyStats(ctx context.Context, stats *models.TestStatistics) error
	
	// Query operations
	GetLatestTestResults(ctx context.Context, limit int) ([]*models.TestResult, error)
	GetNodeTestHistory(ctx context.Context, zone, net, node int, days int) ([]*models.TestResult, error)
	
	// Lifecycle
	Close() error
}