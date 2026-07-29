package storage

import (
	"context"
	"sync"
	"time"

	"github.com/nodelistdb/internal/database"
)

// Storage is the flat, thread-safe surface over the specialized operation
// components. It delegates every method to whichever component owns it.
//
// The test-derived components used to sit behind one more facade,
// TestOperationsRefactored, which added 231 lines of single-line delegation
// and nothing else - and which the geo, software, modem and other-networks
// components were routed through despite none of them being about test
// history. They are named here for what they are.
type Storage struct {
	db           database.DatabaseInterface
	queryBuilder QueryBuilderInterface
	resultParser ResultParserInterface

	nodeOperations      *NodeOperations
	pointOperations     *PointOperations
	searchOperations    *SearchOperations
	statsOperations     *StatisticsOperations
	analyticsOperations *AnalyticsOperations
	whoisOperations     *WhoisOperations
	pstnDeadOperations  *PSTNDeadOperations

	// Components over node_test_results, the daemon's log of what it probed.
	testHistoryOperations   *TestHistoryOperations
	reachabilityOperations  *ReachabilityOperations
	protocolOperations      *ProtocolQueryOperations
	ipv6Operations          *IPv6QueryOperations
	akaMismatchOperations   *AKAMismatchOperations
	modemOperations         *ModemQueryOperations
	softwareOperations      *SoftwareAnalyticsOperations
	geoOperations           *GeoAnalyticsOperations
	otherNetworksOperations *OtherNetworksOperations

	mu sync.RWMutex
}

// NodeOps returns the node operations component for CRUD operations on nodes
func (s *Storage) NodeOps() *NodeOperations {
	return s.nodeOperations
}

// PointOps returns the point operations component for pointlist data
func (s *Storage) PointOps() *PointOperations {
	return s.pointOperations
}

// SearchOps returns the search operations component for advanced search queries
func (s *Storage) SearchOps() *SearchOperations {
	return s.searchOperations
}

// StatsOps returns the statistics operations component for network statistics
func (s *Storage) StatsOps() *StatisticsOperations {
	return s.statsOperations
}

// AnalyticsOps returns the analytics operations component for historical analytics
func (s *Storage) AnalyticsOps() *AnalyticsOperations {
	return s.analyticsOperations
}

// WhoisOps returns the WHOIS operations component for domain expiration data
func (s *Storage) WhoisOps() *WhoisOperations {
	return s.whoisOperations
}

// PSTNDeadOps returns the PSTN dead node operations component
func (s *Storage) PSTNDeadOps() *PSTNDeadOperations {
	return s.pstnDeadOperations
}

// New creates a new Storage instance with ClickHouse-specific components
func New(db database.DatabaseInterface) (*Storage, error) {
	// Always use ClickHouse components (only supported database type)
	queryBuilder := NewQueryBuilder()
	resultParser := NewClickHouseResultParser()

	// Create the storage instance with ClickHouse components
	storage := &Storage{
		db:           db,
		queryBuilder: queryBuilder,
		resultParser: resultParser,
	}

	// Create specialized operation components
	storage.nodeOperations = NewNodeOperations(db, queryBuilder, resultParser)
	storage.pointOperations = NewPointOperations(db, queryBuilder, resultParser)
	storage.searchOperations = NewSearchOperations(db, queryBuilder, resultParser, storage.nodeOperations)
	storage.statsOperations = NewStatisticsOperations(db, queryBuilder, resultParser)
	storage.pstnDeadOperations = NewPSTNDeadOperations(db)
	storage.analyticsOperations = NewAnalyticsOperations(db, queryBuilder, resultParser, storage.pstnDeadOperations)
	storage.whoisOperations = NewWhoisOperations(db)

	testQueryBuilder := NewTestQueryBuilder()
	storage.testHistoryOperations = NewTestHistoryOperations(db, testQueryBuilder, resultParser)
	storage.reachabilityOperations = NewReachabilityOperations(db, testQueryBuilder, resultParser)
	storage.protocolOperations = NewProtocolQueryOperations(db, testQueryBuilder, resultParser)
	storage.ipv6Operations = NewIPv6QueryOperations(db, testQueryBuilder, resultParser)
	storage.akaMismatchOperations = NewAKAMismatchOperations(db, testQueryBuilder, resultParser)
	storage.modemOperations = NewModemQueryOperations(db)
	storage.softwareOperations = NewSoftwareAnalyticsOperations(db)
	storage.geoOperations = NewGeoAnalyticsOperations(db)
	storage.otherNetworksOperations = NewOtherNetworksOperations(db)

	return storage, nil
}

