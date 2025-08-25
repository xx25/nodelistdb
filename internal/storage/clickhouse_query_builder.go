package storage

import (
	"fmt"
	"strings"

	"github.com/nodelistdb/internal/database"
)

// ClickHouseQueryBuilder provides ClickHouse-specific SQL query construction
type ClickHouseQueryBuilder struct {
	*QueryBuilder // Embed the base query builder for common functionality
}

// NewClickHouseQueryBuilder creates a new ClickHouse-specific QueryBuilder instance
func NewClickHouseQueryBuilder() *ClickHouseQueryBuilder {
	return &ClickHouseQueryBuilder{
		QueryBuilder: NewQueryBuilder(),
	}
}

// InsertNodesInChunks performs optimized batch inserts for ClickHouse with proper array formatting
func (cqb *ClickHouseQueryBuilder) InsertNodesInChunks(db database.DatabaseInterface, nodes []database.Node) error {
	if len(nodes) == 0 {
		return nil
	}

	conn := db.Conn()
	
	// Start a transaction for better performance
	tx, err := conn.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()
	
	// Create a ClickHouse result parser for proper array formatting
	resultParser := NewClickHouseResultParser()
	
	// Process nodes in optimal-sized chunks
	const OPTIMAL_BATCH_SIZE = 1000
	for i := 0; i < len(nodes); i += OPTIMAL_BATCH_SIZE {
		end := i + OPTIMAL_BATCH_SIZE
		if end > len(nodes) {
			end = len(nodes)
		}
		
		chunk := nodes[i:end]
		
		// Use ClickHouse-specific SQL building with proper array handling
		insertSQL := cqb.BuildDirectBatchInsertSQL(chunk, resultParser.ResultParser)
		
		// Execute the chunk insert
		if _, err := tx.Exec(insertSQL); err != nil {
			return fmt.Errorf("failed to insert chunk %d-%d: %w", i, end-1, err)
		}
	}
	
	return tx.Commit()
}

// BuildDirectBatchInsertSQL creates a direct VALUES-based INSERT for ClickHouse with proper array handling
func (cqb *ClickHouseQueryBuilder) BuildDirectBatchInsertSQL(nodes []database.Node, rp *ResultParser) string {
	if len(nodes) == 0 {
		return ""
	}

	// Use ClickHouse-specific result parser for array formatting
	crp := NewClickHouseResultParser()

	var buf strings.Builder
	buf.WriteString(`INSERT INTO nodes (
		zone, net, node, nodelist_date, day_number,
		system_name, location, sysop_name, phone, node_type, region, max_speed,
		is_cm, is_mo,
		flags, modem_flags,
		conflict_sequence, has_conflict, has_inet, internet_config, fts_id
	) VALUES `)

	for i, node := range nodes {
		if i > 0 {
			buf.WriteByte(',')
		}

		// Compute FTS ID if not set
		if node.FtsId == "" {
			node.ComputeFtsId()
		}

		// Build direct VALUES clause for ClickHouse
		buf.WriteByte('(')

		// Core fields
		buf.WriteString(fmt.Sprintf("%d,%d,%d,'%s',%d,",
			node.Zone, node.Net, node.Node,
			node.NodelistDate.Format("2006-01-02"), node.DayNumber))

		// String fields (escaped)
		buf.WriteString(fmt.Sprintf("'%s','%s','%s','%s','%s',",
			cqb.escapeClickHouseSQL(node.SystemName), cqb.escapeClickHouseSQL(node.Location),
			cqb.escapeClickHouseSQL(node.SysopName), cqb.escapeClickHouseSQL(node.Phone),
			cqb.escapeClickHouseSQL(node.NodeType)))

		// Region (nullable)
		if node.Region != nil {
			buf.WriteString(fmt.Sprintf("%d,", *node.Region))
		} else {
			buf.WriteString("NULL,")
		}

		// Max speed
		buf.WriteString(fmt.Sprintf("%d,", node.MaxSpeed))

		// Boolean flags
		buf.WriteString(fmt.Sprintf("%t,%t,",
			node.IsCM, node.IsMO))

		// Arrays (ClickHouse format)
		buf.WriteString(fmt.Sprintf("%s,%s,",
			crp.formatArrayForDB(node.Flags),
			crp.formatArrayForDB(node.ModemFlags)))

		// Final fields
		buf.WriteString(fmt.Sprintf("%d,%t,%t,",
			node.ConflictSequence, node.HasConflict, node.HasInet))

		// Internet config JSON
		if node.InternetConfig != nil && len(node.InternetConfig) > 0 {
			buf.WriteString(fmt.Sprintf("'%s',", cqb.escapeClickHouseSQL(string(node.InternetConfig))))
		} else {
			buf.WriteString("'{}',")
		}

		// FTS ID
		buf.WriteString(fmt.Sprintf("'%s')", cqb.escapeClickHouseSQL(node.FtsId)))
	}

	return buf.String()
}

