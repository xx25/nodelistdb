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

	// GetLastKnownIPs returns the most recent addresses a hostname resolved to,
	// for probing a node whose DNS has since broken. Returns nil when there is
	// no successful resolution within maxAge.
	GetLastKnownIPs(ctx context.Context, zone, net, node int, domain, hostname string, maxAge time.Duration) (*LastKnownIPs, error)

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

// LastKnownIPs is the most recent successful DNS resolution recorded for a
// (node, hostname) pair.
//
// ObservedAt is what makes the record useful: the question a fallback address
// has to answer is not "is this address right" but "is an address this old
// still right", and that can only be measured if the age travels with it.
type LastKnownIPs struct {
	IPv4       []string
	IPv6       []string
	ObservedAt time.Time
}
