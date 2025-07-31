package storage

import (
	"fmt"
	"strings"

	"nodelistdb/internal/database"
)

// QueryBuilder provides safe SQL query construction with parameter binding
type QueryBuilder struct{}

// NewQueryBuilder creates a new QueryBuilder instance
func NewQueryBuilder() *QueryBuilder {
	return &QueryBuilder{}
}

// InsertNodeSQL builds a parameterized INSERT statement for nodes
func (qb *QueryBuilder) InsertNodeSQL() string {
	return `
	INSERT INTO nodes (
		zone, net, node, nodelist_date, day_number,
		system_name, location, sysop_name, phone, node_type, region, max_speed,
		is_cm, is_mo, has_binkp, has_telnet, is_down, is_hold, is_pvt, is_active,
		flags, modem_flags, internet_protocols, internet_hostnames, internet_ports, internet_emails,
		conflict_sequence, has_conflict, has_inet, internet_config, fts_id
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 
		?, ?, ?, ?, ?, ?,
		?, ?, ?, ?, ?)`
}

// BuildBatchInsertSQL creates a batch INSERT statement with proper parameterization
func (qb *QueryBuilder) BuildBatchInsertSQL(batchSize int) string {
	if batchSize <= 0 {
		return qb.InsertNodeSQL()
	}

	// Create placeholder for one row with direct array binding (no JSON casting)
	valuePlaceholder := "(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"

	// Build batch values
	values := make([]string, batchSize)
	for i := 0; i < batchSize; i++ {
		values[i] = valuePlaceholder
	}

	return fmt.Sprintf(`
		INSERT INTO nodes (
			zone, net, node, nodelist_date, day_number,
			system_name, location, sysop_name, phone, node_type, region, max_speed,
			is_cm, is_mo, has_binkp, has_telnet, is_down, is_hold, is_pvt, is_active,
			flags, modem_flags, internet_protocols, internet_hostnames, internet_ports, internet_emails,
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
		buf.WriteString(fmt.Sprintf("%t,%t,%t,%t,%t,%t,%t,%t,",
			node.IsCM, node.IsMO, node.HasBinkp, node.HasTelnet,
			node.IsDown, node.IsHold, node.IsPvt, node.IsActive))

		// Arrays (optimized format)
		buf.WriteString(fmt.Sprintf("%s,%s,%s,%s,%s,%s,",
			rp.formatArrayForDB(node.Flags),
			rp.formatArrayForDB(node.ModemFlags),
			rp.formatArrayForDB(node.InternetProtocols),
			rp.formatArrayForDB(node.InternetHostnames),
			rp.formatIntArrayForDB(node.InternetPorts),
			rp.formatArrayForDB(node.InternetEmails)))

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

// NodeSelectSQL returns the base SELECT statement for nodes
func (qb *QueryBuilder) NodeSelectSQL() string {
	return `
	SELECT zone, net, node, nodelist_date, day_number,
		   system_name, location, sysop_name, phone, node_type, region, max_speed,
		   is_cm, is_mo, has_binkp, has_telnet, is_down, is_hold, is_pvt, is_active,
		   flags, modem_flags, internet_protocols, internet_hostnames, internet_ports, internet_emails,
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
			   is_cm, is_mo, has_binkp, has_telnet, is_down, is_hold, is_pvt, is_active,
			   flags, modem_flags, internet_protocols, internet_hostnames, internet_ports, internet_emails,
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
			   n.is_cm, n.is_mo, n.has_binkp, n.has_telnet, n.is_down, n.is_hold, n.is_pvt, n.is_active,
			   n.flags, n.modem_flags, n.internet_protocols, n.internet_hostnames, n.internet_ports, n.internet_emails,
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
	baseSQL += " ORDER BY zone, net, node, nodelist_date DESC"

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

// StatsSQL returns the SQL for network statistics
func (qb *QueryBuilder) StatsSQL() string {
	return `
	SELECT 
		nodelist_date,
		COUNT(*) as total_nodes,
		COUNT(*) FILTER (WHERE is_active AND NOT is_down AND NOT is_hold) as active_nodes,
		COUNT(*) FILTER (WHERE is_cm) as cm_nodes,
		COUNT(*) FILTER (WHERE is_mo) as mo_nodes,
		COUNT(*) FILTER (WHERE has_binkp) as binkp_nodes,
		COUNT(*) FILTER (WHERE has_telnet) as telnet_nodes,
		COUNT(*) FILTER (WHERE is_pvt) as pvt_nodes,
		COUNT(*) FILTER (WHERE is_down) as down_nodes,
		COUNT(*) FILTER (WHERE is_hold) as hold_nodes,
		COUNT(*) FILTER (WHERE array_length(internet_protocols) > 0) as internet_nodes
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
		   is_cm, is_mo, has_binkp, has_telnet, is_down, is_hold, is_pvt, is_active,
		   flags, modem_flags, internet_protocols, internet_hostnames, internet_ports, internet_emails,
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
		CASE WHEN EXISTS (
			SELECT 1 FROM nodes n 
			WHERE n.zone = nr.zone AND n.net = nr.net AND n.node = nr.node 
			AND n.nodelist_date = (SELECT MAX(nodelist_date) FROM nodes)
		) THEN true ELSE false END as currently_active
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
		CASE WHEN EXISTS (
			SELECT 1 FROM nodes n 
			WHERE n.zone = nr.zone AND n.net = nr.net AND n.node = nr.node 
			AND n.nodelist_date = (SELECT MAX(nodelist_date) FROM nodes)
		) THEN true ELSE false END as currently_active
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
		   n.is_cm, n.is_mo, n.has_binkp, n.has_telnet, n.is_down, n.is_hold, n.is_pvt, n.is_active,
		   n.flags, n.modem_flags, n.internet_protocols, n.internet_hostnames, n.internet_ports, n.internet_emails,
		   n.conflict_sequence, n.has_conflict, n.has_inet, n.internet_config,
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
	if filter.IsActive != nil {
		conditions = append(conditions, "n.is_active = ?")
		args = append(args, *filter.IsActive)
	}
	if filter.IsCM != nil {
		conditions = append(conditions, "n.is_cm = ?")
		args = append(args, *filter.IsCM)
	}
	if filter.HasBinkp != nil {
		conditions = append(conditions, "n.has_binkp = ?")
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
			WHERE ? = ANY(flags) OR ? = ANY(internet_protocols) OR ? = ANY(modem_flags)
		)
		SELECT zone, net, node, nodelist_date, system_name, location, sysop_name
		FROM first_appearances
		WHERE rn = 1
	`
}

// FlagUsageByYearSQL returns SQL for counting flag usage by year
func (qb *QueryBuilder) FlagUsageByYearSQL() string {
	return `
		WITH yearly_stats AS (
			SELECT 
				EXTRACT(YEAR FROM nodelist_date) as year,
				COUNT(DISTINCT (zone || ':' || net || '/' || node)) as total_nodes
			FROM nodes
			GROUP BY year
		),
		flag_stats AS (
			SELECT 
				EXTRACT(YEAR FROM nodelist_date) as year,
				COUNT(DISTINCT (zone || ':' || net || '/' || node)) as nodes_with_flag
			FROM nodes
			WHERE ? = ANY(flags) OR ? = ANY(internet_protocols) OR ? = ANY(modem_flags)
			GROUP BY year
		)
		SELECT 
			ys.year,
			COALESCE(fs.nodes_with_flag, 0) as node_count,
			ys.total_nodes,
			CASE 
				WHEN ys.total_nodes > 0 
				THEN ROUND((COALESCE(fs.nodes_with_flag, 0)::NUMERIC / ys.total_nodes) * 100, 2)
				ELSE 0 
			END as percentage
		FROM yearly_stats ys
		LEFT JOIN flag_stats fs ON ys.year = fs.year
		ORDER BY ys.year
	`
}