// escapeClickHouseSQL escapes strings for ClickHouse SQL literals
func (cqb *ClickHouseQueryBuilder) escapeClickHouseSQL(s string) string {
	// ClickHouse escape rules: single quotes are escaped with backslash
	s = strings.ReplaceAll(s, "\\", "\\\\") // Escape backslashes first
	s = strings.ReplaceAll(s, "'", "\\'")   // Escape single quotes
	return s
}

// BuildClickHouseFTSQuery builds a ClickHouse-compatible FTS query
func (cqb *ClickHouseQueryBuilder) BuildClickHouseFTSQuery(filter database.NodeFilter) (string, []interface{}, bool) {
	// ClickHouse doesn't have FTS indexes like DuckDB, so we use LIKE with bloom filters
	var conditions []string
	var args []interface{}
	usedFTS := false

	// When doing text searches with LatestOnly=false, use the proper grouping query
	if (filter.Location != nil || filter.SystemName != nil || filter.SysopName != nil) &&
		(filter.LatestOnly == nil || !*filter.LatestOnly) {
		// Use the main BuildNodesQuery which has proper grouping logic
		query, queryArgs := cqb.BuildNodesQuery(filter)
		return query, queryArgs, true
	}

	baseQuery := cqb.NodeSelectSQL()

	// Text search using ILIKE for optimal performance
	if filter.SystemName != nil && *filter.SystemName != "" {
		conditions = append(conditions, "system_name ILIKE ?")
		args = append(args, "%"+*filter.SystemName+"%")
		usedFTS = true
	}

	if filter.Location != nil && *filter.Location != "" {
		conditions = append(conditions, "location ILIKE ?")
		args = append(args, "%"+*filter.Location+"%")
		usedFTS = true
	}

	if filter.SysopName != nil && *filter.SysopName != "" {
		conditions = append(conditions, "sysop_name ILIKE ?")
		args = append(args, "%"+*filter.SysopName+"%")
		usedFTS = true
	}

	// Add other non-text filters
	if filter.Zone != nil {
		conditions = append(conditions, "zone = ?")
		args = append(args, *filter.Zone)
	}

	if filter.Net != nil {
		conditions = append(conditions, "net = ?")
		args = append(args, *filter.Net)
	}

	if filter.Node != nil {
		conditions = append(conditions, "node = ?")
		args = append(args, *filter.Node)
	}

	if filter.NodeType != nil {
		conditions = append(conditions, "node_type = ?")
		args = append(args, *filter.NodeType)
	}

	// Boolean filters
	if filter.IsCM != nil {
		conditions = append(conditions, "is_cm = ?")
		args = append(args, *filter.IsCM)
	}

	if filter.IsMO != nil {
		conditions = append(conditions, "is_mo = ?")
		args = append(args, *filter.IsMO)
	}

	if filter.HasBinkp != nil {
		// HasBinkp is now determined from JSON: check for IBN or BND protocols
		conditions = append(conditions, "(JSON_EXISTS(toString(internet_config), '$.protocols.IBN') OR JSON_EXISTS(toString(internet_config), '$.protocols.BND')) = ?")
		args = append(args, *filter.HasBinkp)
	}

	// Date filters
	if filter.DateFrom != nil {
		conditions = append(conditions, "nodelist_date >= ?")
		args = append(args, *filter.DateFrom)
	}

	if filter.DateTo != nil {
		conditions = append(conditions, "nodelist_date <= ?")
		args = append(args, *filter.DateTo)
	}

	// Latest only
	if filter.LatestOnly != nil && *filter.LatestOnly {
		conditions = append(conditions, "nodelist_date = (SELECT MAX(nodelist_date) FROM nodes)")
	}

	// Build final query
	query := baseQuery
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	// Order by for consistent results
	query += " ORDER BY zone, net, node, nodelist_date DESC, conflict_sequence"

	// Limit and offset
	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", filter.Limit)
		if filter.Offset > 0 {
			query += fmt.Sprintf(" OFFSET %d", filter.Offset)
		}
	}

	return query, args, usedFTS
}