// Close closes all database connections and prepared statements
func (s *Storage) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// The individual components don't have close methods currently,
	// but we maintain this for backward compatibility
	return nil
}

// --- Legacy Delegation Methods for Operations Interface Compatibility ---
// These methods provide backward compatibility for code using the Operations interface.
// New code should use the component accessors directly (e.g., storage.NodeOps().GetNodes(ctx)).

// Node Operations delegated methods
func (s *Storage) GetNodes(ctx context.Context, filter database.NodeFilter) ([]database.Node, error) {
	return s.nodeOperations.GetNodes(ctx, filter)
}

func (s *Storage) GetNodeHistory(ctx context.Context, zone, net, node int, domain string) ([]database.Node, error) {
	return s.nodeOperations.GetNodeHistory(ctx, zone, net, node, domain)
}

func (s *Storage) GetNodeDateRange(ctx context.Context, zone, net, node int, domain string) (firstDate, lastDate time.Time, err error) {
	return s.nodeOperations.GetNodeDateRange(ctx, zone, net, node, domain)
}

func (s *Storage) InsertNodes(nodes []database.Node) error {
	return s.nodeOperations.InsertNodes(nodes)
}

func (s *Storage) IsNodelistProcessed(nodelistDate time.Time, domain string) (bool, error) {
	return s.nodeOperations.IsNodelistProcessed(nodelistDate, domain)
}

func (s *Storage) FindConflictingNode(zone, net, node int, date time.Time, domain string) (bool, error) {
	return s.nodeOperations.FindConflictingNode(zone, net, node, date, domain)
}

func (s *Storage) GetMaxNodelistDate(ctx context.Context, domain string) (time.Time, error) {
	return s.nodeOperations.GetMaxNodelistDate(ctx, domain)
}

func (s *Storage) GetDomains(ctx context.Context) ([]DomainInfo, error) {
	return s.nodeOperations.GetDomains(ctx)
}

// Point Operations delegated methods
func (s *Storage) GetPointsByBoss(ctx context.Context, domain string, zone, net, node int, asOf *time.Time) ([]database.Point, error) {
	return s.pointOperations.GetPointsByBoss(ctx, domain, zone, net, node, asOf)
}

func (s *Storage) GetPointHistory(ctx context.Context, domain string, zone, net, node, point int) ([]database.Point, error) {
	return s.pointOperations.GetPointHistory(ctx, domain, zone, net, node, point)
}

func (s *Storage) SearchPoints(ctx context.Context, filter database.PointFilter) ([]database.Point, error) {
	return s.pointOperations.SearchPoints(ctx, filter)
}

func (s *Storage) SearchPointsWithLifetime(ctx context.Context, filter database.PointFilter) ([]PointSummary, error) {
	return s.pointOperations.SearchPointsWithLifetime(ctx, filter)
}

func (s *Storage) GetPointStats(ctx context.Context, domain string, asOf *time.Time) (*PointStats, error) {
	return s.pointOperations.GetPointStats(ctx, domain, asOf)
}

func (s *Storage) GetPointCountsByNet(ctx context.Context, domain string, zone, net int, asOf *time.Time) (map[int]uint64, error) {
	return s.pointOperations.GetPointCountsByNet(ctx, domain, zone, net, asOf)
}

// CountNodes returns how many nodes one nodelist holds in one network.
func (s *Storage) CountNodes(ctx context.Context, date time.Time, domain string) (int, error) {
	return s.nodeOperations.CountNodes(ctx, date, domain)
}

// GetNodeDomains lists the FTN networks a 3D address exists in.
func (s *Storage) GetNodeDomains(ctx context.Context, zone, net, node int) ([]string, error) {
	return s.nodeOperations.GetNodeDomains(ctx, zone, net, node)
}

