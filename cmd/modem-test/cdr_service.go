// Package main provides CDR (Call Detail Record) integration for modem testing.
// This service queries AudioCodes CDR data from PostgreSQL or MySQL to enrich
// modem test results with VoIP quality metrics.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql" // MySQL driver
	_ "github.com/lib/pq"              // PostgreSQL driver
)

// CDRData represents VoIP quality metrics from AudioCodes CDR
type CDRData struct {
	SessionID   string
	SetupTime   time.Time
	ConnectTime time.Time
	ReleaseTime time.Time
	Duration    int
	SrcPhoneNum string
	DstPhoneNum string
	Codec       string

	// Quality metrics
	RTPJitter        int // in ms
	RTPDelay         int // in ms
	PacketLoss       int // local packet loss
	RemotePacketLoss int // remote packet loss

	// MOS/R-Factor scores (raw values from CDR)
	LocalMOSCQ   int // MOS score (typically 0-50, representing 0.0-5.0)
	RemoteMOSCQ  int
	LocalRFactor int // R-factor (0-100)
	RemoteRFactor int

	// Termination info
	TermReason         string
	TermReasonCategory string
	TermSide           string // LCL/RMT/UNKN
	PSTNTermReason     int
}

// CDRService manages CDR database queries
type CDRService struct {
	db         *sql.DB
	tableName  string
	timeWindow time.Duration
	enabled    bool
	driver     string // "postgres" or "mysql"
}

// NewCDRService creates a CDR service from config
func NewCDRService(cfg CDRConfig) (*CDRService, error) {
	if !cfg.Enabled || cfg.DSN == "" {
		return &CDRService{enabled: false}, nil
	}

	// Default to postgres for backward compatibility
	driver := cfg.Driver
	if driver == "" {
		driver = "postgres"
	}
	if driver != "postgres" && driver != "mysql" {
		return nil, fmt.Errorf("unsupported CDR database driver: %s (use 'postgres' or 'mysql')", driver)
	}

	db, err := sql.Open(driver, cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("failed to open CDR database: %w", err)
	}

	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(time.Hour)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping CDR database: %w", err)
	}

	tableName := cfg.TableName
	if tableName == "" {
		tableName = "cdr"
	}

	timeWindow := time.Duration(cfg.TimeWindowSec) * time.Second
	if timeWindow == 0 {
		timeWindow = 120 * time.Second
	}

	return &CDRService{
		db:         db,
		tableName:  tableName,
		timeWindow: timeWindow,
		enabled:    true,
		driver:     driver,
	}, nil
}

// IsEnabled returns true if CDR service is active
func (s *CDRService) IsEnabled() bool {
	return s.enabled
}

