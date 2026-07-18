package storage

import (
	"fmt"
	"strings"

	"github.com/nodelistdb/internal/database"
)

// Point-related SQL. The insert column list is defined ONCE here; every
// insert path (native batch and SQL fallback) must use these constants so
// the column order can never drift between paths.
//
// NOTE: the dormant native storage path (clickhouse_storage.go) does not
// implement points; PointOperations below is the only writer.

// pointsColumnsSQL is the canonical points column list, in insert order.
const pointsColumnsSQL = `domain, zone, net, node, point, pointlist_date, day_number,
	list_source, source_priority, source_format,
	system_name, location, sysop_name, phone, max_speed,
	is_cm, is_mo, has_inet,
	flags, modem_flags, internet_config,
	conflict_sequence, has_conflict, fts_id, raw_line`

// InsertPointsBatchSQL returns the column-qualified INSERT used with the
// native PrepareBatch API (values appended per row, no placeholders).
func (qb *QueryBuilder) InsertPointsBatchSQL() string {
	return `INSERT INTO points (` + pointsColumnsSQL + `)`
}

// InsertPointSQL returns a parameterized single-row INSERT (SQL fallback path).
func (qb *QueryBuilder) InsertPointSQL() string {
	return `INSERT INTO points (` + pointsColumnsSQL + `) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
}

// PointSelectSQL returns the base SELECT for points (column order matches
// ParsePointRow).
func (qb *QueryBuilder) PointSelectSQL() string {
	return `SELECT ` + pointsColumnsSQL + ` FROM points`
}

// IsPointlistImportedSQL checks the import gate.
// Binds: domain, list_source, pointlist_date.
func (qb *QueryBuilder) IsPointlistImportedSQL() string {
	return `SELECT COUNT(*) FROM pointlist_files WHERE domain = ? AND list_source = ? AND pointlist_date = ?`
}

// RegisterPointlistFileSQL registers one imported file in the gate table.
// Binds: domain, list_source, pointlist_date, day_number, filename,
// source_format, points_count, bosses_count.
func (qb *QueryBuilder) RegisterPointlistFileSQL() string {
	return `INSERT INTO pointlist_files
		(domain, list_source, pointlist_date, day_number, filename, source_format, points_count, bosses_count)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
}

// DeletePointsForFileSQL removes all point rows of one (domain, source, date)
// file — used to clean partial remnants before retry and for -reimport.
// Binds: domain, list_source, pointlist_date.
func (qb *QueryBuilder) DeletePointsForFileSQL() string {
	return `DELETE FROM points WHERE domain = ? AND list_source = ? AND pointlist_date = ?`
}

// DeletePointlistFileGateSQL removes the gate row of one file (-reimport).
// Binds: domain, list_source, pointlist_date.
func (qb *QueryBuilder) DeletePointlistFileGateSQL() string {
	return `DELETE FROM pointlist_files WHERE domain = ? AND list_source = ? AND pointlist_date = ?`
}

// NearestPointlistCountSQL returns points_count and date of the already-
// imported issue of the same series nearest in time to the given date
// (sanity-threshold baseline). Nearest-by-date, not "previous": bulk import
// cannot walk compressed weeklies chronologically (the .Z## suffix is
// day-mod-100), so the previous issue may not be imported yet.
// Binds: domain, list_source, pointlist_date, pointlist_date.
func (qb *QueryBuilder) NearestPointlistCountSQL() string {
	return `SELECT points_count, pointlist_date FROM pointlist_files FINAL
		WHERE domain = ? AND list_source = ? AND pointlist_date != ?
		ORDER BY abs(dateDiff('day', pointlist_date, toDate(?))) ASC LIMIT 1`
}

// CountPointsForFileSQL counts stored point rows of one file (verification).
// Binds: domain, list_source, pointlist_date.
func (qb *QueryBuilder) CountPointsForFileSQL() string {
	return `SELECT COUNT(*) FROM points WHERE domain = ? AND list_source = ? AND pointlist_date = ?`
}

