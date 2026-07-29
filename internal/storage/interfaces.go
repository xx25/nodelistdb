package storage

import (
	"context"
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
	GetNodes(filter database.NodeFilter) ([]database.Node, error)
	GetNodeHistory(zone, net, node int, domain string) ([]database.Node, error)
	GetNodeDateRange(zone, net, node int, domain string) (firstDate, lastDate time.Time, err error)
	GetNodeDomains(zone, net, node int) ([]string, error)
	CountNodes(date time.Time, domain string) (int, error)
	InsertNodes(nodes []database.Node) error

	// Point operations
	GetPointsByBoss(domain string, zone, net, node int, asOf *time.Time) ([]database.Point, error)
	GetPointHistory(domain string, zone, net, node, point int) ([]database.Point, error)
	SearchPoints(filter database.PointFilter) ([]database.Point, error)
	SearchPointsWithLifetime(ctx context.Context, filter database.PointFilter) ([]PointSummary, error)
	GetPointStats(domain string, asOf *time.Time) (*PointStats, error)
	GetPointCountsByNet(domain string, zone, net int, asOf *time.Time) (map[int]uint64, error)
	GetPointDomains(zone, net, node int, point *int) ([]string, error)
	GetPointlistDates(domain, listSource string) ([]database.PointlistFile, error)
	GetPointlistSources(domain string) ([]PointlistSourceInfo, error)
	LatestPointlistDate(domain string) (time.Time, bool, error)

	// Search operations
	SearchNodesBySysop(sysopName string, limit int, domain string) ([]NodeSummary, error)
	GetNodeChanges(zone, net, node int, domain string) ([]database.NodeChange, error)
	GetUniqueSysops(nameFilter string, limit, offset int) ([]SysopInfo, error)
	GetNodesBySysop(sysopName string, limit int) ([]database.Node, error)
	SearchNodesWithLifetime(filter database.NodeFilter) ([]NodeSummary, error)

	// Analytics operations
	GetFlagFirstAppearance(flagName string, domain string) (*FlagFirstAppearance, error)
	GetFlagUsageByYear(flagName string, domain string) ([]FlagUsageByYear, error)
	GetNetworkHistory(zone, net int, domain string) (*NetworkHistory, error)

	// Statistics operations
	GetStats(date time.Time, domain string) (*database.NetworkStats, error)
	GetLatestStatsDate(domain string) (time.Time, error)
	GetAvailableDates(domain string) ([]time.Time, error)
	GetNearestAvailableDate(requestedDate time.Time, domain string) (time.Time, error)
	GetNodeCountHistory(domain string) ([]NodeCountByDate, error)

	// Hierarchy browser operations
	GetBrowseZones(date time.Time, domain string) ([]BrowseZone, error)
	GetBrowseRegions(date time.Time, zone int, domain string) ([]BrowseRegion, error)
	GetBrowseNets(date time.Time, zone, region int, domain string) ([]BrowseNet, error)
	GetBrowseNodes(date time.Time, zone, net int, domain string) ([]database.Node, error)

	// Test operations
	GetNodeTestHistory(zone, net, node int, days int, domain string) ([]NodeTestResult, error)
	GetDetailedTestResult(zone, net, node int, testTime string, domain string) (*NodeTestResult, error)
	GetNodeReachabilityStats(zone, net, node int, days int, domain string) (*NodeReachabilityStats, error)
	GetReachabilityTrends(days int, domain string) ([]ReachabilityTrend, error)
	GetReachabilityTrendsAllTime(domain string) ([]ReachabilityTrend, error)
	SearchNodesByReachability(operational bool, limit int, days int, domain string) ([]NodeTestResult, error)
	GetIPv6EnabledNodes(limit int, days int, includeZeroNodes bool, domain string) ([]NodeTestResult, error)
	GetIPv6NonWorkingNodes(limit int, days int, includeZeroNodes bool, domain string) ([]NodeTestResult, error)
	GetIPv6AdvertisedIPv4OnlyNodes(limit int, days int, includeZeroNodes bool, domain string) ([]NodeTestResult, error)
	GetIPv6OnlyNodes(limit int, days int, includeZeroNodes bool, domain string) ([]NodeTestResult, error)
	GetPureIPv6OnlyNodes(limit int, days int, includeZeroNodes bool, domain string) ([]NodeTestResult, error)
	GetIPv6WeeklyNews(limit int, includeZeroNodes bool, domain string) (*IPv6WeeklyNews, error)
	GetBinkPEnabledNodes(limit int, days int, includeZeroNodes bool, domain string) ([]NodeTestResult, error)
	GetIfcicoEnabledNodes(limit int, days int, includeZeroNodes bool, domain string) ([]NodeTestResult, error)
	GetTelnetEnabledNodes(limit int, days int, includeZeroNodes bool, domain string) ([]NodeTestResult, error)
	GetVModemEnabledNodes(limit int, days int, includeZeroNodes bool, domain string) ([]NodeTestResult, error)
	GetVModemUnconfirmedNodes(limit int, days int, includeZeroNodes bool, domain string) ([]NodeTestResult, error)
	GetFTPEnabledNodes(limit int, days int, includeZeroNodes bool, domain string) ([]NodeTestResult, error)
	GetAKAMismatchNodes(limit int, days int, includeZeroNodes bool, domain string) ([]NodeTestResult, error)
	GetIPv6IncorrectIPv4CorrectNodes(limit int, days int, includeZeroNodes bool, domain string) ([]AKAIPVersionMismatchNode, error)
	GetIPv4IncorrectIPv6CorrectNodes(limit int, days int, includeZeroNodes bool, domain string) ([]AKAIPVersionMismatchNode, error)
	GetOtherNetworksSummary(days int, domain string) ([]OtherNetworkSummary, error)
	GetNodesInNetwork(networkName string, limit int, days int, domain string) ([]OtherNetworkNode, error)
	GetBinkPSoftwareDistribution(days int, domain string) (*SoftwareDistribution, error)
	GetIFCICOSoftwareDistribution(days int, domain string) (*SoftwareDistribution, error)
	GetBinkdDetailedStats(days int, domain string) (*SoftwareDistribution, error)
	GetGeoHostingDistribution(days int, domain string) (*GeoHostingDistribution, error)
	GetNodesByCountry(countryCode string, days int, domain string) ([]NodeTestResult, error)
	GetNodesByProvider(provider string, days int, domain string) ([]NodeTestResult, error)
	GetOnThisDayNodes(month, day, limit int, activeOnly bool, domain string) ([]OnThisDayNode, error)
	GetPioneersByRegion(zone, region, limit int, domain string) ([]PioneerNode, error)
	GetPSTNCMNodes(limit int) ([]PSTNNode, error)
	GetPSTNNodes(limit int, zone int, domain string) ([]PSTNNode, error)
	MarkPSTNDead(zone, net, node int, reason, markedBy string) error
	UnmarkPSTNDead(zone, net, node int, markedBy string) error
	GetPSTNDeadNodes() ([]PSTNDeadNode, error)
	GetFileRequestNodes(limit int, domain string) ([]FileRequestNode, error)
	GetEmailCapableNodes(limit int, useFieldFallback bool, domain string) ([]EmailCapableNode, error)
	GetEmailFlagTrend(domain string) ([]EmailFlagTrendPoint, error)
	GetModemAccessibleNodes(limit int, days int, includeZeroNodes bool, domain string) ([]ModemAccessibleNode, error)
	GetModemNoAnswerNodes(limit int, days int, includeZeroNodes bool, domain string) ([]ModemNoAnswerNode, error)
	GetRecentModemSuccessPhones(days int) ([]string, error)
	GetDetailedModemTestResult(zone, net, node int, testTime string) (*ModemTestDetail, error)
	GetIPv6NodeList(limit int, days int, includeZeroNodes bool, domain string) ([]IPv6NodeListEntry, error)

	// WHOIS operations
	GetAllWhoisResults(domain string) ([]DomainWhoisResult, error)
	GetNodesByDomain(domain string, days int) ([]NodeTestResult, error)

	// Utility operations
	IsNodelistProcessed(nodelistDate time.Time, domain string) (bool, error)
	FindConflictingNode(zone, net, node int, date time.Time, domain string) (bool, error)
	GetMaxNodelistDate(domain string) (time.Time, error)
	GetDomains() ([]DomainInfo, error)

	// Lifecycle
	Close() error
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