// NodeSelectSQL returns the ClickHouse-compatible base SELECT statement for nodes
func (cqb *ClickHouseQueryBuilder) NodeSelectSQL() string {
	return `
	SELECT zone, net, node, nodelist_date, day_number,
		system_name, location, sysop_name, phone, node_type, region, max_speed,
		is_cm, is_mo,
		flags, modem_flags,
		conflict_sequence, has_conflict, has_inet, internet_config, fts_id
	FROM nodes`
}

// Implement QueryBuilderInterface methods by delegating to embedded QueryBuilder
// where appropriate, overriding only ClickHouse-specific methods

func (cqb *ClickHouseQueryBuilder) InsertNodeSQL() string {
	return cqb.QueryBuilder.InsertNodeSQL()
}

func (cqb *ClickHouseQueryBuilder) BuildBatchInsertSQL(batchSize int) string {
	return cqb.QueryBuilder.BuildBatchInsertSQL(batchSize)
}

func (cqb *ClickHouseQueryBuilder) BuildNodesQuery(filter database.NodeFilter) (string, []interface{}) {
	var baseSQL string
	var args []interface{}

	if filter.LatestOnly != nil && *filter.LatestOnly {
		// ClickHouse-compatible latest only query
		baseSQL = `
		SELECT zone, net, node, nodelist_date, day_number,
			   system_name, location, sysop_name, phone, node_type, region, max_speed,
			   is_cm, is_mo,
			   flags, modem_flags,
			   conflict_sequence, has_conflict, has_inet, internet_config, fts_id
		FROM nodes
		WHERE (zone, net, node, nodelist_date) IN (
			SELECT zone, net, node, MAX(nodelist_date) as max_date
			FROM nodes
			GROUP BY zone, net, node
		)`

		conditions, conditionArgs := cqb.buildClickHouseWhereConditions(filter)
		if len(conditions) > 0 {
			baseSQL += " AND " + strings.Join(conditions, " AND ")
			args = append(args, conditionArgs...)
		}
	} else {
		// Historical search - ClickHouse optimized version without JOINs
		conditions, conditionArgs := cqb.buildClickHouseWhereConditions(filter)
		args = append(args, conditionArgs...)

		var whereClause string
		if len(conditions) > 0 {
			whereClause = " WHERE " + strings.Join(conditions, " AND ")
		}

		// For historical search: get latest entry for each node that matches criteria
		// Use window function approach - more reliable in ClickHouse
		baseSQL = `
		SELECT 
			zone, net, node, nodelist_date, day_number,
			system_name, location, sysop_name, phone, node_type, region, max_speed,
			is_cm, is_mo,
			flags, modem_flags,
			conflict_sequence, has_conflict, has_inet, internet_config, fts_id
		FROM (
			SELECT *,
				   row_number() OVER (PARTITION BY zone, net, node ORDER BY nodelist_date DESC, conflict_sequence ASC) as rn
			FROM nodes
			WHERE (zone, net, node) IN (
				SELECT DISTINCT zone, net, node
				FROM nodes` + whereClause + `
			)
		) ranked
		WHERE rn = 1`
	}

	// Add ORDER BY
	baseSQL += " ORDER BY zone, net, node, nodelist_date DESC"

	// Add LIMIT and OFFSET
	if filter.Limit > 0 {
		baseSQL += fmt.Sprintf(" LIMIT %d", filter.Limit)
		if filter.Offset > 0 {
			baseSQL += fmt.Sprintf(" OFFSET %d", filter.Offset)
		}
	}

	return baseSQL, args
}

