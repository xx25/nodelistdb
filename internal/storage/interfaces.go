package storage

import (
	"context"
	"github.com/nodelistdb/internal/pingtrace"
	"time"

	"github.com/nodelistdb/internal/database"
)

// The contracts this package offers its consumers and asks of its parts.

// Operations is what a server-side consumer of storage sees: flat methods
// only.
//
// It used to expose the operation components as well - NodeOps(), StatsOps()
// and five more, each returning a concrete struct with unexported fields. That
// made the interface unimplementable outside this package, which is why both
// the API and the web packages tested copies of their handlers instead of the
// handlers. It also opened a hole around CachedStorage, which overrides the
// flat methods but cannot override an accessor: every call through one reached
// the uncached component.
//
// The components stay available on *Storage itself for cmd/parser, which does
// bulk imports that have no business going through a read cache.
type Operations interface {
	// Node operations
	GetNodes(ctx context.Context, filter database.NodeFilter) ([]database.Node, error)
	GetNodeHistory(ctx context.Context, zone, net, node int, domain string) ([]database.Node, error)
	GetNodeDateRange(ctx context.Context, zone, net, node int, domain string) (firstDate, lastDate time.Time, err error)
	GetNodeDomains(ctx context.Context, zone, net, node int) ([]string, error)
	CountNodes(ctx context.Context, date time.Time, domain string) (int, error)
	InsertNodes(nodes []database.Node) error

	// Point operations
	GetPointsByBoss(ctx context.Context, domain string, zone, net, node int, asOf *time.Time) ([]database.Point, error)
	GetPointHistory(ctx context.Context, domain string, zone, net, node, point int) ([]database.Point, error)
	SearchPoints(ctx context.Context, filter database.PointFilter) ([]database.Point, error)
	SearchPointsWithLifetime(ctx context.Context, filter database.PointFilter) ([]PointSummary, error)
	GetPointStats(ctx context.Context, domain string, asOf *time.Time) (*PointStats, error)
	GetPointCountsByNet(ctx context.Context, domain string, zone, net int, asOf *time.Time) (map[int]uint64, error)
	GetPointDomains(ctx context.Context, zone, net, node int, point *int) ([]string, error)
	GetPointlistDates(ctx context.Context, domain, listSource string) ([]database.PointlistFile, error)
	GetPointlistSources(ctx context.Context, domain string) ([]PointlistSourceInfo, error)
	LatestPointlistDate(ctx context.Context, domain string) (time.Time, bool, error)

	// Search operations
	SearchNodesBySysop(ctx context.Context, sysopName string, limit int, domain string) ([]NodeSummary, error)
	GetNodeChanges(ctx context.Context, zone, net, node int, domain string) ([]database.NodeChange, error)
	GetUniqueSysops(ctx context.Context, nameFilter string, limit, offset int) ([]SysopInfo, error)
	GetNodesBySysop(ctx context.Context, sysopName string, limit int) ([]database.Node, error)
	SearchNodesWithLifetime(ctx context.Context, filter database.NodeFilter) ([]NodeSummary, error)

	// Analytics operations
	GetFlagFirstAppearance(ctx context.Context, flagName string, domain string) (*FlagFirstAppearance, error)
	GetFlagUsageByYear(ctx context.Context, flagName string, domain string) ([]FlagUsageByYear, error)
	GetNetworkHistory(ctx context.Context, zone, net int, domain string) (*NetworkHistory, error)

	// Statistics operations
	GetStats(ctx context.Context, date time.Time, domain string) (*database.NetworkStats, error)
	GetLatestStatsDate(ctx context.Context, domain string) (time.Time, error)
	GetAvailableDates(ctx context.Context, domain string) ([]time.Time, error)
	GetNearestAvailableDate(ctx context.Context, requestedDate time.Time, domain string) (time.Time, error)
	GetNodeCountHistory(ctx context.Context, domain string) ([]NodeCountByDate, error)

	// Hierarchy browser operations
	GetBrowseZones(ctx context.Context, date time.Time, domain string) ([]BrowseZone, error)
	GetBrowseRegions(ctx context.Context, date time.Time, zone int, domain string) ([]BrowseRegion, error)
	GetBrowseNets(ctx context.Context, date time.Time, zone, region int, domain string) ([]BrowseNet, error)
	GetBrowseNodes(ctx context.Context, date time.Time, zone, net int, domain string) ([]database.Node, error)

	// Test operations
	GetNodeTestHistory(ctx context.Context, zone, net, node int, days int, domain string) ([]NodeTestResult, error)
	GetDetailedTestResult(ctx context.Context, zone, net, node int, testTime string, domain string) (*NodeTestResult, error)
	GetNodeReachabilityStats(ctx context.Context, zone, net, node int, days int, domain string) (*NodeReachabilityStats, error)
	GetReachabilityTrends(ctx context.Context, days int, domain string) ([]ReachabilityTrend, error)
	GetReachabilityTrendsAllTime(ctx context.Context, domain string) ([]ReachabilityTrend, error)
	SearchNodesByReachability(ctx context.Context, operational bool, limit int, days int, domain string) ([]NodeTestResult, error)
	GetIPv6EnabledNodes(ctx context.Context, limit int, days int, includeZeroNodes bool, domain string) ([]NodeTestResult, error)
	GetIPv6NonWorkingNodes(ctx context.Context, limit int, days int, includeZeroNodes bool, domain string) ([]NodeTestResult, error)
	GetIPv6AdvertisedIPv4OnlyNodes(ctx context.Context, limit int, days int, includeZeroNodes bool, domain string) ([]NodeTestResult, error)
	GetIPv6OnlyNodes(ctx context.Context, limit int, days int, includeZeroNodes bool, domain string) ([]NodeTestResult, error)
	GetPureIPv6OnlyNodes(ctx context.Context, limit int, days int, includeZeroNodes bool, domain string) ([]NodeTestResult, error)
	GetIPv6WeeklyNews(ctx context.Context, limit int, includeZeroNodes bool, domain string) (*IPv6WeeklyNews, error)
	GetBinkPEnabledNodes(ctx context.Context, limit int, days int, includeZeroNodes bool, domain string) ([]NodeTestResult, error)
	GetIfcicoEnabledNodes(ctx context.Context, limit int, days int, includeZeroNodes bool, domain string) ([]NodeTestResult, error)
	GetTelnetEnabledNodes(ctx context.Context, limit int, days int, includeZeroNodes bool, domain string) ([]NodeTestResult, error)
	GetVModemEnabledNodes(ctx context.Context, limit int, days int, includeZeroNodes bool, domain string) ([]NodeTestResult, error)
	GetVModemUnconfirmedNodes(ctx context.Context, limit int, days int, includeZeroNodes bool, domain string) ([]NodeTestResult, error)
	GetFTPEnabledNodes(ctx context.Context, limit int, days int, includeZeroNodes bool, domain string) ([]NodeTestResult, error)
	GetAKAMismatchNodes(ctx context.Context, limit int, days int, includeZeroNodes bool, domain string) ([]NodeTestResult, error)
	GetIPv6IncorrectIPv4CorrectNodes(ctx context.Context, limit int, days int, includeZeroNodes bool, domain string) ([]AKAIPVersionMismatchNode, error)
	GetIPv4IncorrectIPv6CorrectNodes(ctx context.Context, limit int, days int, includeZeroNodes bool, domain string) ([]AKAIPVersionMismatchNode, error)
	GetOtherNetworksSummary(ctx context.Context, days int, domain string) ([]OtherNetworkSummary, error)
	GetNodesInNetwork(ctx context.Context, networkName string, limit int, days int, domain string) ([]OtherNetworkNode, error)
	GetBinkPSoftwareDistribution(ctx context.Context, days int, domain string) (*SoftwareDistribution, error)
	GetIFCICOSoftwareDistribution(ctx context.Context, days int, domain string) (*SoftwareDistribution, error)
	GetBinkdDetailedStats(ctx context.Context, days int, domain string) (*SoftwareDistribution, error)
	GetGeoHostingDistribution(ctx context.Context, days int, domain string) (*GeoHostingDistribution, error)
	GetNodesByCountry(ctx context.Context, countryCode string, days int, domain string) ([]NodeTestResult, error)
	GetNodesByProvider(ctx context.Context, provider string, days int, domain string) ([]NodeTestResult, error)
	GetOnThisDayNodes(ctx context.Context, month, day, limit int, activeOnly bool, domain string) ([]OnThisDayNode, error)
	GetPioneersByRegion(ctx context.Context, zone, region, limit int, domain string) ([]PioneerNode, error)
	GetPSTNCMNodes(ctx context.Context, limit int) ([]PSTNNode, error)
	GetPSTNNodes(ctx context.Context, limit int, zone int, domain string) ([]PSTNNode, error)
	MarkPSTNDead(ctx context.Context, zone, net, node int, reason, markedBy string) error
	UnmarkPSTNDead(ctx context.Context, zone, net, node int, markedBy string) error
	GetPSTNDeadNodes(ctx context.Context) ([]PSTNDeadNode, error)
	GetFileRequestNodes(ctx context.Context, limit int, domain string) ([]FileRequestNode, error)
	GetEmailCapableNodes(ctx context.Context, limit int, useFieldFallback bool, domain string) ([]EmailCapableNode, error)
	GetEmailFlagTrend(ctx context.Context, domain string) ([]EmailFlagTrendPoint, error)
	GetModemAccessibleNodes(ctx context.Context, limit int, days int, includeZeroNodes bool, domain string) ([]ModemAccessibleNode, error)
	GetModemNoAnswerNodes(ctx context.Context, limit int, days int, includeZeroNodes bool, domain string) ([]ModemNoAnswerNode, error)
	GetRecentModemSuccessPhones(ctx context.Context, days int) ([]string, error)
	GetDetailedModemTestResult(ctx context.Context, zone, net, node int, testTime string) (*ModemTestDetail, error)
	GetIPv6NodeList(ctx context.Context, limit int, days int, includeZeroNodes bool, domain string) ([]IPv6NodeListEntry, error)

	// WHOIS operations
	GetAllWhoisResults(ctx context.Context, domain string) ([]DomainWhoisResult, error)
	GetNodesByDomain(ctx context.Context, domain string, days int) ([]NodeTestResult, error)

	// Utility operations
	IsNodelistProcessed(nodelistDate time.Time, domain string) (bool, error)
	FindConflictingNode(zone, net, node int, date time.Time, domain string) (bool, error)
	GetMaxNodelistDate(ctx context.Context, domain string) (time.Time, error)
	GetDomains(ctx context.Context) ([]DomainInfo, error)

	// Lifecycle
	Close() error

	// FTS-4010 netmail PING/TRACE (ping_tests / ping_replies)
	GetPingTraceSummary(ctx context.Context, domain string, days int) (*PingTraceSummary, error)
	GetNodePings(ctx context.Context, domain string, zone, net, node int, limit int) ([]pingtrace.Ping, error)
	GetNodePingReplies(ctx context.Context, domain string, zone, net, node int, limit int) ([]PingReplyRow, error)
}