// PointlistSourcesSQL summarizes imported pointlist series per domain.
// Binds: domain, domain (empty = all domains).
func (qb *QueryBuilder) PointlistSourcesSQL() string {
	return `SELECT domain, list_source,
			MIN(pointlist_date) AS first_date,
			MAX(pointlist_date) AS last_date,
			COUNT(*) AS file_count,
			SUM(points_count) AS total_points
		FROM pointlist_files FINAL
		WHERE ` + optionalDomainSQL + `
		GROUP BY domain, list_source
		ORDER BY domain, list_source`
}

// --- Phase 2: snapshot readers ---

// PointSnapshotStalenessDays bounds how far back a source's latest issue may
// lie behind the as-of date and still contribute to a snapshot. Prevents dead
// series (r22 ended 2014) from resurrecting points into current views.
const PointSnapshotStalenessDays = 60

// pointSnapshotInnerSQL returns the snapshot core: points as of a date with
// cross-source overlap resolved. Per (domain, list_source) the latest imported
// issue at or before as-of (within the staleness window) is selected from the
// gate table, then overlapping rows for the same 4-D address collapse to the
// most authoritative source (source_priority ASC; net-level < regional < zone
// rollup), newest issue first on ties.
//
// innerWhere may add IDENTITY predicates only (zone/net/node/point,
// list_source — columns the LIMIT BY groups on, or the source axis itself):
// they narrow which snapshot groups exist without changing any group's
// winner. Attribute predicates (system_name, sysop_name, dates, ...) must go
// through the outer WHERE of PointSnapshotSQL — applied before the collapse
// they could select a non-authoritative row whose attribute happens to match.
// Binds: domain, domain, [subquery] domain, domain, asOf, asOf, [innerWhere binds].
func (qb *QueryBuilder) pointSnapshotInnerSQL(innerWhere string) string {
	q := `SELECT ` + pointsColumnsSQL + `
	FROM points
	WHERE ` + optionalDomainSQL + `
	  AND (domain, list_source, pointlist_date) IN (
		SELECT domain, list_source, max(pointlist_date)
		FROM pointlist_files FINAL
		WHERE ` + optionalDomainSQL + `
		  AND pointlist_date <= toDate(?)
		  AND pointlist_date > toDate(?) - INTERVAL ` + fmt.Sprintf("%d", PointSnapshotStalenessDays) + ` DAY
		GROUP BY domain, list_source
	  )`
	if innerWhere != "" {
		q += "\n	  AND " + innerWhere
	}
	q += `
	ORDER BY source_priority ASC, pointlist_date DESC, conflict_sequence ASC
	LIMIT 1 BY domain, zone, net, node, point`
	return q
}

// PointSnapshotSQL wraps the snapshot core in a stable address ordering.
// outerWhere filters the COLLAPSED rows (attribute predicates; may be empty).
// Binds: see pointSnapshotInnerSQL, then [outerWhere binds], then
// [limit, offset when withPaging].
func (qb *QueryBuilder) PointSnapshotSQL(innerWhere, outerWhere string, withPaging bool) string {
	q := `SELECT ` + pointsColumnsSQL + ` FROM (` + qb.pointSnapshotInnerSQL(innerWhere) + `
	)`
	if outerWhere != "" {
		q += ` WHERE ` + outerWhere
	}
	q += ` ORDER BY domain, zone, net, node, point`
	if withPaging {
		q += ` LIMIT ? OFFSET ?`
	}
	return q
}

// PointStatsByZoneSQL aggregates a snapshot per zone (totals derived in Go).
// Binds: see pointSnapshotInnerSQL.
func (qb *QueryBuilder) PointStatsByZoneSQL() string {
	return `SELECT zone, count() AS points, uniqExact(net, node) AS bosses
	FROM (` + qb.pointSnapshotInnerSQL("") + `)
	GROUP BY zone ORDER BY zone`
}

// PointCountsByNetSQL counts snapshot points per boss node within one net.
// Binds: see pointSnapshotInnerSQL with zone/net extras.
func (qb *QueryBuilder) PointCountsByNetSQL() string {
	return `SELECT node, count() AS points
	FROM (` + qb.pointSnapshotInnerSQL("zone = ? AND net = ?") + `)
	GROUP BY node`
}

