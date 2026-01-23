// Package main provides PostgreSQL storage for modem test results.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq" // PostgreSQL driver
)

// PostgresResultsConfig contains PostgreSQL results database settings
type PostgresResultsConfig struct {
	Enabled   bool   `yaml:"enabled"`    // Enable PostgreSQL result writing (default: false)
	DSN       string `yaml:"dsn"`        // PostgreSQL connection string
	TableName string `yaml:"table_name"` // Table name (default: "modem_test_results")
}

// PostgresResultsWriter writes test results to PostgreSQL
type PostgresResultsWriter struct {
	db        *sql.DB
	tableName string
	enabled   bool
	stmt      *sql.Stmt // Prepared statement for inserts
}

// Table creation DDL
const createTableSQL = `
CREATE TABLE IF NOT EXISTS %s (
    id                      BIGSERIAL PRIMARY KEY,
    timestamp               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    test_num                INTEGER NOT NULL,
    phone                   VARCHAR(32) NOT NULL,
    modem_name              VARCHAR(64) NOT NULL DEFAULT '',
    success                 BOOLEAN NOT NULL,

    -- Connection
    dial_time_seconds       REAL NOT NULL DEFAULT 0,
    connect_speed           INTEGER NOT NULL DEFAULT 0,
    connect_string          VARCHAR(256) NOT NULL DEFAULT '',
    emsi_time_seconds       REAL NOT NULL DEFAULT 0,
    emsi_error              VARCHAR(128) NOT NULL DEFAULT '',

    -- Remote system (EMSI)
    remote_address          VARCHAR(256) NOT NULL DEFAULT '',
    remote_system           VARCHAR(128) NOT NULL DEFAULT '',
    remote_location         VARCHAR(128) NOT NULL DEFAULT '',
    remote_sysop            VARCHAR(128) NOT NULL DEFAULT '',
    remote_mailer           VARCHAR(128) NOT NULL DEFAULT '',

    -- Line statistics
    tx_speed                INTEGER NOT NULL DEFAULT 0,
    rx_speed                INTEGER NOT NULL DEFAULT 0,
    protocol                VARCHAR(32) NOT NULL DEFAULT '',
    compression             VARCHAR(32) NOT NULL DEFAULT '',
    line_quality            INTEGER NOT NULL DEFAULT 0,
    rx_level                INTEGER NOT NULL DEFAULT 0,
    retrains                INTEGER NOT NULL DEFAULT 0,
    termination             VARCHAR(64) NOT NULL DEFAULT '',
    stats_notes             TEXT NOT NULL DEFAULT '',

    -- AudioCodes CDR
    cdr_session_id          VARCHAR(64) NOT NULL DEFAULT '',
    cdr_codec               VARCHAR(32) NOT NULL DEFAULT '',
    cdr_rtp_jitter_ms       INTEGER NOT NULL DEFAULT 0,
    cdr_rtp_delay_ms        INTEGER NOT NULL DEFAULT 0,
    cdr_packet_loss         INTEGER NOT NULL DEFAULT 0,
    cdr_remote_packet_loss  INTEGER NOT NULL DEFAULT 0,
    cdr_local_mos           REAL NOT NULL DEFAULT 0,
    cdr_remote_mos          REAL NOT NULL DEFAULT 0,
    cdr_local_r_factor      INTEGER NOT NULL DEFAULT 0,
    cdr_remote_r_factor     INTEGER NOT NULL DEFAULT 0,
    cdr_term_reason         VARCHAR(64) NOT NULL DEFAULT '',
    cdr_term_category       VARCHAR(64) NOT NULL DEFAULT '',

    -- Asterisk CDR
    ast_disposition         VARCHAR(32) NOT NULL DEFAULT '',
    ast_peer                VARCHAR(64) NOT NULL DEFAULT '',
    ast_duration            INTEGER NOT NULL DEFAULT 0,
    ast_billsec             INTEGER NOT NULL DEFAULT 0,

    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_%s_timestamp ON %s (timestamp);
CREATE INDEX IF NOT EXISTS idx_%s_phone ON %s (phone);
CREATE INDEX IF NOT EXISTS idx_%s_modem_name ON %s (modem_name);
CREATE INDEX IF NOT EXISTS idx_%s_success ON %s (success);
`

