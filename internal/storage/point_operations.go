package storage

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/nodelistdb/internal/database"
	"github.com/nodelistdb/internal/logging"
)

// PointOperations handles all pointlist-related database operations.
// Writers implement the gate protocol from the pointlist design: point rows
// first, gate row (pointlist_files) registered last, so a crash mid-import
// never looks like a completed file.
type PointOperations struct {
	db           database.DatabaseInterface
	queryBuilder *QueryBuilder
	resultParser ResultParserInterface
	mu           sync.RWMutex
}

// PointlistSourceInfo summarizes one imported pointlist series
type PointlistSourceInfo struct {
	Domain      string    `json:"domain"`
	ListSource  string    `json:"list_source"`
	FirstDate   time.Time `json:"first_date"`
	LastDate    time.Time `json:"last_date"`
	FileCount   uint64    `json:"file_count"`
	TotalPoints uint64    `json:"total_points"`
}

// NewPointOperations creates a new PointOperations instance
func NewPointOperations(db database.DatabaseInterface, queryBuilder *QueryBuilder, resultParser ResultParserInterface) *PointOperations {
	return &PointOperations{
		db:           db,
		queryBuilder: queryBuilder,
		resultParser: resultParser,
	}
}

// InsertPoints inserts a batch of points. It prefers the native ClickHouse
// batch API (typed, no SQL string building) and falls back to parameterized
// SQL when only the database/sql interface is available (HTTP protocol).
func (po *PointOperations) InsertPoints(points []database.Point) error {
	if len(points) == 0 {
		return nil
	}

	po.mu.Lock()
	defer po.mu.Unlock()

	if provider, ok := po.db.(nativeConnProvider); ok {
		if conn := provider.NativeConn(); conn != nil {
			return po.insertPointsNative(conn, points)
		}
	}
	return po.insertPointsSQL(points)
}

// pointInsertValues returns one point's values in pointsColumnsSQL order,
// with types matching the column types exactly (the native driver is strict).
func pointInsertValues(p *database.Point) []interface{} {
	if p.Domain == "" {
		p.Domain = database.DefaultDomain
	}
	if p.FtsId == "" {
		p.ComputeFtsId()
	}
	internetConfigStr := ""
	if len(p.InternetConfig) > 0 {
		internetConfigStr = string(p.InternetConfig)
	}
	flags := p.Flags
	if flags == nil {
		flags = []string{}
	}
	modemFlags := p.ModemFlags
	if modemFlags == nil {
		modemFlags = []string{}
	}

	return []interface{}{
		p.Domain,
		int32(p.Zone), int32(p.Net), int32(p.Node), int32(p.PointNum),
		p.PointlistDate, int32(p.DayNumber),
		p.ListSource, p.SourcePriority, p.SourceFormat,
		p.SystemName, p.Location, p.SysopName, p.Phone, p.MaxSpeed,
		p.IsCM, p.IsMO, p.HasInet,
		flags, modemFlags, internetConfigStr,
		uint16(p.ConflictSequence), p.HasConflict, p.FtsId, p.RawLine,
	}
}

// insertPointsNative uses ClickHouse's native batch API in chunks.
func (po *PointOperations) insertPointsNative(conn driver.Conn, points []database.Point) error {
	ctx := context.Background()

	chunkSize := DefaultBatchInsertConfig().ChunkSize
	for i := 0; i < len(points); i += chunkSize {
		end := i + chunkSize
		if end > len(points) {
			end = len(points)
		}

		batch, err := conn.PrepareBatch(ctx, po.queryBuilder.InsertPointsBatchSQL())
		if err != nil {
			return fmt.Errorf("failed to prepare points batch: %w", err)
		}
		for j := i; j < end; j++ {
			if err := batch.Append(pointInsertValues(&points[j])...); err != nil {
				return fmt.Errorf("failed to append point row: %w", err)
			}
		}
		if err := batch.Send(); err != nil {
			return fmt.Errorf("failed to send points chunk %d-%d: %w", i, end-1, err)
		}
	}

	return nil
}

