package database

import (
	"database/sql"
	"fmt"
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

// Migrate creates the database schema
func (db *DB) Migrate() error {
	db.mu.Lock()
	defer db.mu.Unlock()

	// Drop existing table for clean migration
	dropSQL := `DROP TABLE IF EXISTS nodes`
	if _, err := db.conn.Exec(dropSQL); err != nil {
		return fmt.Errorf("failed to drop existing table: %w", err)
	}

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
		
		-- Metadata
		raw_line TEXT NOT NULL,
		file_path TEXT NOT NULL,
		file_crc INTEGER NOT NULL,
		first_seen TIMESTAMP NOT NULL,
		last_seen TIMESTAMP NOT NULL,
		
		-- Conflict tracking
		conflict_sequence INTEGER DEFAULT 0,  -- 0 = original, 1+ = conflict duplicates
		has_conflict BOOLEAN DEFAULT FALSE,   -- Flag for easy querying of conflicts
		
		PRIMARY KEY (zone, net, node, nodelist_date, conflict_sequence)
	)`

	if _, err := db.conn.Exec(createSQL); err != nil {
		return fmt.Errorf("failed to create nodes table: %w", err)
	}

	// Create optimized indexes for DuckDB
	indexes := []string{
		"CREATE INDEX idx_nodes_date ON nodes(nodelist_date)",
		"CREATE INDEX idx_nodes_location ON nodes(zone, net)",
		"CREATE INDEX idx_nodes_system ON nodes(system_name)",
		"CREATE INDEX idx_nodes_type ON nodes(node_type)",
		"CREATE INDEX idx_nodes_flags ON nodes(is_cm, is_mo, has_binkp, has_telnet)",
	}

	for _, indexSQL := range indexes {
		if _, err := db.conn.Exec(indexSQL); err != nil {
			return fmt.Errorf("failed to create index: %w", err)
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