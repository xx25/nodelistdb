package storage

// Statistics-related SQL query methods

// StatsSQL returns SQL for network statistics
func (qb *QueryBuilder) StatsSQL() string {
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

// OptimizedLargestRegionsSQL returns optimized SQL for largest regions stats
func (qb *QueryBuilder) OptimizedLargestRegionsSQL() string {
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