// buildClickHouseWhereConditions creates WHERE clause conditions compatible with ClickHouse
func (cqb *ClickHouseQueryBuilder) buildClickHouseWhereConditions(filter database.NodeFilter) ([]string, []interface{}) {
	var conditions []string
	var args []interface{}

	if filter.Zone != nil {
		conditions = append(conditions, "zone = ?")
		args = append(args, *filter.Zone)
	}
	if filter.Net != nil {
		conditions = append(conditions, "net = ?")
		args = append(args, *filter.Net)
	}
	if filter.Node != nil {
		conditions = append(conditions, "node = ?")
		args = append(args, *filter.Node)
	}
	if filter.DateFrom != nil {
		conditions = append(conditions, "nodelist_date >= ?")
		args = append(args, *filter.DateFrom)
	}
	if filter.DateTo != nil {
		conditions = append(conditions, "nodelist_date <= ?")
		args = append(args, *filter.DateTo)
	}
	if filter.SystemName != nil {
		// Use ILIKE for case-insensitive matching - performs as well as materialized columns
		conditions = append(conditions, "system_name ILIKE ?")
		args = append(args, "%"+*filter.SystemName+"%")
	}
	if filter.Location != nil {
		// Use ILIKE for case-insensitive matching - performs as well as materialized columns
		conditions = append(conditions, "location ILIKE ?")
		args = append(args, "%"+*filter.Location+"%")
	}
	if filter.SysopName != nil {
		// Use ILIKE for case-insensitive matching - performs as well as materialized columns
		conditions = append(conditions, "sysop_name ILIKE ?")
		args = append(args, "%"+*filter.SysopName+"%")
	}
	if filter.NodeType != nil {
		conditions = append(conditions, "node_type = ?")
		args = append(args, *filter.NodeType)
	}
	if filter.IsCM != nil {
		conditions = append(conditions, "is_cm = ?")
		args = append(args, *filter.IsCM)
	}
	if filter.HasInet != nil {
		conditions = append(conditions, "has_inet = ?")
		args = append(args, *filter.HasInet)
	}
	if filter.HasBinkp != nil {
		// HasBinkp is now determined from JSON: check for IBN or BND protocols
		conditions = append(conditions, "(JSON_EXISTS(toString(internet_config), '$.protocols.IBN') OR JSON_EXISTS(toString(internet_config), '$.protocols.BND')) = ?")
		args = append(args, *filter.HasBinkp)
	}

	return conditions, args
}

func (cqb *ClickHouseQueryBuilder) BuildFTSQuery(filter database.NodeFilter) (string, []interface{}, bool) {
	// Use ClickHouse-specific FTS query
	return cqb.BuildClickHouseFTSQuery(filter)
}

func (cqb *ClickHouseQueryBuilder) StatsSQL() string {
	// ClickHouse-compatible statistics query using countIf instead of FILTER
	return `
	SELECT 
		nodelist_date,
		COUNT(*) as total_nodes,
		countIf(node_type NOT IN ('Down', 'Hold')) as active_nodes,
		countIf(is_cm) as cm_nodes,
		countIf(is_mo) as mo_nodes,
		countIf(JSON_EXISTS(toString(internet_config), '$.protocols.IBN') OR JSON_EXISTS(toString(internet_config), '$.protocols.BND')) as binkp_nodes,
		countIf(JSON_EXISTS(toString(internet_config), '$.protocols.ITN')) as telnet_nodes,
		countIf(node_type = 'Pvt') as pvt_nodes,
		countIf(node_type = 'Down') as down_nodes,
		countIf(node_type = 'Hold') as hold_nodes,
		countIf(node_type = 'Hub') as hub_nodes,
		countIf(node_type = 'Zone') as zone_nodes,
		countIf(node_type = 'Region') as region_nodes,
		countIf(node_type = 'Host') as host_nodes,
		countIf(has_inet = true) as internet_nodes
	FROM nodes 
	WHERE nodelist_date = ?
	GROUP BY nodelist_date`
}

