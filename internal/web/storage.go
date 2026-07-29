package web

import (
	"context"
	"time"

	"github.com/nodelistdb/internal/database"
	"github.com/nodelistdb/internal/storage"
)

// The storage this package needs, declared by the consumer rather than taken
// wholesale from the provider - the same split internal/api took, applied to
// the other consumer.
//
// The ratio is less dramatic here: the web interface reaches 62 of
// storage.Operations' 84 methods, so this removes about a quarter of the
// surface rather than two thirds. It was worth doing anyway because the ctx
// migration was already rewriting every one of these call sites, and because
// the grouping is the documentation: it says, in one file, that the web layer
// reads nodelists, pointlists, test results and analytics, and writes nothing.
//
// The 22 it does not use are the API-only reports, the sysop and PSTN write
// paths, and the nodelist import methods cmd/parser drives.

// NodeReader is the nodelist itself: nodes, their history, and the networks they appear in.
type NodeReader interface {
	GetNodeHistory(ctx context.Context, zone, net, node int, domain string) ([]database.Node, error)
	GetNodeDomains(ctx context.Context, zone, net, node int) ([]string, error)
	GetNodeChanges(ctx context.Context, zone, net, node int, domain string) ([]database.NodeChange, error)
	SearchNodesWithLifetime(ctx context.Context, filter database.NodeFilter) ([]storage.NodeSummary, error)
	SearchNodesBySysop(ctx context.Context, sysopName string, limit int, domain string) ([]storage.NodeSummary, error)
	GetBrowseZones(ctx context.Context, date time.Time, domain string) ([]storage.BrowseZone, error)
	GetBrowseRegions(ctx context.Context, date time.Time, zone int, domain string) ([]storage.BrowseRegion, error)
	GetBrowseNets(ctx context.Context, date time.Time, zone, region int, domain string) ([]storage.BrowseNet, error)
	GetBrowseNodes(ctx context.Context, date time.Time, zone, net int, domain string) ([]database.Node, error)
}

// PointReader is the pointlist side of the same addresses.
type PointReader interface {
	GetPointsByBoss(ctx context.Context, domain string, zone, net, node int, asOf *time.Time) ([]database.Point, error)
	GetPointHistory(ctx context.Context, domain string, zone, net, node, point int) ([]database.Point, error)
	GetPointDomains(ctx context.Context, zone, net, node int, point *int) ([]string, error)
	GetPointStats(ctx context.Context, domain string, asOf *time.Time) (*storage.PointStats, error)
	GetPointCountsByNet(ctx context.Context, domain string, zone, net int, asOf *time.Time) (map[int]uint64, error)
	SearchPointsWithLifetime(ctx context.Context, filter database.PointFilter) ([]storage.PointSummary, error)
}

// StatsReader is one nodelist date, and which dates exist.
type StatsReader interface {
	GetStats(ctx context.Context, date time.Time, domain string) (*database.NetworkStats, error)
	GetLatestStatsDate(ctx context.Context, domain string) (time.Time, error)
	GetAvailableDates(ctx context.Context, domain string) ([]time.Time, error)
	GetNearestAvailableDate(ctx context.Context, requestedDate time.Time, domain string) (time.Time, error)
	GetNodeCountHistory(ctx context.Context, domain string) ([]storage.NodeCountByDate, error)
}

// FlagReader is flag and network provenance: when something first showed up and how it spread.
type FlagReader interface {
	GetFlagFirstAppearance(ctx context.Context, flagName string, domain string) (*storage.FlagFirstAppearance, error)
	GetFlagUsageByYear(ctx context.Context, flagName string, domain string) ([]storage.FlagUsageByYear, error)
	GetNetworkHistory(ctx context.Context, zone, net int, domain string) (*storage.NetworkHistory, error)
	GetOnThisDayNodes(ctx context.Context, month, day, limit int, activeOnly bool, domain string) ([]storage.OnThisDayNode, error)
	GetPioneersByRegion(ctx context.Context, zone, region, limit int, domain string) ([]storage.PioneerNode, error)
}

// ReachabilityReader is what the test daemon found: per-node history, per-test detail and the trends behind the charts.
type ReachabilityReader interface {
	GetNodeTestHistory(ctx context.Context, zone, net, node int, days int, domain string) ([]storage.NodeTestResult, error)
	GetDetailedTestResult(ctx context.Context, zone, net, node int, testTime string, domain string) (*storage.NodeTestResult, error)
	GetNodeReachabilityStats(ctx context.Context, zone, net, node int, days int, domain string) (*storage.NodeReachabilityStats, error)
	GetReachabilityTrends(ctx context.Context, days int, domain string) ([]storage.ReachabilityTrend, error)
	GetReachabilityTrendsAllTime(ctx context.Context, domain string) ([]storage.ReachabilityTrend, error)
	SearchNodesByReachability(ctx context.Context, operational bool, limit int, days int, domain string) ([]storage.NodeTestResult, error)
}

