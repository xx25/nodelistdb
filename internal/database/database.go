package database

import (
	"database/sql"
	"fmt"
	"strings"
	"sync"

	_ "github.com/marcboeker/go-duckdb"
)

// DB wraps a DuckDB connection with thread-safety
type DB struct {
	conn *sql.DB
	mu   sync.RWMutex
}

// New creates a new DuckDB connection
func New(path string) (*DB, error) {
	// Configure DuckDB connection string with optimizations
	dsn := fmt.Sprintf("%s?memory_limit=8GB&threads=4", path)
	
	conn, err := sql.Open("duckdb", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open DuckDB connection: %w", err)
	}

	// Test connection
	if err := conn.Ping(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to ping DuckDB: %w", err)
	}

	db := &DB{
		conn: conn,
	}

	return db, nil
}

// NewReadOnly creates a new read-only DuckDB connection
func NewReadOnly(path string) (*DB, error) {
	// Configure DuckDB connection string with read-only access
	dsn := fmt.Sprintf("%s?access_mode=read_only&memory_limit=8GB&threads=4", path)
	
	conn, err := sql.Open("duckdb", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open read-only DuckDB connection: %w", err)
	}

	// Test connection
	if err := conn.Ping(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to ping DuckDB: %w", err)
	}

	db := &DB{
		conn: conn,
	}

	return db, nil
}

// Close closes the database connection
func (db *DB) Close() error {
	db.mu.Lock()
	defer db.mu.Unlock()
	
	if db.conn != nil {
		return db.conn.Close()
	}
	return nil
}

// Conn returns the underlying sql.DB connection (thread-safe)
func (db *DB) Conn() *sql.DB {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.conn
}

// CreateSchema creates the database schema
func (db *DB) CreateSchema() error {
	db.mu.Lock()
	defer db.mu.Unlock()

	// Create nodes table optimized for DuckDB
	createSQL := `
	CREATE TABLE nodes (
		zone INTEGER NOT NULL,
		net INTEGER NOT NULL, 
		node INTEGER NOT NULL,
		nodelist_date DATE NOT NULL,
		day_number INTEGER NOT NULL,
		system_name TEXT NOT NULL,
		location TEXT NOT NULL,
		sysop_name TEXT NOT NULL,
		phone TEXT NOT NULL,
		node_type TEXT NOT NULL,
		region INTEGER,
		max_speed TEXT NOT NULL,
		
		-- Boolean flags (computed from raw flags)
		is_cm BOOLEAN DEFAULT FALSE,
		is_mo BOOLEAN DEFAULT FALSE,
		has_binkp BOOLEAN DEFAULT FALSE,
		has_telnet BOOLEAN DEFAULT FALSE,
		is_down BOOLEAN DEFAULT FALSE,
		is_hold BOOLEAN DEFAULT FALSE,
		is_pvt BOOLEAN DEFAULT FALSE,
		is_active BOOLEAN DEFAULT TRUE,
		
		-- Arrays for flexibility (DuckDB native arrays)
		flags TEXT[] DEFAULT [],
		modem_flags TEXT[] DEFAULT [],
		internet_protocols TEXT[] DEFAULT [],
		internet_hostnames TEXT[] DEFAULT [],
		internet_ports INTEGER[] DEFAULT [],
		internet_emails TEXT[] DEFAULT [],
		
		-- Internet connectivity analysis
		has_inet BOOLEAN DEFAULT FALSE,       -- Any internet connectivity
		internet_config JSON,                 -- JSON configuration for internet settings
		
		-- Conflict tracking
		conflict_sequence INTEGER DEFAULT 0,  -- 0 = original, 1+ = conflict duplicates
		has_conflict BOOLEAN DEFAULT FALSE,   -- Flag for easy querying of conflicts
		
		PRIMARY KEY (zone, net, node, nodelist_date, conflict_sequence)
	)`

	if _, err := db.conn.Exec(createSQL); err != nil {
		// If table already exists, that's fine
		if !strings.Contains(err.Error(), "already exists") {
			return fmt.Errorf("failed to create nodes table: %w", err)
		}
	}

	// Create optimized indexes for DuckDB
	indexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_nodes_date ON nodes(nodelist_date)",
		"CREATE INDEX IF NOT EXISTS idx_nodes_location ON nodes(zone, net)",
		"CREATE INDEX IF NOT EXISTS idx_nodes_system ON nodes(system_name)",
		"CREATE INDEX IF NOT EXISTS idx_nodes_type ON nodes(node_type)",
		"CREATE INDEX IF NOT EXISTS idx_nodes_flags ON nodes(is_cm, is_mo, has_binkp, has_telnet, has_inet)",
		// Performance optimizations for stats queries
		"CREATE INDEX IF NOT EXISTS idx_nodes_date_zone ON nodes(nodelist_date, zone)",
		"CREATE INDEX IF NOT EXISTS idx_nodes_date_region ON nodes(nodelist_date, zone, region)",
		"CREATE INDEX IF NOT EXISTS idx_nodes_date_net_type ON nodes(nodelist_date, zone, net, node_type)",
		"CREATE INDEX IF NOT EXISTS idx_nodes_stats_flags ON nodes(nodelist_date, is_active, is_down, is_hold, is_cm, is_mo, has_binkp, has_telnet, is_pvt)",
	}

	for _, indexSQL := range indexes {
		if _, err := db.conn.Exec(indexSQL); err != nil {
			// Ignore index already exists errors
			if !strings.Contains(err.Error(), "already exists") {
				return fmt.Errorf("failed to create index: %w", err)
			}
		}
	}

	return nil
}

// GetVersion returns the DuckDB version
func (db *DB) GetVersion() (string, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	var version string
	err := db.conn.QueryRow("SELECT version()").Scan(&version)
	if err != nil {
		return "", fmt.Errorf("failed to get DuckDB version: %w", err)
	}

	return version, nil
}