// LookupByPhone queries CDR by destination phone number and time window
func (s *CDRService) LookupByPhone(ctx context.Context, phone string, callTime time.Time) (*CDRData, error) {
	if !s.enabled || s.db == nil {
		return nil, nil
	}

	// Query CDR within time window, matching dst_phone_num
	// Use release_time as the anchor since CDR is written at call end
	startTime := callTime.Add(-s.timeWindow)
	endTime := callTime.Add(s.timeWindow)

	// Match phone with wildcard prefix (CDR may have full number with area code)
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
			SELECT session_id, setup_time, connect_time, release_time, durat,
			       src_phone_num, dst_phone_num, coder,
			       rtp_jitter, rtp_delay, pack_loss, remote_pack_loss,
			       local_mos_cq, remote_mos_cq, local_r_factor, remote_r_factor,
			       trm_reason, trm_reason_category, trm_sd, pstn_term_reason
			FROM %s
			WHERE dst_phone_num LIKE ?
			  AND (setup_time BETWEEN ? AND ?
			       OR release_time BETWEEN ? AND ?
			       OR connect_time BETWEEN ? AND ?)
			ORDER BY ABS(TIMESTAMPDIFF(SECOND, COALESCE(release_time, setup_time), ?)) ASC
			LIMIT 1
		`, s.tableName)
		row = s.db.QueryRowContext(ctx, query, phonePattern, startStr, endStr, startStr, endStr, startStr, endStr, callTimeStr)
	} else {
		// PostgreSQL query with $N placeholders and EXTRACT(EPOCH)
		query = fmt.Sprintf(`
			SELECT session_id, setup_time, connect_time, release_time, durat,
			       src_phone_num, dst_phone_num, coder,
			       rtp_jitter, rtp_delay, pack_loss, remote_pack_loss,
			       local_mos_cq, remote_mos_cq, local_r_factor, remote_r_factor,
			       trm_reason, trm_reason_category, trm_sd, pstn_term_reason
			FROM %s
			WHERE dst_phone_num LIKE $1
			  AND (setup_time BETWEEN $2 AND $3
			       OR release_time BETWEEN $2 AND $3
			       OR connect_time BETWEEN $2 AND $3)
			ORDER BY ABS(EXTRACT(EPOCH FROM (COALESCE(release_time, setup_time) - $4))) ASC
			LIMIT 1
		`, s.tableName)
		row = s.db.QueryRowContext(ctx, query, phonePattern, startTime, endTime, callTime)
	}

	var cdr CDRData
	var setupTime, connectTime, releaseTime sql.NullTime
	var durat, rtpJitter, rtpDelay, packLoss, remotePackLoss sql.NullInt64
	var localMOS, remoteMOS, localR, remoteR, pstnTerm sql.NullInt64
	var sessionID, srcPhone, dstPhone, coder sql.NullString
	var trmReason, trmCategory, trmSide sql.NullString

	err := row.Scan(
		&sessionID, &setupTime, &connectTime, &releaseTime, &durat,
		&srcPhone, &dstPhone, &coder,
		&rtpJitter, &rtpDelay, &packLoss, &remotePackLoss,
		&localMOS, &remoteMOS, &localR, &remoteR,
		&trmReason, &trmCategory, &trmSide, &pstnTerm,
	)

	if err == sql.ErrNoRows {
		return nil, nil // No matching CDR found
	}
	if err != nil {
		return nil, fmt.Errorf("CDR query failed: %w", err)
	}

	// Map nullable values to CDRData
	cdr.SessionID = sessionID.String
	cdr.SrcPhoneNum = srcPhone.String
	cdr.DstPhoneNum = dstPhone.String
	cdr.Codec = coder.String

	if setupTime.Valid {
		cdr.SetupTime = setupTime.Time
	}
	if connectTime.Valid {
		cdr.ConnectTime = connectTime.Time
	}
	if releaseTime.Valid {
		cdr.ReleaseTime = releaseTime.Time
	}
	if durat.Valid {
		cdr.Duration = int(durat.Int64)
	}

	if rtpJitter.Valid {
		cdr.RTPJitter = int(rtpJitter.Int64)
	}
	if rtpDelay.Valid {
		cdr.RTPDelay = int(rtpDelay.Int64)
	}
	if packLoss.Valid {
		cdr.PacketLoss = int(packLoss.Int64)
	}
	if remotePackLoss.Valid {
		cdr.RemotePacketLoss = int(remotePackLoss.Int64)
	}

	if localMOS.Valid {
		cdr.LocalMOSCQ = int(localMOS.Int64)
	}
	if remoteMOS.Valid {
		cdr.RemoteMOSCQ = int(remoteMOS.Int64)
	}
	if localR.Valid {
		cdr.LocalRFactor = int(localR.Int64)
	}
	if remoteR.Valid {
		cdr.RemoteRFactor = int(remoteR.Int64)
	}

	cdr.TermReason = trmReason.String
	cdr.TermReasonCategory = trmCategory.String
	cdr.TermSide = trmSide.String
	if pstnTerm.Valid {
		cdr.PSTNTermReason = int(pstnTerm.Int64)
	}

	return &cdr, nil
}

// Close closes the database connection
func (s *CDRService) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}