// ProtocolReader is the ten config-driven protocol and IPv6 listings. These are bound as method values to protocolNodesFetcher, so they migrate as a set or not at all.
type ProtocolReader interface {
	GetBinkPEnabledNodes(ctx context.Context, limit int, days int, includeZeroNodes bool, domain string) ([]storage.NodeTestResult, error)
	GetIfcicoEnabledNodes(ctx context.Context, limit int, days int, includeZeroNodes bool, domain string) ([]storage.NodeTestResult, error)
	GetTelnetEnabledNodes(ctx context.Context, limit int, days int, includeZeroNodes bool, domain string) ([]storage.NodeTestResult, error)
	GetVModemEnabledNodes(ctx context.Context, limit int, days int, includeZeroNodes bool, domain string) ([]storage.NodeTestResult, error)
	GetVModemUnconfirmedNodes(ctx context.Context, limit int, days int, includeZeroNodes bool, domain string) ([]storage.NodeTestResult, error)
	GetFTPEnabledNodes(ctx context.Context, limit int, days int, includeZeroNodes bool, domain string) ([]storage.NodeTestResult, error)
	GetIPv6EnabledNodes(ctx context.Context, limit int, days int, includeZeroNodes bool, domain string) ([]storage.NodeTestResult, error)
	GetIPv6NonWorkingNodes(ctx context.Context, limit int, days int, includeZeroNodes bool, domain string) ([]storage.NodeTestResult, error)
	GetIPv6AdvertisedIPv4OnlyNodes(ctx context.Context, limit int, days int, includeZeroNodes bool, domain string) ([]storage.NodeTestResult, error)
	GetIPv6OnlyNodes(ctx context.Context, limit int, days int, includeZeroNodes bool, domain string) ([]storage.NodeTestResult, error)
	GetPureIPv6OnlyNodes(ctx context.Context, limit int, days int, includeZeroNodes bool, domain string) ([]storage.NodeTestResult, error)
	GetIPv6NodeList(ctx context.Context, limit int, days int, includeZeroNodes bool, domain string) ([]storage.IPv6NodeListEntry, error)
	GetIPv6WeeklyNews(ctx context.Context, limit int, includeZeroNodes bool, domain string) (*storage.IPv6WeeklyNews, error)
	GetAKAMismatchNodes(ctx context.Context, limit int, days int, includeZeroNodes bool, domain string) ([]storage.NodeTestResult, error)
	GetIPv6IncorrectIPv4CorrectNodes(ctx context.Context, limit int, days int, includeZeroNodes bool, domain string) ([]storage.AKAIPVersionMismatchNode, error)
	GetIPv4IncorrectIPv6CorrectNodes(ctx context.Context, limit int, days int, includeZeroNodes bool, domain string) ([]storage.AKAIPVersionMismatchNode, error)
}

// AnalyticsReader is the remaining reports: geography, other networks, PSTN, modem, file request and email.
type AnalyticsReader interface {
	GetGeoHostingDistribution(ctx context.Context, days int, domain string) (*storage.GeoHostingDistribution, error)
	GetNodesByCountry(ctx context.Context, countryCode string, days int, domain string) ([]storage.NodeTestResult, error)
	GetNodesByProvider(ctx context.Context, provider string, days int, domain string) ([]storage.NodeTestResult, error)
	GetOtherNetworksSummary(ctx context.Context, days int, domain string) ([]storage.OtherNetworkSummary, error)
	GetNodesInNetwork(ctx context.Context, networkName string, limit int, days int, domain string) ([]storage.OtherNetworkNode, error)
	GetPSTNNodes(ctx context.Context, limit int, zone int, domain string) ([]storage.PSTNNode, error)
	GetPSTNDeadNodes(ctx context.Context) ([]storage.PSTNDeadNode, error)
	GetModemAccessibleNodes(ctx context.Context, limit int, days int, includeZeroNodes bool, domain string) ([]storage.ModemAccessibleNode, error)
	GetModemNoAnswerNodes(ctx context.Context, limit int, days int, includeZeroNodes bool, domain string) ([]storage.ModemNoAnswerNode, error)
	GetDetailedModemTestResult(ctx context.Context, zone, net, node int, testTime string) (*storage.ModemTestDetail, error)
	GetFileRequestNodes(ctx context.Context, limit int, domain string) ([]storage.FileRequestNode, error)
	GetEmailCapableNodes(ctx context.Context, limit int, useFieldFallback bool, domain string) ([]storage.EmailCapableNode, error)
	GetEmailFlagTrend(ctx context.Context, domain string) ([]storage.EmailFlagTrendPoint, error)
}

// WhoisReader is registrar and expiry data for the hostnames nodes publish.
type WhoisReader interface {
	GetAllWhoisResults(ctx context.Context, domain string) ([]storage.DomainWhoisResult, error)
	GetNodesByDomain(ctx context.Context, domain string, days int) ([]storage.NodeTestResult, error)
}

// Storage is everything the web interface reads. It reads only:
// nothing in this set writes.
type Storage interface {
	NodeReader
	PointReader
	StatsReader
	FlagReader
	ReachabilityReader
	ProtocolReader
	AnalyticsReader
	WhoisReader
}

// storage.Operations must remain a superset of what this package needs, or
// cmd/server cannot pass its CachedStorage in. Checked at compile time.
var _ Storage = (storage.Operations)(nil)