func (s *Storage) GetPointDomains(ctx context.Context, zone, net, node int, point *int) ([]string, error) {
	return s.pointOperations.GetPointDomains(ctx, zone, net, node, point)
}

func (s *Storage) GetPointlistDates(ctx context.Context, domain, listSource string) ([]database.PointlistFile, error) {
	return s.pointOperations.GetPointlistDates(ctx, domain, listSource)
}

func (s *Storage) GetPointlistSources(ctx context.Context, domain string) ([]PointlistSourceInfo, error) {
	return s.pointOperations.GetPointlistSources(ctx, domain)
}

func (s *Storage) LatestPointlistDate(ctx context.Context, domain string) (time.Time, bool, error) {
	return s.pointOperations.LatestPointlistDate(ctx, domain)
}

// Search Operations delegated methods
func (s *Storage) SearchNodesBySysop(ctx context.Context, sysopName string, limit int, domain string) ([]NodeSummary, error) {
	return s.searchOperations.SearchNodesBySysop(ctx, sysopName, limit, domain)
}

func (s *Storage) GetNodeChanges(ctx context.Context, zone, net, node int, domain string) ([]database.NodeChange, error) {
	return s.searchOperations.GetNodeChanges(ctx, zone, net, node, domain)
}

func (s *Storage) GetUniqueSysops(ctx context.Context, nameFilter string, limit, offset int) ([]SysopInfo, error) {
	return s.searchOperations.GetUniqueSysops(ctx, nameFilter, limit, offset)
}

func (s *Storage) GetNodesBySysop(ctx context.Context, sysopName string, limit int) ([]database.Node, error) {
	return s.searchOperations.GetNodesBySysop(ctx, sysopName, limit)
}

func (s *Storage) SearchNodesWithLifetime(ctx context.Context, filter database.NodeFilter) ([]NodeSummary, error) {
	return s.searchOperations.SearchNodesWithLifetime(ctx, filter)
}

// Analytics Operations delegated methods
func (s *Storage) GetFlagFirstAppearance(ctx context.Context, flagName string, domain string) (*FlagFirstAppearance, error) {
	return s.analyticsOperations.GetFlagFirstAppearance(ctx, flagName, domain)
}

func (s *Storage) GetFlagUsageByYear(ctx context.Context, flagName string, domain string) ([]FlagUsageByYear, error) {
	return s.analyticsOperations.GetFlagUsageByYear(ctx, flagName, domain)
}

func (s *Storage) GetNetworkHistory(ctx context.Context, zone, net int, domain string) (*NetworkHistory, error) {
	return s.analyticsOperations.GetNetworkHistory(ctx, zone, net, domain)
}

func (s *Storage) UpdateFlagStatistics(nodelistDate time.Time, domain string) error {
	return s.analyticsOperations.UpdateFlagStatistics(nodelistDate, domain)
}

// Statistics Operations delegated methods
func (s *Storage) GetStats(ctx context.Context, date time.Time, domain string) (*database.NetworkStats, error) {
	return s.statsOperations.GetStats(ctx, date, domain)
}

func (s *Storage) GetLatestStatsDate(ctx context.Context, domain string) (time.Time, error) {
	return s.statsOperations.GetLatestStatsDate(ctx, domain)
}

func (s *Storage) GetAvailableDates(ctx context.Context, domain string) ([]time.Time, error) {
	return s.statsOperations.GetAvailableDates(ctx, domain)
}

func (s *Storage) GetNearestAvailableDate(ctx context.Context, requestedDate time.Time, domain string) (time.Time, error) {
	return s.statsOperations.GetNearestAvailableDate(ctx, requestedDate, domain)
}

func (s *Storage) GetNodeCountHistory(ctx context.Context, domain string) ([]NodeCountByDate, error) {
	return s.statsOperations.GetNodeCountHistory(ctx, domain)
}

func (s *Storage) GetBrowseZones(ctx context.Context, date time.Time, domain string) ([]BrowseZone, error) {
	return s.statsOperations.GetBrowseZones(ctx, date, domain)
}

func (s *Storage) GetBrowseRegions(ctx context.Context, date time.Time, zone int, domain string) ([]BrowseRegion, error) {
	return s.statsOperations.GetBrowseRegions(ctx, date, zone, domain)
}

