// Package main provides SQLite storage for modem test results.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3" // SQLite driver
)

// SQLiteResultsConfig contains SQLite results database settings
type SQLiteResultsConfig struct {
	Enabled   bool   `yaml:"enabled"`    // Enable SQLite result writing (default: false)
	Path      string `yaml:"path"`       // Path to SQLite database file
	TableName string `yaml:"table_name"` // Table name (default: "modem_test_results")
}

// SQLiteResultsWriter writes test results to SQLite
type SQLiteResultsWriter struct {
	db        *sql.DB
	tableName string
	enabled   bool
	stmt      *sql.Stmt // Prepared statement for inserts
}

// Table creation DDL for SQLite
const sqliteCreateTableSQL = `
CREATE TABLE IF NOT EXISTS %s (
    id                      INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp               TEXT NOT NULL DEFAULT (strftime('%%Y-%%m-%%dT%%H:%%M:%%f', 'now')),
    test_num                INTEGER NOT NULL,
    phone                   TEXT NOT NULL,
    modem_name              TEXT NOT NULL DEFAULT '',
    operator_name           TEXT NOT NULL DEFAULT '',
    operator_prefix         TEXT NOT NULL DEFAULT '',
    success                 INTEGER NOT NULL,

    -- Connection
    dial_time_seconds       REAL NOT NULL DEFAULT 0,
    connect_speed           INTEGER NOT NULL DEFAULT 0,
    connect_string          TEXT NOT NULL DEFAULT '',
    emsi_time_seconds       REAL NOT NULL DEFAULT 0,
    emsi_error              TEXT NOT NULL DEFAULT '',

    -- Remote system (EMSI)
    remote_address          TEXT NOT NULL DEFAULT '',
    remote_system           TEXT NOT NULL DEFAULT '',
    remote_location         TEXT NOT NULL DEFAULT '',
    remote_sysop            TEXT NOT NULL DEFAULT '',
    remote_mailer           TEXT NOT NULL DEFAULT '',

    -- Line statistics
    tx_speed                INTEGER NOT NULL DEFAULT 0,
    rx_speed                INTEGER NOT NULL DEFAULT 0,
    protocol                TEXT NOT NULL DEFAULT '',
    compression             TEXT NOT NULL DEFAULT '',
    line_quality            INTEGER NOT NULL DEFAULT 0,
    rx_level                INTEGER NOT NULL DEFAULT 0,
    retrains                INTEGER NOT NULL DEFAULT 0,
    termination             TEXT NOT NULL DEFAULT '',
    stats_notes             TEXT NOT NULL DEFAULT '',

    -- AudioCodes CDR
    cdr_session_id          TEXT NOT NULL DEFAULT '',
    cdr_codec               TEXT NOT NULL DEFAULT '',
    cdr_rtp_jitter_ms       INTEGER NOT NULL DEFAULT 0,
    cdr_rtp_delay_ms        INTEGER NOT NULL DEFAULT 0,
    cdr_packet_loss         INTEGER NOT NULL DEFAULT 0,
    cdr_remote_packet_loss  INTEGER NOT NULL DEFAULT 0,
    cdr_local_mos           REAL NOT NULL DEFAULT 0,
    cdr_remote_mos          REAL NOT NULL DEFAULT 0,
    cdr_local_r_factor      INTEGER NOT NULL DEFAULT 0,
    cdr_remote_r_factor     INTEGER NOT NULL DEFAULT 0,
    cdr_term_reason         TEXT NOT NULL DEFAULT '',
    cdr_term_category       TEXT NOT NULL DEFAULT '',

    -- Asterisk CDR
    ast_disposition         TEXT NOT NULL DEFAULT '',
    ast_peer                TEXT NOT NULL DEFAULT '',
    ast_duration            INTEGER NOT NULL DEFAULT 0,
    ast_billsec             INTEGER NOT NULL DEFAULT 0,
    ast_hangupcause         INTEGER NOT NULL DEFAULT 0,
    ast_hangupsource        TEXT NOT NULL DEFAULT '',
    ast_early_media         INTEGER NOT NULL DEFAULT 0,

    created_at              TEXT NOT NULL DEFAULT (strftime('%%Y-%%m-%%dT%%H:%%M:%%f', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_%s_timestamp ON %s (timestamp);
CREATE INDEX IF NOT EXISTS idx_%s_phone ON %s (phone);
CREATE INDEX IF NOT EXISTS idx_%s_modem_name ON %s (modem_name);
CREATE INDEX IF NOT EXISTS idx_%s_operator_name ON %s (operator_name);
CREATE INDEX IF NOT EXISTS idx_%s_success ON %s (success);
`

