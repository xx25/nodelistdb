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
		conflict_sequence, has_conflict, has_inet, internet_config
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 
		json(?)::VARCHAR[], json(?)::VARCHAR[], json(?)::VARCHAR[], json(?)::VARCHAR[], json(?)::INTEGER[], json(?)::VARCHAR[],
		?, ?, ?, ?)`
}

// BuildBatchInsertSQL creates a batch INSERT statement with proper parameterization
func (qb *QueryBuilder) BuildBatchInsertSQL(batchSize int) string {
	if batchSize <= 0 {
		return qb.InsertNodeSQL()
	}

	// Create placeholder for one row with JSON casting for array fields
	valuePlaceholder := "(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, json(?)::VARCHAR[], json(?)::VARCHAR[], json(?)::VARCHAR[], json(?)::VARCHAR[], json(?)::INTEGER[], json(?)::VARCHAR[], ?, ?, ?, ?)"

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
			conflict_sequence, has_conflict, has_inet, internet_config
		) VALUES %s
		ON CONFLICT (zone, net, node, nodelist_date, conflict_sequence) 
		DO NOTHING`, strings.Join(values, ","))
}

// NodeSelectSQL returns the base SELECT statement for nodes
func (qb *QueryBuilder) NodeSelectSQL() string {
	return `
	SELECT zone, net, node, nodelist_date, day_number,
		   system_name, location, sysop_name, phone, node_type, region, max_speed,
		   is_cm, is_mo, has_binkp, has_telnet, is_down, is_hold, is_pvt, is_active,
		   flags, modem_flags, internet_protocols, internet_hostnames, internet_ports, internet_emails,
		   conflict_sequence, has_conflict, has_inet, internet_config
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
		baseSQL = qb.NodeSelectSQL()

		conditions, conditionArgs := qb.buildWhereConditions(filter)
		if len(conditions) > 0 {
			baseSQL += " WHERE " + strings.Join(conditions, " AND ")
			args = append(args, conditionArgs...)
		}
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
		   conflict_sequence, has_conflict, has_inet, internet_config
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
