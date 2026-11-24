// Package main tests the CSV results writer's handling of the header across
// versions.
package main

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func readCSV(t *testing.T, path string) [][]string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1 // report widths instead of failing on them
	rows, err := r.ReadAll()
	if err != nil {
		t.Fatalf("read CSV: %v", err)
	}
	return rows
}

func voiceRecord() *TestRecord {
	return &TestRecord{
		Timestamp:     time.Date(2026, 11, 23, 22, 15, 4, 0, time.UTC),
		TestNum:       7,
		Phone:         "74951234567",
		NodeAddress:   "2:5020/100",
		AstEarlyMedia: true,
		VoiceAnswered: true,
		FailureDetail: "voice answered",
	}
}

func TestCSVWriter_NewFileEndsWithVoiceColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "results.csv")
	w, err := NewCSVWriter(path)
	if err != nil {
		t.Fatalf("NewCSVWriter() error = %v", err)
	}
	if w.LegacyLayout() {
		t.Error("a new file reported as legacy layout")
	}
	if err := w.WriteRecord(voiceRecord()); err != nil {
		t.Fatalf("WriteRecord() error = %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	rows := readCSV(t, path)
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want header + 1", len(rows))
	}
	header, row := rows[0], rows[1]
	n := len(header)
	// Appended at the END: every earlier column keeps its position for
	// whatever already reads these files.
	if header[n-2] != "voice_answered" || header[n-1] != "failure_detail" {
		t.Errorf("header ends with %q, %q; want voice_answered, failure_detail", header[n-2], header[n-1])
	}
	if header[n-3] != "ast_early_media" {
		t.Errorf("column before the new ones = %q, want ast_early_media", header[n-3])
	}
	if len(row) != n {
		t.Fatalf("row has %d fields, header %d", len(row), n)
	}
	if row[n-3] != "true" || row[n-2] != "true" || row[n-1] != "voice answered" {
		t.Errorf("row ends with %q, want [true true voice answered]", row[n-3:])
	}
}

func TestCSVWriter_ExistingFileWithOlderHeaderKeepsItsLayout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "results.csv")
	oldHeader := csvHeader[:csvLegacyColumns]
	if oldHeader[len(oldHeader)-1] != "ast_early_media" {
		t.Fatalf("csvLegacyColumns does not end the header at ast_early_media: %q", oldHeader[len(oldHeader)-1])
	}
	if err := os.WriteFile(path, []byte(strings.Join(oldHeader, ",")+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	w, err := NewCSVWriter(path)
	if err != nil {
		t.Fatalf("NewCSVWriter() rejected a file with the older header: %v", err)
	}
	if !w.LegacyLayout() {
		t.Error("LegacyLayout() = false for a file with the older header")
	}
	if err := w.WriteRecord(voiceRecord()); err != nil {
		t.Fatalf("WriteRecord() error = %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	rows := readCSV(t, path)
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want header + 1", len(rows))
	}
	// Rows must stay as wide as the file's own header, or it stops parsing.
	if len(rows[1]) != len(oldHeader) {
		t.Errorf("row has %d fields in a %d-column file", len(rows[1]), len(oldHeader))
	}
	if last := rows[1][len(rows[1])-1]; last != "true" {
		t.Errorf("last field = %q, want ast_early_media's true", last)
	}
}

func TestCSVWriter_ExistingFileWithCurrentHeaderIsAppended(t *testing.T) {
	path := filepath.Join(t.TempDir(), "results.csv")
	for i := 0; i < 2; i++ {
		w, err := NewCSVWriter(path)
		if err != nil {
			t.Fatalf("NewCSVWriter() #%d error = %v", i+1, err)
		}
		if err := w.WriteRecord(voiceRecord()); err != nil {
			t.Fatal(err)
		}
		w.Close()
	}
	rows := readCSV(t, path)
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want one header + 2", len(rows))
	}
	for i, row := range rows {
		if len(row) != len(csvHeader) {
			t.Errorf("row %d has %d fields, want %d", i, len(row), len(csvHeader))
		}
	}
}

func TestCSVWriter_UnknownHeaderIsStillRejected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "results.csv")
	if err := os.WriteFile(path, []byte("timestamp,test_num,phone\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewCSVWriter(path); err == nil {
		t.Error("NewCSVWriter() accepted a file with an unrelated header")
	}
}

func TestNodelistDBWriter_ConvertRecordCarriesVoiceFields(t *testing.T) {
	w := &NodelistDBWriter{}
	req := w.convertRecord(voiceRecord())
	if !req.VoiceAnswered || req.FailureDetail != "voice answered" {
		t.Errorf("request = voice %v, detail %q; want true, voice answered", req.VoiceAnswered, req.FailureDetail)
	}
	if req.Zone != 2 || req.Net != 5020 || req.Node != 100 || req.PhoneDialed != "74951234567" {
		t.Errorf("request addresses %d:%d/%d phone %q", req.Zone, req.Net, req.Node, req.PhoneDialed)
	}
}
