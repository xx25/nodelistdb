package database

import (
	"context"
	"crypto/tls"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

// ClickHouseDB wraps a ClickHouse connection with thread-safety
type ClickHouseDB struct {
	conn   driver.Conn
	sqlDB  *sql.DB // For compatibility with existing code
	mu     sync.RWMutex
	config *ClickHouseConfig
}

// ClickHouseConfig holds ClickHouse connection configuration
type ClickHouseConfig struct {
	Host         string
	Port         int
	Database     string
	Username     string
	Password     string
	UseSSL       bool
	MaxOpenConns int
	MaxIdleConns int
	DialTimeout  time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	Compression  string
}

// NewClickHouse creates a new ClickHouse connection
func NewClickHouse(config *ClickHouseConfig) (*ClickHouseDB, error) {
	// Create ClickHouse options
	// IMPORTANT: Do NOT set MaxOpenConns/MaxIdleConns in Options due to driver bug
	// They must be set on the sql.DB after OpenDB
	options := &clickhouse.Options{
		Addr: []string{fmt.Sprintf("%s:%d", config.Host, config.Port)},
		Auth: clickhouse.Auth{
			Database: config.Database,
			Username: config.Username,
			Password: config.Password,
		},
		DialTimeout: config.DialTimeout,
		Compression: &clickhouse.Compression{
			Method: clickhouse.CompressionLZ4,
		},
	}

	// Configure TLS if enabled
	if config.UseSSL {
		options.TLS = &tls.Config{}
	}

	// Create native connection
	conn, err := clickhouse.Open(options)
	if err != nil {
		return nil, fmt.Errorf("failed to open ClickHouse connection: %w", err)
	}

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := conn.Ping(ctx); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to ping ClickHouse: %w", err)
	}

	// Create SQL DB for compatibility
	sqlDB := clickhouse.OpenDB(options)

	// CRITICAL: Set pool settings AFTER OpenDB due to driver bug
	// If MaxOpenConns/MaxIdleConns are in Options, the driver fails with "invalid settings"
	sqlDB.SetMaxOpenConns(config.MaxOpenConns)
	sqlDB.SetMaxIdleConns(config.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(time.Hour)

	db := &ClickHouseDB{
		conn:   conn,
		sqlDB:  sqlDB,
		config: config,
	}

	// Register this database type
	RegisterDatabase("clickhouse", func(config interface{}) (DatabaseInterface, error) {
		chConfig, ok := config.(*ClickHouseConfig)
		if !ok {
			return nil, fmt.Errorf("clickhouse config must be *ClickHouseConfig")
		}
		return NewClickHouse(chConfig)
	})

	return db, nil
}

// NewClickHouseReadOnly creates a new read-only ClickHouse connection
func NewClickHouseReadOnly(config *ClickHouseConfig) (*ClickHouseDB, error) {
	// ClickHouse doesn't have a read-only mode like DuckDB, but we can
	// create a connection with limited permissions or use a read-only user
	return NewClickHouse(config)
}

// Close closes the ClickHouse connection
func (db *ClickHouseDB) Close() error {
	db.mu.Lock()
	defer db.mu.Unlock()

	var err error
	if db.sqlDB != nil {
		if sqlErr := db.sqlDB.Close(); sqlErr != nil {
			err = sqlErr
		}
	}

	if db.conn != nil {
		if connErr := db.conn.Close(); connErr != nil && err == nil {
			err = connErr
		}
	}

	return err
}

// Conn returns a sql.DB connection for compatibility
func (db *ClickHouseDB) Conn() *sql.DB {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.sqlDB
}

// NativeConn returns the native ClickHouse connection
func (db *ClickHouseDB) NativeConn() driver.Conn {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.conn
}

// CreateSchema creates the ClickHouse database schema
func (db *ClickHouseDB) CreateSchema() error {
	db.mu.Lock()
	defer db.mu.Unlock()

	ctx := context.Background()

	// Create nodes table optimized for ClickHouse
	createSQL := `
	CREATE TABLE IF NOT EXISTS nodes (
		zone Int32,
		net Int32,
		node Int32,
		nodelist_date Date,
		day_number Int32,
		system_name String,
		location String,
		sysop_name String,
		phone String,
		node_type LowCardinality(String),
		region Nullable(Int32),
		max_speed UInt32 DEFAULT 0,
		
		-- Boolean flags (computed from raw flags)
		is_cm Bool DEFAULT false,
		is_mo Bool DEFAULT false,
		has_binkp Bool DEFAULT false,
		has_telnet Bool DEFAULT false,
		is_down Bool DEFAULT false,
		is_hold Bool DEFAULT false,
		is_pvt Bool DEFAULT false,
		is_active Bool DEFAULT true,
		
		-- Arrays for flexibility (ClickHouse native arrays)
		flags Array(String) DEFAULT [],
		modem_flags Array(String) DEFAULT [],
		internet_protocols Array(String) DEFAULT [],
		internet_hostnames Array(String) DEFAULT [],
		internet_ports Array(Int32) DEFAULT [],
		internet_emails Array(String) DEFAULT [],
		
		-- Internet connectivity analysis
		has_inet Bool DEFAULT false,
		internet_config JSON DEFAULT '{}',
		
		-- Conflict tracking
		conflict_sequence Int32 DEFAULT 0,
		has_conflict Bool DEFAULT false,
		
		-- FTS unique identifier
		fts_id String,
		
		-- Materialized columns for optimized case-insensitive searches
		location_lower String MATERIALIZED lower(location),
		sysop_name_lower String MATERIALIZED lower(sysop_name),
		system_name_lower String MATERIALIZED lower(system_name)
	) ENGINE = MergeTree()
	ORDER BY (zone, net, node, nodelist_date, conflict_sequence)
	PARTITION BY toYYYYMM(nodelist_date)
	SETTINGS index_granularity = 8192`

	if err := db.conn.Exec(ctx, createSQL); err != nil {
		return fmt.Errorf("failed to create nodes table: %w", err)
	}

	// Create optimized indexes for ClickHouse
	indexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_nodes_date ON nodes(nodelist_date) TYPE minmax GRANULARITY 1",
		"CREATE INDEX IF NOT EXISTS idx_nodes_system ON nodes(system_name) TYPE bloom_filter GRANULARITY 1",
		"CREATE INDEX IF NOT EXISTS idx_nodes_location ON nodes(location) TYPE bloom_filter GRANULARITY 1",
		"CREATE INDEX IF NOT EXISTS idx_nodes_sysop ON nodes(sysop_name) TYPE bloom_filter GRANULARITY 1",
		"CREATE INDEX IF NOT EXISTS idx_nodes_type ON nodes(node_type) TYPE set(100) GRANULARITY 1",
		"CREATE INDEX IF NOT EXISTS idx_nodes_fts_id ON nodes(fts_id) TYPE bloom_filter GRANULARITY 1",
		// Optimized indexes for materialized lowercase columns
		"CREATE INDEX IF NOT EXISTS idx_location_lower_bloom ON nodes(location_lower) TYPE bloom_filter GRANULARITY 1",
		"CREATE INDEX IF NOT EXISTS idx_sysop_lower_bloom ON nodes(sysop_name_lower) TYPE bloom_filter GRANULARITY 1",
		"CREATE INDEX IF NOT EXISTS idx_system_lower_bloom ON nodes(system_name_lower) TYPE bloom_filter GRANULARITY 1",
	}

	for _, indexSQL := range indexes {
		if err := db.conn.Exec(ctx, indexSQL); err != nil {
			// ClickHouse may not support all index types, so we log but continue
			fmt.Printf("Warning: Could not create index: %v\n", err)
		}
	}

	return nil
}