func (s *Storage) GetBrowseNets(ctx context.Context, date time.Time, zone, region int, domain string) ([]BrowseNet, error) {
	return s.statsOperations.GetBrowseNets(ctx, date, zone, region, domain)
}

func (s *Storage) GetBrowseNodes(ctx context.Context, date time.Time, zone, net int, domain string) ([]database.Node, error) {
	return s.statsOperations.GetBrowseNodes(ctx, date, zone, net, domain)
}

// Test Operations delegated methods
func (s *Storage) GetNodeTestHistory(ctx context.Context, zone, net, node int, days int, domain string) ([]NodeTestResult, error) {
	return s.testHistoryOperations.GetNodeTestHistory(ctx, zone, net, node, days, domain)
}

func (s *Storage) GetDetailedTestResult(ctx context.Context, zone, net, node int, testTime string, domain string) (*NodeTestResult, error) {
	return s.testHistoryOperations.GetDetailedTestResult(ctx, zone, net, node, testTime, domain)
}

func (s *Storage) GetNodeReachabilityStats(ctx context.Context, zone, net, node int, days int, domain string) (*NodeReachabilityStats, error) {
	return s.reachabilityOperations.GetNodeReachabilityStats(ctx, zone, net, node, days, domain)
}

func (s *Storage) GetReachabilityTrendsAllTime(ctx context.Context, domain string) ([]ReachabilityTrend, error) {
	return s.reachabilityOperations.GetReachabilityTrendsAllTime(ctx, domain)
}

func (s *Storage) GetReachabilityTrends(ctx context.Context, days int, domain string) ([]ReachabilityTrend, error) {
	return s.reachabilityOperations.GetReachabilityTrends(ctx, days, domain)
}

func (s *Storage) SearchNodesByReachability(ctx context.Context, operational bool, limit int, days int, domain string) ([]NodeTestResult, error) {
	return s.reachabilityOperations.SearchNodesByReachability(ctx, operational, limit, days, domain)
}

func (s *Storage) GetIPv6EnabledNodes(ctx context.Context, limit int, days int, includeZeroNodes bool, domain string) ([]NodeTestResult, error) {
	return s.ipv6Operations.GetIPv6EnabledNodes(ctx, limit, days, includeZeroNodes, domain)
}

func (s *Storage) GetIPv6NonWorkingNodes(ctx context.Context, limit int, days int, includeZeroNodes bool, domain string) ([]NodeTestResult, error) {
	return s.ipv6Operations.GetIPv6NonWorkingNodes(ctx, limit, days, includeZeroNodes, domain)
}

func (s *Storage) GetIPv6AdvertisedIPv4OnlyNodes(ctx context.Context, limit int, days int, includeZeroNodes bool, domain string) ([]NodeTestResult, error) {
	return s.ipv6Operations.GetIPv6AdvertisedIPv4OnlyNodes(ctx, limit, days, includeZeroNodes, domain)
}

func (s *Storage) GetIPv6OnlyNodes(ctx context.Context, limit int, days int, includeZeroNodes bool, domain string) ([]NodeTestResult, error) {
	return s.ipv6Operations.GetIPv6OnlyNodes(ctx, limit, days, includeZeroNodes, domain)
}

func (s *Storage) GetPureIPv6OnlyNodes(ctx context.Context, limit int, days int, includeZeroNodes bool, domain string) ([]NodeTestResult, error) {
	return s.ipv6Operations.GetPureIPv6OnlyNodes(ctx, limit, days, includeZeroNodes, domain)
}

func (s *Storage) GetIPv6NodeList(ctx context.Context, limit int, days int, includeZeroNodes bool, domain string) ([]IPv6NodeListEntry, error) {
	return s.ipv6Operations.GetIPv6NodeList(ctx, limit, days, includeZeroNodes, domain)
}

func (s *Storage) GetIPv6WeeklyNews(ctx context.Context, limit int, includeZeroNodes bool, domain string) (*IPv6WeeklyNews, error) {
	return s.ipv6Operations.GetIPv6WeeklyNews(ctx, limit, includeZeroNodes, domain)
}

