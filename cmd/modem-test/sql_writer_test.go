// Package main tests the shared SQL results writer. SQLite gives a real
// end-to-end path (DDL + insert + readback); the placeholder and column
// consistency tests cover what PostgreSQL/MySQL can't exercise without a
// server.
package main

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestResultColumnsMatchRecordArgs(t *testing.T) {
	args := recordArgs(&TestRecord{})
	if len(args) != len(resultColumns) {
		t.Fatalf("recordArgs returns %d values for %d columns; the lists must stay in lockstep", len(args), len(resultColumns))
	}
}

func TestDialectDDLDeclaresEveryColumn(t *testing.T) {
	for _, d := range []sqlDialect{postgresDialect, mysqlDialect, sqliteDialect} {
		for _, col := range resultColumns {
			if !strings.Contains(d.ddl, "\n    "+col+" ") {
				t.Errorf("%s DDL is missing column %q", d.name, col)
			}
		}
	}
}

func TestInsertQueryPlaceholders(t *testing.T) {
	pg := &SQLResultsWriter{dialect: postgresDialect, tableName: "results"}
	q := pg.insertQuery()
	if !strings.Contains(q, "$1") || !strings.Contains(q, fmt.Sprintf("$%d", len(resultColumns))) {
		t.Errorf("PostgreSQL query must use numbered placeholders $1..$%d: %s", len(resultColumns), q)
	}
	if strings.Contains(q, "?") {
		t.Errorf("PostgreSQL query must not contain ? placeholders: %s", q)
	}

	my := &SQLResultsWriter{dialect: mysqlDialect, tableName: "results"}
	q = my.insertQuery()
	if got := strings.Count(q, "?"); got != len(resultColumns) {
		t.Errorf("MySQL query has %d placeholders, want %d", got, len(resultColumns))
	}
}

func TestWritersDisabledByConfig(t *testing.T) {
	if w, err := NewPostgresResultsWriter(PostgresResultsConfig{}); w != nil || err != nil {
		t.Errorf("disabled PostgreSQL writer: got (%v, %v), want (nil, nil)", w, err)
	}
	if w, err := NewMySQLResultsWriter(MySQLResultsConfig{}); w != nil || err != nil {
		t.Errorf("disabled MySQL writer: got (%v, %v), want (nil, nil)", w, err)
	}
	if w, err := NewSQLiteResultsWriter(SQLiteResultsConfig{}); w != nil || err != nil {
		t.Errorf("disabled SQLite writer: got (%v, %v), want (nil, nil)", w, err)
	}
	// Enabled but without a DSN/path is still disabled
	if w, err := NewPostgresResultsWriter(PostgresResultsConfig{Enabled: true}); w != nil || err != nil {
		t.Errorf("PostgreSQL writer without DSN: got (%v, %v), want (nil, nil)", w, err)
	}
}

func TestSQLiteWriterEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping SQLite end-to-end test in short mode")
	}

	dbPath := filepath.Join(t.TempDir(), "results.db")
	w, err := NewSQLiteResultsWriter(SQLiteResultsConfig{Enabled: true, Path: dbPath})
	if err != nil {
		t.Fatalf("NewSQLiteResultsWriter() error = %v", err)
	}
	if w.Name() != "SQLite" {
		t.Errorf("Name() = %q, want SQLite", w.Name())
	}
	if w.TableName() != "modem_test_results" {
		t.Errorf("TableName() = %q, want default modem_test_results", w.TableName())
	}

	// A record with distinctive values at the start, middle and end of the
	// column list, so a column/argument misalignment cannot go unnoticed.
	rec := &TestRecord{
		Timestamp:      time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC),
		TestNum:        7,
		Phone:          "79001234567",
		ModemName:      "modem1",
		OperatorName:   "Primary",
		OperatorPrefix: "1#",
		NodeAddress:    "2:5001/100",
		NodeSystemName: "Test BBS",
		Success:        true,
		DialTime:       21500 * time.Millisecond,
		ConnectSpeed:   33600,
		ConnectString:  "CONNECT 33600/ARQ/V34",
		EMSITime:       3200 * time.Millisecond,
		RemoteMailer:   "T-Mail 2608",
		TXSpeed:        31200,
		RXSpeed:        33600,
		Protocol:       "V.34",
		LineQuality:    42,
		StatsNotes:     "clean line",
		CDRLocalMOS:    4.3,
		CDRTermReason:  "BYE",
		AstDisposition: "ANSWERED",
		AstHangupCause: 16,
		AstEarlyMedia:  true,
	}
	if err := w.WriteRecord(rec); err != nil {
		t.Fatalf("WriteRecord() error = %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("reopen database: %v", err)
	}
	defer db.Close()

	var (
		phone, connectString, protocol, termReason, disposition string
		testNum, connectSpeed, lineQuality, hangupCause         int
		success, earlyMedia                                     bool
		dialSeconds, localMOS                                   float64
	)
	row := db.QueryRow(`SELECT phone, test_num, success, dial_time_seconds, connect_speed,
		connect_string, protocol, line_quality, cdr_local_mos, cdr_term_reason,
		ast_disposition, ast_hangupcause, ast_early_media FROM modem_test_results`)
	if err := row.Scan(&phone, &testNum, &success, &dialSeconds, &connectSpeed,
		&connectString, &protocol, &lineQuality, &localMOS, &termReason,
		&disposition, &hangupCause, &earlyMedia); err != nil {
		t.Fatalf("read back row: %v", err)
	}

	if phone != rec.Phone {
		t.Errorf("phone = %q, want %q", phone, rec.Phone)
	}
	if testNum != rec.TestNum {
		t.Errorf("test_num = %d, want %d", testNum, rec.TestNum)
	}
	if !success {
		t.Error("success = false, want true")
	}
	if dialSeconds != 21.5 {
		t.Errorf("dial_time_seconds = %v, want 21.5", dialSeconds)
	}
	if connectSpeed != rec.ConnectSpeed {
		t.Errorf("connect_speed = %d, want %d", connectSpeed, rec.ConnectSpeed)
	}
	if connectString != rec.ConnectString {
		t.Errorf("connect_string = %q, want %q", connectString, rec.ConnectString)
	}
	if protocol != rec.Protocol {
		t.Errorf("protocol = %q, want %q", protocol, rec.Protocol)
	}
	if lineQuality != rec.LineQuality {
		t.Errorf("line_quality = %d, want %d", lineQuality, rec.LineQuality)
	}
	if localMOS != rec.CDRLocalMOS {
		t.Errorf("cdr_local_mos = %v, want %v", localMOS, rec.CDRLocalMOS)
	}
	if termReason != rec.CDRTermReason {
		t.Errorf("cdr_term_reason = %q, want %q", termReason, rec.CDRTermReason)
	}
	if disposition != rec.AstDisposition {
		t.Errorf("ast_disposition = %q, want %q", disposition, rec.AstDisposition)
	}
	if hangupCause != rec.AstHangupCause {
		t.Errorf("ast_hangupcause = %d, want %d", hangupCause, rec.AstHangupCause)
	}
	if !earlyMedia {
		t.Error("ast_early_media = false, want true")
	}
}