// pointGatedIssuesSQL restricts a points query to fully-imported issues.
// The gate protocol registers the pointlist_files row only after every chunk
// is inserted; without this predicate a query racing a running import (or
// crash leftovers awaiting the delete-before-retry cleanup) would surface
// half-imported rows. The snapshot shape gets this via its own gate-table
// join; every other reader must add it explicitly.
const pointGatedIssuesSQL = `(domain, list_source, pointlist_date) IN (
		SELECT domain, list_source, pointlist_date FROM pointlist_files FINAL)`

// PointHistorySQL returns every stored row of one 4-D address across all
// sources and dates (history views label rows with list_source).
// Binds: domain, domain, zone, net, node, point.
func (qb *QueryBuilder) PointHistorySQL() string {
	return `SELECT ` + pointsColumnsSQL + ` FROM points
	WHERE ` + optionalDomainSQL + ` AND zone = ? AND net = ? AND node = ? AND point = ?
	  AND ` + pointGatedIssuesSQL + `
	ORDER BY pointlist_date DESC, source_priority ASC, list_source, conflict_sequence ASC`
}

// PointDomainsSQL lists the domains a 4-D address (or, with a negative point
// bind, any point of a boss) exists in — domain resolution for API/web when
// ?domain= is omitted.
// Binds: zone, net, node, point, point (-1 = any point).
func (qb *QueryBuilder) PointDomainsSQL() string {
	return `SELECT DISTINCT domain FROM points
	WHERE zone = ? AND net = ? AND node = ? AND (? < 0 OR point = ?) ORDER BY domain`
}

// LatestPointlistDateSQL returns the newest imported issue date per domain
// filter (epoch when nothing is imported — caller detects).
// Binds: domain, domain.
func (qb *QueryBuilder) LatestPointlistDateSQL() string {
	return `SELECT max(pointlist_date) FROM pointlist_files FINAL WHERE ` + optionalDomainSQL
}

// PointlistDatesSQL lists imported pointlist files (gate rows), newest first.
// Binds: domain, domain, list_source, list_source (empty = all sources).
func (qb *QueryBuilder) PointlistDatesSQL() string {
	return `SELECT domain, list_source, pointlist_date, day_number, filename,
			source_format, points_count, bosses_count, imported_at
	FROM pointlist_files FINAL
	WHERE ` + optionalDomainSQL + ` AND (? = '' OR list_source = ?)
	ORDER BY pointlist_date DESC, domain, list_source`
}

// BuildPointFilterConditions translates a PointFilter into two predicate
// sets: IDENTITY conditions (address components + list_source — safe inside
// the snapshot collapse, see pointSnapshotInnerSQL) and ATTRIBUTE conditions
// (text matches, date bounds — must apply after the collapse in snapshot
// mode). History-mode callers just AND the two together.
func (qb *QueryBuilder) BuildPointFilterConditions(filter database.PointFilter) (identityWhere string, identityArgs []interface{}, attrWhere string, attrArgs []interface{}) {
	var identity, attrs []string

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
	if filter.PointNum != nil {
		identity = append(identity, "point = ?")
		identityArgs = append(identityArgs, *filter.PointNum)
	}
	// list_source is identity-like: in snapshot mode it restricts the
	// snapshot to that source's own latest issue (per-source view), not to
	// "overall winners that happen to come from it".
	if filter.ListSource != nil && *filter.ListSource != "" {
		identity = append(identity, "list_source = ?")
		identityArgs = append(identityArgs, *filter.ListSource)
	}

	// Text matches are written as lowerUTF8(col) LIKE lowerUTF8(pattern), NOT
	// col ILIKE pattern, so the ngrambf_v1 skip indexes (defined over the same
	// lowerUTF8 expressions, migration 010) can prune granules — ClickHouse
	// never consults a skip index for ILIKE. lowerUTF8 (not lower) keeps
	// ILIKE's Unicode case folding: Cyrillic pointlist entries must still
	// match case-insensitively.
	if filter.SystemName != nil && *filter.SystemName != "" {
		attrs = append(attrs, "lowerUTF8(system_name) LIKE lowerUTF8(?)")
		attrArgs = append(attrArgs, "%"+*filter.SystemName+"%")
	}
	if filter.Location != nil && *filter.Location != "" {
		attrs = append(attrs, "lowerUTF8(location) LIKE lowerUTF8(?)")
		attrArgs = append(attrArgs, "%"+*filter.Location+"%")
	}
	if filter.SysopName != nil && *filter.SysopName != "" {
		// Sysop names store spaces as underscores; match either form.
		attrs = append(attrs, "lowerUTF8(replaceAll(sysop_name, '_', ' ')) LIKE lowerUTF8(concat('%', replaceAll(?, '_', ' '), '%'))")
		attrArgs = append(attrArgs, *filter.SysopName)
	}
	if filter.DateFrom != nil {
		attrs = append(attrs, "pointlist_date >= ?")
		attrArgs = append(attrArgs, *filter.DateFrom)
	}
	if filter.DateTo != nil {
		attrs = append(attrs, "pointlist_date <= ?")
		attrArgs = append(attrArgs, *filter.DateTo)
	}

	return strings.Join(identity, " AND "), identityArgs, strings.Join(attrs, " AND "), attrArgs
}

