package storage

// AnalyticsQueryBuilder handles analytics-related SQL queries
type AnalyticsQueryBuilder struct {
	base *QueryBuilder
}

// FlagFirstAppearance returns SQL for finding the first appearance of a flag
func (aqb *AnalyticsQueryBuilder) FlagFirstAppearance() string {
	return aqb.base.FlagFirstAppearanceSQL()
}

// FlagUsageByYear returns SQL for counting flag usage by year
func (aqb *AnalyticsQueryBuilder) FlagUsageByYear() string {
	return aqb.base.FlagUsageByYearSQL()
}

// NetworkName returns SQL for getting network name from coordinator node
func (aqb *AnalyticsQueryBuilder) NetworkName() string {
	return aqb.base.NetworkNameSQL()
}

// NetworkHistory returns SQL for getting network appearance periods
func (aqb *AnalyticsQueryBuilder) NetworkHistory() string {
	return aqb.base.NetworkHistorySQL()
}

// LEGACY METHODS - Analytics-related SQL query methods (kept for backward compatibility)

// FlagFirstAppearanceSQL returns SQL for finding the first appearance of a flag in ClickHouse
// Deprecated: Use QueryBuilder.Analytics().FlagFirstAppearance() instead
// Optimized: Uses pre-aggregated flag_statistics table for instant lookups
func (qb *QueryBuilder) FlagFirstAppearanceSQL() string {
	return `
		SELECT
			first_zone as zone,
			first_net as net,
			first_node as node,
			first_nodelist_date as nodelist_date,
			first_system_name as system_name,
			first_location as location,
			first_sysop_name as sysop_name
		FROM flag_statistics
		WHERE flag = ?
		ORDER BY first_nodelist_date ASC
		LIMIT 1
	`
}

// FlagUsageByYearSQL returns SQL for counting flag usage by year in ClickHouse
// Deprecated: Use QueryBuilder.Analytics().FlagUsageByYear() instead
// Optimized: Uses pre-aggregated flag_statistics table for instant lookups
func (qb *QueryBuilder) FlagUsageByYearSQL() string {
	return `
		WITH
		-- Get total nodes per year (using pre-calculated column)
		total_nodes_per_year AS (
			SELECT
				year,
				any(total_nodes_in_year) as total_nodes
			FROM flag_statistics
			GROUP BY year
		),
		-- Get nodes with specific flag per year
		flagged_nodes_per_year AS (
			SELECT
				year,
				unique_nodes as node_count
			FROM flag_statistics
			WHERE flag = ?
		)
		SELECT
			t.year,
			toUInt32(t.total_nodes) as total_nodes,
			COALESCE(f.node_count, 0) as node_count,
			CASE
				WHEN t.total_nodes > 0
				THEN round((COALESCE(f.node_count, 0) / t.total_nodes) * 100, 2)
				ELSE 0
			END as percentage
		FROM total_nodes_per_year t
		LEFT JOIN flagged_nodes_per_year f ON t.year = f.year
		ORDER BY t.year
	`
}

// NetworkNameSQL returns SQL for getting network name from coordinator node in ClickHouse
// Deprecated: Use QueryBuilder.Analytics().NetworkName() instead
func (qb *QueryBuilder) NetworkNameSQL() string {
	return `
		SELECT system_name
		FROM nodes
		WHERE zone = ? AND net = ? AND node = 0
		ORDER BY nodelist_date DESC
		LIMIT 1
	`
}

// NetworkHistorySQL returns SQL for getting network appearance periods in ClickHouse
// Deprecated: Use QueryBuilder.Analytics().NetworkHistory() instead
func (qb *QueryBuilder) NetworkHistorySQL() string {
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
