package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
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
	domainNodes := make(map[string]map[string]struct{}) // domain → set of "zone:net/node"
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

// GetNodesByDomain returns all operational nodes whose hostname maps to the given domain.
// Domain extraction uses publicsuffix in Go, so we fetch hostname→node mappings and filter in Go,
// then query ClickHouse for full node details.
func (w *WhoisOperations) GetNodesByDomain(targetDomain string, days int) ([]NodeTestResult, error) {
	ctx := context.Background()

	// Step 1: Get hostname→node mappings from recent test results
	hostnameNodes, err := w.getHostnameNodeMappings(ctx, days)
	if err != nil {
		return nil, fmt.Errorf("failed to get hostname mappings: %w", err)
	}

	// Step 2: Filter for target domain, collect unique (zone, net, node) tuples
	type nodeKey struct{ zone, net, node int }
	matchedNodes := make(map[nodeKey]string) // nodeKey → first matching hostname
	for _, hn := range hostnameNodes {
		d := domain.ExtractRegistrableDomain(hn.hostname)
		if d == targetDomain {
			nk := nodeKey{}
			fmt.Sscanf(hn.nodeKey, "%d:%d/%d", &nk.zone, &nk.net, &nk.node)
			if _, exists := matchedNodes[nk]; !exists {
				matchedNodes[nk] = hn.hostname
			}
		}
	}

	if len(matchedNodes) == 0 {
		return []NodeTestResult{}, nil
	}

	// Step 3: Build IN-clause tuples for ClickHouse query
	var tuples []string
	for nk := range matchedNodes {
		tuples = append(tuples, fmt.Sprintf("(%d, %d, %d)", nk.zone, nk.net, nk.node))
	}

	query := fmt.Sprintf(`
		WITH latest_nodes AS (
			SELECT
				zone, net, node,
				argMax(system_name, nodelist_date) as system_name,
				argMax(sysop_name, nodelist_date) as sysop_name
			FROM nodes
			GROUP BY zone, net, node
		)
		SELECT
			r.zone, r.net, r.node,
			COALESCE(n.system_name, r.binkp_system_name, r.ifcico_system_name) as binkp_system_name,
			COALESCE(n.sysop_name, r.binkp_sysop) as binkp_sysop,
			r.binkp_location,
			r.hostname,
			r.country, r.country_code, r.city, r.isp, r.org, r.asn,
			r.resolved_ipv4, r.resolved_ipv6,
			r.binkp_success, r.binkp_ipv6_success,
			r.ifcico_success, r.ifcico_ipv6_success,
			r.telnet_success, r.telnet_ipv6_success,
			r.ftp_success, r.ftp_ipv6_success,
			r.vmodem_success, r.vmodem_ipv6_success
		FROM (
			SELECT
				zone, net, node,
				argMax(binkp_system_name, test_time) as binkp_system_name,
				argMax(ifcico_system_name, test_time) as ifcico_system_name,
				argMax(binkp_sysop, test_time) as binkp_sysop,
				argMax(binkp_location, test_time) as binkp_location,
				argMax(hostname, test_time) as hostname,
				argMax(country, test_time) as country,
				argMax(country_code, test_time) as country_code,
				argMax(city, test_time) as city,
				argMax(isp, test_time) as isp,
				argMax(org, test_time) as org,
				argMax(asn, test_time) as asn,
				argMax(resolved_ipv4, test_time) as resolved_ipv4,
				argMax(resolved_ipv6, test_time) as resolved_ipv6,
				argMax(binkp_success, test_time) as binkp_success,
				argMax(binkp_ipv6_success, test_time) as binkp_ipv6_success,
				argMax(ifcico_success, test_time) as ifcico_success,
				argMax(ifcico_ipv6_success, test_time) as ifcico_ipv6_success,
				argMax(telnet_success, test_time) as telnet_success,
				argMax(telnet_ipv6_success, test_time) as telnet_ipv6_success,
				argMax(ftp_success, test_time) as ftp_success,
				argMax(ftp_ipv6_success, test_time) as ftp_ipv6_success,
				argMax(vmodem_success, test_time) as vmodem_success,
				argMax(vmodem_ipv6_success, test_time) as vmodem_ipv6_success
			FROM node_test_results
			WHERE is_aggregated = false
				AND test_date >= today() - %d
				AND (zone, net, node) IN (%s)
			GROUP BY zone, net, node
		) AS r
		LEFT JOIN latest_nodes n ON r.zone = n.zone AND r.net = n.net AND r.node = n.node
		ORDER BY r.zone, r.net, r.node
	`, days, strings.Join(tuples, ", "))

	conn := w.db.Conn()
	rows, err := conn.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query nodes by domain: %w", err)
	}
	defer rows.Close()

	var results []NodeTestResult
	for rows.Next() {
		var result NodeTestResult
		var resolvedIPv4, resolvedIPv6 []string

		err := rows.Scan(
			&result.Zone, &result.Net, &result.Node,
			&result.BinkPSystemName, &result.BinkPSysop, &result.BinkPLocation,
			&result.Hostname,
			&result.Country, &result.CountryCode, &result.City,
			&result.ISP, &result.Org, &result.ASN,
			&resolvedIPv4, &resolvedIPv6,
			&result.BinkPSuccess, &result.BinkPIPv6Success,
			&result.IfcicoSuccess, &result.IfcicoIPv6Success,
			&result.TelnetSuccess, &result.TelnetIPv6Success,
			&result.FTPSuccess, &result.FTPIPv6Success,
			&result.VModemSuccess, &result.VModemIPv6Success,
		)
		if err != nil {
			continue
		}

		result.ResolvedIPv4 = resolvedIPv4
		result.ResolvedIPv6 = resolvedIPv6
		results = append(results, result)
	}

	return results, nil
}

// getWhoisEntries fetches all entries from domain_whois_cache
func (w *WhoisOperations) getWhoisEntries(ctx context.Context) ([]DomainWhoisResult, error) {
	query := `SELECT
		domain, expiration_date, creation_date, registrar, whois_status, check_time, check_error
		FROM domain_whois_cache
		ORDER BY domain, check_time DESC
		LIMIT 1 BY domain`

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
	nodeKey  string // "zone:net/node" for dedup
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
