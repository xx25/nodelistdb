package storage

import (
	"fmt"
	"sync"
	"time"

	"github.com/nodelistdb/internal/database"
)

// FlagFirstAppearance represents the first occurrence of a flag
type FlagFirstAppearance struct {
	Zone         int       `json:"zone"`
	Net          int       `json:"net"`
	Node         int       `json:"node"`
	NodelistDate time.Time `json:"nodelist_date"`
	SystemName   string    `json:"system_name"`
	Location     string    `json:"location"`
	SysopName    string    `json:"sysop_name"`
}

// FlagUsageByYear represents flag usage statistics for a year
type FlagUsageByYear struct {
	Year       int     `json:"year"`
	NodeCount  int     `json:"node_count"`
	TotalNodes int     `json:"total_nodes"`
	Percentage float64 `json:"percentage"`
}

// NetworkAppearance represents a continuous period when a network was active
type NetworkAppearance struct {
	StartDate    time.Time `json:"start_date"`
	EndDate      time.Time `json:"end_date"`
	StartDayNum  int       `json:"start_day_num"`
	EndDayNum    int       `json:"end_day_num"`
	DurationDays int       `json:"duration_days"`
	NodelistCount int      `json:"nodelist_count"`
}

// NetworkHistory represents the complete history of a network
type NetworkHistory struct {
	Zone         int                 `json:"zone"`
	Net          int                 `json:"net"`
	NetworkName  string              `json:"network_name"`
	FirstSeen    time.Time           `json:"first_seen"`
	LastSeen     time.Time           `json:"last_seen"`
	TotalDays    int                 `json:"total_days"`
	ActiveDays   int                 `json:"active_days"`
	Appearances  []NetworkAppearance `json:"appearances"`
}

// AnalyticsOperations handles analytics-related database operations
type AnalyticsOperations struct {
	db           database.DatabaseInterface
	queryBuilder QueryBuilderInterface
	resultParser ResultParserInterface
	mu           sync.RWMutex
}

// NewAnalyticsOperations creates a new AnalyticsOperations instance
func NewAnalyticsOperations(db database.DatabaseInterface, queryBuilder QueryBuilderInterface, resultParser ResultParserInterface) *AnalyticsOperations {
	return &AnalyticsOperations{
		db:           db,
		queryBuilder: queryBuilder,
		resultParser: resultParser,
	}
}

// GetFlagFirstAppearance finds the first node that used a specific flag
func (ao *AnalyticsOperations) GetFlagFirstAppearance(flag string) (*FlagFirstAppearance, error) {
	ao.mu.RLock()
	defer ao.mu.RUnlock()

	if flag == "" {
		return nil, fmt.Errorf("flag cannot be empty")
	}

	conn := ao.db.Conn()
	query := ao.queryBuilder.FlagFirstAppearanceSQL()

	// Query uses pre-aggregated flag_statistics table
	row := conn.QueryRow(query, flag)

	var fa FlagFirstAppearance
	err := row.Scan(
		&fa.Zone,
		&fa.Net,
		&fa.Node,
		&fa.NodelistDate,
		&fa.SystemName,
		&fa.Location,
		&fa.SysopName,
	)

	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, nil // No results found
		}
		return nil, fmt.Errorf("failed to scan flag first appearance: %w", err)
	}

	return &fa, nil
}

