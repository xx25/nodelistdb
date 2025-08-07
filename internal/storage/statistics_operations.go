package storage

import (
	"database/sql"
	"fmt"
	"sync"
	"time"

	"nodelistdb/internal/database"
)

// StatisticsOperations handles all statistics-related database operations
type StatisticsOperations struct {
	db           database.DatabaseInterface
	queryBuilder QueryBuilderInterface
	resultParser *ResultParser
	mu           sync.RWMutex
	// Cache for stats results to improve performance for distant dates
	statsCache map[string]*database.NetworkStats
	cacheMu    sync.RWMutex
}

// NewStatisticsOperations creates a new StatisticsOperations instance
func NewStatisticsOperations(db database.DatabaseInterface, queryBuilder QueryBuilderInterface, resultParser *ResultParser) *StatisticsOperations {
	return &StatisticsOperations{
		db:           db,
		queryBuilder: queryBuilder,
		resultParser: resultParser,
		statsCache:   make(map[string]*database.NetworkStats),
	}
}

// GetStats retrieves network statistics for a specific date with caching
func (so *StatisticsOperations) GetStats(date time.Time) (*database.NetworkStats, error) {
	// Check cache first for distant dates (older than 30 days) to improve performance
	cacheKey := date.Format("2006-01-02")
	if date.Before(time.Now().AddDate(0, 0, -30)) {
		so.cacheMu.RLock()
		if cached, exists := so.statsCache[cacheKey]; exists {
			so.cacheMu.RUnlock()
			return cached, nil
		}
		so.cacheMu.RUnlock()
	}

	so.mu.RLock()
	defer so.mu.RUnlock()

	conn := so.db.Conn()

	// Get main statistics
	statsQuery := so.queryBuilder.StatsSQL()
	row := conn.QueryRow(statsQuery, date)

	stats, err := so.resultParser.ParseNetworkStatsRow(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("no data found for date %v", date)
		}
		return nil, fmt.Errorf("failed to get stats: %w", err)
	}

	// Get zone distribution
	stats.ZoneDistribution = make(map[int]int)
	zoneQuery := so.queryBuilder.ZoneDistributionSQL()
	rows, err := conn.Query(zoneQuery, date)
	if err != nil {
		return nil, fmt.Errorf("failed to get zone distribution: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var zone, count int
		if err := rows.Scan(&zone, &count); err != nil {
			return nil, fmt.Errorf("failed to scan zone distribution: %w", err)
		}
		stats.ZoneDistribution[zone] = count
	}

	// Get largest regions (top 10) - using optimized query
	stats.LargestRegions = []database.RegionInfo{}
	regionQuery := so.queryBuilder.OptimizedLargestRegionsSQL()
	rows, err = conn.Query(regionQuery, date)
	if err != nil {
		return nil, fmt.Errorf("failed to get largest regions: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		region, err := so.resultParser.ParseRegionInfoRow(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to parse region info: %w", err)
		}
		stats.LargestRegions = append(stats.LargestRegions, region)
	}

	// Get largest nets (top 10) - using optimized query
	stats.LargestNets = []database.NetInfo{}
	netQuery := so.queryBuilder.OptimizedLargestNetsSQL()
	rows, err = conn.Query(netQuery, date, date)
	if err != nil {
		return nil, fmt.Errorf("failed to get largest nets: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		net, err := so.resultParser.ParseNetInfoRow(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to parse net info: %w", err)
		}
		stats.LargestNets = append(stats.LargestNets, net)
	}

	// Cache distant dates (older than 30 days) to improve future performance
	if date.Before(time.Now().AddDate(0, 0, -30)) {
		so.cacheMu.Lock()
		so.statsCache[cacheKey] = stats
		so.cacheMu.Unlock()
	}

	return stats, nil
}

// GetLatestStatsDate retrieves the most recent date that has statistics
func (so *StatisticsOperations) GetLatestStatsDate() (time.Time, error) {
	so.mu.RLock()
	defer so.mu.RUnlock()

	conn := so.db.Conn()
	var latestDate time.Time

	query := so.queryBuilder.LatestDateSQL()
	err := conn.QueryRow(query).Scan(&latestDate)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to get latest stats date: %w", err)
	}
	return latestDate, nil
}

