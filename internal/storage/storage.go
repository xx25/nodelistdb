package storage

import (
	"database/sql"
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
		raw_line, file_path, file_crc, first_seen, last_seen,
		conflict_sequence, has_conflict
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

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
		   raw_line, file_path, file_crc, first_seen, last_seen
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

// InsertNodes inserts a batch of nodes using bulk insert for better performance
func (s *Storage) InsertNodes(nodes []database.Node) error {
	if len(nodes) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	conn := s.db.Conn()

	// Start transaction for bulk operation
	tx, err := conn.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Build bulk insert with UPSERT for conflict handling
	insertSQL := `
	INSERT INTO nodes (
		zone, net, node, nodelist_date, day_number,
		system_name, location, sysop_name, phone, node_type, region, max_speed,
		is_cm, is_mo, has_binkp, has_telnet, is_down, is_hold, is_pvt, is_active,
		flags, modem_flags, internet_protocols, internet_hostnames, internet_ports, internet_emails,
		raw_line, file_path, file_crc, first_seen, last_seen,
		conflict_sequence, has_conflict
	) VALUES `

	var valuePlaceholders []string
	var args []interface{}

	for i, node := range nodes {
		valuePlaceholders = append(valuePlaceholders, "(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)")
		
		args = append(args,
			node.Zone, node.Net, node.Node, node.NodelistDate, node.DayNumber,
			node.SystemName, node.Location, node.SysopName, node.Phone,
			node.NodeType, node.Region, node.MaxSpeed,
			node.IsCM, node.IsMO, node.HasBinkp, node.HasTelnet,
			node.IsDown, node.IsHold, node.IsPvt, node.IsActive,
			formatStringArray(node.Flags), formatStringArray(node.ModemFlags),
			formatStringArray(node.InternetProtocols), formatStringArray(node.InternetHostnames),
			formatIntArray(node.InternetPorts), formatStringArray(node.InternetEmails),
			node.RawLine, node.FilePath, node.FileCRC, node.FirstSeen, node.LastSeen,
			0, false, // conflict_sequence=0, has_conflict=false for initial insert
		)

		// Split into chunks to avoid query size limits
		if (i+1)%500 == 0 || i == len(nodes)-1 {
			fullSQL := insertSQL + strings.Join(valuePlaceholders, ",") +
				` ON CONFLICT (zone, net, node, nodelist_date, conflict_sequence) 
				  DO NOTHING`
			
			_, err := tx.Exec(fullSQL, args...)
			if err != nil {
				return fmt.Errorf("failed to bulk insert nodes: %w", err)
			}
			
			// Reset for next chunk
			valuePlaceholders = nil
			args = nil
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
		   raw_line, file_path, file_crc, first_seen, last_seen,
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
		var flagsStr, modemFlagsStr, protocolsStr, hostsStr, portsStr, emailsStr string

		err := rows.Scan(
			&node.Zone, &node.Net, &node.Node, &node.NodelistDate, &node.DayNumber,
			&node.SystemName, &node.Location, &node.SysopName, &node.Phone,
			&node.NodeType, &node.Region, &node.MaxSpeed,
			&node.IsCM, &node.IsMO, &node.HasBinkp, &node.HasTelnet,
			&node.IsDown, &node.IsHold, &node.IsPvt, &node.IsActive,
			&flagsStr, &modemFlagsStr, &protocolsStr, &hostsStr, &portsStr, &emailsStr,
			&node.RawLine, &node.FilePath, &node.FileCRC, &node.FirstSeen, &node.LastSeen,
			&node.ConflictSequence, &node.HasConflict,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan node: %w", err)
		}

		// Parse arrays from DuckDB format
		node.Flags = parseStringArray(flagsStr)
		node.ModemFlags = parseStringArray(modemFlagsStr)
		node.InternetProtocols = parseStringArray(protocolsStr)
		node.InternetHostnames = parseStringArray(hostsStr)
		node.InternetPorts = parseIntArray(portsStr)
		node.InternetEmails = parseStringArray(emailsStr)

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

// IsFileProcessed checks if a file has already been processed based on path and CRC
func (s *Storage) IsFileProcessed(filePath string, fileCRC int) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	conn := s.db.Conn()

	var count int
	err := conn.QueryRow(
		"SELECT COUNT(*) FROM nodes WHERE file_path = ? AND file_crc = ?", 
		filePath, fileCRC,
	).Scan(&count)
	
	if err != nil {
		return false, fmt.Errorf("failed to check if file is processed: %w", err)
	}

	return count > 0, nil
}

// Helper functions for array formatting (DuckDB array handling)
func formatStringArray(arr []string) string {
	if len(arr) == 0 {
		return "[]"
	}
	return "['" + strings.Join(arr, "','") + "']"
}

func formatIntArray(arr []int) string {
	if len(arr) == 0 {
		return "[]"
	}
	strs := make([]string, len(arr))
	for i, v := range arr {
		strs[i] = fmt.Sprintf("%d", v)
	}
	return "[" + strings.Join(strs, ",") + "]"
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