package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/nodelistdb/internal/database"
	"github.com/nodelistdb/internal/domain"
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

	// Step 2: Get hostname→node mappings from recent test results
	hostnameNodes, err := w.getHostnameNodeMappings(ctx, 30)
	if err != nil {
		// Return WHOIS results without node counts rather than failing
		return whoisResults, nil
	}

	// Step 3: Count unique nodes per domain in Go
	domainNodes := make(map[string]map[string]struct{}) // domain → set of "zone:net:node"
	for _, hn := range hostnameNodes {
		d := domain.ExtractRegistrableDomain(hn.hostname)
		if d == "" {
			continue
		}
		if domainNodes[d] == nil {
			domainNodes[d] = make(map[string]struct{})
		}
		domainNodes[d][hn.nodeKey] = struct{}{}
	}

	// Step 4: Merge node counts into WHOIS results
	for i := range whoisResults {
		if nodes, ok := domainNodes[whoisResults[i].Domain]; ok {
			whoisResults[i].NodeCount = len(nodes)
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

type hostnameNode struct {
	hostname string
	nodeKey  string // "zone:net:node" for dedup
}

// getHostnameNodeMappings fetches distinct (hostname, zone, net, node) tuples from recent test results
func (w *WhoisOperations) getHostnameNodeMappings(ctx context.Context, days int) ([]hostnameNode, error) {
	query := `SELECT DISTINCT hostname, zone, net, node
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

	var results []hostnameNode
	for rows.Next() {
		var (
			h                string
			zone, net, node  int
		)
		if err := rows.Scan(&h, &zone, &net, &node); err != nil {
			return nil, err
		}
		results = append(results, hostnameNode{
			hostname: h,
			nodeKey:  fmt.Sprintf("%d:%d/%d", zone, net, node),
		})
	}

	return results, rows.Err()
}
