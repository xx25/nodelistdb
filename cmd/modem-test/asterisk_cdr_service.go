// Package main provides Asterisk CDR integration for modem testing.
// This service queries Asterisk CDR data from PostgreSQL or MySQL to enrich
// modem test results with call routing and disposition information.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql" // MySQL driver
	_ "github.com/lib/pq"              // PostgreSQL driver
)

// AsteriskCDRData represents call routing info from Asterisk CDR
type AsteriskCDRData struct {
	UniqueID    string
	CallDate    time.Time
	Source      string // src - originating extension
	Destination string // dst - dialed number
	Duration    int    // total duration (ring + talk)
	BillSec     int    // billable seconds (talk time)
	Disposition string // ANSWERED, NO ANSWER, BUSY, FAILED, CONGESTION
	Channel     string // source channel
	DstChannel  string // destination channel (contains peer info)
	Peer        string // extracted peer name from dstchannel

	// SIP-level fields
	HangupCause  int    // SIP hangup cause code (e.g., 16=Normal, 17=Busy, 19=No Answer)
	HangupSource string // Which side hung up (channel name)
	EarlyMedia   bool   // Whether early media (ringback/announcements) was received
}

// retryDispositions are CDR dispositions that trigger retry when billsec=0
var retryDispositions = map[string]bool{
	"NO ANSWER":  true,
	"BUSY":       true,
	"FAILED":     true,
	"CONGESTION": true,
}

// ShouldRetry returns true if the call should be retried based on CDR data.
// A call should be retried if billsec=0 (no actual talk time) AND
// disposition indicates a failed/unanswered call (NO ANSWER, BUSY, FAILED, CONGESTION).
func (cdr *AsteriskCDRData) ShouldRetry() bool {
	if cdr == nil {
		return false // No CDR data, don't retry
	}

	// Only retry if billsec is 0 (no actual talk time)
	if cdr.BillSec > 0 {
		return false
	}

	// Check if disposition is one that warrants retry
	return retryDispositions[cdr.Disposition]
}

// RetryReason returns a human-readable reason if the call should be retried.
// Returns empty string if no retry is needed.
func (cdr *AsteriskCDRData) RetryReason() string {
	if cdr == nil {
		return ""
	}
	if cdr.BillSec > 0 {
		return ""
	}
	if retryDispositions[cdr.Disposition] {
		return fmt.Sprintf("CDR: %s (billsec=0)", cdr.Disposition)
	}
	return ""
}

// q931Causes maps Q.850/Q.931 cause codes to human-readable descriptions.
// Synced with Asterisk 22 include/asterisk/causes.h.
var q931Causes = map[int]string{
	0:   "Not defined",
	1:   "Unallocated number",
	2:   "No route to transit network",
	3:   "No route to destination",
	5:   "Misdialled trunk prefix",
	6:   "Channel unacceptable",
	7:   "Call awarded/delivered",
	8:   "Pre-empted",
	14:  "Number ported, not here",
	16:  "Normal clearing",
	17:  "User busy",
	18:  "No user responding",
	19:  "No answer",
	20:  "Subscriber absent",
	21:  "Call rejected",
	22:  "Number changed",
	23:  "Redirected to new destination",
	26:  "Answered elsewhere",
	27:  "Destination out of order",
	28:  "Invalid number format",
	29:  "Facility rejected",
	30:  "Response to status enquiry",
	31:  "Normal, unspecified",
	34:  "No circuit/channel available",
	38:  "Network out of order",
	41:  "Temporary failure",
	42:  "Switching equipment congestion",
	43:  "Access info discarded",
	44:  "Requested channel not available",
	50:  "Facility not subscribed",
	52:  "Outgoing calls barred",
	54:  "Incoming calls barred",
	57:  "Bearer capability not authorized",
	58:  "Bearer capability not available",
	65:  "Bearer capability not implemented",
	66:  "Channel not implemented",
	69:  "Facility not implemented",
	81:  "Invalid call reference",
	88:  "Incompatible destination",
	95:  "Invalid message, unspecified",
	96:  "Mandatory IE missing",
	97:  "Message type nonexistent",
	98:  "Wrong message",
	99:  "IE nonexistent",
	100: "Invalid IE contents",
	101: "Wrong call state",
	102: "Recovery on timer expiry",
	103: "Mandatory IE length error",
	111: "Protocol error",
	127: "Interworking, unspecified",
}

// HangupCauseString returns a formatted string like "17 (User busy)".
// Falls back to just the number if the code is unknown.
func (cdr *AsteriskCDRData) HangupCauseString() string {
	if desc, ok := q931Causes[cdr.HangupCause]; ok {
		return fmt.Sprintf("%d (%s)", cdr.HangupCause, desc)
	}
	return fmt.Sprintf("%d", cdr.HangupCause)
}

// AsteriskCDRService manages Asterisk CDR database queries
type AsteriskCDRService struct {
	db         *sql.DB
	tableName  string
	timeWindow time.Duration
	enabled    bool
	driver     string // "postgres" or "mysql"
}

// NewAsteriskCDRService creates an Asterisk CDR service from config
func NewAsteriskCDRService(cfg AsteriskCDRConfig) (*AsteriskCDRService, error) {
	if !cfg.Enabled || cfg.DSN == "" {
		return &AsteriskCDRService{enabled: false}, nil
	}

	// Default to postgres for backward compatibility
	driver := cfg.Driver
	if driver == "" {
		driver = "postgres"
	}
	if driver != "postgres" && driver != "mysql" {
		return nil, fmt.Errorf("unsupported Asterisk CDR database driver: %s (use 'postgres' or 'mysql')", driver)
	}

	db, err := sql.Open(driver, cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("failed to open Asterisk CDR database: %w", err)
	}

	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(time.Hour)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping Asterisk CDR database: %w", err)
	}

	tableName := cfg.TableName
	if tableName == "" {
		tableName = "cdr"
	}

	timeWindow := time.Duration(cfg.TimeWindowSec) * time.Second
	if timeWindow == 0 {
		timeWindow = 120 * time.Second
	}

	return &AsteriskCDRService{
		db:         db,
		tableName:  tableName,
		timeWindow: timeWindow,
		enabled:    true,
		driver:     driver,
	}, nil
}

