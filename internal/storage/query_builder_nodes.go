package storage

import (
	"fmt"
	"strings"

	"github.com/nodelistdb/internal/database"
)

// Node-related SQL query methods

// InsertNodesInChunks performs optimized batch inserts for ClickHouse with proper array formatting
func (qb *QueryBuilder) InsertNodesInChunks(db database.DatabaseInterface, nodes []database.Node) error {
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
		insertSQL := qb.BuildDirectBatchInsertSQL(chunk, resultParser.ResultParser)

		// Execute the chunk insert
		if _, err := tx.Exec(insertSQL); err != nil {
			return fmt.Errorf("failed to insert chunk %d-%d: %w", i, end-1, err)
		}
	}

	return tx.Commit()
}

// BuildDirectBatchInsertSQL creates a direct VALUES-based INSERT for ClickHouse with proper array handling
func (qb *QueryBuilder) BuildDirectBatchInsertSQL(nodes []database.Node, rp *ResultParser) string {
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
		conflict_sequence, has_conflict, has_inet, internet_config, fts_id, raw_line
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
			qb.escapeSQL(node.SystemName), qb.escapeSQL(node.Location),
			qb.escapeSQL(node.SysopName), qb.escapeSQL(node.Phone),
			qb.escapeSQL(node.NodeType)))

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
			buf.WriteString(fmt.Sprintf("'%s',", qb.escapeSQL(string(node.InternetConfig))))
		} else {
			buf.WriteString("'{}',")
		}

		// FTS ID and raw line
		buf.WriteString(fmt.Sprintf("'%s','%s')", qb.escapeSQL(node.FtsId), qb.escapeSQL(node.RawLine)))
	}

	return buf.String()
}

// NodeSelectSQL returns the ClickHouse-compatible base SELECT statement for nodes
func (qb *QueryBuilder) NodeSelectSQL() string {
	return `
	SELECT zone, net, node, nodelist_date, day_number,
		system_name, location, sysop_name, phone, node_type, region, max_speed,
		is_cm, is_mo,
		flags, modem_flags,
		conflict_sequence, has_conflict, has_inet, internet_config, fts_id, raw_line
	FROM nodes`
}

// InsertNodeSQL builds a parameterized INSERT statement for nodes
func (qb *QueryBuilder) InsertNodeSQL() string {
	return `
	INSERT INTO nodes (
		zone, net, node, nodelist_date, day_number,
		system_name, location, sysop_name, phone, node_type, region, max_speed,
		is_cm, is_mo,
		flags, modem_flags,
		conflict_sequence, has_conflict, has_inet, internet_config, fts_id, raw_line
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
		?, ?,
		?, ?, ?, ?, ?, ?)`
}

// BuildBatchInsertSQL creates a batch INSERT statement with proper parameterization
func (qb *QueryBuilder) BuildBatchInsertSQL(batchSize int) string {
	if batchSize <= 0 {
		return qb.InsertNodeSQL()
	}

	// Create placeholder for one row with direct array binding (no JSON casting)
	valuePlaceholder := "(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"

	// Build batch values
	values := make([]string, batchSize)
	for i := 0; i < batchSize; i++ {
		values[i] = valuePlaceholder
	}

	return fmt.Sprintf(`
		INSERT INTO nodes (
			zone, net, node, nodelist_date, day_number,
			system_name, location, sysop_name, phone, node_type, region, max_speed,
			is_cm, is_mo,
			flags, modem_flags,
			conflict_sequence, has_conflict, has_inet, internet_config, fts_id, raw_line
		) VALUES %s
		ON CONFLICT (zone, net, node, nodelist_date, conflict_sequence)
		DO NOTHING`, strings.Join(values, ","))
}

