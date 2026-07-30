// Package main provides SQL database storage for modem test results.
// PostgreSQL, MySQL and SQLite share one writer; only the driver, the DDL
// dialect and the placeholder style differ.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql" // MySQL driver
	_ "github.com/lib/pq"              // PostgreSQL driver
	_ "github.com/mattn/go-sqlite3"    // SQLite driver
)

// PostgresResultsConfig contains PostgreSQL results database settings
type PostgresResultsConfig struct {
	Enabled   bool   `yaml:"enabled"`    // Enable PostgreSQL result writing (default: false)
	DSN       string `yaml:"dsn"`        // PostgreSQL connection string
	TableName string `yaml:"table_name"` // Table name (default: "modem_test_results")
}

// MySQLResultsConfig contains MySQL results database settings
type MySQLResultsConfig struct {
	Enabled   bool   `yaml:"enabled"`    // Enable MySQL result writing (default: false)
	DSN       string `yaml:"dsn"`        // MySQL connection string (user:password@tcp(host:port)/database)
	TableName string `yaml:"table_name"` // Table name (default: "modem_test_results")
}

// SQLiteResultsConfig contains SQLite results database settings
type SQLiteResultsConfig struct {
	Enabled   bool   `yaml:"enabled"`    // Enable SQLite result writing (default: false)
	Path      string `yaml:"path"`       // Path to SQLite database file
	TableName string `yaml:"table_name"` // Table name (default: "modem_test_results")
}

// resultColumns is the single authoritative column list for the results
// table. recordArgs must produce values in exactly this order; the DDL
// templates below declare the same columns per dialect.
var resultColumns = []string{
	"timestamp", "test_num", "phone", "modem_name", "operator_name", "operator_prefix",
	"node_address", "node_system_name", "node_location", "node_sysop", "success",
	"dial_time_seconds", "connect_speed", "connect_string", "emsi_time_seconds", "emsi_error",
	"remote_address", "remote_system", "remote_location", "remote_sysop", "remote_mailer",
	"tx_speed", "rx_speed", "protocol", "compression", "line_quality", "rx_level", "retrains", "termination", "stats_notes",
	"cdr_session_id", "cdr_codec", "cdr_rtp_jitter_ms", "cdr_rtp_delay_ms", "cdr_packet_loss", "cdr_remote_packet_loss",
	"cdr_local_mos", "cdr_remote_mos", "cdr_local_r_factor", "cdr_remote_r_factor", "cdr_term_reason", "cdr_term_category",
	"ast_disposition", "ast_peer", "ast_duration", "ast_billsec",
	"ast_hangupcause", "ast_hangupsource", "ast_early_media",
}

// recordArgs flattens a TestRecord into insert values, in resultColumns order.
func recordArgs(rec *TestRecord) []any {
	return []any{
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
	}
}

// sqlDialect captures everything that differs between the supported databases.
// The DDL uses {table} where the configured table name is substituted.
type sqlDialect struct {
	name        string // display name for logs and error messages
	driver      string // database/sql driver name
	ddl         string // CREATE TABLE (+ indexes) template
	numbered    bool   // true = $1..$N placeholders (PostgreSQL), false = ?
	configureDB func(db *sql.DB)
}

// sharedPoolConfig is the connection pool setup the client/server databases
// use (same as the CDR services).
func sharedPoolConfig(db *sql.DB) {
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(time.Hour)
}

var postgresDialect = sqlDialect{
	name:        "PostgreSQL",
	driver:      "postgres",
	numbered:    true,
	configureDB: sharedPoolConfig,
	ddl: `
CREATE TABLE IF NOT EXISTS {table} (
    id                      BIGSERIAL PRIMARY KEY,
    timestamp               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    test_num                INTEGER NOT NULL,
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
    ast_hangupcause         INTEGER NOT NULL DEFAULT 0,
    ast_hangupsource        VARCHAR(80) NOT NULL DEFAULT '',
    ast_early_media         BOOLEAN NOT NULL DEFAULT FALSE,

    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_{table}_timestamp ON {table} (timestamp);
CREATE INDEX IF NOT EXISTS idx_{table}_phone ON {table} (phone);
CREATE INDEX IF NOT EXISTS idx_{table}_modem_name ON {table} (modem_name);
CREATE INDEX IF NOT EXISTS idx_{table}_operator_name ON {table} (operator_name);
CREATE INDEX IF NOT EXISTS idx_{table}_success ON {table} (success);
`,
}

var mysqlDialect = sqlDialect{
	name:        "MySQL",
	driver:      "mysql",
	numbered:    false,
	configureDB: sharedPoolConfig,
	ddl: `
CREATE TABLE IF NOT EXISTS {table} (
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
`,
}