// insertPointsSQL is the parameterized database/sql fallback.
func (po *PointOperations) insertPointsSQL(points []database.Point) error {
	conn := po.db.Conn()

	tx, err := conn.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.Prepare(po.queryBuilder.InsertPointSQL())
	if err != nil {
		return fmt.Errorf("failed to prepare points insert: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for i := range points {
		if _, err := stmt.Exec(pointInsertValues(&points[i])...); err != nil {
			return fmt.Errorf("failed to insert point row: %w", err)
		}
	}

	return tx.Commit()
}

// IsPointlistImported checks the import gate for one (domain, source, date) file.
func (po *PointOperations) IsPointlistImported(domain, listSource string, date time.Time) (bool, error) {
	po.mu.RLock()
	defer po.mu.RUnlock()

	if domain == "" {
		domain = database.DefaultDomain
	}

	var count int
	err := po.db.Conn().QueryRow(po.queryBuilder.IsPointlistImportedSQL(), domain, listSource, date).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check pointlist gate: %w", err)
	}
	return count > 0, nil
}

// RegisterPointlistFile writes the gate row. Call this ONLY after every point
// chunk of the file has been inserted successfully.
func (po *PointOperations) RegisterPointlistFile(file database.PointlistFile) error {
	po.mu.Lock()
	defer po.mu.Unlock()

	if file.Domain == "" {
		file.Domain = database.DefaultDomain
	}

	_, err := po.db.Conn().Exec(po.queryBuilder.RegisterPointlistFileSQL(),
		file.Domain, file.ListSource, file.PointlistDate, int32(file.DayNumber),
		file.Filename, file.SourceFormat, file.PointsCount, file.BossesCount)
	if err != nil {
		return fmt.Errorf("failed to register pointlist file: %w", err)
	}
	return nil
}

// DeletePointlistData removes the point rows of one (domain, source, date)
// file. With includeGate it also removes the gate row (-reimport); without it
// this is the partial-remnant cleanup run before retrying an un-gated file.
func (po *PointOperations) DeletePointlistData(domain, listSource string, date time.Time, includeGate bool) error {
	po.mu.Lock()
	defer po.mu.Unlock()

	if domain == "" {
		domain = database.DefaultDomain
	}

	conn := po.db.Conn()
	// Order matters for crash safety: the gate row goes FIRST. A crash
	// between the two deletes then leaves points-without-gate — exactly the
	// state the delete-before-retry cleanup already heals. The reverse order
	// would leave a gate row with no backing points, silently skipped forever.
	if includeGate {
		if _, err := conn.Exec(po.queryBuilder.DeletePointlistFileGateSQL(), domain, listSource, date); err != nil {
			return fmt.Errorf("failed to delete pointlist gate row: %w", err)
		}
	}
	// DELETE is a ClickHouse mutation (expensive part rewrite). The common
	// bulk-import case is a first-time file with zero remnant rows — check
	// with a cheap indexed count and skip the mutation entirely.
	var remnants int
	if err := conn.QueryRow(po.queryBuilder.CountPointsForFileSQL(), domain, listSource, date).Scan(&remnants); err != nil {
		return fmt.Errorf("failed to count remnant point rows: %w", err)
	}
	if remnants == 0 {
		return nil
	}
	if _, err := conn.Exec(po.queryBuilder.DeletePointsForFileSQL(), domain, listSource, date); err != nil {
		return fmt.Errorf("failed to delete point rows: %w", err)
	}
	return nil
}

// NearestPointsCount returns the points_count (and date) of the imported
// issue of the series nearest in time to date. found is false when this is
// the series' first issue.
func (po *PointOperations) NearestPointsCount(domain, listSource string, date time.Time) (count int, nearestDate time.Time, found bool, err error) {
	po.mu.RLock()
	defer po.mu.RUnlock()

	if domain == "" {
		domain = database.DefaultDomain
	}

	err = po.db.Conn().QueryRow(po.queryBuilder.NearestPointlistCountSQL(), domain, listSource, date, date).Scan(&count, &nearestDate)
	if err == sql.ErrNoRows {
		return 0, time.Time{}, false, nil
	}
	if err != nil {
		return 0, time.Time{}, false, fmt.Errorf("failed to get nearest pointlist count: %w", err)
	}
	return count, nearestDate, true, nil
}