// NewSQLiteResultsWriter creates a new SQLite results writer
func NewSQLiteResultsWriter(cfg SQLiteResultsConfig) (*SQLiteResultsWriter, error) {
	if !cfg.Enabled || cfg.Path == "" {
		return &SQLiteResultsWriter{enabled: false}, nil
	}

	db, err := sql.Open("sqlite3", cfg.Path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("failed to open results database: %w", err)
	}

	// SQLite works best with a single connection for writes
	db.SetMaxOpenConns(1)

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

	w := &SQLiteResultsWriter{
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
func (w *SQLiteResultsWriter) IsEnabled() bool {
	return w.enabled
}

// WriteRecord writes a test record to SQLite
func (w *SQLiteResultsWriter) WriteRecord(rec *TestRecord) error {
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
		rec.OperatorName,
		rec.OperatorPrefix,
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
		rec.AstHangupCause,
		rec.AstHangupSource,
		rec.AstEarlyMedia,
	)
	if err != nil {
		return fmt.Errorf("failed to insert test result: %w", err)
	}
	return nil
}

// Close closes the database connection
func (w *SQLiteResultsWriter) Close() error {
	if w.stmt != nil {
		w.stmt.Close()
	}
	if w.db != nil {
		return w.db.Close()
	}
	return nil
}

// createTable creates the results table if it doesn't exist
func (w *SQLiteResultsWriter) createTable(ctx context.Context) error {
	ddl := fmt.Sprintf(sqliteCreateTableSQL,
		w.tableName,
		w.tableName, w.tableName, // timestamp index
		w.tableName, w.tableName, // phone index
		w.tableName, w.tableName, // modem_name index
		w.tableName, w.tableName, // operator_name index
		w.tableName, w.tableName, // success index
	)
	_, err := w.db.ExecContext(ctx, ddl)
	return err
}

// prepareStatement prepares the insert statement for reuse
func (w *SQLiteResultsWriter) prepareStatement() error {
	query := fmt.Sprintf(`
		INSERT INTO %s (
			timestamp, test_num, phone, modem_name, operator_name, operator_prefix, success,
			dial_time_seconds, connect_speed, connect_string, emsi_time_seconds, emsi_error,
			remote_address, remote_system, remote_location, remote_sysop, remote_mailer,
			tx_speed, rx_speed, protocol, compression, line_quality, rx_level, retrains, termination, stats_notes,
			cdr_session_id, cdr_codec, cdr_rtp_jitter_ms, cdr_rtp_delay_ms, cdr_packet_loss, cdr_remote_packet_loss,
			cdr_local_mos, cdr_remote_mos, cdr_local_r_factor, cdr_remote_r_factor, cdr_term_reason, cdr_term_category,
			ast_disposition, ast_peer, ast_duration, ast_billsec,
			ast_hangupcause, ast_hangupsource, ast_early_media
		) VALUES (
			?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
			?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
			?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
			?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
			?, ?, ?, ?, ?
		)
	`, w.tableName)

	stmt, err := w.db.Prepare(query)
	if err != nil {
		return err
	}
	w.stmt = stmt
	return nil
}
