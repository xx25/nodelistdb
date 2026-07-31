package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/nodelistdb/internal/testing/models"
)

// whoisInsertSQL carries no VALUES placeholders on purpose, because it is
// prepared with PrepareBatch and streamed in the native binary format.
//
// The Exec(ctx, "... VALUES (?, ...)", args...) form has no server-side bind:
// clickhouse-go renders the arguments into SQL text, and a time.Time becomes
// toDateTime('<unix seconds>'). The server's Values fast-path parser accepts
// only literals, so it throws CANNOT_PARSE_INPUT_ASSERTION_FAILED, then falls
// back to the expression interpreter and succeeds. The row lands and no query
// ever fails, so the only visible trace is system.errors: measured at two bumps
// per call, one per toDateTime() rendered into the row, or roughly 400/day from
// the WHOIS sweep alone. Keep this batch-shaped.
const whoisInsertSQL = `INSERT INTO domain_whois_cache
	(domain, expiration_date, creation_date, registrar, whois_status, check_time, check_error)`

// StoreWhoisResult stores a WHOIS lookup result in the domain_whois_cache table
func (s *ClickHouseStorage) StoreWhoisResult(ctx context.Context, result *models.WhoisResult) error {
	batch, err := s.conn.PrepareBatch(ctx, whoisInsertSQL)
	if err != nil {
		return fmt.Errorf("failed to prepare batch: %w", err)
	}

	// Nullable(DateTime) columns: a nil *time.Time appends as NULL.
	err = batch.Append(
		result.Domain,
		result.ExpirationDate,
		result.CreationDate,
		result.Registrar,
		result.Status,
		time.Now(),
		result.Error,
	)
	if err != nil {
		return fmt.Errorf("failed to append to batch: %w", err)
	}

	if err := batch.Send(); err != nil {
		return fmt.Errorf("failed to send batch: %w", err)
	}

	return nil
}

// GetRecentWhoisResult retrieves a cached WHOIS result if it was checked within maxAge
func (s *ClickHouseStorage) GetRecentWhoisResult(ctx context.Context, domain string, maxAge time.Duration) (*models.WhoisResult, error) {
	query := `SELECT
		domain, expiration_date, creation_date, registrar, whois_status, check_time, check_error
		FROM domain_whois_cache
		WHERE domain = ? AND check_time >= ?
		ORDER BY check_time DESC
		LIMIT 1`

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
