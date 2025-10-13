package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/nodelistdb/internal/testing/models"
)

// ClickHouseConfig holds ClickHouse connection configuration
type ClickHouseConfig struct {
	MaxOpenConns  int
	MaxIdleConns  int
	DialTimeout   time.Duration
	ReadTimeout   time.Duration
	WriteTimeout  time.Duration
	Compression   string
	BatchSize     int
	FlushInterval time.Duration
}

// ClickHouseStorage implements Storage interface for ClickHouse
type ClickHouseStorage struct {
	conn      driver.Conn
	db        *sql.DB
	batchSize int

	// Batch accumulator
	resultsBatch  []*models.TestResult
	lastFlush     time.Time
	flushInterval time.Duration
}

// NewClickHouseStorage creates a new ClickHouse storage
func NewClickHouseStorage(host string, port int, database, username, password string) (*ClickHouseStorage, error) {
	return NewClickHouseStorageWithConfig(host, port, database, username, password, nil)
}

// NewClickHouseStorageWithConfig creates a new ClickHouse storage with custom config
func NewClickHouseStorageWithConfig(host string, port int, database, username, password string, cfg *ClickHouseConfig) (*ClickHouseStorage, error) {
	// Set defaults
	maxOpenConns := 10
	maxIdleConns := 5
	dialTimeout := 10 * time.Second
	readTimeout := 5 * time.Minute
	compression := clickhouse.CompressionLZ4
	batchSize := 1000
	flushInterval := 30 * time.Second

	// Override with config if provided
	if cfg != nil {
		if cfg.MaxOpenConns > 0 {
			maxOpenConns = cfg.MaxOpenConns
		}
		if cfg.MaxIdleConns > 0 {
			maxIdleConns = cfg.MaxIdleConns
		}
		if cfg.DialTimeout > 0 {
			dialTimeout = cfg.DialTimeout
		}
		if cfg.ReadTimeout > 0 {
			readTimeout = cfg.ReadTimeout
		}
		if cfg.BatchSize > 0 {
			batchSize = cfg.BatchSize
		}
		if cfg.FlushInterval > 0 {
			flushInterval = cfg.FlushInterval
		}
		// Handle compression
		switch strings.ToLower(cfg.Compression) {
		case "lz4":
			compression = clickhouse.CompressionLZ4
		case "zstd":
			compression = clickhouse.CompressionZSTD
		case "gzip":
			compression = clickhouse.CompressionGZIP
		case "none", "":
			compression = clickhouse.CompressionNone
		}
	}

	// Create connection options
	// IMPORTANT: Do NOT set MaxOpenConns/MaxIdleConns in Options due to driver bug
	// They must be set on the sql.DB after OpenDB
	options := &clickhouse.Options{
		Addr: []string{fmt.Sprintf("%s:%d", host, port)},
		Auth: clickhouse.Auth{
			Database: database,
			Username: username,
			Password: password,
		},
		Settings: clickhouse.Settings{
			"max_execution_time": 60,
		},
		Compression: &clickhouse.Compression{
			Method: compression,
		},
		DialTimeout: dialTimeout,
		ReadTimeout: readTimeout,
		// DO NOT SET MaxOpenConns/MaxIdleConns here - driver bug!
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
	sqlDB.SetMaxOpenConns(maxOpenConns)
	sqlDB.SetMaxIdleConns(maxIdleConns)
	sqlDB.SetConnMaxLifetime(time.Hour)

	// Note: We don't ping the SQL DB here as it can cause issues.
	// The native connection ping above is sufficient.

	storage := &ClickHouseStorage{
		conn:          conn,
		db:            sqlDB,
		batchSize:     batchSize,
		resultsBatch:  make([]*models.TestResult, 0, batchSize),
		lastFlush:     time.Now(),
		flushInterval: flushInterval,
	}

	// Initialize schema
	if err := storage.initSchema(context.Background()); err != nil {
		storage.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return storage, nil
}

// Close closes the database connection
func (s *ClickHouseStorage) Close() error {
	// Flush any pending results
	if len(s.resultsBatch) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		s.flushBatch(ctx)
	}

	if s.conn != nil {
		if err := s.conn.Close(); err != nil {
			return err
		}
	}
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}
