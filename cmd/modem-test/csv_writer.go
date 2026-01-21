// Package main provides CSV output for modem test results.
package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"strings"
	"time"
)

// CSVWriter writes test results to a CSV file
type CSVWriter struct {
	file   *os.File
	writer *csv.Writer
}

// TestRecord represents a single test result for CSV output
type TestRecord struct {
	Timestamp     time.Time
	TestNum       int
	Phone         string
	Success       bool
	DialTime      time.Duration
	ConnectSpeed  int
	ConnectString string
	EMSITime      time.Duration
	EMSIError     string

	// Remote system info (from EMSI)
	RemoteAddress string
	RemoteSystem  string
	RemoteLocation string
	RemoteSysop   string
	RemoteMailer  string

	// Line statistics (parsed)
	TXSpeed       int
	RXSpeed       int
	Protocol      string
	Compression   string
	LineQuality   int
	RxLevel       int
	Retrains      int
	Termination   string
	StatsNotes    string
}

// NewCSVWriter creates a new CSV writer for the given file path
// If the file doesn't exist, it creates it with a header row
// If the file exists, it appends to it
func NewCSVWriter(path string) (*CSVWriter, error) {
	// Check if file exists
	exists := false
	if _, err := os.Stat(path); err == nil {
		exists = true
	}

	// Open file for append (create if doesn't exist)
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open CSV file: %w", err)
	}

	writer := csv.NewWriter(file)

	// Write header if new file
	if !exists {
		header := []string{
			"timestamp",
			"test_num",
			"phone",
			"success",
			"dial_time_s",
			"connect_speed",
			"connect_string",
			"emsi_time_s",
			"emsi_error",
			"remote_address",
			"remote_system",
			"remote_location",
			"remote_sysop",
			"remote_mailer",
			"tx_speed",
			"rx_speed",
			"protocol",
			"compression",
			"line_quality",
			"rx_level",
			"retrains",
			"termination",
			"stats_notes",
		}
		if err := writer.Write(header); err != nil {
			file.Close()
			return nil, fmt.Errorf("failed to write CSV header: %w", err)
		}
		writer.Flush()
	}

	return &CSVWriter{
		file:   file,
		writer: writer,
	}, nil
}

// WriteRecord writes a test record to the CSV file
func (w *CSVWriter) WriteRecord(rec *TestRecord) error {
	success := "0"
	if rec.Success {
		success = "1"
	}

	row := []string{
		rec.Timestamp.Format(time.RFC3339),
		fmt.Sprintf("%d", rec.TestNum),
		rec.Phone,
		success,
		fmt.Sprintf("%.1f", rec.DialTime.Seconds()),
		fmt.Sprintf("%d", rec.ConnectSpeed),
		rec.ConnectString,
		fmt.Sprintf("%.1f", rec.EMSITime.Seconds()),
		rec.EMSIError,
		rec.RemoteAddress,
		rec.RemoteSystem,
		rec.RemoteLocation,
		rec.RemoteSysop,
		rec.RemoteMailer,
		fmt.Sprintf("%d", rec.TXSpeed),
		fmt.Sprintf("%d", rec.RXSpeed),
		rec.Protocol,
		rec.Compression,
		fmt.Sprintf("%d", rec.LineQuality),
		fmt.Sprintf("%d", rec.RxLevel),
		fmt.Sprintf("%d", rec.Retrains),
		rec.Termination,
		rec.StatsNotes,
	}

	if err := w.writer.Write(row); err != nil {
		return fmt.Errorf("failed to write CSV row: %w", err)
	}
	w.writer.Flush()
	return w.writer.Error()
}

// Close closes the CSV file
func (w *CSVWriter) Close() error {
	w.writer.Flush()
	return w.file.Close()
}

// RecordFromTestResult creates a TestRecord from test result data
func RecordFromTestResult(
	testNum int,
	phone string,
	success bool,
	dialTime time.Duration,
	connectSpeed int,
	connectString string,
	emsiTime time.Duration,
	emsiErr error,
	emsiInfo *EMSIDetails,
	lineStats *LineStats,
) *TestRecord {
	rec := &TestRecord{
		Timestamp:    time.Now(),
		TestNum:      testNum,
		Phone:        phone,
		Success:      success,
		DialTime:     dialTime,
		ConnectSpeed: connectSpeed,
		ConnectString: connectString,
		EMSITime:     emsiTime,
	}

	if emsiErr != nil {
		rec.EMSIError = "error" // Just flag that error occurred, details in log
	}

	if emsiInfo != nil {
		if len(emsiInfo.Addresses) > 0 {
			rec.RemoteAddress = strings.Join(emsiInfo.Addresses, " ")
		}
		rec.RemoteSystem = emsiInfo.SystemName
		rec.RemoteLocation = emsiInfo.Location
		rec.RemoteSysop = emsiInfo.Sysop
		if emsiInfo.MailerName != "" {
			rec.RemoteMailer = emsiInfo.MailerName
			if emsiInfo.MailerVersion != "" {
				rec.RemoteMailer += " " + emsiInfo.MailerVersion
			}
		}
	}

	if lineStats != nil {
		rec.TXSpeed = lineStats.LastTXRate
		rec.RXSpeed = lineStats.LastRXRate
		rec.Protocol = lineStats.Protocol
		rec.Compression = lineStats.Compression
		rec.LineQuality = lineStats.LineQuality
		rec.RxLevel = lineStats.RxLevel
		rec.Retrains = lineStats.LocalRetrain + lineStats.RemoteRetrain
		rec.Termination = lineStats.TerminationReason
		if len(lineStats.Messages) > 0 {
			rec.StatsNotes = strings.Join(lineStats.Messages, "; ")
		}
	}

	return rec
}
