package storage

import (
	"fmt"
	"strings"

	"github.com/nodelistdb/internal/database"
)

// QueryBuilder provides safe SQL query construction with parameter binding
type QueryBuilder struct{}

// NewQueryBuilder creates a new QueryBuilder instance
func NewQueryBuilder() *QueryBuilder {
	return &QueryBuilder{}
}

// Optimal batch size for database inserts - balance between memory usage and performance
const OPTIMAL_BATCH_SIZE = 1000

// InsertNodeSQL builds a parameterized INSERT statement for nodes
func (qb *QueryBuilder) InsertNodeSQL() string {
	return `
	INSERT INTO nodes (
		zone, net, node, nodelist_date, day_number,
		system_name, location, sysop_name, phone, node_type, region, max_speed,
		is_cm, is_mo,
		flags, modem_flags,
		conflict_sequence, has_conflict, has_inet, internet_config, fts_id
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 
		?, ?,
		?, ?, ?, ?, ?)`
}

// BuildBatchInsertSQL creates a batch INSERT statement with proper parameterization
func (qb *QueryBuilder) BuildBatchInsertSQL(batchSize int) string {
	if batchSize <= 0 {
		return qb.InsertNodeSQL()
	}

	// Create placeholder for one row with direct array binding (no JSON casting)
	valuePlaceholder := "(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"

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
			conflict_sequence, has_conflict, has_inet, internet_config, fts_id
		) VALUES %s
		ON CONFLICT (zone, net, node, nodelist_date, conflict_sequence) 
		DO NOTHING`, strings.Join(values, ","))
}

// BuildDirectBatchInsertSQL creates a direct VALUES-based INSERT for maximum performance
func (qb *QueryBuilder) BuildDirectBatchInsertSQL(nodes []database.Node, rp *ResultParser) string {
	if len(nodes) == 0 {
		return ""
	}

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

		// Build direct VALUES clause
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

		// Arrays (optimized format)
		buf.WriteString(fmt.Sprintf("%s,%s,",
			rp.formatArrayForDB(node.Flags),
			rp.formatArrayForDB(node.ModemFlags)))

		// Final fields
		buf.WriteString(fmt.Sprintf("%d,%t,%t,",
			node.ConflictSequence, node.HasConflict, node.HasInet))

		// Internet config JSON
		if node.InternetConfig != nil && len(node.InternetConfig) > 0 {
			buf.WriteString(fmt.Sprintf("'%s',", qb.escapeSQL(string(node.InternetConfig))))
		} else {
			buf.WriteString("NULL,")
		}

		// FTS ID
		buf.WriteString(fmt.Sprintf("'%s')", qb.escapeSQL(node.FtsId)))
	}

	buf.WriteString(" ON CONFLICT (zone, net, node, nodelist_date, conflict_sequence) DO NOTHING")
	return buf.String()
}

// escapeSQL escapes single quotes for SQL string literals
func (qb *QueryBuilder) escapeSQL(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// InsertNodesInChunks performs optimized batch inserts with chunking to avoid
// large memory allocations and improve database performance
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
	
	// Create a result parser for direct SQL building (needed for array handling)
	resultParser := NewResultParser()
	
	// Process nodes in optimal-sized chunks
	for i := 0; i < len(nodes); i += OPTIMAL_BATCH_SIZE {
		end := i + OPTIMAL_BATCH_SIZE
		if end > len(nodes) {
			end = len(nodes)
		}
		
		chunk := nodes[i:end]
		
		// Use direct SQL building for proper array handling in DuckDB
		// This is necessary because DuckDB doesn't support []string parameters in prepared statements
		insertSQL := qb.BuildDirectBatchInsertSQL(chunk, resultParser)
		
		// Execute the chunk insert
		if _, err := tx.Exec(insertSQL); err != nil {
			return fmt.Errorf("failed to insert chunk %d-%d: %w", i, end-1, err)
		}
	}
	
	return tx.Commit()
}

// NodeSelectSQL returns the base SELECT statement for nodes
func (qb *QueryBuilder) NodeSelectSQL() string {
	return `
	SELECT zone, net, node, nodelist_date, day_number,
		   system_name, location, sysop_name, phone, node_type, region, max_speed,
		   is_cm, is_mo,
		   flags, modem_flags,
		   conflict_sequence, has_conflict, has_inet, internet_config, fts_id
	FROM nodes`
}

// BuildNodesQuery constructs a safe query with WHERE conditions for node filtering
func (qb *QueryBuilder) BuildNodesQuery(filter database.NodeFilter) (string, []interface{}) {
	var baseSQL string
	var args []interface{}

	if filter.LatestOnly != nil && *filter.LatestOnly {
		baseSQL = `
		SELECT zone, net, node, nodelist_date, day_number,
			   system_name, location, sysop_name, phone, node_type, region, max_speed,
			   is_cm, is_mo,
			   flags, modem_flags,
			   conflict_sequence, has_conflict, has_inet, internet_config
		FROM (
			SELECT *, 
				   ROW_NUMBER() OVER (PARTITION BY zone, net, node ORDER BY nodelist_date DESC, conflict_sequence ASC) as rn
			FROM nodes
		) ranked WHERE rn = 1`

		conditions, conditionArgs := qb.buildWhereConditions(filter)
		if len(conditions) > 0 {
			baseSQL += " AND " + strings.Join(conditions, " AND ")
			args = append(args, conditionArgs...)
		}
	} else {
		// Historical search - find nodes that have ever matched criteria, show most recent info
		conditions, conditionArgs := qb.buildWhereConditions(filter)
		args = append(args, conditionArgs...)

		var whereClause string
		if len(conditions) > 0 {
			whereClause = " WHERE " + strings.Join(conditions, " AND ")
		}

		baseSQL = `
		WITH matching_nodes AS (
			SELECT DISTINCT zone, net, node
			FROM nodes` + whereClause + `
		)
		SELECT n.zone, n.net, n.node, n.nodelist_date, n.day_number,
			   n.system_name, n.location, n.sysop_name, n.phone, n.node_type, n.region, n.max_speed,
			   n.is_cm, n.is_mo,
			   n.flags, n.modem_flags,
			   n.conflict_sequence, n.has_conflict, n.has_inet, n.internet_config, n.fts_id
		FROM (
			SELECT *, 
				   ROW_NUMBER() OVER (PARTITION BY zone, net, node ORDER BY nodelist_date DESC, conflict_sequence ASC) as rn
			FROM nodes
		) n
		INNER JOIN matching_nodes mn ON (n.zone = mn.zone AND n.net = mn.net AND n.node = mn.node)
		WHERE n.rn = 1`
	}

	// Add ORDER BY
	baseSQL += " ORDER BY n.zone, n.net, n.node, n.nodelist_date DESC"

	// Add LIMIT and OFFSET
	if filter.Limit > 0 {
		baseSQL += " LIMIT ?"
		args = append(args, filter.Limit)

		if filter.Offset > 0 {
			baseSQL += " OFFSET ?"
			args = append(args, filter.Offset)
		}
	}

	return baseSQL, args
}

// buildWhereConditions creates WHERE clause conditions with proper parameter binding
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
		conditions = append(conditions, "system_name ILIKE ?")
		args = append(args, "%"+*filter.SystemName+"%")
	}
	if filter.Location != nil {
		conditions = append(conditions, "location ILIKE ?")
		args = append(args, "%"+*filter.Location+"%")
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
		conditions = append(conditions, "(json_extract(internet_config, '$.protocols.IBN') IS NOT NULL OR json_extract(internet_config, '$.protocols.BND') IS NOT NULL) = ?")
		args = append(args, *filter.HasBinkp)
	}

	return conditions, args
}

// StatsSQL returns the SQL for network statistics
func (qb *QueryBuilder) StatsSQL() string {
	return `
	SELECT 
		nodelist_date,
		COUNT(*) as total_nodes,
		COUNT(*) FILTER (WHERE node_type NOT IN ('Down', 'Hold')) as active_nodes,
		COUNT(*) FILTER (WHERE is_cm) as cm_nodes,
		COUNT(*) FILTER (WHERE is_mo) as mo_nodes,
		COUNT(*) FILTER (WHERE json_extract(internet_config, '$.protocols.IBN') IS NOT NULL OR json_extract(internet_config, '$.protocols.BND') IS NOT NULL) as binkp_nodes,
		COUNT(*) FILTER (WHERE json_extract(internet_config, '$.protocols.ITN') IS NOT NULL) as telnet_nodes,
		COUNT(*) FILTER (WHERE node_type = 'Pvt') as pvt_nodes,
		COUNT(*) FILTER (WHERE node_type = 'Down') as down_nodes,
		COUNT(*) FILTER (WHERE node_type = 'Hold') as hold_nodes,
		COUNT(*) FILTER (WHERE node_type = 'Hub') as hub_nodes,
		COUNT(*) FILTER (WHERE node_type = 'Zone') as zone_nodes,
		COUNT(*) FILTER (WHERE node_type = 'Region') as region_nodes,
		COUNT(*) FILTER (WHERE node_type = 'Host') as host_nodes,
		COUNT(*) FILTER (WHERE has_inet = true) as internet_nodes
	FROM nodes 
	WHERE nodelist_date = ?
	GROUP BY nodelist_date`
}

// OptimizedLargestRegionsSQL returns optimized SQL for largest regions stats with better indexing
func (qb *QueryBuilder) OptimizedLargestRegionsSQL() string {
	return `
	SELECT zone, region, COUNT(*) as count,
		   FIRST(system_name) FILTER (WHERE node_type = 'Region') as region_name
	FROM nodes 
	WHERE nodelist_date = ? AND region > 0
	GROUP BY zone, region
	ORDER BY count DESC 
	LIMIT 10`
}

// OptimizedLargestNetsSQL returns optimized SQL for largest nets stats with better performance
func (qb *QueryBuilder) OptimizedLargestNetsSQL() string {
	return `
	SELECT nc.zone, nc.net, nc.count, hn.host_name
	FROM (
		SELECT zone, net, COUNT(*) as count
		FROM nodes 
		WHERE nodelist_date = ? AND node_type IN ('Node', 'Hub', 'Pvt', 'Hold', 'Down')
		GROUP BY zone, net
		ORDER BY count DESC
		LIMIT 10
	) nc
	LEFT JOIN (
		SELECT zone, net, system_name as host_name
		FROM nodes
		WHERE nodelist_date = ? AND node_type = 'Host'
	) hn ON nc.zone = hn.zone AND nc.net = hn.net
	ORDER BY nc.count DESC`
}

// ZoneDistributionSQL returns SQL for zone distribution stats
func (qb *QueryBuilder) ZoneDistributionSQL() string {
	return "SELECT zone, COUNT(*) FROM nodes WHERE nodelist_date = ? GROUP BY zone"
}

// LargestRegionsSQL returns SQL for largest regions stats
func (qb *QueryBuilder) LargestRegionsSQL() string {
	return `
	WITH RegionCounts AS (
		SELECT zone, region, COUNT(*) as count,
			   MAX(CASE WHEN node_type = 'Region' THEN system_name ELSE NULL END) as region_name
		FROM nodes 
		WHERE nodelist_date = ? AND region > 0
		GROUP BY zone, region
	)
	SELECT zone, region, count, region_name
	FROM RegionCounts
	ORDER BY count DESC 
	LIMIT 10`
}

// LargestNetsSQL returns SQL for largest nets stats
func (qb *QueryBuilder) LargestNetsSQL() string {
	return `
	WITH NetCounts AS (
		SELECT zone, net, COUNT(*) as count
		FROM nodes 
		WHERE nodelist_date = ? AND node_type IN ('Node', 'Hub', 'Pvt', 'Hold', 'Down')
		GROUP BY zone, net
	),
	HostNames AS (
		SELECT zone, net, system_name as host_name
		FROM nodes
		WHERE nodelist_date = ? AND node_type = 'Host'
	)
	SELECT nc.zone, nc.net, nc.count, hn.host_name
	FROM NetCounts nc
	LEFT JOIN HostNames hn ON nc.zone = hn.zone AND nc.net = hn.net
	ORDER BY nc.count DESC 
	LIMIT 10`
}

// NodeHistorySQL returns SQL for retrieving node history
func (qb *QueryBuilder) NodeHistorySQL() string {
	return `
	SELECT zone, net, node, nodelist_date, day_number,
		   system_name, location, sysop_name, phone, node_type, region, max_speed,
		   is_cm, is_mo,
		   flags, modem_flags,
		   conflict_sequence, has_conflict, has_inet, internet_config, fts_id
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