// IsEnabled returns true if Asterisk CDR service is active
func (s *AsteriskCDRService) IsEnabled() bool {
	return s.enabled
}

// LookupByPhone queries Asterisk CDR by destination phone number and time window
func (s *AsteriskCDRService) LookupByPhone(ctx context.Context, phone string, callTime time.Time) (*AsteriskCDRData, error) {
	if !s.enabled || s.db == nil {
		return nil, nil
	}

	// Query CDR within time window, matching dst
	// Use calldate as the anchor (call start time)
	startTime := callTime.Add(-s.timeWindow)
	endTime := callTime.Add(s.timeWindow)

	// Match phone - may need wildcard prefix for full numbers
	phonePattern := "%" + phone

	var query string
	var row *sql.Row

	if s.driver == "mysql" {
		// Format times as strings to avoid Go MySQL driver's UTC conversion
		// (database stores local time, driver converts time.Time to UTC)
		const timeFmt = "2006-01-02 15:04:05"
		startStr := startTime.Format(timeFmt)
		endStr := endTime.Format(timeFmt)
		callTimeStr := callTime.Format(timeFmt)

		// MySQL query with ? placeholders and TIMESTAMPDIFF
		query = fmt.Sprintf(`
			SELECT uniqueid, calldate, src, dst, duration, billsec,
			       disposition, channel, dstchannel,
			       hangupcause, hangupsource, early_media_rx
			FROM %s
			WHERE dst LIKE ?
			  AND calldate BETWEEN ? AND ?
			ORDER BY ABS(TIMESTAMPDIFF(SECOND, calldate, ?)) ASC
			LIMIT 1
		`, s.tableName)
		row = s.db.QueryRowContext(ctx, query, phonePattern, startStr, endStr, callTimeStr)
	} else {
		// PostgreSQL query with $N placeholders and EXTRACT(EPOCH)
		query = fmt.Sprintf(`
			SELECT uniqueid, calldate, src, dst, duration, billsec,
			       disposition, channel, dstchannel,
			       hangupcause, hangupsource, early_media_rx
			FROM %s
			WHERE dst LIKE $1
			  AND calldate BETWEEN $2 AND $3
			ORDER BY ABS(EXTRACT(EPOCH FROM (calldate - $4))) ASC
			LIMIT 1
		`, s.tableName)
		row = s.db.QueryRowContext(ctx, query, phonePattern, startTime, endTime, callTime)
	}

	var cdr AsteriskCDRData
	var uniqueID, src, dst, disposition, channel, dstChannel sql.NullString
	var hangupsource sql.NullString
	var callDate sql.NullTime
	var duration, billSec, hangupcause, earlyMediaRx sql.NullInt64

	err := row.Scan(
		&uniqueID, &callDate, &src, &dst, &duration, &billSec,
		&disposition, &channel, &dstChannel,
		&hangupcause, &hangupsource, &earlyMediaRx,
	)

	if err == sql.ErrNoRows {
		return nil, nil // No matching CDR found
	}
	if err != nil {
		return nil, fmt.Errorf("Asterisk CDR query failed: %w", err)
	}

	// Map nullable values to AsteriskCDRData
	cdr.UniqueID = uniqueID.String
	cdr.Source = src.String
	cdr.Destination = dst.String
	cdr.Disposition = disposition.String
	cdr.Channel = channel.String
	cdr.DstChannel = dstChannel.String
	cdr.HangupSource = hangupsource.String

	if callDate.Valid {
		cdr.CallDate = callDate.Time
	}
	if duration.Valid {
		cdr.Duration = int(duration.Int64)
	}
	if billSec.Valid {
		cdr.BillSec = int(billSec.Int64)
	}
	if hangupcause.Valid {
		cdr.HangupCause = int(hangupcause.Int64)
	}
	if earlyMediaRx.Valid {
		cdr.EarlyMedia = earlyMediaRx.Int64 != 0
	}

	// Extract peer name from dstchannel
	// Format: PJSIP/peer-uniqueid or SIP/peer-uniqueid or DAHDI/channel-uniqueid
	cdr.Peer = extractPeerFromChannel(cdr.DstChannel)

	return &cdr, nil
}

// extractPeerFromChannel extracts the peer/trunk name from an Asterisk channel string
// Examples:
//   - "PJSIP/bbs-00000041" -> "bbs"
//   - "SIP/trunk1-00001234" -> "trunk1"
//   - "DAHDI/1-1" -> "1"
//   - "" -> ""
func extractPeerFromChannel(channel string) string {
	if channel == "" {
		return ""
	}

	// Find the / separator
	slashIdx := strings.Index(channel, "/")
	if slashIdx < 0 {
		return channel // No slash, return as-is
	}

	// Get everything after the slash
	afterSlash := channel[slashIdx+1:]

	// Find the - separator (before the unique ID)
	dashIdx := strings.Index(afterSlash, "-")
	if dashIdx < 0 {
		return afterSlash // No dash, return everything after slash
	}

	return afterSlash[:dashIdx]
}

// Close closes the database connection
func (s *AsteriskCDRService) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}