// CountPointsForFile counts stored point rows of one file (verification).
func (po *PointOperations) CountPointsForFile(domain, listSource string, date time.Time) (int, error) {
	po.mu.RLock()
	defer po.mu.RUnlock()

	if domain == "" {
		domain = database.DefaultDomain
	}

	var count int
	err := po.db.Conn().QueryRow(po.queryBuilder.CountPointsForFileSQL(), domain, listSource, date).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count point rows: %w", err)
	}
	return count, nil
}

// --- Phase 2 readers ---
// "Current/as-of" readers use snapshot semantics: per source the latest issue
// at or before the as-of date (within the staleness window), overlap resolved
// by source_priority. History readers return every source, labeled.

// PointSummary is one 4-D address aggregated across its whole pointlist
// history (search results view, mirrors NodeSummary).
type PointSummary struct {
	Zone            int       `json:"zone"`
	Net             int       `json:"net"`
	Node            int       `json:"node"`
	PointNum        int       `json:"point"`
	Domain          string    `json:"domain,omitempty"`
	SystemName      string    `json:"system_name"`
	Location        string    `json:"location"`
	SysopName       string    `json:"sysop_name"`
	FirstDate       time.Time `json:"first_date"`
	LastDate        time.Time `json:"last_date"`
	CurrentlyActive bool      `json:"currently_active"`
}

// PointZoneCount is one zone's slice of a point snapshot.
type PointZoneCount struct {
	Zone   int    `json:"zone"`
	Points uint64 `json:"points"`
	Bosses uint64 `json:"bosses"`
}

// PointStats summarizes a point snapshot for one domain.
type PointStats struct {
	Domain      string           `json:"domain"`
	AsOf        time.Time        `json:"as_of"`
	TotalPoints uint64           `json:"total_points"`
	TotalBosses uint64           `json:"total_bosses"`
	Zones       []PointZoneCount `json:"zones,omitempty"`
}

// LatestPointlistDate returns the newest imported issue date for the domain
// (empty = any domain). found is false when nothing is imported.
func (po *PointOperations) LatestPointlistDate(domain string) (time.Time, bool, error) {
	po.mu.RLock()
	defer po.mu.RUnlock()
	return po.latestPointlistDateLocked(domain)
}

func (po *PointOperations) latestPointlistDateLocked(domain string) (time.Time, bool, error) {
	var latest time.Time
	err := po.db.Conn().QueryRow(po.queryBuilder.LatestPointlistDateSQL(), domain, domain).Scan(&latest)
	if err == sql.ErrNoRows {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, fmt.Errorf("failed to get latest pointlist date: %w", err)
	}
	// max() over zero rows yields the epoch default, not ErrNoRows.
	if latest.Year() < 1980 {
		return time.Time{}, false, nil
	}
	return latest, true, nil
}

// resolveAsOf turns an optional as-of date into the snapshot anchor: nil means
// "as of the newest imported issue" (per-domain), so views stay populated even
// if the weekly sync lags. found=false means the domain has no pointlists.
func (po *PointOperations) resolveAsOf(domain string, asOf *time.Time) (time.Time, bool, error) {
	if asOf != nil && !asOf.IsZero() {
		return *asOf, true, nil
	}
	return po.latestPointlistDateLocked(domain)
}

// snapshotArgs builds the bind list for pointSnapshotInnerSQL-based queries.
func snapshotArgs(domain string, asOf time.Time, extra ...interface{}) []interface{} {
	args := []interface{}{domain, domain, domain, domain, asOf, asOf}
	return append(args, extra...)
}

// queryPoints runs a points SELECT and parses rows via ParsePointRow.
func (po *PointOperations) queryPoints(ctx context.Context, query string, args ...interface{}) ([]database.Point, error) {
	rows, err := po.db.Conn().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query points: %w", err)
	}
	defer rows.Close()

	var points []database.Point
	for rows.Next() {
		point, err := po.resultParser.ParsePointRow(rows)
		if err != nil {
			return nil, err
		}
		points = append(points, point)
	}
	return points, rows.Err()
}