var sqliteDialect = sqlDialect{
	name:     "SQLite",
	driver:   "sqlite3",
	numbered: false,
	configureDB: func(db *sql.DB) {
		// SQLite works best with a single connection for writes
		db.SetMaxOpenConns(1)
	},
	ddl: `
CREATE TABLE IF NOT EXISTS {table} (
    id                      INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp               TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%f', 'now')),
    test_num                INTEGER NOT NULL,
    phone                   TEXT NOT NULL,
    modem_name              TEXT NOT NULL DEFAULT '',
    operator_name           TEXT NOT NULL DEFAULT '',
    operator_prefix         TEXT NOT NULL DEFAULT '',
    node_address            TEXT NOT NULL DEFAULT '',
    node_system_name        TEXT NOT NULL DEFAULT '',
    node_location           TEXT NOT NULL DEFAULT '',
    node_sysop              TEXT NOT NULL DEFAULT '',
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

    created_at              TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%f', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_{table}_timestamp ON {table} (timestamp);
CREATE INDEX IF NOT EXISTS idx_{table}_phone ON {table} (phone);
CREATE INDEX IF NOT EXISTS idx_{table}_modem_name ON {table} (modem_name);
CREATE INDEX IF NOT EXISTS idx_{table}_operator_name ON {table} (operator_name);
CREATE INDEX IF NOT EXISTS idx_{table}_success ON {table} (success);
`,
}

// SQLResultsWriter writes test results to a SQL database.
type SQLResultsWriter struct {
	db        *sql.DB
	dialect   sqlDialect
	tableName string
	stmt      *sql.Stmt // Prepared statement for inserts
}

// NewPostgresResultsWriter creates a PostgreSQL results writer.
// Returns (nil, nil) when the writer is disabled by configuration.
func NewPostgresResultsWriter(cfg PostgresResultsConfig) (*SQLResultsWriter, error) {
	if !cfg.Enabled || cfg.DSN == "" {
		return nil, nil
	}
	return newSQLResultsWriter(postgresDialect, cfg.DSN, cfg.TableName)
}

// NewMySQLResultsWriter creates a MySQL results writer.
// Returns (nil, nil) when the writer is disabled by configuration.
func NewMySQLResultsWriter(cfg MySQLResultsConfig) (*SQLResultsWriter, error) {
	if !cfg.Enabled || cfg.DSN == "" {
		return nil, nil
	}
	return newSQLResultsWriter(mysqlDialect, cfg.DSN, cfg.TableName)
}

// NewSQLiteResultsWriter creates a SQLite results writer.
// Returns (nil, nil) when the writer is disabled by configuration.
func NewSQLiteResultsWriter(cfg SQLiteResultsConfig) (*SQLResultsWriter, error) {
	if !cfg.Enabled || cfg.Path == "" {
		return nil, nil
	}
	return newSQLResultsWriter(sqliteDialect, cfg.Path+"?_journal_mode=WAL&_busy_timeout=5000", cfg.TableName)
}

// newSQLResultsWriter opens the database, ensures the results table exists and
// prepares the insert statement.
func newSQLResultsWriter(dialect sqlDialect, dsn, tableName string) (*SQLResultsWriter, error) {
	db, err := sql.Open(dialect.driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open results database: %w", err)
	}

	dialect.configureDB(db)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping results database: %w", err)
	}

	if tableName == "" {
		tableName = "modem_test_results"
	}

	w := &SQLResultsWriter{
		db:        db,
		dialect:   dialect,
		tableName: tableName,
	}

	// Create table if not exists
	ddl := strings.ReplaceAll(dialect.ddl, "{table}", tableName)
	if _, err := db.ExecContext(ctx, ddl); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create results table: %w", err)
	}

	// Prepare insert statement
	stmt, err := db.Prepare(w.insertQuery())
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to prepare insert statement: %w", err)
	}
	w.stmt = stmt

	return w, nil
}

// insertQuery builds the INSERT statement from the shared column list.
func (w *SQLResultsWriter) insertQuery() string {
	placeholders := make([]string, len(resultColumns))
	for i := range resultColumns {
		if w.dialect.numbered {
			placeholders[i] = fmt.Sprintf("$%d", i+1)
		} else {
			placeholders[i] = "?"
		}
	}
	return fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		w.tableName,
		strings.Join(resultColumns, ", "),
		strings.Join(placeholders, ", "),
	)
}

// Name returns the dialect's display name for log messages.
func (w *SQLResultsWriter) Name() string {
	return w.dialect.name
}

// TableName returns the results table this writer inserts into.
func (w *SQLResultsWriter) TableName() string {
	return w.tableName
}

// WriteRecord writes a test record to the database.
func (w *SQLResultsWriter) WriteRecord(rec *TestRecord) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := w.stmt.ExecContext(ctx, recordArgs(rec)...); err != nil {
		return fmt.Errorf("failed to insert test result: %w", err)
	}
	return nil
}

// Close closes the database connection.
func (w *SQLResultsWriter) Close() error {
	if w.stmt != nil {
		w.stmt.Close()
	}
	return w.db.Close()
}