// GetVersion returns the ClickHouse version
func (db *ClickHouseDB) GetVersion() (string, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	ctx := context.Background()
	var version string

	row := db.conn.QueryRow(ctx, "SELECT version()")
	if err := row.Scan(&version); err != nil {
		return "", fmt.Errorf("failed to get ClickHouse version: %w", err)
	}

	return version, nil
}

// Ping tests the database connection
func (db *ClickHouseDB) Ping() error {
	db.mu.RLock()
	defer db.mu.RUnlock()

	if db.conn == nil {
		return fmt.Errorf("database connection is nil")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return db.conn.Ping(ctx)
}

// CreateFTSIndexes creates Full-Text Search capabilities
func (db *ClickHouseDB) CreateFTSIndexes() error {
	db.mu.Lock()
	defer db.mu.Unlock()

	ctx := context.Background()

	// ClickHouse doesn't have FTS like DuckDB, but we can create bloom filter indexes
	// for text search functionality, including optimized materialized columns
	ftsIndexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_fts_location ON nodes(location) TYPE bloom_filter GRANULARITY 1",
		"CREATE INDEX IF NOT EXISTS idx_fts_sysop ON nodes(sysop_name) TYPE bloom_filter GRANULARITY 1",
		"CREATE INDEX IF NOT EXISTS idx_fts_system ON nodes(system_name) TYPE bloom_filter GRANULARITY 1",
		// Additional indexes for materialized lowercase columns
		"CREATE INDEX IF NOT EXISTS idx_fts_location_lower ON nodes(location_lower) TYPE bloom_filter GRANULARITY 1",
		"CREATE INDEX IF NOT EXISTS idx_fts_sysop_lower ON nodes(sysop_name_lower) TYPE bloom_filter GRANULARITY 1",
		"CREATE INDEX IF NOT EXISTS idx_fts_system_lower ON nodes(system_name_lower) TYPE bloom_filter GRANULARITY 1",
	}

	for _, indexSQL := range ftsIndexes {
		if err := db.conn.Exec(ctx, indexSQL); err != nil {
			// If index already exists or is not supported, continue
			if !strings.Contains(err.Error(), "already exists") {
				fmt.Printf("Warning: Could not create FTS index: %v\n", err)
			}
		}
	}

	return nil
}

// DropFTSIndexes drops Full-Text Search indexes
func (db *ClickHouseDB) DropFTSIndexes() error {
	db.mu.Lock()
	defer db.mu.Unlock()

	ctx := context.Background()

	// Drop FTS-related indexes
	dropIndexes := []string{
		"DROP INDEX IF EXISTS idx_fts_location ON nodes",
		"DROP INDEX IF EXISTS idx_fts_sysop ON nodes",
		"DROP INDEX IF EXISTS idx_fts_system ON nodes",
		// Drop optimized lowercase column indexes
		"DROP INDEX IF EXISTS idx_fts_location_lower ON nodes",
		"DROP INDEX IF EXISTS idx_fts_sysop_lower ON nodes",
		"DROP INDEX IF EXISTS idx_fts_system_lower ON nodes",
	}

	for _, dropSQL := range dropIndexes {
		if err := db.conn.Exec(ctx, dropSQL); err != nil {
			fmt.Printf("Warning: Could not drop FTS index: %v\n", err)
		}
	}

	return nil
}

// Ensure ClickHouseDB implements DatabaseInterface
var _ DatabaseInterface = (*ClickHouseDB)(nil)