// GetPointsByBoss returns the snapshot points under one boss node.
// asOf nil = as of the newest imported issue.
func (po *PointOperations) GetPointsByBoss(domain string, zone, net, node int, asOf *time.Time) ([]database.Point, error) {
	po.mu.RLock()
	defer po.mu.RUnlock()

	anchor, found, err := po.resolveAsOf(domain, asOf)
	if err != nil || !found {
		return nil, err
	}

	query := po.queryBuilder.PointSnapshotSQL("zone = ? AND net = ? AND node = ?", "", false)
	return po.queryPoints(context.Background(), query, snapshotArgs(domain, anchor, zone, net, node)...)
}

// GetPointHistory returns every stored row of one 4-D address across all
// sources and dates.
func (po *PointOperations) GetPointHistory(domain string, zone, net, node, point int) ([]database.Point, error) {
	po.mu.RLock()
	defer po.mu.RUnlock()

	return po.queryPoints(context.Background(), po.queryBuilder.PointHistorySQL(), domain, domain, zone, net, node, point)
}

// SearchPoints searches points by filter. LatestOnly applies snapshot
// semantics (DateTo doubles as the as-of anchor); otherwise every matching
// historical row is returned, newest first.
func (po *PointOperations) SearchPoints(filter database.PointFilter) ([]database.Point, error) {
	return po.searchPoints(context.Background(), filter)
}

