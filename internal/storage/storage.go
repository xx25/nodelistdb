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
		conflict_sequence, has_conflict
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

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
		   flags, modem_flags, internet_protocols, internet_hostnames, internet_ports, internet_emails
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
func (s *Storage) InsertNodes(nodes []database.Node) error {
	if len(nodes) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	conn := s.db.Conn()

	// Start transaction
	tx, err := conn.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// For DuckDB v1.1.3, use direct SQL execution with native array syntax
	// This is more efficient than building huge parameter lists
	
	// Process in smaller chunks to optimize memory usage
	chunkSize := 100
	for i := 0; i < len(nodes); i += chunkSize {
		end := i + chunkSize
		if end > len(nodes) {
			end = len(nodes)
		}
		chunk := nodes[i:end]
		
		// Build VALUES clause with DuckDB native syntax
		var values []string
		for _, node := range chunk {
			value := fmt.Sprintf(
				"(%d,%d,%d,'%s',%d,'%s','%s','%s','%s','%s',%s,'%s',%t,%t,%t,%t,%t,%t,%t,%t,%s,%s,%s,%s,%s,%s,%d,%t)",
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
			)
			values = append(values, value)
		}
		
		// Execute chunk insert
		insertSQL := fmt.Sprintf(`
			INSERT INTO nodes (
				zone, net, node, nodelist_date, day_number,
				system_name, location, sysop_name, phone, node_type, region, max_speed,
				is_cm, is_mo, has_binkp, has_telnet, is_down, is_hold, is_pvt, is_active,
				flags, modem_flags, internet_protocols, internet_hostnames, internet_ports, internet_emails,
				conflict_sequence, has_conflict
			) VALUES %s
			ON CONFLICT (zone, net, node, nodelist_date, conflict_sequence) 
			DO NOTHING
		`, strings.Join(values, ","))
		
		_, err := tx.Exec(insertSQL)
		if err != nil {
			return fmt.Errorf("failed to insert chunk: %w", err)
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
func (s *Storage) GetNodes(filter database.NodeFilter) ([]database.Node, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	conn := s.db.Conn()

	// Build dynamic query based on filter
	baseSQL := `
	SELECT zone, net, node, nodelist_date, day_number,
		   system_name, location, sysop_name, phone, node_type, region, max_speed,
		   is_cm, is_mo, has_binkp, has_telnet, is_down, is_hold, is_pvt, is_active,
		   flags, modem_flags, internet_protocols, internet_hostnames, internet_ports, internet_emails,
		   conflict_sequence, has_conflict
	FROM nodes WHERE 1=1`

	var conditions []string
	var args []interface{}

	// Add filter conditions
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

	// Build final query
	if len(conditions) > 0 {
		baseSQL += " AND " + strings.Join(conditions, " AND ")
	}

	baseSQL += " ORDER BY zone, net, node, nodelist_date DESC"

	// Add pagination
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
		var node database.Node
		var flags, modemFlags, protocols, hosts, emails interface{}
		var ports interface{}

		err := rows.Scan(
			&node.Zone, &node.Net, &node.Node, &node.NodelistDate, &node.DayNumber,
			&node.SystemName, &node.Location, &node.SysopName, &node.Phone,
			&node.NodeType, &node.Region, &node.MaxSpeed,
			&node.IsCM, &node.IsMO, &node.HasBinkp, &node.HasTelnet,
			&node.IsDown, &node.IsHold, &node.IsPvt, &node.IsActive,
			&flags, &modemFlags, &protocols, &hosts, &ports, &emails,
			&node.ConflictSequence, &node.HasConflict,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan node: %w", err)
		}

		// Parse arrays from DuckDB native format
		node.Flags = parseInterfaceToStringArray(flags)
		node.ModemFlags = parseInterfaceToStringArray(modemFlags)
		node.InternetProtocols = parseInterfaceToStringArray(protocols)
		node.InternetHostnames = parseInterfaceToStringArray(hosts)
		node.InternetPorts = parseInterfaceToIntArray(ports)
		node.InternetEmails = parseInterfaceToStringArray(emails)

		nodes = append(nodes, node)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return nodes, nil
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

// parseInterfaceToStringArray converts DuckDB array results to []string
func parseInterfaceToStringArray(v interface{}) []string {
	if v == nil {
		return []string{}
	}
	
	switch arr := v.(type) {
	case []interface{}:
		result := make([]string, 0, len(arr))
		for _, item := range arr {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	case string:
		// Fallback for old format
		return parseStringArray(arr)
	default:
		return []string{}
	}
}

// parseInterfaceToIntArray converts DuckDB array results to []int
func parseInterfaceToIntArray(v interface{}) []int {
	if v == nil {
		return []int{}
	}
	
	switch arr := v.(type) {
	case []interface{}:
		result := make([]int, 0, len(arr))
		for _, item := range arr {
			switch val := item.(type) {
			case int:
				result = append(result, val)
			case int32:
				result = append(result, int(val))
			case int64:
				result = append(result, int(val))
			case float64:
				result = append(result, int(val))
			}
		}
		return result
	case string:
		// Fallback for old format
		return parseIntArray(arr)
	default:
		return []int{}
	}
}