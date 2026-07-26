package storage

import (
	"context"
	"time"

	"github.com/nodelistdb/internal/emailflags"
	"github.com/nodelistdb/internal/testing/models"
)

// Storage defines the interface for database operations
type Storage interface {
	// Node reading operations
	GetNodesWithInternet(ctx context.Context, limit int) ([]*models.Node, error)
	GetNodesByZone(ctx context.Context, zone int) ([]*models.Node, error)
	GetNodesByProtocol(ctx context.Context, protocol string, limit int) ([]*models.Node, error)
	GetLatestNodelistDate(ctx context.Context) (time.Time, error)
	GetNodelistFingerprint(ctx context.Context) (string, error)
	
	// Test result storage operations
	StoreTestResult(ctx context.Context, result *models.TestResult) error
	StoreTestResults(ctx context.Context, results []*models.TestResult) error
	StoreDailyStats(ctx context.Context, stats *models.TestStatistics) error
	
	// Query operations
	GetLatestTestResults(ctx context.Context, limit int) ([]*models.TestResult, error)
	GetNodeTestHistory(ctx context.Context, zone, net, node int, domain string, days int) ([]*models.TestResult, error)
	GetRecentAnnouncedAKAs(ctx context.Context, days int) ([]models.AnnouncedAKARecord, error)

	// WHOIS operations
	StoreWhoisResult(ctx context.Context, result *models.WhoisResult) error
	GetRecentWhoisResult(ctx context.Context, domain string, maxAge time.Duration) (*models.WhoisResult, error)

	// Email domain verification (backs the /analytics/email report)
	GetEmailDomainsToCheck(ctx context.Context, staleAfter time.Duration) ([]string, error)
	GetEmailDomainCheck(ctx context.Context, domain string) (*StoredEmailDomainCheck, error)
	StoreEmailDomainCheck(ctx context.Context, result emailflags.DomainResult, previous *StoredEmailDomainCheck) error

	// Lifecycle
	Close() error
}