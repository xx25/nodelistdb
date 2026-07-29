package storage

import (
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/nodelistdb/internal/database"
)

// StatisticsOperations handles all statistics-related database operations
type StatisticsOperations struct {
	db           database.DatabaseInterface
	queryBuilder QueryBuilderInterface
	resultParser ResultParserInterface
	mu           sync.RWMutex
}

// NewStatisticsOperations creates a new StatisticsOperations instance
func NewStatisticsOperations(db database.DatabaseInterface, queryBuilder QueryBuilderInterface, resultParser ResultParserInterface) *StatisticsOperations {
	return &StatisticsOperations{
		db:           db,
		queryBuilder: queryBuilder,
		resultParser: resultParser,
	}
}

// GetStats retrieves network statistics for a specific date.
// An empty domain aggregates across all networks. Caching belongs to
// CachedStorage, which wraps this call.
func (so *StatisticsOperations) GetStats(date time.Time, domain string) (*database.NetworkStats, error) {
	so.mu.RLock()
	defer so.mu.RUnlock()

	conn := so.db.Conn()

	// Get main statistics
	statsQuery := so.queryBuilder.StatsSQL()
	row := conn.QueryRow(statsQuery, date, domain, domain)

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
	rows, err := conn.Query(zoneQuery, date, domain, domain)
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

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read zone distribution: %w", err)
	}

	// Get largest regions (top 10) - using optimized query
	stats.LargestRegions = []database.RegionInfo{}
	regionQuery := so.queryBuilder.OptimizedLargestRegionsSQL()
	rows, err = conn.Query(regionQuery, date, domain, domain)
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

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read largest regions: %w", err)
	}

	// Get largest nets (top 10) - using optimized query
	stats.LargestNets = []database.NetInfo{}
	netQuery := so.queryBuilder.OptimizedLargestNetsSQL()
	rows, err = conn.Query(netQuery, date, domain, domain, date, domain, domain)
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

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read largest nets: %w", err)
	}

	return stats, nil
}

// GetLatestStatsDate retrieves the most recent date that has statistics.
// An empty domain looks across all networks.
func (so *StatisticsOperations) GetLatestStatsDate(domain string) (time.Time, error) {
	so.mu.RLock()
	defer so.mu.RUnlock()

	conn := so.db.Conn()
	var latestDate time.Time

	query := so.queryBuilder.LatestDateSQL()
	err := conn.QueryRow(query, domain, domain).Scan(&latestDate)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to get latest stats date: %w", err)
	}
	return latestDate, nil
}