// searchPoints is SearchPoints with an explicit context, shared with
// SearchPointsWithLifetime's LatestOnly branch so its cancellation reaches
// the snapshot query too.
func (po *PointOperations) searchPoints(ctx context.Context, filter database.PointFilter) ([]database.Point, error) {
	po.mu.RLock()
	defer po.mu.RUnlock()

	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > MaxSearchLimit {
		limit = MaxSearchLimit
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	domain := ""
	if filter.Domain != nil {
		domain = *filter.Domain
	}

	if filter.LatestOnly != nil && *filter.LatestOnly {
		anchor, found, err := po.resolveAsOf(domain, filter.DateTo)
		if err != nil || !found {
			return nil, err
		}
		// DateTo becomes the snapshot anchor; DateFrom stays as a post-
		// collapse bound ("winning row no older than"). Attribute filters
		// apply after the collapse so they cannot promote a
		// non-authoritative row into the snapshot.
		snapFilter := filter
		snapFilter.DateTo = nil
		identityWhere, identityArgs, attrWhere, attrArgs := po.queryBuilder.BuildPointFilterConditions(snapFilter)
		args := snapshotArgs(domain, anchor, identityArgs...)
		args = append(args, attrArgs...)
		args = append(args, limit, offset)
		return po.queryPoints(ctx, po.queryBuilder.PointSnapshotSQL(identityWhere, attrWhere, true), args...)
	}

	identityWhere, identityArgs, attrWhere, attrArgs := po.queryBuilder.BuildPointFilterConditions(filter)
	where := identityWhere
	if attrWhere != "" {
		if where != "" {
			where += " AND " + attrWhere
		} else {
			where = attrWhere
		}
	}
	args := append([]interface{}{domain, domain}, identityArgs...)
	args = append(args, attrArgs...)
	args = append(args, limit, offset)
	return po.queryPoints(ctx, po.queryBuilder.SearchPointsHistorySQL(where), args...)
}

// SearchPointsWithLifetime searches points and returns one summary per 4-D
// address. LatestOnly restricts to the current snapshot (listed period =
// snapshot issue date); otherwise the whole history is aggregated, with
// CurrentlyActive meaning "still listed within the staleness window of the
// domain's newest issue". The context cancels the underlying query in either
// branch — this runs on every web search, so an abandoned request must not
// keep occupying a pool connection.
func (po *PointOperations) SearchPointsWithLifetime(ctx context.Context, filter database.PointFilter) ([]PointSummary, error) {
	if filter.LatestOnly != nil && *filter.LatestOnly {
		points, err := po.searchPoints(ctx, filter)
		if err != nil {
			return nil, err
		}
		summaries := make([]PointSummary, 0, len(points))
		for _, p := range points {
			summaries = append(summaries, PointSummary{
				Zone: p.Zone, Net: p.Net, Node: p.Node, PointNum: p.PointNum,
				Domain:     p.Domain,
				SystemName: p.SystemName, Location: p.Location, SysopName: p.SysopName,
				FirstDate: p.PointlistDate, LastDate: p.PointlistDate,
				CurrentlyActive: true,
			})
		}
		return summaries, nil
	}

	po.mu.RLock()
	defer po.mu.RUnlock()

	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > MaxSearchLimit {
		limit = MaxSearchLimit
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	domain := ""
	if filter.Domain != nil {
		domain = *filter.Domain
	}

	identityWhere, identityArgs, attrWhere, attrArgs := po.queryBuilder.BuildPointFilterConditions(filter)
	where := identityWhere
	if attrWhere != "" {
		if where != "" {
			where += " AND " + attrWhere
		} else {
			where = attrWhere
		}
	}
	// The two-phase lifetime SQL repeats the WHERE (outer aggregation +
	// inner identity-selection subquery), so the WHERE binds go in twice.
	whereArgs := append([]interface{}{domain, domain}, identityArgs...)
	whereArgs = append(whereArgs, attrArgs...)
	args := make([]interface{}, 0, 2*len(whereArgs)+3)
	args = append(args, whereArgs...)
	args = append(args, whereArgs...)
	args = append(args, limit, offset, limit)

	rows, err := po.db.Conn().QueryContext(ctx, po.queryBuilder.SearchPointsLifetimeSQL(where), args...)
	if err != nil {
		return nil, fmt.Errorf("failed to search points: %w", err)
	}
	defer rows.Close()

	var summaries []PointSummary
	for rows.Next() {
		var s PointSummary
		var zone, net, node, point int32
		if err := rows.Scan(&s.Domain, &zone, &net, &node, &point,
			&s.SystemName, &s.Location, &s.SysopName, &s.FirstDate, &s.LastDate); err != nil {
			return nil, fmt.Errorf("failed to scan point summary: %w", err)
		}
		s.Zone, s.Net, s.Node, s.PointNum = int(zone), int(net), int(node), int(point)
		summaries = append(summaries, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// CurrentlyActive = last listed within the staleness window of the
	// point's domain's newest imported issue.
	cutoffs := make(map[string]time.Time)
	for i := range summaries {
		s := &summaries[i]
		cutoff, ok := cutoffs[s.Domain]
		if !ok {
			latest, found, err := po.latestPointlistDateLocked(s.Domain)
			if err != nil {
				// Zero cutoff labels the domain's rows Historical; say why
				// instead of letting the mislabel pass silently.
				logging.Warnf("point lifetime search: latest pointlist date for domain %q failed: %v", s.Domain, err)
			} else if found {
				cutoff = latest.AddDate(0, 0, -PointSnapshotStalenessDays)
			}
			cutoffs[s.Domain] = cutoff
		}
		s.CurrentlyActive = !cutoff.IsZero() && s.LastDate.After(cutoff)
	}
	return summaries, nil
}

// GetPointStats summarizes the snapshot for one domain (totals + per zone).
func (po *PointOperations) GetPointStats(domain string, asOf *time.Time) (*PointStats, error) {
	po.mu.RLock()
	defer po.mu.RUnlock()

	stats := &PointStats{Domain: domain}

	anchor, found, err := po.resolveAsOf(domain, asOf)
	if err != nil {
		return nil, err
	}
	if !found {
		return stats, nil
	}
	stats.AsOf = anchor

	rows, err := po.db.Conn().Query(po.queryBuilder.PointStatsByZoneSQL(), snapshotArgs(domain, anchor)...)
	if err != nil {
		return nil, fmt.Errorf("failed to query point stats: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var zone int32
		var zc PointZoneCount
		if err := rows.Scan(&zone, &zc.Points, &zc.Bosses); err != nil {
			return nil, fmt.Errorf("failed to scan point zone stats: %w", err)
		}
		zc.Zone = int(zone)
		stats.Zones = append(stats.Zones, zc)
		stats.TotalPoints += zc.Points
		stats.TotalBosses += zc.Bosses
	}
	return stats, rows.Err()
}

// GetPointCountsByNet returns snapshot point counts keyed by boss node number
// for one net (browse net page column).
func (po *PointOperations) GetPointCountsByNet(domain string, zone, net int, asOf *time.Time) (map[int]uint64, error) {
	po.mu.RLock()
	defer po.mu.RUnlock()

	counts := make(map[int]uint64)

	anchor, found, err := po.resolveAsOf(domain, asOf)
	if err != nil || !found {
		return counts, err
	}

	rows, err := po.db.Conn().Query(po.queryBuilder.PointCountsByNetSQL(), snapshotArgs(domain, anchor, zone, net)...)
	if err != nil {
		return nil, fmt.Errorf("failed to query point counts: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var node int32
		var n uint64
		if err := rows.Scan(&node, &n); err != nil {
			return nil, fmt.Errorf("failed to scan point count: %w", err)
		}
		counts[int(node)] = n
	}
	return counts, rows.Err()
}

// GetPointDomains lists the domains a 4-D address exists in. A nil point
// resolves at boss level (any point under the boss).
func (po *PointOperations) GetPointDomains(zone, net, node int, point *int) ([]string, error) {
	po.mu.RLock()
	defer po.mu.RUnlock()

	pointNum := -1
	if point != nil {
		pointNum = *point
	}
	rows, err := po.db.Conn().Query(po.queryBuilder.PointDomainsSQL(), zone, net, node, pointNum, pointNum)
	if err != nil {
		return nil, fmt.Errorf("failed to query point domains: %w", err)
	}
	defer rows.Close()

	var domains []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, fmt.Errorf("failed to scan point domain: %w", err)
		}
		domains = append(domains, d)
	}
	return domains, rows.Err()
}

// GetPointlistDates lists imported pointlist files (gate rows), newest first.
// Empty domain or listSource = no filter on that axis.
func (po *PointOperations) GetPointlistDates(domain, listSource string) ([]database.PointlistFile, error) {
	po.mu.RLock()
	defer po.mu.RUnlock()

	rows, err := po.db.Conn().Query(po.queryBuilder.PointlistDatesSQL(), domain, domain, listSource, listSource)
	if err != nil {
		return nil, fmt.Errorf("failed to query pointlist dates: %w", err)
	}
	defer rows.Close()

	var files []database.PointlistFile
	for rows.Next() {
		var f database.PointlistFile
		var dayNumber int32
		if err := rows.Scan(&f.Domain, &f.ListSource, &f.PointlistDate, &dayNumber,
			&f.Filename, &f.SourceFormat, &f.PointsCount, &f.BossesCount, &f.ImportedAt); err != nil {
			return nil, fmt.Errorf("failed to scan pointlist file: %w", err)
		}
		f.DayNumber = int(dayNumber)
		files = append(files, f)
	}
	return files, rows.Err()
}

// GetPointlistSources summarizes imported pointlist series. An empty domain
// lists all networks.
func (po *PointOperations) GetPointlistSources(domain string) ([]PointlistSourceInfo, error) {
	po.mu.RLock()
	defer po.mu.RUnlock()

	rows, err := po.db.Conn().Query(po.queryBuilder.PointlistSourcesSQL(), domain, domain)
	if err != nil {
		return nil, fmt.Errorf("failed to query pointlist sources: %w", err)
	}
	defer rows.Close()

	var result []PointlistSourceInfo
	for rows.Next() {
		var si PointlistSourceInfo
		if err := rows.Scan(&si.Domain, &si.ListSource, &si.FirstDate, &si.LastDate, &si.FileCount, &si.TotalPoints); err != nil {
			return nil, fmt.Errorf("failed to scan pointlist source info: %w", err)
		}
		result = append(result, si)
	}
	return result, rows.Err()
}