func (cqb *ClickHouseQueryBuilder) ZoneDistributionSQL() string {
	return cqb.QueryBuilder.ZoneDistributionSQL()
}

func (cqb *ClickHouseQueryBuilder) LargestRegionsSQL() string {
	return cqb.QueryBuilder.LargestRegionsSQL()
}

func (cqb *ClickHouseQueryBuilder) LargestNetsSQL() string {
	return cqb.QueryBuilder.LargestNetsSQL()
}

func (cqb *ClickHouseQueryBuilder) OptimizedLargestRegionsSQL() string {
	// ClickHouse-compatible largest regions query using argMax instead of FIRST
	return `
	SELECT zone, region, COUNT(*) as count,
		   argMax(system_name, CASE WHEN node_type = 'Region' THEN 1 ELSE 0 END) as region_name
	FROM nodes 
	WHERE nodelist_date = ? AND region > 0
	GROUP BY zone, region
	ORDER BY count DESC 
	LIMIT 10`
}

func (cqb *ClickHouseQueryBuilder) OptimizedLargestNetsSQL() string {
	return cqb.QueryBuilder.OptimizedLargestNetsSQL()
}

func (cqb *ClickHouseQueryBuilder) NodeHistorySQL() string {
	return cqb.QueryBuilder.NodeHistorySQL()
}

func (cqb *ClickHouseQueryBuilder) NodeDateRangeSQL() string {
	return cqb.QueryBuilder.NodeDateRangeSQL()
}

func (cqb *ClickHouseQueryBuilder) SysopSearchSQL() string {
	// ClickHouse-optimized sysop search using window functions to avoid aggregation issues
	return `
	WITH 
	global_max AS (
		SELECT MAX(nodelist_date) as max_date FROM nodes
	),
	ranked_nodes AS (
		SELECT 
			zone, net, node, nodelist_date, system_name, location, sysop_name,
			row_number() OVER (PARTITION BY zone, net, node ORDER BY nodelist_date DESC) as rn,
			MIN(nodelist_date) OVER (PARTITION BY zone, net, node) as first_date,
			MAX(nodelist_date) OVER (PARTITION BY zone, net, node) as last_date
		FROM nodes
		WHERE replaceAll(sysop_name, '_', ' ') ILIKE concat('%', replaceAll(?, '_', ' '), '%')
	),
	latest_per_node AS (
		SELECT 
			zone, net, node, system_name, location, sysop_name,
			first_date, last_date, nodelist_date
		FROM ranked_nodes
		WHERE rn = 1
	)
	SELECT 
		zone, net, node, system_name, location, sysop_name,
		first_date, last_date,
		CASE WHEN last_date = (SELECT max_date FROM global_max) THEN true ELSE false END as currently_active
	FROM latest_per_node
	ORDER BY first_date DESC
	LIMIT ?`
}