// QueryBuilderInterface defines the contract for query building
type QueryBuilderInterface interface {
	// Basic queries
	InsertNodeSQL() string
	NodeSelectSQL() string
	BuildBatchInsertSQL(batchSize int) string
	BuildDirectBatchInsertSQL(nodes []database.Node, rp *ResultParser) string
	InsertNodesInChunks(db database.DatabaseInterface, nodes []database.Node) error
	BuildNodesQuery(filter database.NodeFilter) (string, []interface{})
	BuildFTSQuery(filter database.NodeFilter) (string, []interface{}, bool)

	// Statistics queries
	StatsSQL() string
	ZoneDistributionSQL() string
	LargestRegionsSQL() string
	LargestNetsSQL() string
	// Optimized statistics queries for better performance
	OptimizedLargestRegionsSQL() string
	OptimizedLargestNetsSQL() string
	// Hierarchy browser queries
	BrowseZonesSQL() string
	BrowseRegionsSQL() string
	BrowseNetsSQL() string
	BrowseNodesSQL() string

	// Node-specific queries
	NodeHistorySQL() string
	NodeDateRangeSQL() string
	SysopSearchSQL() string
	NodeSummarySearchSQL(activeOnly bool) string

	// Utility queries
	ConflictCheckSQL() string
	MarkConflictSQL() string
	IsProcessedSQL() string
	LatestDateSQL() string
	AvailableDatesSQL() string
	ExactDateExistsSQL() string
	NearestDateBeforeSQL() string
	NearestDateAfterSQL() string
	ConsecutiveNodelistCheckSQL() string
	NextNodelistDateSQL() string

	// Sysop queries
	UniqueSysopsWithFilterSQL() string
	UniqueSysopsSQL() string

	// Analytics queries
	FlagFirstAppearanceSQL() string
	FlagUsageByYearSQL() string
	NetworkNameSQL() string
	NetworkHistorySQL() string
}

// ResultParserInterface defines the contract for parsing database results
type ResultParserInterface interface {
	ParseNodeRow(scanner RowScanner) (database.Node, error)
	ParsePointRow(scanner RowScanner) (database.Point, error)
	ParseNodeSummaryRow(scanner RowScanner) (NodeSummary, error)
	ParseNetworkStatsRow(scanner RowScanner) (*database.NetworkStats, error)
	ParseRegionInfoRow(scanner RowScanner) (database.RegionInfo, error)
	ParseNetInfoRow(scanner RowScanner) (database.NetInfo, error)
	ParseTestResultRow(scanner RowScanner, result *NodeTestResult) error
	ValidateNodeFilter(filter database.NodeFilter) error
	SanitizeStringInput(input string) string
}

// RowScanner interface abstracts sql.Rows and sql.Row for easier testing
type RowScanner interface {
	Scan(dest ...interface{}) error
}

// singleRowScanner wraps sql.Row to implement RowScanner interface
type singleRowScanner struct {
	Row interface {
		Scan(dest ...interface{}) error
	}
}

func (s *singleRowScanner) Scan(dest ...interface{}) error {
	return s.Row.Scan(dest...)
}
