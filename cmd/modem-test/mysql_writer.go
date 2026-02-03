// Package main provides MySQL storage for modem test results.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql" // MySQL driver
)

// MySQLResultsConfig contains MySQL results database settings
type MySQLResultsConfig struct {
	Enabled   bool   `yaml:"enabled"`    // Enable MySQL result writing (default: false)
	DSN       string `yaml:"dsn"`        // MySQL connection string (user:password@tcp(host:port)/database)
	TableName string `yaml:"table_name"` // Table name (default: "modem_test_results")
}

// MySQLResultsWriter writes test results to MySQL
type MySQLResultsWriter struct {
	db        *sql.DB
	tableName string
	enabled   bool
	stmt      *sql.Stmt // Prepared statement for inserts
}

// Table creation DDL for MySQL
const mysqlCreateTableSQL = `
CREATE TABLE IF NOT EXISTS %s (
    id                      BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    timestamp               DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    test_num                INT NOT NULL,
    phone                   VARCHAR(32) NOT NULL,
    modem_name              VARCHAR(64) NOT NULL DEFAULT '',
    operator_name           VARCHAR(64) NOT NULL DEFAULT '',
    operator_prefix         VARCHAR(16) NOT NULL DEFAULT '',
    node_address            VARCHAR(32) NOT NULL DEFAULT '',
    node_system_name        VARCHAR(128) NOT NULL DEFAULT '',
    node_location           VARCHAR(128) NOT NULL DEFAULT '',
    node_sysop              VARCHAR(128) NOT NULL DEFAULT '',
    success                 BOOLEAN NOT NULL,

    -- Connection
    dial_time_seconds       FLOAT NOT NULL DEFAULT 0,
    connect_speed           INT NOT NULL DEFAULT 0,
    connect_string          VARCHAR(256) NOT NULL DEFAULT '',
    emsi_time_seconds       FLOAT NOT NULL DEFAULT 0,
    emsi_error              VARCHAR(128) NOT NULL DEFAULT '',

    -- Remote system (EMSI)
    remote_address          VARCHAR(256) NOT NULL DEFAULT '',
    remote_system           VARCHAR(128) NOT NULL DEFAULT '',
    remote_location         VARCHAR(128) NOT NULL DEFAULT '',
    remote_sysop            VARCHAR(128) NOT NULL DEFAULT '',
    remote_mailer           VARCHAR(128) NOT NULL DEFAULT '',

    -- Line statistics
    tx_speed                INT NOT NULL DEFAULT 0,
    rx_speed                INT NOT NULL DEFAULT 0,
    protocol                VARCHAR(32) NOT NULL DEFAULT '',
    compression             VARCHAR(32) NOT NULL DEFAULT '',
    line_quality            INT NOT NULL DEFAULT 0,
    rx_level                INT NOT NULL DEFAULT 0,
    retrains                INT NOT NULL DEFAULT 0,
    termination             VARCHAR(64) NOT NULL DEFAULT '',
    stats_notes             TEXT NOT NULL,

    -- AudioCodes CDR
    cdr_session_id          VARCHAR(64) NOT NULL DEFAULT '',
    cdr_codec               VARCHAR(32) NOT NULL DEFAULT '',
    cdr_rtp_jitter_ms       INT NOT NULL DEFAULT 0,
    cdr_rtp_delay_ms        INT NOT NULL DEFAULT 0,
    cdr_packet_loss         INT NOT NULL DEFAULT 0,
    cdr_remote_packet_loss  INT NOT NULL DEFAULT 0,
    cdr_local_mos           FLOAT NOT NULL DEFAULT 0,
    cdr_remote_mos          FLOAT NOT NULL DEFAULT 0,
    cdr_local_r_factor      INT NOT NULL DEFAULT 0,
    cdr_remote_r_factor     INT NOT NULL DEFAULT 0,
    cdr_term_reason         VARCHAR(64) NOT NULL DEFAULT '',
    cdr_term_category       VARCHAR(64) NOT NULL DEFAULT '',

    -- Asterisk CDR
    ast_disposition         VARCHAR(32) NOT NULL DEFAULT '',
    ast_peer                VARCHAR(64) NOT NULL DEFAULT '',
    ast_duration            INT NOT NULL DEFAULT 0,
    ast_billsec             INT NOT NULL DEFAULT 0,
    ast_hangupcause         INT NOT NULL DEFAULT 0,
    ast_hangupsource        VARCHAR(80) NOT NULL DEFAULT '',
    ast_early_media         TINYINT(1) NOT NULL DEFAULT 0,

    created_at              DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),

    INDEX idx_timestamp (timestamp),
    INDEX idx_phone (phone),
    INDEX idx_modem_name (modem_name),
    INDEX idx_operator_name (operator_name),
    INDEX idx_success (success)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
`

// NewMySQLResultsWriter creates a new MySQL results writer
func NewMySQLResultsWriter(cfg MySQLResultsConfig) (*MySQLResultsWriter, error) {
	if !cfg.Enabled || cfg.DSN == "" {
		return &MySQLResultsWriter{enabled: false}, nil
	}

	db, err := sql.Open("mysql", cfg.DSN)
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

	w := &MySQLResultsWriter{
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
func (w *MySQLResultsWriter) IsEnabled() bool {
	return w.enabled
}

// WriteRecord writes a test record to MySQL
func (w *MySQLResultsWriter) WriteRecord(rec *TestRecord) error {
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
		rec.NodeAddress,
		rec.NodeSystemName,
		rec.NodeLocation,
		rec.NodeSysop,
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
func (w *MySQLResultsWriter) Close() error {
	if w.stmt != nil {
		w.stmt.Close()
	}
	if w.db != nil {
		return w.db.Close()
	}
	return nil
}

// createTable creates the results table if it doesn't exist
func (w *MySQLResultsWriter) createTable(ctx context.Context) error {
	ddl := fmt.Sprintf(mysqlCreateTableSQL, w.tableName)
	_, err := w.db.ExecContext(ctx, ddl)
	return err
}

// prepareStatement prepares the insert statement for reuse
func (w *MySQLResultsWriter) prepareStatement() error {
	query := fmt.Sprintf(`
		INSERT INTO %s (
			timestamp, test_num, phone, modem_name, operator_name, operator_prefix,
			node_address, node_system_name, node_location, node_sysop, success,
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
			?, ?, ?, ?, ?, ?, ?, ?, ?
		)
	`, w.tableName)

	stmt, err := w.db.Prepare(query)
	if err != nil {
		return err
	}
	w.stmt = stmt
	return nil
}
