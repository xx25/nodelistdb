package storage

import (
	"fmt"
	"strings"

	"nodelistdb/internal/database"
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
		is_cm, is_mo, has_binkp, has_telnet, is_down, is_hold, is_pvt, is_active,
		flags, modem_flags, internet_protocols, internet_hostnames, internet_ports, internet_emails,
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
		buf.WriteString(fmt.Sprintf("%t,%t,%t,%t,%t,%t,%t,%t,", 
			node.IsCM, node.IsMO, node.HasBinkp, node.HasTelnet,
			node.IsDown, node.IsHold, node.IsPvt, node.IsActive))
		
		// Arrays (ClickHouse format)
		buf.WriteString(fmt.Sprintf("%s,%s,%s,%s,%s,%s,", 
			crp.formatArrayForDB(node.Flags),
			crp.formatArrayForDB(node.ModemFlags),
			crp.formatArrayForDB(node.InternetProtocols),
			crp.formatArrayForDB(node.InternetHostnames),
			crp.formatIntArrayForDB(node.InternetPorts),
			crp.formatArrayForDB(node.InternetEmails)))
		
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

	// Text search using materialized lowercase columns with bloom filter indexes
	if filter.SystemName != nil && *filter.SystemName != "" {
		conditions = append(conditions, "lower(system_name) LIKE ?")
		args = append(args, "%"+strings.ToLower(*filter.SystemName)+"%")
		usedFTS = true
	}

	if filter.Location != nil && *filter.Location != "" {
		conditions = append(conditions, "lower(location) LIKE ?")
		args = append(args, "%"+strings.ToLower(*filter.Location)+"%")
		usedFTS = true
	}

	if filter.SysopName != nil && *filter.SysopName != "" {
		conditions = append(conditions, "lower(sysop_name) LIKE ?")
		args = append(args, "%"+strings.ToLower(*filter.SysopName)+"%")
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
		conditions = append(conditions, "has_binkp = ?")
		args = append(args, *filter.HasBinkp)
	}

	if filter.HasTelnet != nil {
		conditions = append(conditions, "has_telnet = ?")
		args = append(args, *filter.HasTelnet)
	}

	if filter.IsActive != nil {
		conditions = append(conditions, "is_active = ?")
		args = append(args, *filter.IsActive)
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
		is_cm, is_mo, has_binkp, has_telnet, is_down, is_hold, is_pvt, is_active,
		flags, modem_flags, internet_protocols, internet_hostnames, internet_ports, internet_emails,
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
			   is_cm, is_mo, has_binkp, has_telnet, is_down, is_hold, is_pvt, is_active,
			   flags, modem_flags, internet_protocols, internet_hostnames, internet_ports, internet_emails,
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
			is_cm, is_mo, has_binkp, has_telnet, is_down, is_hold, is_pvt, is_active,
			flags, modem_flags, internet_protocols, internet_hostnames, internet_ports, internet_emails,
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
		// Use inline lower() function since materialized columns don't exist
		conditions = append(conditions, "lower(system_name) LIKE ?")
		args = append(args, "%"+strings.ToLower(*filter.SystemName)+"%")
	}
	if filter.Location != nil {
		// Use inline lower() function since materialized columns don't exist
		conditions = append(conditions, "lower(location) LIKE ?")
		args = append(args, "%"+strings.ToLower(*filter.Location)+"%")
	}
	if filter.SysopName != nil {
		// Use inline lower() function since materialized columns don't exist
		conditions = append(conditions, "lower(sysop_name) LIKE ?")
		args = append(args, "%"+strings.ToLower(*filter.SysopName)+"%")
	}
	if filter.NodeType != nil {
		conditions = append(conditions, "node_type = ?")
		args = append(args, *filter.NodeType)
	}
	if filter.IsActive != nil {
		conditions = append(conditions, "is_active = ?")
		args = append(args, *filter.IsActive)
	}
	if filter.IsCM != nil {
		conditions = append(conditions, "is_cm = ?")
		args = append(args, *filter.IsCM)
	}
	if filter.HasBinkp != nil {
		conditions = append(conditions, "has_binkp = ?")
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
		countIf(is_active AND NOT is_down AND NOT is_hold) as active_nodes,
		countIf(is_cm) as cm_nodes,
		countIf(is_mo) as mo_nodes,
		countIf(has_binkp) as binkp_nodes,
		countIf(has_telnet) as telnet_nodes,
		countIf(is_pvt) as pvt_nodes,
		countIf(is_down) as down_nodes,
		countIf(is_hold) as hold_nodes,
		countIf(length(internet_protocols) > 0) as internet_nodes
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
	// ClickHouse-optimized sysop search query without JOINs
	return `
	WITH 
	max_date AS (
		SELECT MAX(nodelist_date) as latest_date FROM nodes
	),
	sysop_nodes AS (
		SELECT 
			zone, net, node,
			MIN(nodelist_date) as first_date,
			MAX(nodelist_date) as last_date,
			argMax(system_name, nodelist_date) as system_name,
			argMax(location, nodelist_date) as location,
			argMax(sysop_name, nodelist_date) as sysop_name,
			-- Check if this sysop is still on this address in latest nodelist
			MAX(CASE WHEN nodelist_date = (SELECT latest_date FROM max_date) THEN 1 ELSE 0 END) as is_in_latest
		FROM nodes
		WHERE lower(sysop_name) LIKE concat('%', lower(?), '%')
		GROUP BY zone, net, node
	)
	SELECT 
		zone, net, node, system_name, location, sysop_name,
		first_date, last_date,
		CASE WHEN is_in_latest = 1 THEN true ELSE false END as currently_active
	FROM sysop_nodes
	ORDER BY first_date DESC
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
				COUNT(DISTINCT CASE WHEN is_active = true THEN concat(toString(zone), ':', toString(net), '/', toString(node)) END) as active_nodes,
				MIN(nodelist_date) as first_seen,
				MAX(nodelist_date) as last_seen,
				arraySort(arrayDistinct(groupArray(zone))) as zones
			FROM nodes
			WHERE lower(sysop_name) LIKE lower(?)
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
				COUNT(DISTINCT CASE WHEN is_active = true THEN concat(toString(zone), ':', toString(net), '/', toString(node)) END) as active_nodes,
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