// NodeSummarySearchSQL returns SQL for searching nodes with lifetime information
func (cqb *ClickHouseQueryBuilder) NodeSummarySearchSQL() string {
	return `
	WITH 
	global_max AS (
		SELECT MAX(nodelist_date) as max_date FROM nodes
	),
	ranked_nodes AS (
		SELECT 
			zone, net, node, nodelist_date, system_name, location, sysop_name,
			row_number() OVER (PARTITION BY zone, net, node ORDER BY nodelist_date DESC) as rn,
			MIN(nodelist_date) OVER (PARTITION BY zone, net, node) as first_date,
			MAX(nodelist_date) OVER (PARTITION BY zone, net, node) as last_date
		FROM nodes
		WHERE 1=1
			AND (? IS NULL OR zone = ?)
			AND (? IS NULL OR net = ?)
			AND (? IS NULL OR node = ?)
			AND (? IS NULL OR system_name ILIKE ?)
			AND (? IS NULL OR location ILIKE ?)
			AND (? IS NULL OR replaceAll(sysop_name, '_', ' ') ILIKE replaceAll(?, '_', ' '))
	),
	latest_per_node AS (
		SELECT 
			zone, net, node, system_name, location, sysop_name,
			first_date, last_date, nodelist_date
		FROM ranked_nodes
		WHERE rn = 1
	)
	SELECT 
		zone, net, node, system_name, location, sysop_name,
		first_date, last_date,
		CASE WHEN last_date = (SELECT max_date FROM global_max) THEN true ELSE false END as currently_active
	FROM latest_per_node
	ORDER BY last_date DESC, zone, net, node
	LIMIT ?`
}

func (cqb *ClickHouseQueryBuilder) ConflictCheckSQL() string {
	return cqb.QueryBuilder.ConflictCheckSQL()
}

func (cqb *ClickHouseQueryBuilder) MarkConflictSQL() string {
	return cqb.QueryBuilder.MarkConflictSQL()
}

func (cqb *ClickHouseQueryBuilder) IsProcessedSQL() string {
	return cqb.QueryBuilder.IsProcessedSQL()
}

func (cqb *ClickHouseQueryBuilder) LatestDateSQL() string {
	return cqb.QueryBuilder.LatestDateSQL()
}

func (cqb *ClickHouseQueryBuilder) AvailableDatesSQL() string {
	return cqb.QueryBuilder.AvailableDatesSQL()
}

func (cqb *ClickHouseQueryBuilder) ExactDateExistsSQL() string {
	return cqb.QueryBuilder.ExactDateExistsSQL()
}

func (cqb *ClickHouseQueryBuilder) NearestDateBeforeSQL() string {
	return cqb.QueryBuilder.NearestDateBeforeSQL()
}

func (cqb *ClickHouseQueryBuilder) NearestDateAfterSQL() string {
	return cqb.QueryBuilder.NearestDateAfterSQL()
}

func (cqb *ClickHouseQueryBuilder) ConsecutiveNodelistCheckSQL() string {
	return cqb.QueryBuilder.ConsecutiveNodelistCheckSQL()
}

func (cqb *ClickHouseQueryBuilder) NextNodelistDateSQL() string {
	return cqb.QueryBuilder.NextNodelistDateSQL()
}

func (cqb *ClickHouseQueryBuilder) UniqueSysopsWithFilterSQL() string {
	// ClickHouse-compatible unique sysops query with filter
	return `
		WITH sysop_stats AS (
			SELECT 
				sysop_name,
				COUNT(DISTINCT concat(toString(zone), ':', toString(net), '/', toString(node))) as node_count,
				COUNT(DISTINCT CASE WHEN nodelist_date = (SELECT MAX(nodelist_date) FROM nodes) THEN concat(toString(zone), ':', toString(net), '/', toString(node)) END) as active_nodes,
				MIN(nodelist_date) as first_seen,
				MAX(nodelist_date) as last_seen,
				arraySort(arrayDistinct(groupArray(zone))) as zones
			FROM nodes
			WHERE replaceAll(sysop_name, '_', ' ') ILIKE concat('%', replaceAll(?, '_', ' '), '%')
			GROUP BY sysop_name
		)
		SELECT 
			sysop_name,
			node_count,
			active_nodes,
			first_seen,
			last_seen,
			zones
		FROM sysop_stats
		ORDER BY node_count DESC, sysop_name
		LIMIT ? OFFSET ?
	`
}