// GetAvailableDates returns all unique dates that have nodelist data
func (so *StatisticsOperations) GetAvailableDates() ([]time.Time, error) {
	so.mu.RLock()
	defer so.mu.RUnlock()

	conn := so.db.Conn()

	query := so.queryBuilder.AvailableDatesSQL()
	rows, err := conn.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get available dates: %w", err)
	}
	defer rows.Close()

	var dates []time.Time
	for rows.Next() {
		var date time.Time
		if err := rows.Scan(&date); err != nil {
			return nil, fmt.Errorf("failed to scan date: %w", err)
		}
		dates = append(dates, date)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating available dates: %w", err)
	}

	return dates, nil
}

// GetNearestAvailableDate finds the closest available date to the requested date
func (so *StatisticsOperations) GetNearestAvailableDate(requestedDate time.Time) (time.Time, error) {
	so.mu.RLock()
	defer so.mu.RUnlock()

	conn := so.db.Conn()

	// First check if the exact date exists
	var count int
	exactQuery := so.queryBuilder.ExactDateExistsSQL()
	err := conn.QueryRow(exactQuery, requestedDate).Scan(&count)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to check if date exists: %w", err)
	}
	if count > 0 {
		return requestedDate, nil
	}

	// Find the nearest date - get one before and one after
	var beforeDate, afterDate sql.NullTime

	// Get the closest date before
	beforeQuery := so.queryBuilder.NearestDateBeforeSQL()
	err = conn.QueryRow(beforeQuery, requestedDate).Scan(&beforeDate)
	if err != nil && err != sql.ErrNoRows {
		return time.Time{}, fmt.Errorf("failed to get date before: %w", err)
	}

	// Get the closest date after
	afterQuery := so.queryBuilder.NearestDateAfterSQL()
	err = conn.QueryRow(afterQuery, requestedDate).Scan(&afterDate)
	if err != nil && err != sql.ErrNoRows {
		return time.Time{}, fmt.Errorf("failed to get date after: %w", err)
	}

	// Return the closest one, or fall back to latest if none found
	if beforeDate.Valid && afterDate.Valid {
		beforeDiff := requestedDate.Sub(beforeDate.Time)
		afterDiff := afterDate.Time.Sub(requestedDate)
		if beforeDiff <= afterDiff {
			return beforeDate.Time, nil
		}
		return afterDate.Time, nil
	} else if beforeDate.Valid {
		return beforeDate.Time, nil
	} else if afterDate.Valid {
		return afterDate.Time, nil
	}

	// If no dates found at all, return the latest available date
	return so.GetLatestStatsDate()
}

// GetDateRangeStats returns statistics for a range of dates
func (so *StatisticsOperations) GetDateRangeStats(startDate, endDate time.Time) ([]database.NetworkStats, error) {
	if startDate.After(endDate) {
		return nil, fmt.Errorf("start date cannot be after end date")
	}

	so.mu.RLock()
	defer so.mu.RUnlock()

	conn := so.db.Conn()

	// Get all dates in the range first
	query := `SELECT DISTINCT nodelist_date FROM nodes 
		WHERE nodelist_date >= ? AND nodelist_date <= ? 
		ORDER BY nodelist_date ASC`

	rows, err := conn.Query(query, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("failed to get dates in range: %w", err)
	}
	defer rows.Close()

	var dates []time.Time
	for rows.Next() {
		var date time.Time
		if err := rows.Scan(&date); err != nil {
			return nil, fmt.Errorf("failed to scan date: %w", err)
		}
		dates = append(dates, date)
	}

	// Get stats for each date
	var allStats []database.NetworkStats
	for _, date := range dates {
		stats, err := so.GetStats(date)
		if err != nil {
			// Log error but continue with other dates
			continue
		}
		allStats = append(allStats, *stats)
	}

	return allStats, nil
}

