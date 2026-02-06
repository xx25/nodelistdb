package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/nodelistdb/internal/testing/models"
)

// StoreWhoisResult stores a WHOIS lookup result in the domain_whois_cache table
func (s *ClickHouseStorage) StoreWhoisResult(ctx context.Context, result *models.WhoisResult) error {
	query := `INSERT INTO domain_whois_cache
		(domain, expiration_date, creation_date, registrar, whois_status, check_time, check_error)
		VALUES (?, ?, ?, ?, ?, ?, ?)`

	var expirationDate, creationDate *time.Time
	if result.ExpirationDate != nil {
		expirationDate = result.ExpirationDate
	}
	if result.CreationDate != nil {
		creationDate = result.CreationDate
	}

	return s.conn.Exec(ctx, query,
		result.Domain,
		expirationDate,
		creationDate,
		result.Registrar,
		result.Status,
		time.Now(),
		result.Error,
	)
}

// GetRecentWhoisResult retrieves a cached WHOIS result if it was checked within maxAge
func (s *ClickHouseStorage) GetRecentWhoisResult(ctx context.Context, domain string, maxAge time.Duration) (*models.WhoisResult, error) {
	query := `SELECT
		domain, expiration_date, creation_date, registrar, whois_status, check_time, check_error
		FROM domain_whois_cache FINAL
		WHERE domain = ? AND check_time >= ?`

	cutoff := time.Now().Add(-maxAge)
	row := s.db.QueryRowContext(ctx, query, domain, cutoff)

	var (
		d              string
		expirationDate sql.NullTime
		creationDate   sql.NullTime
		registrar      string
		whoisStatus    string
		checkTime      time.Time
		checkError     string
	)

	if err := row.Scan(&d, &expirationDate, &creationDate, &registrar, &whoisStatus, &checkTime, &checkError); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("no recent WHOIS result for %s", domain)
		}
		return nil, err
	}

	result := &models.WhoisResult{
		Domain:    d,
		Registrar: registrar,
		Status:    whoisStatus,
		Error:     checkError,
		Cached:    true,
	}

	if expirationDate.Valid {
		t := expirationDate.Time
		result.ExpirationDate = &t
	}
	if creationDate.Valid {
		t := creationDate.Time
		result.CreationDate = &t
	}

	return result, nil
}
