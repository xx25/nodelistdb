package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"nodelistdb/internal/database"
)

// Storage provides thread-safe database operations with prepared statements
type Storage struct {
	db         *database.DB
	insertStmt *sql.Stmt
	selectStmt *sql.Stmt
	statsStmt  *sql.Stmt
	mu         sync.RWMutex
}

// New creates a new Storage instance with prepared statements
func New(db *database.DB) (*Storage, error) {
	storage := &Storage{
		db: db,
	}

	if err := storage.prepareStatements(); err != nil {
		return nil, fmt.Errorf("failed to prepare statements: %w", err)
	}

	return storage, nil
}

// prepareStatements prepares all SQL statements
func (s *Storage) prepareStatements() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	conn := s.db.Conn()

	// Prepare insert statement with conflict tracking fields
	insertSQL := `
	INSERT INTO nodes (
		zone, net, node, nodelist_date, day_number,
		system_name, location, sysop_name, phone, node_type, region, max_speed,
		is_cm, is_mo, has_binkp, has_telnet, is_down, is_hold, is_pvt, is_active,
		flags, modem_flags, internet_protocols, internet_hostnames, internet_ports, internet_emails,
		conflict_sequence, has_conflict, has_inet, internet_config
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	var err error
	s.insertStmt, err = conn.Prepare(insertSQL)
	if err != nil {
		return fmt.Errorf("failed to prepare insert statement: %w", err)
	}

	// Prepare select statement for node queries
	selectSQL := `
	SELECT zone, net, node, nodelist_date, day_number,
		   system_name, location, sysop_name, phone, node_type, region, max_speed,
		   is_cm, is_mo, has_binkp, has_telnet, is_down, is_hold, is_pvt, is_active,
		   flags, modem_flags, internet_protocols, internet_hostnames, internet_ports, internet_emails,
		   has_inet, internet_config
	FROM nodes
	WHERE 1=1`

	s.selectStmt, err = conn.Prepare(selectSQL)
	if err != nil {
		s.insertStmt.Close()
		return fmt.Errorf("failed to prepare select statement: %w", err)
	}

	// Prepare stats statement
	statsSQL := `
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

	s.statsStmt, err = conn.Prepare(statsSQL)
	if err != nil {
		s.insertStmt.Close()
		s.selectStmt.Close()
		return fmt.Errorf("failed to prepare stats statement: %w", err)
	}

	return nil
}

