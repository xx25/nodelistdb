// Package main provides Asterisk CDR integration for modem testing.
// This service queries Asterisk CDR data from PostgreSQL to enrich
// modem test results with call routing and disposition information.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/lib/pq" // PostgreSQL driver
)

// AsteriskCDRData represents call routing info from Asterisk CDR
type AsteriskCDRData struct {
	UniqueID    string
	CallDate    time.Time
	Source      string // src - originating extension
	Destination string // dst - dialed number
	Duration    int    // total duration (ring + talk)
	BillSec     int    // billable seconds (talk time)
	Disposition string // ANSWERED, NO ANSWER, BUSY, FAILED
	Channel     string // source channel
	DstChannel  string // destination channel (contains peer info)
	Peer        string // extracted peer name from dstchannel
}

// AsteriskCDRService manages Asterisk CDR database queries
type AsteriskCDRService struct {
	db         *sql.DB
	tableName  string
	timeWindow time.Duration
	enabled    bool
}

// NewAsteriskCDRService creates an Asterisk CDR service from config
func NewAsteriskCDRService(cfg AsteriskCDRConfig) (*AsteriskCDRService, error) {
	if !cfg.Enabled || cfg.DSN == "" {
		return &AsteriskCDRService{enabled: false}, nil
	}

	db, err := sql.Open("postgres", cfg.DSN)
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

	query := fmt.Sprintf(`
		SELECT uniqueid, calldate, src, dst, duration, billsec,
		       disposition, channel, dstchannel
		FROM %s
		WHERE dst LIKE $1
		  AND calldate BETWEEN $2 AND $3
		ORDER BY ABS(EXTRACT(EPOCH FROM (calldate - $4))) ASC
		LIMIT 1
	`, s.tableName)

	// Match phone - may need wildcard prefix for full numbers
	phonePattern := "%%" + phone

	row := s.db.QueryRowContext(ctx, query, phonePattern, startTime, endTime, callTime)

	var cdr AsteriskCDRData
	var uniqueID, src, dst, disposition, channel, dstChannel sql.NullString
	var callDate sql.NullTime
	var duration, billSec sql.NullInt64

	err := row.Scan(
		&uniqueID, &callDate, &src, &dst, &duration, &billSec,
		&disposition, &channel, &dstChannel,
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

	if callDate.Valid {
		cdr.CallDate = callDate.Time
	}
	if duration.Valid {
		cdr.Duration = int(duration.Int64)
	}
	if billSec.Valid {
		cdr.BillSec = int(billSec.Int64)
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
