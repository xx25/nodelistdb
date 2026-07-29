package api

import (
	"context"
	"time"

	"github.com/nodelistdb/internal/database"
	"github.com/nodelistdb/internal/storage"
)

// The storage this package needs, declared by the consumer rather than taken
// wholesale from the provider.
//
// storage.Operations carries 89 methods because it is the union of everything
// every consumer wants. Depending on it here made two things worse: a reader
// could not tell which of the 89 the API actually calls, and a test double had
// to satisfy all of them. Splitting it into five per-subject readers costs
// nothing at the call site - *storage.CachedStorage satisfies them all without
// being told - and makes the API's storage footprint the twenty-seven methods
// listed below.

// NodeReader is the nodelist itself: what a node is, was, and which networks
// it appears in.
type NodeReader interface {
	GetNodes(ctx context.Context, filter database.NodeFilter) ([]database.Node, error)
	GetNodeHistory(ctx context.Context, zone, net, node int, domain string) ([]database.Node, error)
	GetNodeDateRange(ctx context.Context, zone, net, node int, domain string) (firstDate, lastDate time.Time, err error)
	GetNodeDomains(ctx context.Context, zone, net, node int) ([]string, error)
	GetNodeChanges(ctx context.Context, zone, net, node int, domain string) ([]database.NodeChange, error)
	GetDomains(ctx context.Context) ([]storage.DomainInfo, error)
}

// PointReader is the pointlist side: points under a boss, one point's history,
// and the imported issues behind both.
type PointReader interface {
	SearchPoints(ctx context.Context, filter database.PointFilter) ([]database.Point, error)
	GetPointsByBoss(ctx context.Context, domain string, zone, net, node int, asOf *time.Time) ([]database.Point, error)
	GetPointHistory(ctx context.Context, domain string, zone, net, node, point int) ([]database.Point, error)
	GetPointDomains(ctx context.Context, zone, net, node int, point *int) ([]string, error)
	GetPointlistDates(ctx context.Context, domain, listSource string) ([]database.PointlistFile, error)
	GetPointlistSources(ctx context.Context, domain string) ([]storage.PointlistSourceInfo, error)
}

// StatsReader answers questions about one nodelist date.
type StatsReader interface {
	GetStats(ctx context.Context, date time.Time, domain string) (*database.NetworkStats, error)
	GetLatestStatsDate(ctx context.Context, domain string) (time.Time, error)
	GetAvailableDates(ctx context.Context, domain string) ([]time.Time, error)
	GetNearestAvailableDate(ctx context.Context, requestedDate time.Time, domain string) (time.Time, error)
}

// SysopReader answers questions about operators rather than nodes.
type SysopReader interface {
	GetUniqueSysops(ctx context.Context, nameFilter string, limit, offset int) ([]storage.SysopInfo, error)
	GetNodesBySysop(ctx context.Context, sysopName string, limit int) ([]database.Node, error)
}

// AnalyticsReader is the reports computed from node_test_results.
type AnalyticsReader interface {
	GetBinkPSoftwareDistribution(ctx context.Context, days int, domain string) (*storage.SoftwareDistribution, error)
	GetIFCICOSoftwareDistribution(ctx context.Context, days int, domain string) (*storage.SoftwareDistribution, error)
	GetBinkdDetailedStats(ctx context.Context, days int, domain string) (*storage.SoftwareDistribution, error)
	GetGeoHostingDistribution(ctx context.Context, days int, domain string) (*storage.GeoHostingDistribution, error)
}

// PSTNStore is the only writable surface the API has: the modem tester's
// record of which phone numbers answer.
type PSTNStore interface {
	GetPSTNNodes(ctx context.Context, limit int, zone int, domain string) ([]storage.PSTNNode, error)
	GetPSTNDeadNodes(ctx context.Context) ([]storage.PSTNDeadNode, error)
	GetRecentModemSuccessPhones(ctx context.Context, days int) ([]string, error)
	MarkPSTNDead(ctx context.Context, zone, net, node int, reason, markedBy string) error
	UnmarkPSTNDead(ctx context.Context, zone, net, node int, markedBy string) error
}

// Storage is everything the API server reads or writes.
type Storage interface {
	NodeReader
	PointReader
	StatsReader
	SysopReader
	AnalyticsReader
	PSTNStore
}

// storage.Operations must remain a superset of what this package needs, or
// cmd/server cannot pass its CachedStorage in. Checked at compile time.
var _ Storage = (storage.Operations)(nil)
