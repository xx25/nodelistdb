package storage

// Analytics-related SQL query methods

// FlagFirstAppearanceSQL returns SQL for finding the first appearance of a flag in ClickHouse
// Optimized: First find MIN(date), then query that specific date only
func (qb *QueryBuilder) FlagFirstAppearanceSQL() string {
	return `
		WITH first_date AS (
			SELECT min(nodelist_date) as d
			FROM nodes
			WHERE has(flags, ?) OR has(modem_flags, ?)
				OR (has_inet = 1 AND positionCaseInsensitive(toString(internet_config), concat('"', ?, '"')) > 0)
		)
		SELECT
			zone, net, node, nodelist_date, system_name, location, sysop_name
		FROM nodes
		WHERE nodelist_date = (SELECT d FROM first_date)
			AND (has(flags, ?) OR has(modem_flags, ?)
				OR (has_inet = 1 AND positionCaseInsensitive(toString(internet_config), concat('"', ?, '"')) > 0))
		ORDER BY zone ASC, net ASC, node ASC
		LIMIT 1
	`
}

// FlagUsageByYearSQL returns SQL for counting flag usage by year in ClickHouse
// Optimized: Uses uniqExact for deduplication and positionCaseInsensitive instead of JSON_EXISTS
func (qb *QueryBuilder) FlagUsageByYearSQL() string {
	return `
		WITH
		-- First, get all unique nodes per year (total counts)
		total_nodes_per_year AS (
			SELECT
				toYear(nodelist_date) as year,
				uniqExact((zone, net, node)) as total_nodes
			FROM nodes
			GROUP BY year
		),
		-- Then, get unique nodes WITH the flag per year
		flagged_nodes_per_year AS (
			SELECT
				toYear(nodelist_date) as year,
				uniqExact((zone, net, node)) as node_count
			FROM nodes
			WHERE has(flags, ?) OR has(modem_flags, ?)
				OR (has_inet = 1 AND positionCaseInsensitive(toString(internet_config), concat('"', ?, '"')) > 0)
			GROUP BY year
		)
		SELECT
			t.year,
			t.total_nodes,
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