// BuildNodesQuery builds the main nodes query with filters
func (qb *QueryBuilder) BuildNodesQuery(filter database.NodeFilter) (string, []interface{}) {
	var baseSQL string
	var args []interface{}

	if filter.LatestOnly != nil && *filter.LatestOnly {
		// ClickHouse-compatible latest only query
		baseSQL = `
		SELECT zone, net, node, nodelist_date, day_number,
			   system_name, location, sysop_name, phone, node_type, region, max_speed,
			   is_cm, is_mo,
			   flags, modem_flags,
			   conflict_sequence, has_conflict, has_inet, internet_config, fts_id, raw_line
		FROM nodes
		WHERE (zone, net, node, nodelist_date) IN (
			SELECT zone, net, node, MAX(nodelist_date) as max_date
			FROM nodes
			GROUP BY zone, net, node
		)`

		conditions, conditionArgs := qb.buildWhereConditions(filter)
		if len(conditions) > 0 {
			baseSQL += " AND " + strings.Join(conditions, " AND ")
			args = append(args, conditionArgs...)
		}
	} else {
		// Historical search - ClickHouse optimized version without JOINs
		conditions, conditionArgs := qb.buildWhereConditions(filter)
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
			conflict_sequence, has_conflict, has_inet, internet_config, fts_id, raw_line
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

// BuildFTSQuery builds a ClickHouse-compatible FTS query
func (qb *QueryBuilder) BuildFTSQuery(filter database.NodeFilter) (string, []interface{}, bool) {
	// ClickHouse doesn't have FTS indexes like DuckDB, so we use LIKE with bloom filters
	var conditions []string
	var args []interface{}
	usedFTS := false

	// When doing text searches with LatestOnly=false, use the proper grouping query
	if (filter.Location != nil || filter.SystemName != nil || filter.SysopName != nil) &&
		(filter.LatestOnly == nil || !*filter.LatestOnly) {
		// Use the main BuildNodesQuery which has proper grouping logic
		query, queryArgs := qb.BuildNodesQuery(filter)
		return query, queryArgs, true
	}

	baseQuery := qb.NodeSelectSQL()

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

// buildWhereConditions creates WHERE clause conditions compatible with ClickHouse
func (qb *QueryBuilder) buildWhereConditions(filter database.NodeFilter) ([]string, []interface{}) {
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

// NodeHistorySQL returns SQL for retrieving node history
func (qb *QueryBuilder) NodeHistorySQL() string {
	return `
	SELECT zone, net, node, nodelist_date, day_number,
		   system_name, location, sysop_name, phone, node_type, region, max_speed,
		   is_cm, is_mo,
		   flags, modem_flags,
		   conflict_sequence, has_conflict, has_inet, internet_config, fts_id, raw_line
	FROM nodes
	WHERE zone = ? AND net = ? AND node = ?
	ORDER BY nodelist_date ASC, conflict_sequence ASC`
}

// NodeDateRangeSQL returns SQL for getting first and last dates of a node
func (qb *QueryBuilder) NodeDateRangeSQL() string {
	return `
	SELECT MIN(nodelist_date) as first_date, MAX(nodelist_date) as last_date
	FROM nodes
	WHERE zone = ? AND net = ? AND node = ?`
}

// ConflictCheckSQL returns SQL for checking if a node already exists for a date
func (qb *QueryBuilder) ConflictCheckSQL() string {
	return `SELECT COUNT(*) FROM nodes
		 WHERE zone = ? AND net = ? AND node = ? AND nodelist_date = ?
		 LIMIT 1`
}

// MarkConflictSQL returns SQL for marking original entry as conflicted
func (qb *QueryBuilder) MarkConflictSQL() string {
	return `UPDATE nodes SET has_conflict = true
		WHERE zone = ? AND net = ? AND node = ? AND nodelist_date = ? AND conflict_sequence = 0`
}

// SysopSearchSQL returns SQL for sysop search with window functions
func (qb *QueryBuilder) SysopSearchSQL() string {
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
func (qb *QueryBuilder) NodeSummarySearchSQL() string {
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

// UniqueSysopsSQL returns SQL for getting unique sysops with statistics
func (qb *QueryBuilder) UniqueSysopsSQL() string {
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

// UniqueSysopsWithFilterSQL returns SQL for getting unique sysops with filter
func (qb *QueryBuilder) UniqueSysopsWithFilterSQL() string {
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