// SearchPointsHistorySQL is the non-snapshot search shape: every matching row
// across all dates and sources, newest first.
// Binds: domain, domain, [extraWhere binds], limit, offset.
func (qb *QueryBuilder) SearchPointsHistorySQL(extraWhere string) string {
	q := `SELECT ` + pointsColumnsSQL + ` FROM points
	WHERE ` + optionalDomainSQL + ` AND ` + pointGatedIssuesSQL
	if extraWhere != "" {
		q += ` AND ` + extraWhere
	}
	q += `
	ORDER BY pointlist_date DESC, source_priority ASC, zone, net, node, point, conflict_sequence
	LIMIT ? OFFSET ?`
	return q
}

// SearchPointsLifetimeSQL aggregates every matching historical row into one
// summary per 4-D address (mirrors the node search's lifetime view — raw
// weekly rows would flood the result set). Identity fields come from the
// newest row, most authoritative source on ties.
//
// Two-phase shape: the inner DISTINCT picks the first LIMIT identities in
// primary-key order (read-in-order short-circuits instead of grouping the
// whole match set), the outer aggregation then only touches those
// identities' rows via primary-key pruning on the tuple IN. A broad
// identity-only search (bare zone) would otherwise GROUP BY millions of
// historical rows just to discard everything past row 100 — measured 13.5s
// vs 1.0s on the production table. The full WHERE is repeated in both
// phases: the outer one must keep the text predicates, or first/last dates
// and argMax picks would suddenly aggregate over the identity's NON-matching
// rows too, changing result semantics. OFFSET belongs to the inner phase
// only; the outer LIMIT is defensive (the IN already caps the group count).
//
// Binds: the WHERE binds twice — domain, domain, [extraWhere binds],
// then domain, domain, [extraWhere binds] again (inner), then limit,
// offset (inner), limit (outer).
func (qb *QueryBuilder) SearchPointsLifetimeSQL(extraWhere string) string {
	where := optionalDomainSQL + ` AND ` + pointGatedIssuesSQL
	if extraWhere != "" {
		where += ` AND ` + extraWhere
	}
	// Aliases must NOT shadow the source column names: ClickHouse substitutes
	// SELECT aliases into WHERE, and "lowerUTF8(system_name) LIKE ?" would
	// then refer to the aggregate (error 184) instead of the column. The -conflict_
	// sequence tiebreaker keeps all three argMax picks on the same physical
	// row when a duplicate address ties on date and priority.
	return `SELECT domain, zone, net, node, point,
		argMax(system_name, (pointlist_date, 255 - source_priority, -conflict_sequence)) AS latest_system_name,
		argMax(location, (pointlist_date, 255 - source_priority, -conflict_sequence)) AS latest_location,
		argMax(sysop_name, (pointlist_date, 255 - source_priority, -conflict_sequence)) AS latest_sysop_name,
		min(pointlist_date) AS first_date,
		max(pointlist_date) AS last_date
	FROM points
	WHERE ` + where + `
		AND (domain, zone, net, node, point) IN (
			SELECT DISTINCT domain, zone, net, node, point
			FROM points
			WHERE ` + where + `
			ORDER BY domain, zone, net, node, point
			LIMIT ? OFFSET ?)
	GROUP BY domain, zone, net, node, point
	ORDER BY domain, zone, net, node, point
	LIMIT ?`
}