// NewPostgresResultsWriter creates a new PostgreSQL results writer
func NewPostgresResultsWriter(cfg PostgresResultsConfig) (*PostgresResultsWriter, error) {
	if !cfg.Enabled || cfg.DSN == "" {
		return &PostgresResultsWriter{enabled: false}, nil
	}

	db, err := sql.Open("postgres", cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("failed to open results database: %w", err)
	}

	// Connection pool settings (same as CDR services)
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(time.Hour)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping results database: %w", err)
	}

	tableName := cfg.TableName
	if tableName == "" {
		tableName = "modem_test_results"
	}

	w := &PostgresResultsWriter{
		db:        db,
		tableName: tableName,
		enabled:   true,
	}

	// Create table if not exists
	if err := w.createTable(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create results table: %w", err)
	}

	// Prepare insert statement
	if err := w.prepareStatement(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to prepare insert statement: %w", err)
	}

	return w, nil
}

// IsEnabled returns true if the writer is active
func (w *PostgresResultsWriter) IsEnabled() bool {
	return w.enabled
}

// WriteRecord writes a test record to PostgreSQL
func (w *PostgresResultsWriter) WriteRecord(rec *TestRecord) error {
	if !w.enabled || w.db == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := w.stmt.ExecContext(ctx,
		rec.Timestamp,
		rec.TestNum,
		rec.Phone,
		rec.ModemName,
		rec.Success,
		rec.DialTime.Seconds(),
		rec.ConnectSpeed,
		rec.ConnectString,
		rec.EMSITime.Seconds(),
		rec.EMSIError,
		rec.RemoteAddress,
		rec.RemoteSystem,
		rec.RemoteLocation,
		rec.RemoteSysop,
		rec.RemoteMailer,
		rec.TXSpeed,
		rec.RXSpeed,
		rec.Protocol,
		rec.Compression,
		rec.LineQuality,
		rec.RxLevel,
		rec.Retrains,
		rec.Termination,
		rec.StatsNotes,
		rec.CDRSessionID,
		rec.CDRCodec,
		rec.CDRRTPJitter,
		rec.CDRRTPDelay,
		rec.CDRPacketLoss,
		rec.CDRRemotePacketLoss,
		rec.CDRLocalMOS,
		rec.CDRRemoteMOS,
		rec.CDRLocalRFactor,
		rec.CDRRemoteRFactor,
		rec.CDRTermReason,
		rec.CDRTermCategory,
		rec.AstDisposition,
		rec.AstPeer,
		rec.AstDuration,
		rec.AstBillSec,
	)
	if err != nil {
		return fmt.Errorf("failed to insert test result: %w", err)
	}
	return nil
}

// Close closes the database connection
func (w *PostgresResultsWriter) Close() error {
	if w.stmt != nil {
		w.stmt.Close()
	}
	if w.db != nil {
		return w.db.Close()
	}
	return nil
}

// createTable creates the results table if it doesn't exist
func (w *PostgresResultsWriter) createTable(ctx context.Context) error {
	// Format DDL with table name for table and indexes
	ddl := fmt.Sprintf(createTableSQL,
		w.tableName,
		w.tableName, w.tableName, // timestamp index
		w.tableName, w.tableName, // phone index
		w.tableName, w.tableName, // modem_name index
		w.tableName, w.tableName, // success index
	)
	_, err := w.db.ExecContext(ctx, ddl)
	return err
}

// prepareStatement prepares the insert statement for reuse
func (w *PostgresResultsWriter) prepareStatement() error {
	query := fmt.Sprintf(`
		INSERT INTO %s (
			timestamp, test_num, phone, modem_name, success,
			dial_time_seconds, connect_speed, connect_string, emsi_time_seconds, emsi_error,
			remote_address, remote_system, remote_location, remote_sysop, remote_mailer,
			tx_speed, rx_speed, protocol, compression, line_quality, rx_level, retrains, termination, stats_notes,
			cdr_session_id, cdr_codec, cdr_rtp_jitter_ms, cdr_rtp_delay_ms, cdr_packet_loss, cdr_remote_packet_loss,
			cdr_local_mos, cdr_remote_mos, cdr_local_r_factor, cdr_remote_r_factor, cdr_term_reason, cdr_term_category,
			ast_disposition, ast_peer, ast_duration, ast_billsec
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
			$11, $12, $13, $14, $15, $16, $17, $18, $19, $20,
			$21, $22, $23, $24, $25, $26, $27, $28, $29, $30,
			$31, $32, $33, $34, $35, $36, $37, $38, $39, $40
		)
	`, w.tableName)

	stmt, err := w.db.Prepare(query)
	if err != nil {
		return err
	}
	w.stmt = stmt
	return nil
}
