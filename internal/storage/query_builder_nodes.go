package storage

import (
	"fmt"
	"strings"

	"github.com/nodelistdb/internal/database"
)

// NodeQueryBuilder handles node-related SQL queries with a cleaner API
type NodeQueryBuilder struct {
	base *QueryBuilder
}

// Select returns the base SELECT statement for nodes
func (nqb *NodeQueryBuilder) Select() string {
	return nqb.base.NodeSelectSQL()
}

// Insert returns a parameterized INSERT statement
func (nqb *NodeQueryBuilder) Insert() string {
	return nqb.base.InsertNodeSQL()
}

// BatchInsert creates a batch INSERT statement with proper parameterization
func (nqb *NodeQueryBuilder) BatchInsert(batchSize int) string {
	return nqb.base.BuildBatchInsertSQL(batchSize)
}

// BuildQuery builds the main nodes query with filters
func (nqb *NodeQueryBuilder) BuildQuery(filter database.NodeFilter) (string, []interface{}) {
	return nqb.base.BuildNodesQuery(filter)
}

// BuildFTSQuery builds a full-text search query
func (nqb *NodeQueryBuilder) BuildFTSQuery(filter database.NodeFilter) (string, []interface{}, bool) {
	return nqb.base.BuildFTSQuery(filter)
}

// History returns SQL for retrieving node history
func (nqb *NodeQueryBuilder) History() string {
	return nqb.base.NodeHistorySQL()
}

// DateRange returns SQL for getting first and last dates of a node
func (nqb *NodeQueryBuilder) DateRange() string {
	return nqb.base.NodeDateRangeSQL()
}

// CheckConflict returns SQL for checking if a node already exists for a date
func (nqb *NodeQueryBuilder) CheckConflict() string {
	return nqb.base.ConflictCheckSQL()
}

// MarkConflict returns SQL for marking original entry as conflicted
func (nqb *NodeQueryBuilder) MarkConflict() string {
	return nqb.base.MarkConflictSQL()
}

// SearchSysop returns SQL for sysop search with window functions
func (nqb *NodeQueryBuilder) SearchSysop() string {
	return nqb.base.SysopSearchSQL()
}

// SearchNodeSummary returns SQL for searching nodes with lifetime information
func (nqb *NodeQueryBuilder) SearchNodeSummary(activeOnly bool) string {
	return nqb.base.NodeSummarySearchSQL(activeOnly)
}

// UniqueSysops returns SQL for getting unique sysops with statistics
func (nqb *NodeQueryBuilder) UniqueSysops() string {
	return nqb.base.UniqueSysopsSQL()
}

// UniqueSysopsWithFilter returns SQL for getting unique sysops with filter
func (nqb *NodeQueryBuilder) UniqueSysopsWithFilter() string {
	return nqb.base.UniqueSysopsWithFilterSQL()
}

// InsertNodesInChunks performs optimized batch inserts.
// Deprecated: Use NodeOperations.InsertNodes instead, which uses
// the native ClickHouse batch API for better safety and performance.
func (nqb *NodeQueryBuilder) InsertNodesInChunks(db database.DatabaseInterface, nodes []database.Node) error {
	return nqb.base.InsertNodesInChunks(db, nodes)
}

// BuildDirectBatchInsertSQL creates a direct VALUES-based INSERT.
// Deprecated: This method uses string interpolation which has SQL injection risks.
// Use NodeOperations.InsertNodes instead, which uses parameterized batch inserts.
func (nqb *NodeQueryBuilder) BuildDirectBatchInsertSQL(nodes []database.Node, rp *ResultParser) string {
	return nqb.base.BuildDirectBatchInsertSQL(nodes, rp)
}

// Node-related SQL query methods (LEGACY - These methods are kept on QueryBuilder for backward compatibility)

// InsertNodesInChunks performs optimized batch inserts for ClickHouse with proper array formatting.
// Deprecated: This method uses string interpolation via BuildDirectBatchInsertSQL.
// Use NodeOperations.InsertNodes instead for safe parameterized inserts.
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
	defer func() { _ = tx.Rollback() }()

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