// GetAvailableDates returns all unique dates that have nodelist data.
// An empty domain looks across all networks.
func (so *StatisticsOperations) GetAvailableDates(domain string) ([]time.Time, error) {
	so.mu.RLock()
	defer so.mu.RUnlock()

	conn := so.db.Conn()

	query := so.queryBuilder.AvailableDatesSQL()
	rows, err := conn.Query(query, domain, domain)
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

// GetNearestAvailableDate finds the closest available date to the requested date.
// An empty domain looks across all networks; a concrete domain keeps other
// networks' denser date grids from leaking into this network's date picker.
func (so *StatisticsOperations) GetNearestAvailableDate(requestedDate time.Time, domain string) (time.Time, error) {
	so.mu.RLock()
	defer so.mu.RUnlock()

	conn := so.db.Conn()

	// First check if the exact date exists
	var count int
	exactQuery := so.queryBuilder.ExactDateExistsSQL()
	err := conn.QueryRow(exactQuery, requestedDate, domain, domain).Scan(&count)
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
	err = conn.QueryRow(beforeQuery, requestedDate, domain, domain).Scan(&beforeDate)
	if err != nil && err != sql.ErrNoRows {
		return time.Time{}, fmt.Errorf("failed to get date before: %w", err)
	}

	// Get the closest date after
	afterQuery := so.queryBuilder.NearestDateAfterSQL()
	err = conn.QueryRow(afterQuery, requestedDate, domain, domain).Scan(&afterDate)
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
	return so.GetLatestStatsDate(domain)
}

// NodeCountByDate holds a single data point for the node count history chart.
type NodeCountByDate struct {
	Date       time.Time
	TotalNodes int
}

// GetNodeCountHistory returns total node count per nodelist date for charting.
// An empty domain counts across all networks.
func (so *StatisticsOperations) GetNodeCountHistory(domain string) ([]NodeCountByDate, error) {
	so.mu.RLock()
	defer so.mu.RUnlock()

	conn := so.db.Conn()

	rows, err := conn.Query(`
		SELECT nodelist_date, count(*) AS total_nodes
		FROM nodes
		WHERE `+optionalDomainSQL+`
		GROUP BY nodelist_date
		ORDER BY nodelist_date ASC
	`, domain, domain)
	if err != nil {
		return nil, fmt.Errorf("failed to query node count history: %w", err)
	}
	defer rows.Close()

	var result []NodeCountByDate
	for rows.Next() {
		var d NodeCountByDate
		if err := rows.Scan(&d.Date, &d.TotalNodes); err != nil {
			return nil, fmt.Errorf("failed to scan node count: %w", err)
		}
		result = append(result, d)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read node count history: %w", err)
	}
	return result, nil
}

// BrowseZone is one row of the hierarchy browser's zone listing.
type BrowseZone struct {
	Zone      int
	NodeCount int
	Name      string // zone-coordinator system name, if present
}

// BrowseRegion is one row of the hierarchy browser's region listing.
// Region 0 represents nodes within the zone that have no region assigned.
type BrowseRegion struct {
	Region    int
	NodeCount int
	Name      string // region-coordinator system name, if present
	Location  string // region-coordinator location, if present
}

// BrowseNet is one row of the hierarchy browser's net listing.
type BrowseNet struct {
	Net       int
	NodeCount int
	Name      string // host-coordinator system name, if present
	Location  string // host-coordinator location, if present
}

// GetBrowseZones lists every zone present in the nodelist for the given date.
func (so *StatisticsOperations) GetBrowseZones(date time.Time, domain string) ([]BrowseZone, error) {
	so.mu.RLock()
	defer so.mu.RUnlock()

	conn := so.db.Conn()
	rows, err := conn.Query(so.queryBuilder.BrowseZonesSQL(), date, domain, domain)
	if err != nil {
		return nil, fmt.Errorf("failed to query browse zones: %w", err)
	}
	defer rows.Close()

	var result []BrowseZone
	for rows.Next() {
		var z BrowseZone
		if err := rows.Scan(&z.Zone, &z.NodeCount, &z.Name); err != nil {
			return nil, fmt.Errorf("failed to scan browse zone: %w", err)
		}
		result = append(result, z)
	}
	return result, rows.Err()
}

// GetBrowseRegions lists every region within a zone for the given date.
func (so *StatisticsOperations) GetBrowseRegions(date time.Time, zone int, domain string) ([]BrowseRegion, error) {
	so.mu.RLock()
	defer so.mu.RUnlock()

	conn := so.db.Conn()
	rows, err := conn.Query(so.queryBuilder.BrowseRegionsSQL(), date, zone, domain, domain)
	if err != nil {
		return nil, fmt.Errorf("failed to query browse regions: %w", err)
	}
	defer rows.Close()

	var result []BrowseRegion
	for rows.Next() {
		var r BrowseRegion
		if err := rows.Scan(&r.Region, &r.NodeCount, &r.Name, &r.Location); err != nil {
			return nil, fmt.Errorf("failed to scan browse region: %w", err)
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// GetBrowseNets lists every net within a zone+region for the given date.
// Pass region 0 to list nets that have no region assigned.
func (so *StatisticsOperations) GetBrowseNets(date time.Time, zone, region int, domain string) ([]BrowseNet, error) {
	so.mu.RLock()
	defer so.mu.RUnlock()

	conn := so.db.Conn()
	rows, err := conn.Query(so.queryBuilder.BrowseNetsSQL(), date, zone, region, domain, domain)
	if err != nil {
		return nil, fmt.Errorf("failed to query browse nets: %w", err)
	}
	defer rows.Close()

	var result []BrowseNet
	for rows.Next() {
		var n BrowseNet
		if err := rows.Scan(&n.Net, &n.NodeCount, &n.Name, &n.Location); err != nil {
			return nil, fmt.Errorf("failed to scan browse net: %w", err)
		}
		result = append(result, n)
	}
	return result, rows.Err()
}

// GetBrowseNodes lists every entry within a zone+net for a single nodelist date.
func (so *StatisticsOperations) GetBrowseNodes(date time.Time, zone, net int, domain string) ([]database.Node, error) {
	so.mu.RLock()
	defer so.mu.RUnlock()

	conn := so.db.Conn()
	rows, err := conn.Query(so.queryBuilder.BrowseNodesSQL(), date, zone, net, domain, domain)
	if err != nil {
		return nil, fmt.Errorf("failed to query browse nodes: %w", err)
	}
	defer rows.Close()

	var result []database.Node
	for rows.Next() {
		node, err := so.resultParser.ParseNodeRow(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to parse browse node row: %w", err)
		}
		result = append(result, node)
	}
	return result, rows.Err()
}
