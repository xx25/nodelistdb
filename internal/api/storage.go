package api

import (
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
	GetNodes(filter database.NodeFilter) ([]database.Node, error)
	GetNodeHistory(zone, net, node int, domain string) ([]database.Node, error)
	GetNodeDateRange(zone, net, node int, domain string) (firstDate, lastDate time.Time, err error)
	GetNodeDomains(zone, net, node int) ([]string, error)
	GetNodeChanges(zone, net, node int, domain string) ([]database.NodeChange, error)
	GetDomains() ([]storage.DomainInfo, error)
}

// PointReader is the pointlist side: points under a boss, one point's history,
// and the imported issues behind both.
type PointReader interface {
	SearchPoints(filter database.PointFilter) ([]database.Point, error)
	GetPointsByBoss(domain string, zone, net, node int, asOf *time.Time) ([]database.Point, error)
	GetPointHistory(domain string, zone, net, node, point int) ([]database.Point, error)
	GetPointDomains(zone, net, node int, point *int) ([]string, error)
	GetPointlistDates(domain, listSource string) ([]database.PointlistFile, error)
	GetPointlistSources(domain string) ([]storage.PointlistSourceInfo, error)
}

// StatsReader answers questions about one nodelist date.
type StatsReader interface {
	GetStats(date time.Time, domain string) (*database.NetworkStats, error)
	GetLatestStatsDate(domain string) (time.Time, error)
	GetAvailableDates(domain string) ([]time.Time, error)
	GetNearestAvailableDate(requestedDate time.Time, domain string) (time.Time, error)
}

// SysopReader answers questions about operators rather than nodes.
type SysopReader interface {
	GetUniqueSysops(nameFilter string, limit, offset int) ([]storage.SysopInfo, error)
	GetNodesBySysop(sysopName string, limit int) ([]database.Node, error)
}

// AnalyticsReader is the reports computed from node_test_results.
type AnalyticsReader interface {
	GetBinkPSoftwareDistribution(days int, domain string) (*storage.SoftwareDistribution, error)
	GetIFCICOSoftwareDistribution(days int, domain string) (*storage.SoftwareDistribution, error)
	GetBinkdDetailedStats(days int, domain string) (*storage.SoftwareDistribution, error)
	GetGeoHostingDistribution(days int, domain string) (*storage.GeoHostingDistribution, error)
}

// PSTNStore is the only writable surface the API has: the modem tester's
// record of which phone numbers answer.
type PSTNStore interface {
	GetPSTNNodes(limit int, zone int, domain string) ([]storage.PSTNNode, error)
	GetPSTNDeadNodes() ([]storage.PSTNDeadNode, error)
	GetRecentModemSuccessPhones(days int) ([]string, error)
	MarkPSTNDead(zone, net, node int, reason, markedBy string) error
	UnmarkPSTNDead(zone, net, node int, markedBy string) error
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