// BuildDirectBatchInsertSQL creates a direct VALUES-based INSERT for ClickHouse with proper array handling.
// Deprecated: This method uses string interpolation with escapeSQL() which has incomplete escaping.
// Use NodeOperations.InsertNodes instead, which uses the native batch API (PrepareBatch/Append).
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
		conflict_sequence, has_conflict, has_inet, internet_config, fts_id, raw_line, domain
	) VALUES `)

	for i, node := range nodes {
		if i > 0 {
			buf.WriteByte(',')
		}

		if node.Domain == "" {
			node.Domain = database.DefaultDomain
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
		if len(node.InternetConfig) > 0 {
			buf.WriteString(fmt.Sprintf("'%s',", qb.escapeSQL(string(node.InternetConfig))))
		} else {
			buf.WriteString("'{}',")
		}

		// FTS ID, raw line and domain
		buf.WriteString(fmt.Sprintf("'%s','%s','%s')",
			qb.escapeSQL(node.FtsId), qb.escapeSQL(node.RawLine), qb.escapeSQL(node.Domain)))
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
		conflict_sequence, has_conflict, has_inet, ` + internetConfigSelectSQL + `, fts_id, raw_line, domain
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
		conflict_sequence, has_conflict, has_inet, internet_config, fts_id, raw_line, domain
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
		?, ?,
		?, ?, ?, ?, ?, ?, ?)`
}