// GetZoneStats returns statistics for a specific zone across all dates
func (so *StatisticsOperations) GetZoneStats(zone int) (map[time.Time]int, error) {
	if zone < 1 || zone > 65535 {
		return nil, fmt.Errorf("invalid zone: %d", zone)
	}

	so.mu.RLock()
	defer so.mu.RUnlock()

	conn := so.db.Conn()

	query := `SELECT nodelist_date, COUNT(*) as node_count 
		FROM nodes 
		WHERE zone = ? 
		GROUP BY nodelist_date 
		ORDER BY nodelist_date ASC`

	rows, err := conn.Query(query, zone)
	if err != nil {
		return nil, fmt.Errorf("failed to get zone stats: %w", err)
	}
	defer rows.Close()

	zoneStats := make(map[time.Time]int)
	for rows.Next() {
		var date time.Time
		var count int
		if err := rows.Scan(&date, &count); err != nil {
			return nil, fmt.Errorf("failed to scan zone stats: %w", err)
		}
		zoneStats[date] = count
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating zone stats: %w", err)
	}

	return zoneStats, nil
}

// GetNodeTypeDistribution returns the distribution of node types for a given date
func (so *StatisticsOperations) GetNodeTypeDistribution(date time.Time) (map[string]int, error) {
	so.mu.RLock()
	defer so.mu.RUnlock()

	conn := so.db.Conn()

	query := `SELECT node_type, COUNT(*) as count 
		FROM nodes 
		WHERE nodelist_date = ? 
		GROUP BY node_type 
		ORDER BY count DESC`

	rows, err := conn.Query(query, date)
	if err != nil {
		return nil, fmt.Errorf("failed to get node type distribution: %w", err)
	}
	defer rows.Close()

	distribution := make(map[string]int)
	for rows.Next() {
		var nodeType string
		var count int
		if err := rows.Scan(&nodeType, &count); err != nil {
			return nil, fmt.Errorf("failed to scan node type distribution: %w", err)
		}
		distribution[nodeType] = count
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating node type distribution: %w", err)
	}

	return distribution, nil
}

// GetConnectivityStats returns connectivity statistics for a given date
func (so *StatisticsOperations) GetConnectivityStats(date time.Time) (*ConnectivityStats, error) {
	so.mu.RLock()
	defer so.mu.RUnlock()

	conn := so.db.Conn()

	query := `SELECT 
		COUNT(*) as total_nodes,
		COUNT(*) FILTER (WHERE json_extract(internet_config, '$.protocols.IBN') IS NOT NULL OR json_extract(internet_config, '$.protocols.BND') IS NOT NULL) as binkp_nodes,
		COUNT(*) FILTER (WHERE json_extract(internet_config, '$.protocols.ITN') IS NOT NULL) as telnet_nodes,
		COUNT(*) FILTER (WHERE has_inet) as inet_nodes,
		COUNT(*) FILTER (WHERE json_extract(internet_config, '$.protocols') IS NOT NULL) as protocol_nodes,
		COUNT(*) FILTER (WHERE json_extract(internet_config, '$.email_protocols') IS NOT NULL) as email_nodes
	FROM nodes 
	WHERE nodelist_date = ?`

	var stats ConnectivityStats
	err := conn.QueryRow(query, date).Scan(
		&stats.TotalNodes,
		&stats.BinkpNodes,
		&stats.TelnetNodes,
		&stats.InternetNodes,
		&stats.ProtocolNodes,
		&stats.EmailNodes,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get connectivity stats: %w", err)
	}

	stats.Date = date
	return &stats, nil
}

// GetTopSysops returns the sysops managing the most nodes for a given date
func (so *StatisticsOperations) GetTopSysops(date time.Time, limit int) ([]SysopStats, error) {
	if limit <= 0 {
		limit = 10
	} else if limit > 100 {
		limit = 100
	}

	so.mu.RLock()
	defer so.mu.RUnlock()

	conn := so.db.Conn()

	query := `SELECT sysop_name, COUNT(*) as node_count,
		COUNT(DISTINCT zone) as zones,
		COUNT(DISTINCT CONCAT(zone, ':', net)) as nets
	FROM nodes 
	WHERE nodelist_date = ? AND sysop_name != '' 
	GROUP BY sysop_name 
	ORDER BY node_count DESC 
	LIMIT ?`

	rows, err := conn.Query(query, date, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get top sysops: %w", err)
	}
	defer rows.Close()

	var sysops []SysopStats
	for rows.Next() {
		var sysop SysopStats
		if err := rows.Scan(&sysop.SysopName, &sysop.NodeCount, &sysop.ZoneCount, &sysop.NetCount); err != nil {
			return nil, fmt.Errorf("failed to scan sysop stats: %w", err)
		}
		sysop.Date = date
		sysops = append(sysops, sysop)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating sysop stats: %w", err)
	}

	return sysops, nil
}

