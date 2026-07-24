package storage

// StatsQueryBuilder handles statistics-related SQL queries
type StatsQueryBuilder struct {
	base *QueryBuilder
}

// NetworkStats returns SQL for network statistics
func (sqb *StatsQueryBuilder) NetworkStats() string {
	return sqb.base.StatsSQL()
}

// ZoneDistribution returns SQL for zone distribution stats
func (sqb *StatsQueryBuilder) ZoneDistribution() string {
	return sqb.base.ZoneDistributionSQL()
}

// LargestRegions returns SQL for largest regions stats
func (sqb *StatsQueryBuilder) LargestRegions() string {
	return sqb.base.LargestRegionsSQL()
}

// OptimizedLargestRegions returns optimized SQL for largest regions stats
func (sqb *StatsQueryBuilder) OptimizedLargestRegions() string {
	return sqb.base.OptimizedLargestRegionsSQL()
}

// LargestNets returns SQL for largest nets stats
func (sqb *StatsQueryBuilder) LargestNets() string {
	return sqb.base.LargestNetsSQL()
}

// OptimizedLargestNets returns optimized SQL for largest nets stats
func (sqb *StatsQueryBuilder) OptimizedLargestNets() string {
	return sqb.base.OptimizedLargestNetsSQL()
}

// BrowseZones returns SQL for the hierarchy browser zone listing
func (sqb *StatsQueryBuilder) BrowseZones() string {
	return sqb.base.BrowseZonesSQL()
}

// BrowseRegions returns SQL for the hierarchy browser region listing
func (sqb *StatsQueryBuilder) BrowseRegions() string {
	return sqb.base.BrowseRegionsSQL()
}

// BrowseNets returns SQL for the hierarchy browser net listing
func (sqb *StatsQueryBuilder) BrowseNets() string {
	return sqb.base.BrowseNetsSQL()
}

// BrowseNodes returns SQL for the hierarchy browser node listing
func (sqb *StatsQueryBuilder) BrowseNodes() string {
	return sqb.base.BrowseNodesSQL()
}

// LEGACY METHODS - Statistics-related SQL query methods (kept for backward compatibility)

// StatsSQL returns SQL for network statistics
// Deprecated: Use QueryBuilder.Stats().NetworkStats() instead
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
	WHERE nodelist_date = ? AND (? = '' OR domain = ?)
	GROUP BY nodelist_date`
}

// ZoneDistributionSQL returns SQL for zone distribution stats
func (qb *QueryBuilder) ZoneDistributionSQL() string {
	return "SELECT zone, COUNT(*) FROM nodes WHERE nodelist_date = ? AND (? = '' OR domain = ?) GROUP BY zone"
}

// LargestRegionsSQL returns SQL for largest regions stats
func (qb *QueryBuilder) LargestRegionsSQL() string {
	return `
	WITH RegionCounts AS (
		SELECT zone, region, COUNT(*) as count,
			   MAX(CASE WHEN node_type = 'Region' THEN system_name ELSE NULL END) as region_name
		FROM nodes
		WHERE nodelist_date = ? AND (? = '' OR domain = ?) AND region > 0
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
	WHERE nodelist_date = ? AND (? = '' OR domain = ?) AND region > 0
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
		WHERE nodelist_date = ? AND (? = '' OR domain = ?) AND node_type IN ('Node', 'Hub', 'Pvt', 'Hold', 'Down')
		GROUP BY zone, net
	),
	HostNames AS (
		SELECT zone, net, system_name as host_name
		FROM nodes
		WHERE nodelist_date = ? AND (? = '' OR domain = ?) AND node_type = 'Host'
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
		WHERE nodelist_date = ? AND (? = '' OR domain = ?) AND node_type IN ('Node', 'Hub', 'Pvt', 'Hold', 'Down')
		GROUP BY zone, net
		ORDER BY count DESC
		LIMIT 10
	) nc
	LEFT JOIN (
		SELECT zone, net, system_name as host_name
		FROM nodes
		WHERE nodelist_date = ? AND (? = '' OR domain = ?) AND node_type = 'Host'
	) hn ON nc.zone = hn.zone AND nc.net = hn.net
	ORDER BY nc.count DESC`
}

// BrowseZonesSQL returns SQL listing every zone present in a nodelist with its
// node count and zone-coordinator name. Used by the hierarchy browser.
func (qb *QueryBuilder) BrowseZonesSQL() string {
	return `
	SELECT
		zone,
		COUNT(*) as node_count,
		anyIf(system_name, node_type = 'Zone') as zone_name
	FROM nodes
	WHERE nodelist_date = ? AND (? = '' OR domain = ?)
	GROUP BY zone
	ORDER BY zone`
}

// BrowseRegionsSQL returns SQL listing every region within a zone with its node
// count and region-coordinator name/location. Nodes with no region are grouped
// under region 0. Used by the hierarchy browser.
func (qb *QueryBuilder) BrowseRegionsSQL() string {
	// The select alias must not be named "region": ClickHouse's analyzer would
	// resolve the column reference inside GROUP BY/ORDER BY to the alias instead
	// of the real nodes.region column, raising NOT_AN_AGGREGATE.
	return `
	SELECT
		ifNull(region, 0) as region_num,
		COUNT(*) as node_count,
		anyIf(system_name, node_type = 'Region') as region_name,
		anyIf(location, node_type = 'Region') as region_location
	FROM nodes
	WHERE nodelist_date = ? AND zone = ? AND (? = '' OR domain = ?)
	GROUP BY region
	ORDER BY ifNull(region, 0)`
}

// BrowseNetsSQL returns SQL listing every net within a zone+region with its node
// count and host-coordinator name/location. Region 0 selects nets that have no
// region assigned. Used by the hierarchy browser.
func (qb *QueryBuilder) BrowseNetsSQL() string {
	return `
	SELECT
		net,
		COUNT(*) as node_count,
		anyIf(system_name, node_type = 'Host') as host_name,
		anyIf(location, node_type = 'Host') as host_location
	FROM nodes
	WHERE nodelist_date = ? AND zone = ? AND ifNull(region, 0) = ? AND (? = '' OR domain = ?)
	GROUP BY net
	ORDER BY net`
}

// BrowseNodesSQL returns SQL listing every entry within a zone+net for a single
// nodelist date. Column order matches ResultParser.ParseNodeRow. Used by the
// hierarchy browser.
func (qb *QueryBuilder) BrowseNodesSQL() string {
	return `
	SELECT
		zone, net, node, nodelist_date, day_number,
		system_name, location, sysop_name, phone, node_type, region, max_speed,
		is_cm, is_mo,
		flags, modem_flags,
		conflict_sequence, has_conflict, has_inet, ` + internetConfigSelectSQL + `, fts_id, raw_line, domain
	FROM nodes
	WHERE nodelist_date = ? AND zone = ? AND net = ? AND (? = '' OR domain = ?)
	ORDER BY node, conflict_sequence`
}