// BuildBatchInsertSQL creates a batch INSERT statement with proper parameterization
func (qb *QueryBuilder) BuildBatchInsertSQL(batchSize int) string {
	if batchSize <= 0 {
		return qb.InsertNodeSQL()
	}

	// Create placeholder for one row with direct array binding (no JSON casting)
	valuePlaceholder := "(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"

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
			conflict_sequence, has_conflict, has_inet, internet_config, fts_id, raw_line, domain
		) VALUES %s
		ON CONFLICT (zone, net, node, nodelist_date, conflict_sequence)
		DO NOTHING`, strings.Join(values, ","))
}

// nodeColumnsSQL is the select list shared by both BuildNodesQuery shapes.
const nodeColumnsSQL = `zone, net, node, nodelist_date, day_number,
			system_name, location, sysop_name, phone, node_type, region, max_speed,
			is_cm, is_mo,
			flags, modem_flags,
			conflict_sequence, has_conflict, has_inet, ` + internetConfigSelectSQL + `, fts_id, raw_line, domain`

// BuildNodesQuery builds the main nodes query with filters.
//
// Both shapes return each matching node's latest row, but they differ in what
// "matching" means, so they cannot share one query:
//
//   - LatestOnly: take every node's latest row, THEN filter. A node whose
//     latest row falls outside the filter drops out. Because the row set is
//     "all rows at the node's max date", this keeps every conflict_sequence
//     row of a duplicated nodelist entry — collapsing them with LIMIT 1 BY
//     would silently drop real records (measured: 52499 rows instead of 52503
//     for zone=2).
//   - Default: find nodes matching on ANY historical row, then return each
//     one's single latest row — so a search for a location a node has since
//     left still finds it, and reports where it is now.
func (qb *QueryBuilder) BuildNodesQuery(filter database.NodeFilter) (string, []interface{}) {
	identity, identityArgs, attrs, attrArgs := qb.buildFilterConditions(filter)
	var args []interface{}

	if filter.LatestOnly != nil && *filter.LatestOnly {
		// "Latest" is per (domain, zone, net, node): the same 3D address may exist
		// in several networks with different nodelist cadences.
		//
		// Identity predicates are pushed into the MAX subquery as well as kept on
		// the outer query: they are invariant per node, so restricting which nodes
		// get a max computed cannot change any surviving row, and it keeps the
		// subquery from aggregating the entire table. Attribute predicates must
		// NOT be pushed - they would move the max to the newest row satisfying
		// them rather than the node's true latest row.
		sub := "SELECT domain, zone, net, node, MAX(nodelist_date) as max_date\n\t\t\tFROM nodes"
		if len(identity) > 0 {
			sub += "\n\t\t\tWHERE " + strings.Join(identity, " AND ")
			args = append(args, identityArgs...)
		}
		sub += "\n\t\t\tGROUP BY domain, zone, net, node"

		sql := "\n\t\tSELECT " + nodeColumnsSQL + `
		FROM nodes
		WHERE (domain, zone, net, node, nodelist_date) IN (
			` + sub + `
		)`
		if all := append(append([]string{}, identity...), attrs...); len(all) > 0 {
			sql += " AND " + strings.Join(all, " AND ")
			args = append(args, identityArgs...)
			args = append(args, attrArgs...)
		}
		sql += " ORDER BY zone, net, node, nodelist_date DESC, domain, conflict_sequence"
		return sql + paginationSQL(filter), args
	}

	// Default shape. ORDER BY / LIMIT 1 BY / LIMIT share one SELECT so ClickHouse
	// can stop reading once it has enough distinct keys; see paginationSQL.
	where := append([]string{}, identity...)
	args = append(args, identityArgs...)

	if len(attrs) > 0 {
		// Attributes vary over a node's history, so they select node keys through
		// a subquery over all of it. Identity predicates are repeated inside to
		// prune that scan; they are already on the outer query, where they also
		// give the primary-key lookup that makes the early stop cheap.
		inner := append(append([]string{}, identity...), attrs...)
		where = append(where, "(domain, zone, net, node) IN (\n\t\t\tSELECT DISTINCT domain, zone, net, node\n\t\t\tFROM nodes WHERE "+
			strings.Join(inner, " AND ")+"\n\t\t)")
		args = append(args, identityArgs...)
		args = append(args, attrArgs...)
	}

	sql := "\n\t\tSELECT " + nodeColumnsSQL + "\n\t\tFROM nodes"
	if len(where) > 0 {
		sql += "\n\t\tWHERE " + strings.Join(where, " AND ")
	}
	sql += "\n\t\tORDER BY zone, net, node, nodelist_date DESC, conflict_sequence ASC, domain" +
		"\n\t\tLIMIT 1 BY domain, zone, net, node"
	return sql + paginationSQL(filter), args
}

// BuildFTSQuery routes text searches to BuildNodesQuery.
//
// It used to carry a second, parallel implementation for the case "text filter
// AND latest_only=true", and the two disagreed about what latest_only means:
// that one restricted results to each domain's newest nodelist ("who is listed
// right now"), while BuildNodesQuery returns each node's own most recent row
// ("one row per node"), which is what openapi.yaml documents. Which definition
// a caller got depended on whether their query happened to contain a text term:
// sysop_name=Ivanov&latest_only=true returned 3 rows, the same search without
// latest_only returned 128.
//
// The parallel path also silently dropped has_inet, and ordered without the
// domain tie-breaker, so a 3-D address present in two networks sorted
// arbitrarily. Deleting it rather than repairing it in place is what stops the
// two from drifting apart a third time - is_mo had already diverged the other
// way, implemented here and missing from the main path.
//
// The name is historical: ClickHouse has no FTS index here and text matching is
// ILIKE on both paths. The bool reports whether the returned query is usable,
// so a filter with no text term falls through to BuildNodesQuery in the caller.
func (qb *QueryBuilder) BuildFTSQuery(filter database.NodeFilter) (string, []interface{}, bool) {
	hasText := (filter.SystemName != nil && *filter.SystemName != "") ||
		(filter.Location != nil && *filter.Location != "") ||
		(filter.SysopName != nil && *filter.SysopName != "")
	if !hasText {
		return "", nil, false
	}

	query, args := qb.BuildNodesQuery(filter)
	return query, args, true
}

// buildFilterConditions splits a NodeFilter into identity and attribute
// predicates, mirroring BuildPointFilterConditions.
//
// The split is what lets BuildNodesQuery pick a query shape, so it is returned
// by the condition builder itself rather than kept as a parallel list of "key
// fields" that could drift out of sync with the conditions below.
//
// Identity predicates constrain (domain, zone, net, node) — the columns that
// ARE the node's identity and so are invariant across its whole history. A
// predicate over only those columns is either true for every row of a node or
// false for all of them, which is why it can be applied directly to a row
// instead of through a "did any historical row match" subquery.
//
// Everything else is an attribute: it describes one point in a node's history.
// nodelist_date belongs here, not with identity — restricting the ranking pool
// to a date window would return the newest row *inside* the window rather than
// the node's true latest row.
func (qb *QueryBuilder) buildFilterConditions(filter database.NodeFilter) (identity []string, identityArgs []interface{}, attrs []string, attrArgs []interface{}) {
	if filter.Domain != nil && *filter.Domain != "" {
		identity = append(identity, "domain = ?")
		identityArgs = append(identityArgs, *filter.Domain)
	}
	if filter.Zone != nil {
		identity = append(identity, "zone = ?")
		identityArgs = append(identityArgs, *filter.Zone)
	}
	if filter.Net != nil {
		identity = append(identity, "net = ?")
		identityArgs = append(identityArgs, *filter.Net)
	}
	if filter.Node != nil {
		identity = append(identity, "node = ?")
		identityArgs = append(identityArgs, *filter.Node)
	}

	if filter.DateFrom != nil {
		attrs = append(attrs, "nodelist_date >= ?")
		attrArgs = append(attrArgs, *filter.DateFrom)
	}
	if filter.DateTo != nil {
		attrs = append(attrs, "nodelist_date <= ?")
		attrArgs = append(attrArgs, *filter.DateTo)
	}
	if filter.SystemName != nil {
		// Use ILIKE for case-insensitive matching - performs as well as materialized columns
		attrs = append(attrs, "system_name ILIKE ?")
		attrArgs = append(attrArgs, "%"+*filter.SystemName+"%")
	}
	if filter.Location != nil {
		// Use ILIKE for case-insensitive matching - performs as well as materialized columns
		attrs = append(attrs, "location ILIKE ?")
		attrArgs = append(attrArgs, "%"+*filter.Location+"%")
	}
	if filter.SysopName != nil {
		// Use ILIKE for case-insensitive matching - performs as well as materialized columns
		attrs = append(attrs, "sysop_name ILIKE ?")
		attrArgs = append(attrArgs, "%"+*filter.SysopName+"%")
	}
	if filter.NodeType != nil {
		attrs = append(attrs, "node_type = ?")
		attrArgs = append(attrArgs, *filter.NodeType)
	}
	if filter.IsCM != nil {
		attrs = append(attrs, "is_cm = ?")
		attrArgs = append(attrArgs, *filter.IsCM)
	}
	if filter.IsMO != nil {
		attrs = append(attrs, "is_mo = ?")
		attrArgs = append(attrArgs, *filter.IsMO)
	}
	if filter.HasInet != nil {
		attrs = append(attrs, "has_inet = ?")
		attrArgs = append(attrArgs, *filter.HasInet)
	}
	if filter.HasBinkp != nil {
		// HasBinkp is now determined from JSON: check for IBN or BND protocols
		attrs = append(attrs, "(JSON_EXISTS(toString(internet_config), '$.protocols.IBN') OR JSON_EXISTS(toString(internet_config), '$.protocols.BND')) = ?")
		attrArgs = append(attrArgs, *filter.HasBinkp)
	}

	return identity, identityArgs, attrs, attrArgs
}

// paginationSQL renders the trailing LIMIT/OFFSET.
//
// Each branch of BuildNodesQuery appends its own complete trailing clause. A
// shared "append ORDER BY + LIMIT to whatever came before" footer used to do
// this, but it cannot express the shape the fast path needs: ClickHouse only
// pushes the outer limit into the read - stopping once it has enough distinct
// keys - when ORDER BY, LIMIT n BY and LIMIT all sit in the SAME SELECT. A
// generic footer can only append a second ORDER BY (a syntax error) or wrap the
// query in a derived table, and the wrapper silently forfeits the optimisation:
// measured on prod, zone=2 limit=5 costs 16.6s / 20.5M rows wrapped versus
// 0.33s / 122k rows combined.
func paginationSQL(filter database.NodeFilter) string {
	if filter.Limit <= 0 {
		return ""
	}
	if filter.Offset > 0 {
		return fmt.Sprintf(" LIMIT %d OFFSET %d", filter.Limit, filter.Offset)
	}
	return fmt.Sprintf(" LIMIT %d", filter.Limit)
}

// NodeHistorySQL returns SQL for retrieving node history.
// Binds: zone, net, node, domain, domain (empty domain matches all networks).
func (qb *QueryBuilder) NodeHistorySQL() string {
	return `
	SELECT zone, net, node, nodelist_date, day_number,
		   system_name, location, sysop_name, phone, node_type, region, max_speed,
		   is_cm, is_mo,
		   flags, modem_flags,
		   conflict_sequence, has_conflict, has_inet, ` + internetConfigSelectSQL + `, fts_id, raw_line, domain
	FROM nodes
	WHERE zone = ? AND net = ? AND node = ? AND ` + optionalDomainSQL + `
	ORDER BY nodelist_date ASC, conflict_sequence ASC`
}

// NodeDateRangeSQL returns SQL for getting first and last dates of a node.
// Binds: zone, net, node, domain, domain.
func (qb *QueryBuilder) NodeDateRangeSQL() string {
	return `
	SELECT MIN(nodelist_date) as first_date, MAX(nodelist_date) as last_date
	FROM nodes
	WHERE zone = ? AND net = ? AND node = ? AND ` + optionalDomainSQL
}

// NodeDomainsSQL returns SQL listing the domains a 3D address exists in.
func (qb *QueryBuilder) NodeDomainsSQL() string {
	return `SELECT DISTINCT domain FROM nodes WHERE zone = ? AND net = ? AND node = ? ORDER BY domain`
}

// ConflictCheckSQL returns SQL for checking if a node already exists for a date.
// The check is domain-scoped: the same 3D address in two networks on the same
// date is legal and must not be flagged as a conflict.
func (qb *QueryBuilder) ConflictCheckSQL() string {
	return `SELECT COUNT(*) FROM nodes
		 WHERE zone = ? AND net = ? AND node = ? AND nodelist_date = ? AND domain = ?
		 LIMIT 1`
}

// MarkConflictSQL returns SQL for marking original entry as conflicted
func (qb *QueryBuilder) MarkConflictSQL() string {
	return `UPDATE nodes SET has_conflict = true
		WHERE zone = ? AND net = ? AND node = ? AND nodelist_date = ? AND domain = ? AND conflict_sequence = 0`
}

// SysopSearchSQL returns SQL for sysop search with window functions
func (qb *QueryBuilder) SysopSearchSQL() string {
	// ClickHouse-optimized sysop search using window functions to avoid aggregation
	// issues. Node identity and "currently active" are evaluated per domain: each
	// network has its own latest nodelist date.
	return `
	WITH
	domain_max AS (
		SELECT domain, MAX(nodelist_date) as max_date FROM nodes GROUP BY domain
	),
	ranked_nodes AS (
		SELECT
			domain, zone, net, node, nodelist_date, system_name, location, sysop_name,
			row_number() OVER (PARTITION BY domain, zone, net, node ORDER BY nodelist_date DESC) as rn,
			MIN(nodelist_date) OVER (PARTITION BY domain, zone, net, node) as first_date,
			MAX(nodelist_date) OVER (PARTITION BY domain, zone, net, node) as last_date
		FROM nodes
		WHERE replaceAll(sysop_name, '_', ' ') ILIKE concat('%', replaceAll(?, '_', ' '), '%')
			AND (? = '' OR domain = ?)
	),
	latest_per_node AS (
		SELECT
			domain, zone, net, node, system_name, location, sysop_name,
			first_date, last_date, nodelist_date
		FROM ranked_nodes
		WHERE rn = 1
	)
	SELECT
		lpn.zone, lpn.net, lpn.node, lpn.system_name, lpn.location, lpn.sysop_name,
		lpn.first_date, lpn.last_date,
		CASE WHEN lpn.last_date = dm.max_date THEN true ELSE false END as currently_active,
		lpn.domain
	FROM latest_per_node lpn
	JOIN domain_max dm ON lpn.domain = dm.domain
	ORDER BY lpn.first_date DESC
	LIMIT ?`
}

// NodeSummarySearchSQL returns the web search's per-node summary query.
//
// activeOnly drops nodes whose last appearance predates their network's newest
// nodelist. The predicate reuses currently_active, the same comparison this
// query already returns as a per-row badge, so the filter and the badge shown
// next to each result cannot disagree.
//
// It is applied after latest_per_node has collapsed each node to one row: "is
// this node still listed" is a property of a node's final row, so filtering
// earlier would keep a departed node alive on the strength of an older row.
func (qb *QueryBuilder) NodeSummarySearchSQL(activeOnly bool) string {
	activeOnlyClause := ""
	if activeOnly {
		activeOnlyClause = "\n\tWHERE lpn.last_date = dm.max_date"
	}
	return `
	WITH
	domain_max AS (
		SELECT domain, MAX(nodelist_date) as max_date FROM nodes GROUP BY domain
	),
	ranked_nodes AS (
		SELECT
			domain, zone, net, node, nodelist_date, system_name, location, sysop_name,
			row_number() OVER (PARTITION BY domain, zone, net, node ORDER BY nodelist_date DESC) as rn,
			MIN(nodelist_date) OVER (PARTITION BY domain, zone, net, node) as first_date,
			MAX(nodelist_date) OVER (PARTITION BY domain, zone, net, node) as last_date
		FROM nodes
		WHERE 1=1
			AND (? IS NULL OR zone = ?)
			AND (? IS NULL OR net = ?)
			AND (? IS NULL OR node = ?)
			AND (? IS NULL OR system_name ILIKE ?)
			AND (? IS NULL OR location ILIKE ?)
			AND (? IS NULL OR replaceAll(sysop_name, '_', ' ') ILIKE replaceAll(?, '_', ' '))
			AND (? = '' OR domain = ?)
	),
	latest_per_node AS (
		SELECT
			domain, zone, net, node, system_name, location, sysop_name,
			first_date, last_date, nodelist_date
		FROM ranked_nodes
		WHERE rn = 1
	)
	SELECT
		lpn.zone, lpn.net, lpn.node, lpn.system_name, lpn.location, lpn.sysop_name,
		lpn.first_date, lpn.last_date,
		CASE WHEN lpn.last_date = dm.max_date THEN true ELSE false END as currently_active,
		lpn.domain
	FROM latest_per_node lpn
	JOIN domain_max dm ON lpn.domain = dm.domain` + activeOnlyClause + `
	ORDER BY lpn.last_date DESC, lpn.zone, lpn.net, lpn.node
	LIMIT ?`
}

// UniqueSysopsSQL returns SQL for getting unique sysops with statistics
func (qb *QueryBuilder) UniqueSysopsSQL() string {
	// ClickHouse-compatible unique sysops query
	return `
		WITH sysop_stats AS (
			SELECT
				sysop_name,
				COUNT(DISTINCT concat(domain, '#', toString(zone), ':', toString(net), '/', toString(node))) as node_count,
				COUNT(DISTINCT CASE WHEN (domain, nodelist_date) IN (SELECT domain, MAX(nodelist_date) FROM nodes GROUP BY domain) THEN concat(domain, '#', toString(zone), ':', toString(net), '/', toString(node)) END) as active_nodes,
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
				COUNT(DISTINCT concat(domain, '#', toString(zone), ':', toString(net), '/', toString(node))) as node_count,
				COUNT(DISTINCT CASE WHEN (domain, nodelist_date) IN (SELECT domain, MAX(nodelist_date) FROM nodes GROUP BY domain) THEN concat(domain, '#', toString(zone), ':', toString(net), '/', toString(node)) END) as active_nodes,
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