// Close closes all prepared statements
func (s *Storage) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var errs []error

	if s.insertStmt != nil {
		if err := s.insertStmt.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if s.selectStmt != nil {
		if err := s.selectStmt.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if s.statsStmt != nil {
		if err := s.statsStmt.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing statements: %v", errs)
	}

	return nil
}

// InsertNodes inserts a batch of nodes using optimized prepared statements
func (s *Storage) buildInsertSQL(values []string) string {
	return fmt.Sprintf(`
		INSERT INTO nodes (
			zone, net, node, nodelist_date, day_number,
			system_name, location, sysop_name, phone, node_type, region, max_speed,
			is_cm, is_mo, has_binkp, has_telnet, is_down, is_hold, is_pvt, is_active,
			flags, modem_flags, internet_protocols, internet_hostnames, internet_ports, internet_emails,
			conflict_sequence, has_conflict, has_inet, internet_config
		) VALUES %s
		ON CONFLICT (zone, net, node, nodelist_date, conflict_sequence) 
		DO NOTHING
	`, strings.Join(values, ","))
}

func (s *Storage) formatNodeValues(node database.Node) string {
	// Handle internet_config JSON
	configStr := "NULL"
	if node.InternetConfig != nil && len(node.InternetConfig) > 0 {
		// Escape the JSON for SQL
		configStr = "'" + escapeSingleQuotes(string(node.InternetConfig)) + "'"
	}
	
	return fmt.Sprintf(
		"(%d,%d,%d,'%s',%d,'%s','%s','%s','%s','%s',%s,'%s',%t,%t,%t,%t,%t,%t,%t,%t,%s,%s,%s,%s,%s,%s,%d,%t,%t,%s)",
		node.Zone, node.Net, node.Node,
		node.NodelistDate.Format("2006-01-02"), node.DayNumber,
		escapeSingleQuotes(node.SystemName),
		escapeSingleQuotes(node.Location),
		escapeSingleQuotes(node.SysopName),
		escapeSingleQuotes(node.Phone),
		node.NodeType,
		formatNullableInt(node.Region),
		node.MaxSpeed,
		node.IsCM, node.IsMO, node.HasBinkp, node.HasTelnet,
		node.IsDown, node.IsHold, node.IsPvt, node.IsActive,
		formatArrayDuckDB(node.Flags),
		formatArrayDuckDB(node.ModemFlags),
		formatArrayDuckDB(node.InternetProtocols),
		formatArrayDuckDB(node.InternetHostnames),
		formatIntArrayDuckDB(node.InternetPorts),
		formatArrayDuckDB(node.InternetEmails),
		node.ConflictSequence, node.HasConflict,
		node.HasInet, configStr,
	)
}

func (s *Storage) insertChunk(tx *sql.Tx, chunk []database.Node) error {
	var values []string
	for _, node := range chunk {
		values = append(values, s.formatNodeValues(node))
	}
	
	insertSQL := s.buildInsertSQL(values)
	_, err := tx.Exec(insertSQL)
	if err != nil {
		return fmt.Errorf("failed to insert chunk: %w", err)
	}
	return nil
}

func (s *Storage) InsertNodes(nodes []database.Node) error {
	if len(nodes) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	conn := s.db.Conn()

	tx, err := conn.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	chunkSize := 100
	for i := 0; i < len(nodes); i += chunkSize {
		end := i + chunkSize
		if end > len(nodes) {
			end = len(nodes)
		}
		chunk := nodes[i:end]
		
		if err := s.insertChunk(tx, chunk); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// markOriginalAsConflicted marks the original entry (conflict_sequence=0) as having conflict
func (s *Storage) markOriginalAsConflicted(conn *sql.DB, zone, net, node int, date time.Time) error {
	updateSQL := `UPDATE nodes SET has_conflict = true WHERE zone = ? AND net = ? AND node = ? AND nodelist_date = ? AND conflict_sequence = 0`
	_, err := conn.Exec(updateSQL, zone, net, node, date)
	return err
}

// GetNodes retrieves nodes based on filter criteria
func (s *Storage) buildBaseQuery(latestOnly bool) string {
	if latestOnly {
		return `
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
	}
	return `
	SELECT zone, net, node, nodelist_date, day_number,
		   system_name, location, sysop_name, phone, node_type, region, max_speed,
		   is_cm, is_mo, has_binkp, has_telnet, is_down, is_hold, is_pvt, is_active,
		   flags, modem_flags, internet_protocols, internet_hostnames, internet_ports, internet_emails,
		   conflict_sequence, has_conflict, has_inet, internet_config
	FROM nodes WHERE 1=1`
}

func (s *Storage) buildWhereConditions(filter database.NodeFilter) ([]string, []interface{}) {
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

func (s *Storage) parseNodeRow(rows *sql.Rows) (database.Node, error) {
	var node database.Node
	var flags, modemFlags, protocols, hosts, emails interface{}
	var ports interface{}
	var internetConfig sql.NullString

	err := rows.Scan(
		&node.Zone, &node.Net, &node.Node, &node.NodelistDate, &node.DayNumber,
		&node.SystemName, &node.Location, &node.SysopName, &node.Phone,
		&node.NodeType, &node.Region, &node.MaxSpeed,
		&node.IsCM, &node.IsMO, &node.HasBinkp, &node.HasTelnet,
		&node.IsDown, &node.IsHold, &node.IsPvt, &node.IsActive,
		&flags, &modemFlags, &protocols, &hosts, &ports, &emails,
		&node.ConflictSequence, &node.HasConflict,
		&node.HasInet, &internetConfig,
	)
	if err != nil {
		return node, fmt.Errorf("failed to scan node: %w", err)
	}

	node.Flags = parseInterfaceToStringArray(flags)
	node.ModemFlags = parseInterfaceToStringArray(modemFlags)
	node.InternetProtocols = parseInterfaceToStringArray(protocols)
	node.InternetHostnames = parseInterfaceToStringArray(hosts)
	node.InternetPorts = parseInterfaceToIntArray(ports)
	node.InternetEmails = parseInterfaceToStringArray(emails)
	
	// Handle JSON field
	if internetConfig.Valid {
		node.InternetConfig = json.RawMessage(internetConfig.String)
	}

	return node, nil
}

func (s *Storage) buildOrderByClause() string {
	return " ORDER BY zone, net, node, nodelist_date DESC"
}

func (s *Storage) GetNodes(filter database.NodeFilter) ([]database.Node, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	conn := s.db.Conn()

	baseSQL := s.buildBaseQuery(filter.LatestOnly != nil && *filter.LatestOnly)
	conditions, args := s.buildWhereConditions(filter)

	if len(conditions) > 0 {
		baseSQL += " AND " + strings.Join(conditions, " AND ")
	}

	baseSQL += s.buildOrderByClause()

	if filter.Limit > 0 {
		baseSQL += fmt.Sprintf(" LIMIT %d", filter.Limit)
		if filter.Offset > 0 {
			baseSQL += fmt.Sprintf(" OFFSET %d", filter.Offset)
		}
	}

	rows, err := conn.Query(baseSQL, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query nodes: %w", err)
	}
	defer rows.Close()

	var nodes []database.Node
	for rows.Next() {
		node, err := s.parseNodeRow(rows)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return nodes, nil
}

// GetLatestStatsDate retrieves the most recent date that has statistics
func (s *Storage) GetLatestStatsDate() (time.Time, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	conn := s.db.Conn()
	var latestDate time.Time
	err := conn.QueryRow("SELECT MAX(nodelist_date) FROM nodes").Scan(&latestDate)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to get latest stats date: %w", err)
	}
	return latestDate, nil
}

// GetStats retrieves network statistics for a specific date
func (s *Storage) GetStats(date time.Time) (*database.NetworkStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	conn := s.db.Conn()

	var stats database.NetworkStats
	row := conn.QueryRow(`
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
		GROUP BY nodelist_date
	`, date)

	err := row.Scan(
		&stats.Date, &stats.TotalNodes, &stats.ActiveNodes,
		&stats.CMNodes, &stats.MONodes, &stats.BinkpNodes, &stats.TelnetNodes,
		&stats.PvtNodes, &stats.DownNodes, &stats.HoldNodes, &stats.InternetNodes,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("no data found for date %v", date)
		}
		return nil, fmt.Errorf("failed to get stats: %w", err)
	}

	// Get zone distribution
	stats.ZoneDistribution = make(map[int]int)
	rows, err := conn.Query("SELECT zone, COUNT(*) FROM nodes WHERE nodelist_date = ? GROUP BY zone", date)
	if err != nil {
		return nil, fmt.Errorf("failed to get zone distribution: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var zone, count int
		if err := rows.Scan(&zone, &count); err != nil {
			return nil, fmt.Errorf("failed to scan zone distribution: %w", err)
		}
		stats.ZoneDistribution[zone] = count
	}

	// Get largest regions (top 10)
	stats.LargestRegions = []database.RegionInfo{}
	rows, err = conn.Query(`
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
		LIMIT 10
	`, date)
	if err != nil {
		return nil, fmt.Errorf("failed to get largest regions: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var region database.RegionInfo
		var regionName sql.NullString
		if err := rows.Scan(&region.Zone, &region.Region, &region.NodeCount, &regionName); err != nil {
			return nil, fmt.Errorf("failed to scan region info: %w", err)
		}
		if regionName.Valid {
			region.Name = regionName.String
		}
		stats.LargestRegions = append(stats.LargestRegions, region)
	}

	// Get largest nets (top 10 per zone, then take overall top 10)
	stats.LargestNets = []database.NetInfo{}
	rows, err = conn.Query(`
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
		LIMIT 10
	`, date, date)
	if err != nil {
		return nil, fmt.Errorf("failed to get largest nets: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var net database.NetInfo
		var hostName sql.NullString
		if err := rows.Scan(&net.Zone, &net.Net, &net.NodeCount, &hostName); err != nil {
			return nil, fmt.Errorf("failed to scan net info: %w", err)
		}
		if hostName.Valid {
			net.Name = hostName.String
		}
		stats.LargestNets = append(stats.LargestNets, net)
	}

	return &stats, nil
}

// IsNodelistProcessed checks if a nodelist has already been processed based on date
func (s *Storage) IsNodelistProcessed(nodelistDate time.Time) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	conn := s.db.Conn()

	var count int
	err := conn.QueryRow(
		"SELECT COUNT(*) FROM nodes WHERE nodelist_date = ? LIMIT 1", 
		nodelistDate,
	).Scan(&count)
	
	if err != nil {
		return false, fmt.Errorf("failed to check if nodelist is processed: %w", err)
	}

	return count > 0, nil
}

// FindConflictingNode checks if a node already exists for the same date
func (s *Storage) FindConflictingNode(zone, net, node int, date time.Time) (bool, error) {
	// Note: Don't use mutex here as this may be called within a transaction
	conn := s.db.Conn()

	// Use a separate transaction with READ COMMITTED isolation to see committed data
	tx, err := conn.Begin()
	if err != nil {
		return false, fmt.Errorf("failed to begin transaction for conflict check: %w", err)
	}
	defer tx.Rollback()

	var count int
	queryErr := tx.QueryRow(
		`SELECT COUNT(*) FROM nodes 
		 WHERE zone = ? AND net = ? AND node = ? AND nodelist_date = ? 
		 LIMIT 1`, 
		zone, net, node, date,
	).Scan(&count)
	
	if queryErr != nil {
		if queryErr == sql.ErrNoRows {
			return false, nil // No conflict found in committed data
		}
		return false, fmt.Errorf("failed to find conflicting node: %w", queryErr)
	}

	tx.Commit()
	return count > 0, nil
}

// Helper functions for array formatting (DuckDB array handling)
func formatStringArray(arr []string) string {
	if len(arr) == 0 {
		return "[]"
	}
	result, _ := json.Marshal(arr)
	return string(result)
}

func formatIntArray(arr []int) string {
	if len(arr) == 0 {
		return "[]"
	}
	result, _ := json.Marshal(arr)
	return string(result)
}

// Helper functions for DuckDB native array syntax (optimized)
func formatArrayDuckDB(arr []string) string {
	if len(arr) == 0 {
		return "ARRAY[]::TEXT[]"
	}
	
	escaped := make([]string, len(arr))
	for i, s := range arr {
		escaped[i] = "'" + escapeSingleQuotes(s) + "'"
	}
	return fmt.Sprintf("ARRAY[%s]", strings.Join(escaped, ","))
}

func formatIntArrayDuckDB(arr []int) string {
	if len(arr) == 0 {
		return "ARRAY[]::INTEGER[]"
	}
	
	strs := make([]string, len(arr))
	for i, n := range arr {
		strs[i] = fmt.Sprintf("%d", n)
	}
	return fmt.Sprintf("ARRAY[%s]", strings.Join(strs, ","))
}

func formatNullableInt(val *int) string {
	if val == nil {
		return "NULL"
	}
	return fmt.Sprintf("%d", *val)
}

func escapeSingleQuotes(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

func parseStringArray(s string) []string {
	if s == "[]" || s == "" {
		return []string{}
	}
	// Simple parsing - in production, use proper JSON parsing
	s = strings.Trim(s, "[]")
	if s == "" {
		return []string{}
	}
	return strings.Split(s, ",")
}

func parseIntArray(s string) []int {
	if s == "[]" || s == "" {
		return []int{}
	}
	// Simple parsing - in production, use proper JSON parsing  
	s = strings.Trim(s, "[]")
	if s == "" {
		return []int{}
	}
	parts := strings.Split(s, ",")
	result := make([]int, 0, len(parts))
	for _, part := range parts {
		// Simple conversion - add error handling in production
		if val := strings.TrimSpace(part); val != "" {
			result = append(result, 0) // Placeholder
		}
	}
	return result
}

// parseInterfaceToArray converts DuckDB array results to []T using generics
func parseInterfaceToArray[T any](v interface{}, convertFunc func(interface{}) (T, bool), fallbackFunc func(string) []T) []T {
	if v == nil {
		return []T{}
	}
	
	switch arr := v.(type) {
	case []interface{}:
		result := make([]T, 0, len(arr))
		for _, item := range arr {
			if converted, ok := convertFunc(item); ok {
				result = append(result, converted)
			}
		}
		return result
	case string:
		// Fallback for old format
		return fallbackFunc(arr)
	default:
		return []T{}
	}
}

// parseInterfaceToStringArray converts DuckDB array results to []string
func parseInterfaceToStringArray(v interface{}) []string {
	return parseInterfaceToArray(v, 
		func(item interface{}) (string, bool) {
			s, ok := item.(string)
			return s, ok
		}, 
		parseStringArray)
}

// parseInterfaceToIntArray converts DuckDB array results to []int
func parseInterfaceToIntArray(v interface{}) []int {
	return parseInterfaceToArray(v,
		func(item interface{}) (int, bool) {
			switch val := item.(type) {
			case int:
				return val, true
			case int32:
				return int(val), true
			case int64:
				return int(val), true
			case float64:
				return int(val), true
			default:
				return 0, false
			}
		},
		parseIntArray)
}

// GetNodeHistory retrieves all historical entries for a specific node
func (s *Storage) GetNodeHistory(zone, net, node int) ([]database.Node, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	conn := s.db.Conn()

	// Query all entries for this node, ordered by date
	query := `
	SELECT zone, net, node, nodelist_date, day_number,
		   system_name, location, sysop_name, phone, node_type, region, max_speed,
		   is_cm, is_mo, has_binkp, has_telnet, is_down, is_hold, is_pvt, is_active,
		   flags, modem_flags, internet_protocols, internet_hostnames, internet_ports, internet_emails,
		   conflict_sequence, has_conflict, has_inet, internet_config
	FROM nodes
	WHERE zone = ? AND net = ? AND node = ?
	ORDER BY nodelist_date ASC, conflict_sequence ASC`

	rows, err := conn.Query(query, zone, net, node)
	if err != nil {
		return nil, fmt.Errorf("failed to query node history: %w", err)
	}
	defer rows.Close()

	var nodes []database.Node
	for rows.Next() {
		var n database.Node
		var flags, modemFlags, protocols, hosts, emails interface{}
		var ports interface{}
		var internetConfig sql.NullString

		err := rows.Scan(
			&n.Zone, &n.Net, &n.Node, &n.NodelistDate, &n.DayNumber,
			&n.SystemName, &n.Location, &n.SysopName, &n.Phone,
			&n.NodeType, &n.Region, &n.MaxSpeed,
			&n.IsCM, &n.IsMO, &n.HasBinkp, &n.HasTelnet,
			&n.IsDown, &n.IsHold, &n.IsPvt, &n.IsActive,
			&flags, &modemFlags, &protocols, &hosts, &ports, &emails,
			&n.ConflictSequence, &n.HasConflict,
			&n.HasInet, &internetConfig,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan node history: %w", err)
		}

		// Parse arrays from DuckDB native format
		n.Flags = parseInterfaceToStringArray(flags)
		n.ModemFlags = parseInterfaceToStringArray(modemFlags)
		n.InternetProtocols = parseInterfaceToStringArray(protocols)
		n.InternetHostnames = parseInterfaceToStringArray(hosts)
		n.InternetPorts = parseInterfaceToIntArray(ports)
		n.InternetEmails = parseInterfaceToStringArray(emails)
		
		// Handle JSON field
		if internetConfig.Valid {
			n.InternetConfig = json.RawMessage(internetConfig.String)
		}

		nodes = append(nodes, n)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return nodes, nil
}

// GetNodeDateRange returns the first and last date when a node was active
func (s *Storage) GetNodeDateRange(zone, net, node int) (firstDate, lastDate time.Time, err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	conn := s.db.Conn()

	query := `
	SELECT MIN(nodelist_date) as first_date, MAX(nodelist_date) as last_date
	FROM nodes
	WHERE zone = ? AND net = ? AND node = ?`

	row := conn.QueryRow(query, zone, net, node)
	
	err = row.Scan(&firstDate, &lastDate)
	if err != nil {
		if err == sql.ErrNoRows {
			return time.Time{}, time.Time{}, fmt.Errorf("node %d:%d/%d not found", zone, net, node)
		}
		return time.Time{}, time.Time{}, fmt.Errorf("failed to get node date range: %w", err)
	}

	return firstDate, lastDate, nil
}

// GetMaxNodelistDate returns the most recent nodelist date in the database
func (s *Storage) GetMaxNodelistDate() (time.Time, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	conn := s.db.Conn()

	var maxDate time.Time
	err := conn.QueryRow("SELECT MAX(nodelist_date) FROM nodes").Scan(&maxDate)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to get max nodelist date: %w", err)
	}

	return maxDate, nil
}

// NodeSummary represents a summary of a node for search results
type NodeSummary struct {
	Zone         int       `json:"zone"`
	Net          int       `json:"net"`
	Node         int       `json:"node"`
	SystemName   string    `json:"system_name"`
	Location     string    `json:"location"`
	SysopName    string    `json:"sysop_name"`
	FirstDate    time.Time `json:"first_date"`
	LastDate     time.Time `json:"last_date"`
	CurrentlyActive bool   `json:"currently_active"`
}

// SearchNodesBySysop finds all nodes associated with a sysop name
func (s *Storage) SearchNodesBySysop(sysopName string, limit int) ([]NodeSummary, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	conn := s.db.Conn()

	// Use ILIKE for case-insensitive search
	query := `
	WITH node_ranges AS (
		SELECT 
			zone, net, node,
			MIN(nodelist_date) as first_date,
			MAX(nodelist_date) as last_date,
			FIRST(system_name ORDER BY nodelist_date DESC) as system_name,
			FIRST(location ORDER BY nodelist_date DESC) as location,
			FIRST(sysop_name ORDER BY nodelist_date DESC) as sysop_name
		FROM nodes
		WHERE sysop_name ILIKE '%' || ? || '%'
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

	rows, err := conn.Query(query, sysopName, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to search nodes by sysop: %w", err)
	}
	defer rows.Close()

	var results []NodeSummary
	for rows.Next() {
		var ns NodeSummary
		err := rows.Scan(
			&ns.Zone, &ns.Net, &ns.Node,
			&ns.SystemName, &ns.Location, &ns.SysopName,
			&ns.FirstDate, &ns.LastDate, &ns.CurrentlyActive,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan node summary: %w", err)
		}
		results = append(results, ns)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return results, nil
}

// parseInternetConfig unmarshals JSON into InternetConfiguration struct
func parseInternetConfig(data json.RawMessage) (*database.InternetConfiguration, error) {
	if len(data) == 0 {
		return nil, nil
	}
	
	var config database.InternetConfiguration
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}
	return &config, nil
}

// detectInternetConfigChanges compares two JSON configs and returns detailed changes
func detectInternetConfigChanges(prev, curr json.RawMessage) map[string]string {
	changes := make(map[string]string)
	
	prevConfig, prevErr := parseInternetConfig(prev)
	currConfig, currErr := parseInternetConfig(curr)
	
	// Handle errors or nil configs
	if prevErr != nil || currErr != nil {
		if len(prev) > 0 && len(curr) == 0 {
			changes["internet_config"] = "Removed all internet configuration"
		} else if len(prev) == 0 && len(curr) > 0 {
			changes["internet_config"] = "Added internet configuration"
		}
		return changes
	}
	
	if prevConfig == nil && currConfig == nil {
		return changes
	}
	
	if prevConfig == nil && currConfig != nil {
		// New config added
		for proto, detail := range currConfig.Protocols {
			if detail.Port > 0 {
				changes[fmt.Sprintf("inet_%s", proto)] = fmt.Sprintf("Added %s:%d", detail.Address, detail.Port)
			} else {
				changes[fmt.Sprintf("inet_%s", proto)] = fmt.Sprintf("Added %s", detail.Address)
			}
		}
		for key, val := range currConfig.Defaults {
			changes[fmt.Sprintf("inet_%s", key)] = fmt.Sprintf("Added %s", val)
		}
		return changes
	}
	
	if prevConfig != nil && currConfig == nil {
		// Config removed
		for proto, detail := range prevConfig.Protocols {
			if detail.Port > 0 {
				changes[fmt.Sprintf("inet_%s", proto)] = fmt.Sprintf("Removed %s:%d", detail.Address, detail.Port)
			} else {
				changes[fmt.Sprintf("inet_%s", proto)] = fmt.Sprintf("Removed %s", detail.Address)
			}
		}
		return changes
	}
	
	// Compare protocols
	for proto, currDetail := range currConfig.Protocols {
		prevDetail, existed := prevConfig.Protocols[proto]
		if !existed {
			if currDetail.Port > 0 {
				changes[fmt.Sprintf("inet_%s", proto)] = fmt.Sprintf("Added %s:%d", currDetail.Address, currDetail.Port)
			} else {
				changes[fmt.Sprintf("inet_%s", proto)] = fmt.Sprintf("Added %s", currDetail.Address)
			}
		} else if prevDetail.Address != currDetail.Address || prevDetail.Port != currDetail.Port {
			oldStr := prevDetail.Address
			newStr := currDetail.Address
			if prevDetail.Port > 0 {
				oldStr = fmt.Sprintf("%s:%d", prevDetail.Address, prevDetail.Port)
			}
			if currDetail.Port > 0 {
				newStr = fmt.Sprintf("%s:%d", currDetail.Address, currDetail.Port)
			}
			changes[fmt.Sprintf("inet_%s", proto)] = fmt.Sprintf("%s → %s", oldStr, newStr)
		}
	}
	
	// Check for removed protocols
	for proto, prevDetail := range prevConfig.Protocols {
		if _, exists := currConfig.Protocols[proto]; !exists {
			if prevDetail.Port > 0 {
				changes[fmt.Sprintf("inet_%s", proto)] = fmt.Sprintf("Removed %s:%d", prevDetail.Address, prevDetail.Port)
			} else {
				changes[fmt.Sprintf("inet_%s", proto)] = fmt.Sprintf("Removed %s", prevDetail.Address)
			}
		}
	}
	
	// Compare defaults
	for key, currVal := range currConfig.Defaults {
		prevVal, existed := prevConfig.Defaults[key]
		if !existed {
			changes[fmt.Sprintf("inet_%s", key)] = fmt.Sprintf("Added %s", currVal)
		} else if prevVal != currVal {
			changes[fmt.Sprintf("inet_%s", key)] = fmt.Sprintf("%s → %s", prevVal, currVal)
		}
	}
	
	// Check for removed defaults
	for key, prevVal := range prevConfig.Defaults {
		if _, exists := currConfig.Defaults[key]; !exists {
			changes[fmt.Sprintf("inet_%s", key)] = fmt.Sprintf("Removed %s", prevVal)
		}
	}
	
	// Compare email protocols
	for proto, currDetail := range currConfig.EmailProtocols {
		prevDetail, existed := prevConfig.EmailProtocols[proto]
		if !existed {
			if currDetail.Email != "" {
				changes[fmt.Sprintf("inet_%s", proto)] = fmt.Sprintf("Added %s", currDetail.Email)
			} else {
				changes[fmt.Sprintf("inet_%s", proto)] = "Added (uses default email)"
			}
		} else if prevDetail.Email != currDetail.Email {
			if currDetail.Email != "" {
				changes[fmt.Sprintf("inet_%s", proto)] = fmt.Sprintf("%s → %s", prevDetail.Email, currDetail.Email)
			}
		}
	}
	
	// Check for removed email protocols
	for proto, prevDetail := range prevConfig.EmailProtocols {
		if _, exists := currConfig.EmailProtocols[proto]; !exists {
			if prevDetail.Email != "" {
				changes[fmt.Sprintf("inet_%s", proto)] = fmt.Sprintf("Removed %s", prevDetail.Email)
			} else {
				changes[fmt.Sprintf("inet_%s", proto)] = "Removed"
			}
		}
	}
	
	// Compare info flags
	prevFlags := make(map[string]bool)
	currFlags := make(map[string]bool)
	
	for _, flag := range prevConfig.InfoFlags {
		prevFlags[flag] = true
	}
	for _, flag := range currConfig.InfoFlags {
		currFlags[flag] = true
	}
	
	for flag := range currFlags {
		if !prevFlags[flag] {
			changes[fmt.Sprintf("inet_flag_%s", flag)] = "Added"
		}
	}
	for flag := range prevFlags {
		if !currFlags[flag] {
			changes[fmt.Sprintf("inet_flag_%s", flag)] = "Removed"
		}
	}
	
	return changes
}

// GetNodeChanges analyzes the history of a node and returns detected changes
func (s *Storage) GetNodeChanges(zone, net, node int, filter ChangeFilter) ([]database.NodeChange, error) {
	// Get all historical entries
	history, err := s.GetNodeHistory(zone, net, node)
	if err != nil {
		return nil, err
	}

	if len(history) == 0 {
		return nil, nil
	}

	var changes []database.NodeChange
	
	// Add the first appearance
	changes = append(changes, database.NodeChange{
		Date:       history[0].NodelistDate,
		DayNumber:  history[0].DayNumber,
		ChangeType: "added",
		Changes:    make(map[string]string),
		NewNode:    &history[0],
	})

	// Track changes between consecutive entries
	for i := 1; i < len(history); i++ {
		prev := &history[i-1]
		curr := &history[i]

		// Skip if same date but different conflict sequence
		if prev.NodelistDate.Equal(curr.NodelistDate) {
			continue
		}

		// Check if node was removed (gap in dates)
		if !isConsecutiveNodelist(prev.NodelistDate, curr.NodelistDate, s) {
			// Node was removed and then re-added
			changes = append(changes, database.NodeChange{
				Date:       getNextNodelistDate(prev.NodelistDate, s),
				DayNumber:  getNextDayNumber(prev.DayNumber, s),
				ChangeType: "removed",
				Changes:    make(map[string]string),
				OldNode:    prev,
			})
			
			changes = append(changes, database.NodeChange{
				Date:       curr.NodelistDate,
				DayNumber:  curr.DayNumber,
				ChangeType: "added",
				Changes:    make(map[string]string),
				NewNode:    curr,
			})
			continue
		}

		// Detect field changes
		fieldChanges := make(map[string]string)

		if !filter.IgnoreStatus && prev.NodeType != curr.NodeType {
			fieldChanges["status"] = fmt.Sprintf("%s → %s", prev.NodeType, curr.NodeType)
		}
		if !filter.IgnoreName && prev.SystemName != curr.SystemName {
			fieldChanges["name"] = fmt.Sprintf("%s → %s", prev.SystemName, curr.SystemName)
		}
		if !filter.IgnoreLocation && prev.Location != curr.Location {
			fieldChanges["location"] = fmt.Sprintf("%s → %s", prev.Location, curr.Location)
		}
		if !filter.IgnoreSysop && prev.SysopName != curr.SysopName {
			fieldChanges["sysop"] = fmt.Sprintf("%s → %s", prev.SysopName, curr.SysopName)
		}
		if !filter.IgnorePhone && prev.Phone != curr.Phone {
			fieldChanges["phone"] = fmt.Sprintf("%s → %s", prev.Phone, curr.Phone)
		}
		if !filter.IgnoreSpeed && prev.MaxSpeed != curr.MaxSpeed {
			fieldChanges["speed"] = fmt.Sprintf("%s → %s", prev.MaxSpeed, curr.MaxSpeed)
		}
		if !filter.IgnoreFlags && !equalStringSlices(prev.Flags, curr.Flags) {
			fieldChanges["flags"] = fmt.Sprintf("%v → %v", prev.Flags, curr.Flags)
		}
		
		// Internet connectivity changes
		if !filter.IgnoreConnectivity {
			if prev.HasBinkp != curr.HasBinkp {
				fieldChanges["binkp"] = fmt.Sprintf("%t → %t", prev.HasBinkp, curr.HasBinkp)
			}
			if prev.HasTelnet != curr.HasTelnet {
				fieldChanges["telnet"] = fmt.Sprintf("%t → %t", prev.HasTelnet, curr.HasTelnet)
			}
		}
		
		if !filter.IgnoreModemFlags && !equalStringSlices(prev.ModemFlags, curr.ModemFlags) {
			fieldChanges["modem_flags"] = fmt.Sprintf("%v → %v", prev.ModemFlags, curr.ModemFlags)
		}
		
		if !filter.IgnoreInternetProtocols && !equalStringSlices(prev.InternetProtocols, curr.InternetProtocols) {
			fieldChanges["internet_protocols"] = fmt.Sprintf("%v → %v", prev.InternetProtocols, curr.InternetProtocols)
		}
		
		if !filter.IgnoreInternetHostnames && !equalStringSlices(prev.InternetHostnames, curr.InternetHostnames) {
			fieldChanges["internet_hostnames"] = fmt.Sprintf("%v → %v", prev.InternetHostnames, curr.InternetHostnames)
		}
		
		if !filter.IgnoreInternetPorts && !equalIntSlices(prev.InternetPorts, curr.InternetPorts) {
			fieldChanges["internet_ports"] = fmt.Sprintf("%v → %v", prev.InternetPorts, curr.InternetPorts)
		}
		
		if !filter.IgnoreInternetEmails && !equalStringSlices(prev.InternetEmails, curr.InternetEmails) {
			fieldChanges["internet_emails"] = fmt.Sprintf("%v → %v", prev.InternetEmails, curr.InternetEmails)
		}
		
		// Check has_inet changes
		if !filter.IgnoreConnectivity && prev.HasInet != curr.HasInet {
			fieldChanges["has_inet"] = fmt.Sprintf("%t → %t", prev.HasInet, curr.HasInet)
		}
		
		// Detect internet config changes using new JSON-based detection
		if !filter.IgnoreConnectivity || !filter.IgnoreInternetProtocols || !filter.IgnoreInternetHostnames {
			configChanges := detectInternetConfigChanges(prev.InternetConfig, curr.InternetConfig)
			for key, value := range configChanges {
				fieldChanges[key] = value
			}
		}

		if len(fieldChanges) > 0 {
			changes = append(changes, database.NodeChange{
				Date:       curr.NodelistDate,
				DayNumber:  curr.DayNumber,
				ChangeType: "modified",
				Changes:    fieldChanges,
				OldNode:    prev,
				NewNode:    curr,
			})
		}
	}

	// Check if node is currently removed
	lastNode := &history[len(history)-1]
	if !isCurrentlyActive(lastNode, s) {
		changes = append(changes, database.NodeChange{
			Date:       getNextNodelistDate(lastNode.NodelistDate, s),
			DayNumber:  getNextDayNumber(lastNode.DayNumber, s),
			ChangeType: "removed",
			Changes:    make(map[string]string),
			OldNode:    lastNode,
		})
	}

	return changes, nil
}

// ChangeFilter allows filtering out specific types of changes
type ChangeFilter struct {
	IgnoreFlags              bool
	IgnorePhone              bool
	IgnoreSpeed              bool
	IgnoreStatus             bool
	IgnoreLocation           bool
	IgnoreName               bool
	IgnoreSysop              bool
	IgnoreConnectivity       bool // Binkp, Telnet capabilities
	IgnoreInternetProtocols  bool
	IgnoreInternetHostnames  bool
	IgnoreInternetPorts      bool
	IgnoreInternetEmails     bool
	IgnoreModemFlags         bool
}

// Helper functions
func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalIntSlices(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func isConsecutiveNodelist(date1, date2 time.Time, s *Storage) bool {
	// Check if there's a nodelist between these two dates
	conn := s.db.Conn()
	var count int
	err := conn.QueryRow(
		"SELECT COUNT(DISTINCT nodelist_date) FROM nodes WHERE nodelist_date > ? AND nodelist_date < ?",
		date1, date2,
	).Scan(&count)
	
	return err == nil && count == 0
}

func getNextNodelistDate(afterDate time.Time, s *Storage) time.Time {
	conn := s.db.Conn()
	var nextDate time.Time
	err := conn.QueryRow(
		"SELECT MIN(nodelist_date) FROM nodes WHERE nodelist_date > ?",
		afterDate,
	).Scan(&nextDate)
	
	if err != nil {
		return afterDate.AddDate(0, 0, 7) // Assume weekly
	}
	return nextDate
}

func getNextDayNumber(afterDay int, s *Storage) int {
	// Simple increment, could be improved with actual lookup
	return afterDay + 7
}

func isCurrentlyActive(node *database.Node, s *Storage) bool {
	conn := s.db.Conn()
	var maxDate time.Time
	err := conn.QueryRow("SELECT MAX(nodelist_date) FROM nodes").Scan(&maxDate)
	
	if err != nil {
		return false
	}
	
	return node.NodelistDate.Equal(maxDate)
}