func (s *Storage) GetBinkPEnabledNodes(ctx context.Context, limit int, days int, includeZeroNodes bool, domain string) ([]NodeTestResult, error) {
	return s.protocolOperations.GetBinkPEnabledNodes(ctx, limit, days, includeZeroNodes, domain)
}

func (s *Storage) GetIfcicoEnabledNodes(ctx context.Context, limit int, days int, includeZeroNodes bool, domain string) ([]NodeTestResult, error) {
	return s.protocolOperations.GetIfcicoEnabledNodes(ctx, limit, days, includeZeroNodes, domain)
}

func (s *Storage) GetTelnetEnabledNodes(ctx context.Context, limit int, days int, includeZeroNodes bool, domain string) ([]NodeTestResult, error) {
	return s.protocolOperations.GetTelnetEnabledNodes(ctx, limit, days, includeZeroNodes, domain)
}

func (s *Storage) GetVModemEnabledNodes(ctx context.Context, limit int, days int, includeZeroNodes bool, domain string) ([]NodeTestResult, error) {
	return s.protocolOperations.GetVModemEnabledNodes(ctx, limit, days, includeZeroNodes, domain)
}

func (s *Storage) GetVModemUnconfirmedNodes(ctx context.Context, limit int, days int, includeZeroNodes bool, domain string) ([]NodeTestResult, error) {
	return s.protocolOperations.GetVModemUnconfirmedNodes(ctx, limit, days, includeZeroNodes, domain)
}

func (s *Storage) GetFTPEnabledNodes(ctx context.Context, limit int, days int, includeZeroNodes bool, domain string) ([]NodeTestResult, error) {
	return s.protocolOperations.GetFTPEnabledNodes(ctx, limit, days, includeZeroNodes, domain)
}

func (s *Storage) GetAKAMismatchNodes(ctx context.Context, limit int, days int, includeZeroNodes bool, domain string) ([]NodeTestResult, error) {
	return s.akaMismatchOperations.GetAKAMismatchNodes(ctx, limit, days, includeZeroNodes, domain)
}

func (s *Storage) GetIPv6IncorrectIPv4CorrectNodes(ctx context.Context, limit int, days int, includeZeroNodes bool, domain string) ([]AKAIPVersionMismatchNode, error) {
	return s.akaMismatchOperations.GetIPv6IncorrectIPv4CorrectNodes(ctx, limit, days, includeZeroNodes, domain)
}

func (s *Storage) GetIPv4IncorrectIPv6CorrectNodes(ctx context.Context, limit int, days int, includeZeroNodes bool, domain string) ([]AKAIPVersionMismatchNode, error) {
	return s.akaMismatchOperations.GetIPv4IncorrectIPv6CorrectNodes(ctx, limit, days, includeZeroNodes, domain)
}

func (s *Storage) GetOtherNetworksSummary(ctx context.Context, days int, domain string) ([]OtherNetworkSummary, error) {
	return s.otherNetworksOperations.GetOtherNetworksSummary(ctx, days, domain)
}

func (s *Storage) GetNodesInNetwork(ctx context.Context, networkName string, limit int, days int, domain string) ([]OtherNetworkNode, error) {
	return s.otherNetworksOperations.GetNodesInNetwork(ctx, networkName, limit, days, domain)
}

func (s *Storage) GetBinkPSoftwareDistribution(ctx context.Context, days int, domain string) (*SoftwareDistribution, error) {
	return s.softwareOperations.GetBinkPSoftwareDistribution(ctx, days, domain)
}

func (s *Storage) GetIFCICOSoftwareDistribution(ctx context.Context, days int, domain string) (*SoftwareDistribution, error) {
	return s.softwareOperations.GetIFCICOSoftwareDistribution(ctx, days, domain)
}

func (s *Storage) GetBinkdDetailedStats(ctx context.Context, days int, domain string) (*SoftwareDistribution, error) {
	return s.softwareOperations.GetBinkdDetailedStats(ctx, days, domain)
}

func (s *Storage) GetGeoHostingDistribution(ctx context.Context, days int, domain string) (*GeoHostingDistribution, error) {
	return s.geoOperations.GetGeoHostingDistribution(ctx, days, domain)
}

