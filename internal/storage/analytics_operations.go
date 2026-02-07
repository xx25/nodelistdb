package storage

import (
	"fmt"
	"regexp"
	"strings"
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
	pstnDeadOps  *PSTNDeadOperations
	mu           sync.RWMutex
}

// NewAnalyticsOperations creates a new AnalyticsOperations instance
func NewAnalyticsOperations(db database.DatabaseInterface, queryBuilder QueryBuilderInterface, resultParser ResultParserInterface, pstnDeadOps *PSTNDeadOperations) *AnalyticsOperations {
	return &AnalyticsOperations{
		db:           db,
		queryBuilder: queryBuilder,
		resultParser: resultParser,
		pstnDeadOps:  pstnDeadOps,
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

// GetOnThisDayNodes finds nodes that were first added on this day (month/day) in previous years
// A node is considered "new" when a sysop first appears with that node address
// YearsActive is calculated from first appearance to final disappearance (ignoring temporary gaps)
func (ao *AnalyticsOperations) GetOnThisDayNodes(month, day int, limit int, activeOnly bool) ([]OnThisDayNode, error) {
	ao.mu.RLock()
	defer ao.mu.RUnlock()

	conn := ao.db.Conn()

	// Build the active filter clause
	activeFilter := ""
	if activeOnly {
		activeFilter = "AND nl.last_seen >= md.latest_date"
	}

	// Build limit clause
	limitClause := ""
	if limit > 0 {
		limitClause = fmt.Sprintf("LIMIT %d", limit)
	}

	// Query explanation:
	// 1. Find the first and last appearance of each (zone, net, node, sysop_name) combination
	// 2. Filter to only those whose first appearance was on the specified month/day
	// 3. Check if still active by comparing last_seen to the max nodelist date
	// 4. Calculate years active from first to last appearance
	query := fmt.Sprintf(`
		WITH
		-- Get the maximum nodelist date to determine if node is still active
		max_date AS (
			SELECT max(nodelist_date) as latest_date FROM nodes
		),
		-- Get first and last appearance for each (zone, net, node, sysop_name) combination
		node_lifetimes AS (
			SELECT
				zone,
				net,
				node,
				sysop_name,
				argMin(system_name, nodelist_date) as first_system_name,
				argMin(location, nodelist_date) as first_location,
				min(nodelist_date) as first_appeared,
				max(nodelist_date) as last_seen,
				argMin(
					concat(
						if(node_type != '', node_type, ''),
						if(node_type != '', ',', ''),
						toString(node), ',',
						system_name, ',',
						location, ',',
						sysop_name, ',',
						phone, ',',
						toString(max_speed),
						if(length(flags) > 0, ',', ''),
						arrayStringConcat(flags, ',')
					),
					nodelist_date
				) as raw_line
			FROM nodes
			GROUP BY zone, net, node, sysop_name
		)
		SELECT
			nl.zone,
			nl.net,
			nl.node,
			nl.sysop_name,
			nl.first_system_name,
			nl.first_location,
			nl.first_appeared,
			nl.last_seen,
			toYear(nl.last_seen) - toYear(nl.first_appeared) + 1 as years_active,
			nl.last_seen >= md.latest_date as still_active,
			nl.raw_line
		FROM node_lifetimes nl
		CROSS JOIN max_date md
		WHERE toMonth(nl.first_appeared) = ?
		  AND toDayOfMonth(nl.first_appeared) = ?
		  AND toYear(nl.first_appeared) < toYear(today())
		  %s
		ORDER BY nl.first_appeared DESC, nl.zone, nl.net, nl.node
		%s
	`, activeFilter, limitClause)

	rows, err := conn.Query(query, month, day)
	if err != nil {
		return nil, fmt.Errorf("failed to query on this day nodes: %w", err)
	}
	defer rows.Close()

	var results []OnThisDayNode
	for rows.Next() {
		var n OnThisDayNode
		err := rows.Scan(
			&n.Zone,
			&n.Net,
			&n.Node,
			&n.SysopName,
			&n.SystemName,
			&n.Location,
			&n.FirstAppeared,
			&n.LastSeen,
			&n.YearsActive,
			&n.StillActive,
			&n.RawLine,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan on this day node row: %w", err)
		}
		results = append(results, n)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating on this day rows: %w", err)
	}

	return results, nil
}

// pstnPhoneCleanRegex removes non-digit characters except leading +
var pstnPhoneCleanRegex = regexp.MustCompile(`[^\d+]`)

// normalizePhone converts a phone number to standard format for display.
// Duplicates modem.NormalizePhone logic to avoid import cycle (modem imports storage).
func normalizePhone(phone string) string {
	if phone == "" {
		return ""
	}
	phone = strings.TrimSpace(phone)
	lower := strings.ToLower(phone)
	if lower == "-unpublished-" || lower == "unpublished" || lower == "-" || lower == "none" {
		return ""
	}
	cleaned := pstnPhoneCleanRegex.ReplaceAllString(phone, "")
	if !strings.HasPrefix(cleaned, "+") {
		cleaned = "+" + cleaned
	}
	if len(cleaned) < 4 {
		return ""
	}
	return cleaned
}

// MaxPSTNSearchLimit is the maximum number of PSTN nodes that can be returned.
// Higher than MaxSearchLimit because modem-tester needs the full list.
const MaxPSTNSearchLimit = 10000

// GetPSTNCMNodes returns nodes from the latest nodelist that have valid phone numbers and CM flag
// Phone numbers like "-Unpublished-" and "000-000-000-000" are excluded
// Down and Hold nodes are excluded as they are not operational
func (ao *AnalyticsOperations) GetPSTNCMNodes(limit int) ([]PSTNNode, error) {
	ao.mu.RLock()
	defer ao.mu.RUnlock()

	if limit <= 0 {
		limit = DefaultSearchLimit
	}
	if limit > MaxSearchLimit {
		limit = MaxSearchLimit
	}

	conn := ao.db.Conn()

	query := `
		WITH latest_date AS (
			SELECT MAX(nodelist_date) as max_date FROM nodes
		)
		SELECT
			zone,
			net,
			node,
			system_name,
			location,
			sysop_name,
			phone,
			is_cm,
			nodelist_date,
			node_type,
			max_speed,
			flags,
			modem_flags
		FROM nodes
		WHERE nodelist_date = (SELECT max_date FROM latest_date)
		  AND conflict_sequence = 0
		  AND is_cm = true
		  AND phone != ''
		  AND phone != '-Unpublished-'
		  AND phone != '000-000-000-000'
		  AND node != 0
		  AND node_type NOT IN ('Down', 'Hold')
		ORDER BY zone, net, node
		LIMIT ?`

	rows, err := conn.Query(query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query PSTN CM nodes: %w", err)
	}
	defer rows.Close()

	var results []PSTNNode
	for rows.Next() {
		var n PSTNNode
		var flags []string
		var modemFlags []string
		err := rows.Scan(
			&n.Zone,
			&n.Net,
			&n.Node,
			&n.SystemName,
			&n.Location,
			&n.SysopName,
			&n.Phone,
			&n.IsCM,
			&n.NodelistDate,
			&n.NodeType,
			&n.MaxSpeed,
			&flags,
			&modemFlags,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan PSTN CM node row: %w", err)
		}
		n.Flags = flags
		n.ModemFlags = modemFlags
		n.PhoneNormalized = normalizePhone(n.Phone)
		results = append(results, n)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating PSTN CM rows: %w", err)
	}

	return results, nil
}

// GetPSTNNodes returns ALL nodes from the latest nodelist that have valid phone numbers.
// Unlike GetPSTNCMNodes, this includes both CM and non-CM nodes.
// Excludes Down/Hold nodes, coordinators (node=0), and unpublished/invalid phones.
// zone=0 returns all zones.
func (ao *AnalyticsOperations) GetPSTNNodes(limit int, zone int) ([]PSTNNode, error) {
	ao.mu.RLock()
	defer ao.mu.RUnlock()

	if limit <= 0 {
		limit = DefaultSearchLimit
	}
	if limit > MaxPSTNSearchLimit {
		limit = MaxPSTNSearchLimit
	}

	conn := ao.db.Conn()

	query := `
		WITH latest_date AS (
			SELECT MAX(nodelist_date) as max_date FROM nodes
		)
		SELECT
			zone,
			net,
			node,
			system_name,
			location,
			sysop_name,
			phone,
			is_cm,
			nodelist_date,
			node_type,
			max_speed,
			flags,
			modem_flags
		FROM nodes
		WHERE nodelist_date = (SELECT max_date FROM latest_date)
		  AND conflict_sequence = 0
		  AND phone != ''
		  AND phone != '-Unpublished-'
		  AND phone != '000-000-000-000'
		  AND node != 0
		  AND node_type NOT IN ('Down', 'Hold')`

	args := []interface{}{}
	if zone > 0 {
		query += " AND zone = ?"
		args = append(args, zone)
	}
	query += " ORDER BY zone, net, node LIMIT ?"
	args = append(args, limit)

	rows, err := conn.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query PSTN nodes: %w", err)
	}
	defer rows.Close()

	var results []PSTNNode
	for rows.Next() {
		var n PSTNNode
		var flags []string
		var modemFlags []string
		err := rows.Scan(
			&n.Zone,
			&n.Net,
			&n.Node,
			&n.SystemName,
			&n.Location,
			&n.SysopName,
			&n.Phone,
			&n.IsCM,
			&n.NodelistDate,
			&n.NodeType,
			&n.MaxSpeed,
			&flags,
			&modemFlags,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan PSTN node row: %w", err)
		}
		n.Flags = flags
		n.ModemFlags = modemFlags
		n.PhoneNormalized = normalizePhone(n.Phone)
		results = append(results, n)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating PSTN rows: %w", err)
	}

	// Enrich with PSTN dead status
	if ao.pstnDeadOps != nil {
		deadSet, err := ao.pstnDeadOps.GetDeadNodeSet()
		if err == nil && len(deadSet) > 0 {
			for i := range results {
				key := [3]int{results[i].Zone, results[i].Net, results[i].Node}
				if reason, ok := deadSet[key]; ok {
					results[i].IsPSTNDead = true
					results[i].PSTNDeadReason = reason
				}
			}
		}
	}

	return results, nil
}

// GetFileRequestNodes returns nodes from the latest nodelist that have file request flags (XA-XX)
// Excludes Down/Hold nodes, coordinators (node=0), and conflict duplicates
func (ao *AnalyticsOperations) GetFileRequestNodes(limit int) ([]FileRequestNode, error) {
	ao.mu.RLock()
	defer ao.mu.RUnlock()

	if limit <= 0 {
		limit = DefaultSearchLimit
	}
	if limit > MaxPSTNSearchLimit {
		limit = MaxPSTNSearchLimit
	}

	conn := ao.db.Conn()

	query := `
		WITH latest_date AS (
			SELECT MAX(nodelist_date) as max_date FROM nodes
		)
		SELECT
			zone,
			net,
			node,
			system_name,
			location,
			sysop_name,
			arrayFirst(f -> f IN ('XA', 'XB', 'XC', 'XP', 'XR', 'XW', 'XX'), flags) as file_request_flag,
			nodelist_date,
			node_type,
			flags
		FROM nodes
		WHERE nodelist_date = (SELECT max_date FROM latest_date)
		  AND conflict_sequence = 0
		  AND hasAny(flags, ['XA', 'XB', 'XC', 'XP', 'XR', 'XW', 'XX'])
		  AND node != 0
		  AND node_type NOT IN ('Down', 'Hold')
		ORDER BY zone, net, node
		LIMIT ?`

	rows, err := conn.Query(query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query file request nodes: %w", err)
	}
	defer rows.Close()

	var results []FileRequestNode
	for rows.Next() {
		var n FileRequestNode
		var flags []string
		err := rows.Scan(
			&n.Zone,
			&n.Net,
			&n.Node,
			&n.SystemName,
			&n.Location,
			&n.SysopName,
			&n.FileRequestFlag,
			&n.NodelistDate,
			&n.NodeType,
			&flags,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan file request node row: %w", err)
		}
		n.Flags = flags
		results = append(results, n)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating file request rows: %w", err)
	}

	return results, nil
}

// Close closes the analytics operations
func (ao *AnalyticsOperations) Close() error {
	ao.mu.Lock()
	defer ao.mu.Unlock()
	// Nothing to close at this level
	return nil
}