// GetFlagUsageByYear returns the usage statistics of a flag by year
func (ao *AnalyticsOperations) GetFlagUsageByYear(flag string) ([]FlagUsageByYear, error) {
	ao.mu.RLock()
	defer ao.mu.RUnlock()

	if flag == "" {
		return nil, fmt.Errorf("flag cannot be empty")
	}

	conn := ao.db.Conn()
	query := ao.queryBuilder.FlagUsageByYearSQL()

	// Query uses pre-aggregated flag_statistics table
	rows, err := conn.Query(query, flag)
	if err != nil {
		return nil, fmt.Errorf("failed to query flag usage by year: %w", err)
	}
	defer rows.Close()

	var results []FlagUsageByYear
	for rows.Next() {
		var fu FlagUsageByYear
		err := rows.Scan(&fu.Year, &fu.TotalNodes, &fu.NodeCount, &fu.Percentage)
		if err != nil {
			return nil, fmt.Errorf("failed to scan flag usage row: %w", err)
		}
		results = append(results, fu)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return results, nil
}

// GetNetworkHistory returns the complete appearance history of a network
func (ao *AnalyticsOperations) GetNetworkHistory(zone, net int) (*NetworkHistory, error) {
	ao.mu.RLock()
	defer ao.mu.RUnlock()

	conn := ao.db.Conn()

	// First, get the network name from node 0 if it exists
	nameQuery := ao.queryBuilder.NetworkNameSQL()
	var networkName string
	err := conn.QueryRow(nameQuery, zone, net).Scan(&networkName)
	if err != nil {
		// Network might not have a coordinator, use default name
		networkName = fmt.Sprintf("Network %d:%d", zone, net)
	}

	// Get all appearances of the network
	historyQuery := ao.queryBuilder.NetworkHistorySQL()
	rows, err := conn.Query(historyQuery, zone, net)
	if err != nil {
		return nil, fmt.Errorf("failed to query network history: %w", err)
	}
	defer rows.Close()

	var appearances []NetworkAppearance
	var firstSeen, lastSeen time.Time
	var totalNodelistCount int

	for rows.Next() {
		var app NetworkAppearance
		err := rows.Scan(
			&app.StartDate,
			&app.EndDate,
			&app.StartDayNum,
			&app.EndDayNum,
			&app.NodelistCount,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan network appearance: %w", err)
		}

		app.DurationDays = int(app.EndDate.Sub(app.StartDate).Hours()/24) + 1
		appearances = append(appearances, app)
		totalNodelistCount += app.NodelistCount

		if firstSeen.IsZero() || app.StartDate.Before(firstSeen) {
			firstSeen = app.StartDate
		}
		if app.EndDate.After(lastSeen) {
			lastSeen = app.EndDate
		}
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	if len(appearances) == 0 {
		return nil, nil // Network not found
	}

	totalDays := int(lastSeen.Sub(firstSeen).Hours()/24) + 1

	return &NetworkHistory{
		Zone:        zone,
		Net:         net,
		NetworkName: networkName,
		FirstSeen:   firstSeen,
		LastSeen:    lastSeen,
		TotalDays:   totalDays,
		ActiveDays:  totalNodelistCount,
		Appearances: appearances,
	}, nil
}

// UpdateFlagStatistics updates flag_statistics table for a specific nodelist date.
// This should be called after inserting new nodes to keep analytics up-to-date.
func (ao *AnalyticsOperations) UpdateFlagStatistics(nodelistDate time.Time) error {
	ao.mu.Lock()
	defer ao.mu.Unlock()

	if nodelistDate.IsZero() {
		return fmt.Errorf("nodelist date cannot be zero")
	}

	conn := ao.db.Conn()

	// Incremental update SQL - processes only nodes from the specific nodelist_date
	updateSQL := `
	INSERT INTO flag_statistics (
		flag,
		year,
		nodelist_date,
		unique_nodes,
		total_nodes_in_year,
		first_zone,
		first_net,
		first_node,
		first_nodelist_date,
		first_day_number,
		first_system_name,
		first_location,
		first_sysop_name,
		first_phone,
		first_node_type,
		first_region,
		first_max_speed,
		first_is_cm,
		first_is_mo,
		first_has_inet,
		first_raw_line
	)
	WITH
	-- Explode all flags from the new nodelist only
	new_node_flags AS (
		SELECT
			zone,
			net,
			node,
			nodelist_date,
			day_number,
			toYear(nodelist_date) AS year,
			system_name,
			location,
			sysop_name,
			phone,
			node_type,
			region,
			max_speed,
			is_cm,
			is_mo,
			has_inet,
			raw_line,
			arrayJoin(arrayConcat(
				flags,
				modem_flags,
				extractAll(toString(internet_config), '"([A-Z]{3})"')
			)) AS flag
		FROM nodes
		WHERE nodelist_date = ?
		  AND (length(flags) > 0 OR length(modem_flags) > 0 OR length(extractAll(toString(internet_config), '"([A-Z]{3})"')) > 0)
	),
	-- Get unique flags from new nodelist
	new_flags AS (
		SELECT DISTINCT flag
		FROM new_node_flags
	),
	-- Get the global first appearance of each flag (may be earlier than this nodelist)
	flag_first_appearance AS (
		SELECT
			n.flag,
			argMin((n.zone, n.net, n.node, n.nodelist_date, n.day_number, n.system_name, n.location, n.sysop_name, n.phone, n.node_type, n.region, n.max_speed, n.is_cm, n.is_mo, n.has_inet, n.raw_line), n.nodelist_date) AS first_node
		FROM (
			SELECT
				zone,
				net,
				node,
				nodelist_date,
				day_number,
				system_name,
				location,
				sysop_name,
				phone,
				node_type,
				region,
				max_speed,
				is_cm,
				is_mo,
				has_inet,
				raw_line,
				arrayJoin(arrayConcat(
					flags,
					modem_flags,
					extractAll(toString(internet_config), '"([A-Z]{3})"')
				)) AS flag
			FROM nodes
		) AS n
		INNER JOIN new_flags nf ON n.flag = nf.flag
		GROUP BY n.flag
	),
	-- Aggregate unique nodes per flag and year from the new nodelist
	flag_year_stats AS (
		SELECT
			flag,
			year,
			max(nodelist_date) AS nodelist_date,
			uniqExact((zone, net, node)) AS unique_nodes
		FROM new_node_flags
		GROUP BY flag, year
	),
	-- Calculate total unique nodes per year (across ALL nodes in that year, not just the new nodelist)
	total_nodes_per_year AS (
		SELECT
			toYear(nodelist_date) AS year,
			uniqExact((zone, net, node)) AS total_nodes
		FROM nodes
		WHERE toYear(nodelist_date) IN (SELECT DISTINCT year FROM flag_year_stats)
		GROUP BY year
	)
	SELECT
		s.flag,
		s.year,
		s.nodelist_date,
		s.unique_nodes,
		t.total_nodes AS total_nodes_in_year,
		tupleElement(f.first_node, 1) AS first_zone,
		tupleElement(f.first_node, 2) AS first_net,
		tupleElement(f.first_node, 3) AS first_node,
		tupleElement(f.first_node, 4) AS first_nodelist_date,
		tupleElement(f.first_node, 5) AS first_day_number,
		tupleElement(f.first_node, 6) AS first_system_name,
		tupleElement(f.first_node, 7) AS first_location,
		tupleElement(f.first_node, 8) AS first_sysop_name,
		tupleElement(f.first_node, 9) AS first_phone,
		tupleElement(f.first_node, 10) AS first_node_type,
		tupleElement(f.first_node, 11) AS first_region,
		tupleElement(f.first_node, 12) AS first_max_speed,
		tupleElement(f.first_node, 13) AS first_is_cm,
		tupleElement(f.first_node, 14) AS first_is_mo,
		tupleElement(f.first_node, 15) AS first_has_inet,
		tupleElement(f.first_node, 16) AS first_raw_line
	FROM flag_year_stats s
	LEFT JOIN flag_first_appearance f ON s.flag = f.flag
	LEFT JOIN total_nodes_per_year t ON s.year = t.year
	`

	_, err := conn.Exec(updateSQL, nodelistDate)
	if err != nil {
		return fmt.Errorf("failed to update flag_statistics for date %s: %w", nodelistDate.Format("2006-01-02"), err)
	}

	return nil
}

// Close closes the analytics operations
func (ao *AnalyticsOperations) Close() error {
	ao.mu.Lock()
	defer ao.mu.Unlock()
	// Nothing to close at this level
	return nil
}