func (s *Storage) GetNodesByCountry(ctx context.Context, countryCode string, days int, domain string) ([]NodeTestResult, error) {
	return s.geoOperations.GetNodesByCountry(ctx, countryCode, days, domain)
}

func (s *Storage) GetNodesByProvider(ctx context.Context, provider string, days int, domain string) ([]NodeTestResult, error) {
	return s.geoOperations.GetNodesByProvider(ctx, provider, days, domain)
}

func (s *Storage) GetOnThisDayNodes(ctx context.Context, month, day, limit int, activeOnly bool, domain string) ([]OnThisDayNode, error) {
	return s.analyticsOperations.GetOnThisDayNodes(ctx, month, day, limit, activeOnly, domain)
}

func (s *Storage) GetPioneersByRegion(ctx context.Context, zone, region, limit int, domain string) ([]PioneerNode, error) {
	return s.searchOperations.GetPioneersByRegion(ctx, zone, region, limit, domain)
}

func (s *Storage) GetPSTNCMNodes(ctx context.Context, limit int) ([]PSTNNode, error) {
	return s.analyticsOperations.GetPSTNCMNodes(ctx, limit)
}

func (s *Storage) GetPSTNNodes(ctx context.Context, limit int, zone int, domain string) ([]PSTNNode, error) {
	return s.analyticsOperations.GetPSTNNodes(ctx, limit, zone, domain)
}

func (s *Storage) MarkPSTNDead(ctx context.Context, zone, net, node int, reason, markedBy string) error {
	return s.pstnDeadOperations.MarkDead(ctx, zone, net, node, reason, markedBy)
}

func (s *Storage) UnmarkPSTNDead(ctx context.Context, zone, net, node int, markedBy string) error {
	return s.pstnDeadOperations.UnmarkDead(ctx, zone, net, node, markedBy)
}

func (s *Storage) GetPSTNDeadNodes(ctx context.Context) ([]PSTNDeadNode, error) {
	return s.pstnDeadOperations.GetAllDeadNodes(ctx)
}

func (s *Storage) GetFileRequestNodes(ctx context.Context, limit int, domain string) ([]FileRequestNode, error) {
	return s.analyticsOperations.GetFileRequestNodes(ctx, limit, domain)
}

func (s *Storage) GetEmailCapableNodes(ctx context.Context, limit int, useFieldFallback bool, domain string) ([]EmailCapableNode, error) {
	return s.analyticsOperations.GetEmailCapableNodes(ctx, limit, useFieldFallback, domain)
}

func (s *Storage) GetEmailFlagTrend(ctx context.Context, domain string) ([]EmailFlagTrendPoint, error) {
	return s.analyticsOperations.GetEmailFlagTrend(ctx, domain)
}

func (s *Storage) GetModemAccessibleNodes(ctx context.Context, limit int, days int, includeZeroNodes bool, domain string) ([]ModemAccessibleNode, error) {
	return s.modemOperations.GetModemAccessibleNodes(ctx, limit, days, includeZeroNodes, domain)
}

func (s *Storage) GetModemNoAnswerNodes(ctx context.Context, limit int, days int, includeZeroNodes bool, domain string) ([]ModemNoAnswerNode, error) {
	return s.modemOperations.GetModemNoAnswerNodes(ctx, limit, days, includeZeroNodes, domain)
}

func (s *Storage) GetRecentModemSuccessPhones(ctx context.Context, days int) ([]string, error) {
	return s.modemOperations.GetRecentModemSuccessPhones(ctx, days)
}

func (s *Storage) GetDetailedModemTestResult(ctx context.Context, zone, net, node int, testTime string) (*ModemTestDetail, error) {
	return s.modemOperations.GetDetailedModemTestResult(ctx, zone, net, node, testTime)
}

// WHOIS Operations delegated methods
func (s *Storage) GetAllWhoisResults(ctx context.Context, domain string) ([]DomainWhoisResult, error) {
	return s.whoisOperations.GetAllWhoisResults(ctx, domain)
}

func (s *Storage) GetNodesByDomain(ctx context.Context, domain string, days int) ([]NodeTestResult, error) {
	return s.whoisOperations.GetNodesByDomain(ctx, domain, days)
}

// --- Utility Methods ---

// --- Health and Monitoring ---
