package storage

import (
	"context"
	"database/sql"
	"time"

	"github.com/nodelistdb/internal/database"
	"github.com/nodelistdb/internal/testing/services"
)

// DomainWhoisResult represents a WHOIS lookup result for the analytics page
type DomainWhoisResult struct {
	Domain         string     `json:"domain"`
	ExpirationDate *time.Time `json:"expiration_date"`
	CreationDate   *time.Time `json:"creation_date"`
	Registrar      string     `json:"registrar"`
	WhoisStatus    string     `json:"whois_status"`
	CheckTime      time.Time  `json:"check_time"`
	CheckError     string     `json:"check_error"`
	NodeCount      int        `json:"node_count"`
}

// WhoisOperations handles WHOIS cache database operations (server-side reads)
type WhoisOperations struct {
	db database.DatabaseInterface
}

// NewWhoisOperations creates a new WhoisOperations instance
func NewWhoisOperations(db database.DatabaseInterface) *WhoisOperations {
	return &WhoisOperations{db: db}
}

// GetAllWhoisResults returns all WHOIS results with node counts computed in Go
func (w *WhoisOperations) GetAllWhoisResults() ([]DomainWhoisResult, error) {
	ctx := context.Background()

	// Step 1: Get all WHOIS results from cache table
	whoisResults, err := w.getWhoisEntries(ctx)
	if err != nil {
		return nil, err
	}

	if len(whoisResults) == 0 {
		return whoisResults, nil
	}

	// Step 2: Get unique hostnames from recent test results
	hostnames, err := w.getUniqueHostnames(ctx, 30)
	if err != nil {
		// Return WHOIS results without node counts rather than failing
		return whoisResults, nil
	}

	// Step 3: Map hostnames to domains and count nodes per domain in Go
	domainCounts := services.ExtractUniqueDomains(hostnames)

	// Step 4: Merge node counts into WHOIS results
	for i := range whoisResults {
		if count, ok := domainCounts[whoisResults[i].Domain]; ok {
			whoisResults[i].NodeCount = count
		}
	}

	return whoisResults, nil
}

// getWhoisEntries fetches all entries from domain_whois_cache
func (w *WhoisOperations) getWhoisEntries(ctx context.Context) ([]DomainWhoisResult, error) {
	query := `SELECT
		domain, expiration_date, creation_date, registrar, whois_status, check_time, check_error
		FROM domain_whois_cache FINAL
		ORDER BY domain`

	conn := w.db.Conn()
	rows, err := conn.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []DomainWhoisResult
	for rows.Next() {
		var (
			r              DomainWhoisResult
			expirationDate sql.NullTime
			creationDate   sql.NullTime
		)

		if err := rows.Scan(
			&r.Domain, &expirationDate, &creationDate,
			&r.Registrar, &r.WhoisStatus, &r.CheckTime, &r.CheckError,
		); err != nil {
			return nil, err
		}

		if expirationDate.Valid {
			t := expirationDate.Time
			r.ExpirationDate = &t
		}
		if creationDate.Valid {
			t := creationDate.Time
			r.CreationDate = &t
		}

		results = append(results, r)
	}

	return results, rows.Err()
}

// getUniqueHostnames fetches unique hostnames from recent test results
func (w *WhoisOperations) getUniqueHostnames(ctx context.Context, days int) ([]string, error) {
	query := `SELECT DISTINCT hostname
		FROM node_test_results
		WHERE hostname != ''
		  AND is_aggregated = false
		  AND test_date >= today() - ?`

	conn := w.db.Conn()
	rows, err := conn.QueryContext(ctx, query, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hostnames []string
	for rows.Next() {
		var h string
		if err := rows.Scan(&h); err != nil {
			return nil, err
		}
		hostnames = append(hostnames, h)
	}

	return hostnames, rows.Err()
}