func (cqb *ClickHouseQueryBuilder) UniqueSysopsSQL() string {
	// ClickHouse-compatible unique sysops query
	return `
		WITH sysop_stats AS (
			SELECT 
				sysop_name,
				COUNT(DISTINCT concat(toString(zone), ':', toString(net), '/', toString(node))) as node_count,
				COUNT(DISTINCT CASE WHEN nodelist_date = (SELECT MAX(nodelist_date) FROM nodes) THEN concat(toString(zone), ':', toString(net), '/', toString(node)) END) as active_nodes,
				MIN(nodelist_date) as first_seen,
				MAX(nodelist_date) as last_seen,
				arraySort(arrayDistinct(groupArray(zone))) as zones
			FROM nodes
			GROUP BY sysop_name
		)
		SELECT 
			sysop_name,
			node_count,
			active_nodes,
			first_seen,
			last_seen,
			zones
		FROM sysop_stats
		ORDER BY node_count DESC, sysop_name
		LIMIT ? OFFSET ?
	`
}

// FlagFirstAppearanceSQL returns SQL for finding the first appearance of a flag in ClickHouse
func (cqb *ClickHouseQueryBuilder) FlagFirstAppearanceSQL() string {
	return `
		SELECT 
			zone, net, node, nodelist_date, system_name, location, sysop_name
		FROM nodes
		WHERE (has(flags, ?) OR has(modem_flags, ?) 
		   OR (has_inet = 1 AND JSON_EXISTS(toString(internet_config), concat('$.protocols.', ?))))
		ORDER BY nodelist_date ASC, zone ASC, net ASC, node ASC
		LIMIT 1
	`
}

// FlagUsageByYearSQL returns SQL for counting flag usage by year in ClickHouse
func (cqb *ClickHouseQueryBuilder) FlagUsageByYearSQL() string {
	return `
		WITH node_years AS (
			SELECT 
				toYear(nodelist_date) as year,
				zone, net, node,
				MAX(CASE WHEN has(flags, ?) OR has(modem_flags, ?) 
					OR (has_inet = 1 AND JSON_EXISTS(toString(internet_config), concat('$.protocols.', ?)))
					THEN 1 ELSE 0 END) as has_flag
			FROM nodes
			GROUP BY year, zone, net, node
		)
		SELECT 
			year,
			COUNT(*) as total_nodes,
			SUM(has_flag) as node_count,
			CASE 
				WHEN COUNT(*) > 0 
				THEN round((SUM(has_flag) / COUNT(*)) * 100, 2)
				ELSE 0 
			END as percentage
		FROM node_years
		GROUP BY year
		ORDER BY year
	`
}

// NetworkNameSQL returns SQL for getting network name from coordinator node in ClickHouse
func (cqb *ClickHouseQueryBuilder) NetworkNameSQL() string {
	return `
		SELECT system_name 
		FROM nodes 
		WHERE zone = ? AND net = ? AND node = 0 
		ORDER BY nodelist_date DESC 
		LIMIT 1
	`
}

// NetworkHistorySQL returns SQL for getting network appearance periods in ClickHouse
func (cqb *ClickHouseQueryBuilder) NetworkHistorySQL() string {
	return `
		WITH network_dates AS (
			SELECT DISTINCT 
				nodelist_date,
				day_number,
				lagInFrame(nodelist_date) OVER (ORDER BY nodelist_date) as prev_date
			FROM nodes
			WHERE zone = ? AND net = ?
			ORDER BY nodelist_date
		),
		appearance_groups AS (
			SELECT 
				nodelist_date,
				day_number,
				CASE 
					WHEN prev_date IS NULL OR dateDiff('day', prev_date, nodelist_date) > 14 THEN 1
					ELSE 0
				END as new_group
			FROM network_dates
		),
		appearance_periods AS (
			SELECT 
				nodelist_date,
				day_number,
				SUM(new_group) OVER (ORDER BY nodelist_date ROWS UNBOUNDED PRECEDING) as group_id
			FROM appearance_groups
		)
		SELECT 
			MIN(nodelist_date) as start_date,
			MAX(nodelist_date) as end_date,
			MIN(day_number) as start_day_num,
			MAX(day_number) as end_day_num,
			COUNT(*) as nodelist_count
		FROM appearance_periods
		GROUP BY group_id
		ORDER BY start_date
	`
}