// GetGrowthStats calculates growth statistics between two dates
func (so *StatisticsOperations) GetGrowthStats(startDate, endDate time.Time) (*GrowthStats, error) {
	if startDate.After(endDate) {
		return nil, fmt.Errorf("start date cannot be after end date")
	}

	startStats, err := so.GetStats(startDate)
	if err != nil {
		return nil, fmt.Errorf("failed to get start date stats: %w", err)
	}

	endStats, err := so.GetStats(endDate)
	if err != nil {
		return nil, fmt.Errorf("failed to get end date stats: %w", err)
	}

	growth := &GrowthStats{
		StartDate:      startDate,
		EndDate:        endDate,
		StartStats:     *startStats,
		EndStats:       *endStats,
		NodeGrowth:     endStats.TotalNodes - startStats.TotalNodes,
		ActiveGrowth:   endStats.ActiveNodes - startStats.ActiveNodes,
		BinkpGrowth:    endStats.BinkpNodes - startStats.BinkpNodes,
		InternetGrowth: endStats.InternetNodes - startStats.InternetNodes,
	}

	// Calculate growth percentages
	if startStats.TotalNodes > 0 {
		growth.NodeGrowthPercent = float64(growth.NodeGrowth) / float64(startStats.TotalNodes) * 100
	}
	if startStats.ActiveNodes > 0 {
		growth.ActiveGrowthPercent = float64(growth.ActiveGrowth) / float64(startStats.ActiveNodes) * 100
	}
	if startStats.BinkpNodes > 0 {
		growth.BinkpGrowthPercent = float64(growth.BinkpGrowth) / float64(startStats.BinkpNodes) * 100
	}
	if startStats.InternetNodes > 0 {
		growth.InternetGrowthPercent = float64(growth.InternetGrowth) / float64(startStats.InternetNodes) * 100
	}

	return growth, nil
}

// ClearStatsCache clears the statistics cache - useful for maintenance or testing
func (so *StatisticsOperations) ClearStatsCache() {
	so.cacheMu.Lock()
	so.statsCache = make(map[string]*database.NetworkStats)
	so.cacheMu.Unlock()
}

// GetCacheStats returns information about the stats cache
func (so *StatisticsOperations) GetCacheStats() map[string]interface{} {
	so.cacheMu.RLock()
	defer so.cacheMu.RUnlock()

	return map[string]interface{}{
		"cached_entries":       len(so.statsCache),
		"cache_enabled":        true,
		"cache_threshold_days": 30,
	}
}

// ConnectivityStats represents connectivity-related statistics
type ConnectivityStats struct {
	Date          time.Time `json:"date"`
	TotalNodes    int       `json:"total_nodes"`
	BinkpNodes    int       `json:"binkp_nodes"`
	TelnetNodes   int       `json:"telnet_nodes"`
	InternetNodes int       `json:"internet_nodes"`
	ProtocolNodes int       `json:"protocol_nodes"`
	EmailNodes    int       `json:"email_nodes"`
}

// SysopStats represents statistics for individual sysops
type SysopStats struct {
	Date      time.Time `json:"date"`
	SysopName string    `json:"sysop_name"`
	NodeCount int       `json:"node_count"`
	ZoneCount int       `json:"zone_count"`
	NetCount  int       `json:"net_count"`
}

// GrowthStats represents growth statistics between two dates
type GrowthStats struct {
	StartDate             time.Time             `json:"start_date"`
	EndDate               time.Time             `json:"end_date"`
	StartStats            database.NetworkStats `json:"start_stats"`
	EndStats              database.NetworkStats `json:"end_stats"`
	NodeGrowth            int                   `json:"node_growth"`
	ActiveGrowth          int                   `json:"active_growth"`
	BinkpGrowth           int                   `json:"binkp_growth"`
	InternetGrowth        int                   `json:"internet_growth"`
	NodeGrowthPercent     float64               `json:"node_growth_percent"`
	ActiveGrowthPercent   float64               `json:"active_growth_percent"`
	BinkpGrowthPercent    float64               `json:"binkp_growth_percent"`
	InternetGrowthPercent float64               `json:"internet_growth_percent"`
}
