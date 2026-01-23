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

	// Multi-modem support
	ModemName string

	// Operator routing support (for comparing different carriers/routes)
	OperatorName   string // Friendly name (e.g., "Verizon", "VoIP-A")
	OperatorPrefix string // Dial prefix used (e.g., "1#", "2#")

	// Remote system info (from EMSI)
	RemoteAddress  string
	RemoteSystem   string
	RemoteLocation string
	RemoteSysop    string
	RemoteMailer   string

	// Line statistics (parsed)
	TXSpeed     int
	RXSpeed     int
	Protocol    string
	Compression string
	LineQuality int
	RxLevel     int
	Retrains    int
	Termination string
	StatsNotes  string

	// CDR VoIP Quality Metrics (from AudioCodes gateway)
	CDRSessionID        string
	CDRCodec            string
	CDRRTPJitter        int     // ms
	CDRRTPDelay         int     // ms
	CDRPacketLoss       int
	CDRRemotePacketLoss int
	CDRLocalMOS         float64 // 1.0-5.0 scale
	CDRRemoteMOS        float64
	CDRLocalRFactor     int
	CDRRemoteRFactor    int
	CDRTermReason       string
	CDRTermCategory     string

	// Asterisk CDR fields (call routing info)
	AstDisposition string // ANSWERED, NO ANSWER, BUSY, FAILED
	AstPeer        string // Outbound peer/trunk name
	AstDuration    int    // Total duration (ring + talk)
	AstBillSec     int    // Billable seconds (talk time)
}

// csvHeader is the current header format with modem_name column
var csvHeader = []string{
	"timestamp",
	"test_num",
	"phone",
	"modem_name",
	"operator_name",
	"operator_prefix",
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
	// CDR VoIP quality metrics (AudioCodes)
	"cdr_session_id",
	"cdr_codec",
	"cdr_rtp_jitter_ms",
	"cdr_rtp_delay_ms",
	"cdr_packet_loss",
	"cdr_remote_packet_loss",
	"cdr_local_mos",
	"cdr_remote_mos",
	"cdr_local_r_factor",
	"cdr_remote_r_factor",
	"cdr_term_reason",
	"cdr_term_category",
	// Asterisk CDR fields
	"ast_disposition",
	"ast_peer",
	"ast_duration",
	"ast_billsec",
}

// NewCSVWriter creates a new CSV writer for the given file path
// If the file doesn't exist, it creates it with a header row
// If the file exists, it checks header compatibility before appending
func NewCSVWriter(path string) (*CSVWriter, error) {
	// Check if file exists and validate header
	exists := false
	if info, err := os.Stat(path); err == nil && info.Size() > 0 {
		exists = true
		// Check header compatibility
		if err := checkCSVHeader(path); err != nil {
			return nil, err
		}
	}

	// Open file for append (create if doesn't exist)
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open CSV file: %w", err)
	}

	writer := csv.NewWriter(file)

	// Write header if new file
	if !exists {
		if err := writer.Write(csvHeader); err != nil {
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

// checkCSVHeader verifies the existing CSV file has a compatible header
func checkCSVHeader(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open CSV file for header check: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	header, err := reader.Read()
	if err != nil {
		return fmt.Errorf("failed to read CSV header: %w", err)
	}

	// Check header length matches expected
	if len(header) != len(csvHeader) {
		return fmt.Errorf("CSV file %s has %d columns, expected %d. "+
			"Use a new file or delete the existing one", path, len(header), len(csvHeader))
	}

	// Check if header has modem_name column in correct position
	if header[3] != "modem_name" {
		return fmt.Errorf("CSV file %s has incompatible header format (missing or misplaced modem_name column). "+
			"Use a new file or delete the existing one", path)
	}

	return nil
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
		rec.ModemName,
		rec.OperatorName,
		rec.OperatorPrefix,
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
		// CDR fields (AudioCodes)
		rec.CDRSessionID,
		rec.CDRCodec,
		fmt.Sprintf("%d", rec.CDRRTPJitter),
		fmt.Sprintf("%d", rec.CDRRTPDelay),
		fmt.Sprintf("%d", rec.CDRPacketLoss),
		fmt.Sprintf("%d", rec.CDRRemotePacketLoss),
		fmt.Sprintf("%.1f", rec.CDRLocalMOS),
		fmt.Sprintf("%.1f", rec.CDRRemoteMOS),
		fmt.Sprintf("%d", rec.CDRLocalRFactor),
		fmt.Sprintf("%d", rec.CDRRemoteRFactor),
		rec.CDRTermReason,
		rec.CDRTermCategory,
		// Asterisk CDR fields
		rec.AstDisposition,
		rec.AstPeer,
		fmt.Sprintf("%d", rec.AstDuration),
		fmt.Sprintf("%d", rec.AstBillSec),
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
	operatorName string,
	operatorPrefix string,
	success bool,
	dialTime time.Duration,
	connectSpeed int,
	connectString string,
	emsiTime time.Duration,
	emsiErr error,
	emsiInfo *EMSIDetails,
	lineStats *LineStats,
	cdrData *CDRData,
	asteriskCDR *AsteriskCDRData,
) *TestRecord {
	rec := &TestRecord{
		Timestamp:      time.Now(),
		TestNum:        testNum,
		Phone:          phone,
		OperatorName:   operatorName,
		OperatorPrefix: operatorPrefix,
		Success:        success,
		DialTime:       dialTime,
		ConnectSpeed:   connectSpeed,
		ConnectString:  connectString,
		EMSITime:       emsiTime,
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

	// Add CDR VoIP quality metrics if available (AudioCodes)
	if cdrData != nil {
		rec.CDRSessionID = cdrData.SessionID
		rec.CDRCodec = cdrData.Codec
		rec.CDRRTPJitter = cdrData.RTPJitter
		rec.CDRRTPDelay = cdrData.RTPDelay
		rec.CDRPacketLoss = cdrData.PacketLoss
		rec.CDRRemotePacketLoss = cdrData.RemotePacketLoss
		// Convert MOS from integer (e.g., 43) to float (4.3)
		rec.CDRLocalMOS = float64(cdrData.LocalMOSCQ) / 10.0
		rec.CDRRemoteMOS = float64(cdrData.RemoteMOSCQ) / 10.0
		rec.CDRLocalRFactor = cdrData.LocalRFactor
		rec.CDRRemoteRFactor = cdrData.RemoteRFactor
		rec.CDRTermReason = cdrData.TermReason
		rec.CDRTermCategory = cdrData.TermReasonCategory
	}

	// Add Asterisk CDR data if available
	if asteriskCDR != nil {
		rec.AstDisposition = asteriskCDR.Disposition
		rec.AstPeer = asteriskCDR.Peer
		rec.AstDuration = asteriskCDR.Duration
		rec.AstBillSec = asteriskCDR.BillSec
	}

	return rec
}