// SysopSearchSQL returns SQL for searching nodes by sysop name
func (qb *QueryBuilder) SysopSearchSQL() string {
	return `
	WITH node_ranges AS (
		SELECT 
			zone, net, node,
			MIN(nodelist_date) as first_date,
			MAX(nodelist_date) as last_date,
			FIRST(system_name ORDER BY nodelist_date DESC) as system_name,
			FIRST(location ORDER BY nodelist_date DESC) as location,
			FIRST(sysop_name ORDER BY nodelist_date DESC) as sysop_name
		FROM nodes
		WHERE REPLACE(sysop_name, '_', ' ') ILIKE '%' || REPLACE(?, '_', ' ') || '%'
		GROUP BY zone, net, node
	)
	SELECT 
		nr.zone, nr.net, nr.node, nr.system_name, nr.location, nr.sysop_name,
		nr.first_date, nr.last_date,
		CASE WHEN nr.last_date = (SELECT MAX(nodelist_date) FROM nodes)
		THEN true ELSE false END as currently_active
	FROM node_ranges nr
	ORDER BY nr.first_date DESC
	LIMIT ?`
}

// NodeSummarySearchSQL returns SQL for searching nodes with lifetime information
func (qb *QueryBuilder) NodeSummarySearchSQL() string {
	return `
	WITH node_ranges AS (
		SELECT 
			zone, net, node,
			MIN(nodelist_date) as first_date,
			MAX(nodelist_date) as last_date,
			FIRST(system_name ORDER BY nodelist_date DESC) as system_name,
			FIRST(location ORDER BY nodelist_date DESC) as location,
			FIRST(sysop_name ORDER BY nodelist_date DESC) as sysop_name
		FROM nodes
		WHERE 1=1
			AND (? IS NULL OR zone = ?)
			AND (? IS NULL OR net = ?)
			AND (? IS NULL OR node = ?)
			AND (? IS NULL OR system_name ILIKE ?)
			AND (? IS NULL OR location ILIKE ?)
			AND (? IS NULL OR sysop_name ILIKE ?)
		GROUP BY zone, net, node
	)
	SELECT 
		nr.zone, nr.net, nr.node, nr.system_name, nr.location, nr.sysop_name,
		nr.first_date, nr.last_date,
		CASE WHEN nr.last_date = (SELECT MAX(nodelist_date) FROM nodes)
		THEN true ELSE false END as currently_active
	FROM node_ranges nr
	ORDER BY nr.last_date DESC, nr.zone, nr.net, nr.node
	LIMIT ?`
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

// IsProcessedSQL returns SQL for checking if a nodelist date is already processed
func (qb *QueryBuilder) IsProcessedSQL() string {
	return "SELECT COUNT(*) FROM nodes WHERE nodelist_date = ? LIMIT 1"
}

// LatestDateSQL returns SQL for getting the latest nodelist date
func (qb *QueryBuilder) LatestDateSQL() string {
	return "SELECT MAX(nodelist_date) FROM nodes"
}

// AvailableDatesSQL returns SQL for getting all available nodelist dates
func (qb *QueryBuilder) AvailableDatesSQL() string {
	return "SELECT DISTINCT nodelist_date FROM nodes ORDER BY nodelist_date DESC"
}

// NearestDateBeforeSQL returns SQL for finding the closest date before a given date
func (qb *QueryBuilder) NearestDateBeforeSQL() string {
	return `SELECT MAX(nodelist_date) 
		FROM nodes 
		WHERE nodelist_date < ?`
}

// NearestDateAfterSQL returns SQL for finding the closest date after a given date
func (qb *QueryBuilder) NearestDateAfterSQL() string {
	return `SELECT MIN(nodelist_date) 
		FROM nodes 
		WHERE nodelist_date > ?`
}

// ExactDateExistsSQL returns SQL for checking if an exact date exists
func (qb *QueryBuilder) ExactDateExistsSQL() string {
	return "SELECT COUNT(*) FROM nodes WHERE nodelist_date = ?"
}

// ConsecutiveNodelistCheckSQL returns SQL for checking gaps between dates
func (qb *QueryBuilder) ConsecutiveNodelistCheckSQL() string {
	return "SELECT COUNT(DISTINCT nodelist_date) FROM nodes WHERE nodelist_date > ? AND nodelist_date < ?"
}

// NextNodelistDateSQL returns SQL for finding the next nodelist date after a given date
func (qb *QueryBuilder) NextNodelistDateSQL() string {
	return "SELECT MIN(nodelist_date) FROM nodes WHERE nodelist_date > ?"
}

// UniqueSysopsWithFilterSQL returns SQL for getting unique sysops with name filter
func (qb *QueryBuilder) UniqueSysopsWithFilterSQL() string {
	return `
		WITH sysop_stats AS (
			SELECT 
				sysop_name,
				COUNT(DISTINCT (zone || ':' || net || '/' || node)) as node_count,
				COUNT(DISTINCT CASE WHEN is_active = true THEN (zone || ':' || net || '/' || node) END) as active_nodes,
				MIN(nodelist_date) as first_seen,
				MAX(nodelist_date) as last_seen,
				ARRAY_AGG(DISTINCT zone ORDER BY zone) as zones
			FROM nodes
			WHERE sysop_name ILIKE ?
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

// UniqueSysopsSQL returns SQL for getting all unique sysops
func (qb *QueryBuilder) UniqueSysopsSQL() string {
	return `
		WITH sysop_stats AS (
			SELECT 
				sysop_name,
				COUNT(DISTINCT (zone || ':' || net || '/' || node)) as node_count,
				COUNT(DISTINCT CASE WHEN is_active = true THEN (zone || ':' || net || '/' || node) END) as active_nodes,
				MIN(nodelist_date) as first_seen,
				MAX(nodelist_date) as last_seen,
				ARRAY_AGG(DISTINCT zone ORDER BY zone) as zones
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

// BuildFTSQuery constructs a Full-Text Search query with fallback to ILIKE
func (qb *QueryBuilder) BuildFTSQuery(filter database.NodeFilter) (string, []interface{}, bool) {
	var conditions []string
	var args []interface{}
	var hasFTSConditions bool

	baseSQL := `
	SELECT n.zone, n.net, n.node, n.nodelist_date, n.day_number,
		   n.system_name, n.location, n.sysop_name, n.phone, n.node_type, n.region, n.max_speed,
		   n.is_cm, n.is_mo,
		   n.flags, n.modem_flags,
		   n.conflict_sequence, n.has_conflict, n.has_inet, n.internet_config, n.fts_id,
		   COALESCE(fts.score, 0) as fts_score
	FROM nodes n`

	// Build FTS join for text search fields
	var ftsJoin string
	var ftsFields []string

	if filter.Location != nil && len(strings.TrimSpace(*filter.Location)) >= 2 {
		ftsFields = append(ftsFields, "location")
		hasFTSConditions = true
	}
	if filter.SysopName != nil && len(strings.TrimSpace(*filter.SysopName)) >= 2 {
		ftsFields = append(ftsFields, "sysop_name")
		hasFTSConditions = true
	}
	if filter.SystemName != nil && len(strings.TrimSpace(*filter.SystemName)) >= 2 {
		ftsFields = append(ftsFields, "system_name")
		hasFTSConditions = true
	}

	if hasFTSConditions {
		// Build combined search term
		var searchTerms []string
		if filter.Location != nil {
			searchTerms = append(searchTerms, strings.TrimSpace(*filter.Location))
		}
		if filter.SysopName != nil {
			searchTerms = append(searchTerms, strings.TrimSpace(*filter.SysopName))
		}
		if filter.SystemName != nil {
			searchTerms = append(searchTerms, strings.TrimSpace(*filter.SystemName))
		}

		searchTerm := strings.Join(searchTerms, " ")

		ftsJoin = fmt.Sprintf(`
		LEFT JOIN (
			SELECT fts_id, 
				   fts_main_nodes.match_bm25('fts_id', ?, fields := '%s') AS score
			FROM nodes
		) fts ON n.fts_id = fts.fts_id AND fts.score IS NOT NULL`,
			strings.Join(ftsFields, ","))

		baseSQL += ftsJoin
		args = append(args, searchTerm)
	}

	// Add other non-FTS conditions
	nonFTSConditions, nonFTSArgs := qb.buildNonTextConditions(filter)
	conditions = append(conditions, nonFTSConditions...)
	args = append(args, nonFTSArgs...)

	// When LatestOnly is false, we still want to show only the latest entry for each address
	// but search through all historical data
	if filter.LatestOnly == nil || !*filter.LatestOnly {
		// Apply grouping to show only latest entry per address
		if hasFTSConditions {
			// For FTS queries, fallback to LIKE-based query with grouping
			query, args := qb.BuildNodesQuery(filter)
			return query, args, false
		}
	}

	if len(conditions) > 0 {
		baseSQL += " WHERE " + strings.Join(conditions, " AND ")
	}

	// Order by FTS score if we have FTS conditions
	if hasFTSConditions {
		baseSQL += " ORDER BY fts_score DESC, n.nodelist_date DESC"
	} else {
		baseSQL += " ORDER BY n.nodelist_date DESC"
	}

	// Add limit
	if filter.Limit > 0 {
		baseSQL += fmt.Sprintf(" LIMIT %d", filter.Limit)
	}

	return baseSQL, args, hasFTSConditions
}

// buildNonTextConditions builds conditions for non-text search fields
func (qb *QueryBuilder) buildNonTextConditions(filter database.NodeFilter) ([]string, []interface{}) {
	var conditions []string
	var args []interface{}

	if filter.Zone != nil {
		conditions = append(conditions, "n.zone = ?")
		args = append(args, *filter.Zone)
	}
	if filter.Net != nil {
		conditions = append(conditions, "n.net = ?")
		args = append(args, *filter.Net)
	}
	if filter.Node != nil {
		conditions = append(conditions, "n.node = ?")
		args = append(args, *filter.Node)
	}
	if filter.DateFrom != nil {
		conditions = append(conditions, "n.nodelist_date >= ?")
		args = append(args, *filter.DateFrom)
	}
	if filter.DateTo != nil {
		conditions = append(conditions, "n.nodelist_date <= ?")
		args = append(args, *filter.DateTo)
	}
	if filter.NodeType != nil {
		conditions = append(conditions, "n.node_type = ?")
		args = append(args, *filter.NodeType)
	}
	if filter.IsCM != nil {
		conditions = append(conditions, "n.is_cm = ?")
		args = append(args, *filter.IsCM)
	}
	if filter.HasBinkp != nil {
		// HasBinkp is now determined from JSON: check for IBN or BND protocols
		conditions = append(conditions, "(json_extract(n.internet_config, '$.protocols.IBN') IS NOT NULL OR json_extract(n.internet_config, '$.protocols.BND') IS NOT NULL) = ?")
		args = append(args, *filter.HasBinkp)
	}

	return conditions, args
}

// FlagFirstAppearanceSQL returns SQL for finding the first appearance of a flag
func (qb *QueryBuilder) FlagFirstAppearanceSQL() string {
	return `
		WITH first_appearances AS (
			SELECT 
				zone, net, node, nodelist_date, system_name, location, sysop_name,
				ROW_NUMBER() OVER (ORDER BY nodelist_date ASC, zone ASC, net ASC, node ASC) as rn
			FROM nodes
			WHERE ? = ANY(flags) OR ? = ANY(modem_flags) 
		   OR json_extract(internet_config, '$.protocols.' || ?) IS NOT NULL
		)
		SELECT zone, net, node, nodelist_date, system_name, location, sysop_name
		FROM first_appearances
		WHERE rn = 1
	`
}

// FlagUsageByYearSQL returns SQL for counting flag usage by year (DuckDB optimized, matches ClickHouse logic)
func (qb *QueryBuilder) FlagUsageByYearSQL() string {
	return `
		WITH node_years AS (
			SELECT 
				EXTRACT(YEAR FROM nodelist_date) as year,
				zone, net, node,
				MAX(CASE WHEN (? = ANY(flags) OR ? = ANY(modem_flags) 
				              OR json_extract(internet_config, '$.protocols.' || ?) IS NOT NULL)
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
				THEN ROUND((SUM(has_flag)::NUMERIC / COUNT(*)) * 100, 2)
				ELSE 0 
			END as percentage
		FROM node_years
		GROUP BY year
		ORDER BY year
	`
}

// NetworkNameSQL returns SQL for getting network name from coordinator node
func (qb *QueryBuilder) NetworkNameSQL() string {
	return `
		SELECT system_name 
		FROM nodes 
		WHERE zone = ? AND net = ? AND node = 0 
		ORDER BY nodelist_date DESC 
		LIMIT 1
	`
}

// NetworkHistorySQL returns SQL for getting network appearance periods
func (qb *QueryBuilder) NetworkHistorySQL() string {
	return `
		WITH network_dates AS (
			SELECT DISTINCT 
				nodelist_date,
				day_number,
				LAG(nodelist_date) OVER (ORDER BY nodelist_date) as prev_date
			FROM nodes
			WHERE zone = ? AND net = ?
			ORDER BY nodelist_date
		),
		appearance_groups AS (
			SELECT 
				nodelist_date,
				day_number,
				CASE 
					WHEN prev_date IS NULL OR nodelist_date - prev_date > INTERVAL '14 days' THEN 1
					ELSE 0
				END as new_group
			FROM network_dates
		),
		appearance_periods AS (
			SELECT 
				nodelist_date,
				day_number,
				SUM(new_group) OVER (ORDER BY nodelist_date) as group_id
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
