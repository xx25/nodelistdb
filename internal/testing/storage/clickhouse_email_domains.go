package storage

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/nodelistdb/internal/emailflags"
)

// StoredEmailDomainCheck is one row of email_domain_checks.
type StoredEmailDomainCheck struct {
	Domain      string
	Status      string
	Detail      string
	ASCIIDomain string
	MXHosts     []emailflags.MXHost
	// CheckTime is when the current verdict was established.
	CheckTime time.Time
	// LastAttemptTime is when the domain was last re-tested.
	LastAttemptTime time.Time
	// CheckError is non-empty when the most recent attempt failed
	// transiently, which means the verdict shown is stale.
	CheckError string
}

// StoreEmailDomainCheck records a verification result.
//
// A transient failure must never destroy a good verdict, so the caller passes
// the previous row (if any) and this carries its verdict forward, advancing
// only last_attempt_time and recording the error. The table is a
// ReplacingMergeTree versioned by last_attempt_time, so every write wins over
// the row it replaces, whether or not the verdict itself moved.
func (s *ClickHouseStorage) StoreEmailDomainCheck(ctx context.Context, result emailflags.DomainResult, previous *StoredEmailDomainCheck) error {
	now := time.Now()

	status := result.Status
	detail := result.Detail
	asciiDomain := result.ASCIIDomain
	hosts := result.MXHosts
	checkTime := now

	if !result.Stable() && previous != nil && emailflags.DomainStatusStable(previous.Status) {
		// Keep showing what was last actually established, and remember when
		// that was, rather than overwriting it with a DNS hiccup.
		status = previous.Status
		detail = previous.Detail
		asciiDomain = previous.ASCIIDomain
		hosts = previous.MXHosts
		checkTime = previous.CheckTime
	}

	prefs := make([]uint16, 0, len(hosts))
	names := make([]string, 0, len(hosts))
	resolved := make([]uint8, 0, len(hosts))
	for _, h := range hosts {
		prefs = append(prefs, h.Preference)
		names = append(names, h.Host)
		if h.Resolved {
			resolved = append(resolved, 1)
		} else {
			resolved = append(resolved, 0)
		}
	}

	query := `INSERT INTO email_domain_checks
		(domain, status, detail, ascii_domain, mx_preferences, mx_hosts, mx_resolved, check_time, last_attempt_time, check_error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	return s.conn.Exec(ctx, query,
		result.Domain,
		status,
		detail,
		asciiDomain,
		prefs,
		names,
		resolved,
		checkTime,
		now,
		result.Error,
	)
}

// GetEmailDomainCheck returns the stored verdict for one domain, or nil when
// the domain has never been checked.
func (s *ClickHouseStorage) GetEmailDomainCheck(ctx context.Context, domain string) (*StoredEmailDomainCheck, error) {
	query := `SELECT domain, status, detail, ascii_domain,
			mx_preferences, mx_hosts, mx_resolved,
			check_time, last_attempt_time, check_error
		FROM email_domain_checks
		WHERE domain = ?
		ORDER BY last_attempt_time DESC
		LIMIT 1`

	rows, err := s.conn.Query(ctx, query, domain)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, nil
	}

	var (
		stored   StoredEmailDomainCheck
		prefs    []uint16
		names    []string
		resolved []uint8
	)
	if err := rows.Scan(
		&stored.Domain,
		&stored.Status,
		&stored.Detail,
		&stored.ASCIIDomain,
		&prefs,
		&names,
		&resolved,
		&stored.CheckTime,
		&stored.LastAttemptTime,
		&stored.CheckError,
	); err != nil {
		return nil, err
	}

	stored.MXHosts = make([]emailflags.MXHost, 0, len(names))
	for i := range names {
		host := emailflags.MXHost{Host: names[i]}
		if i < len(prefs) {
			host.Preference = prefs[i]
		}
		if i < len(resolved) {
			host.Resolved = resolved[i] == 1
		}
		stored.MXHosts = append(stored.MXHosts, host)
	}

	return &stored, rows.Err()
}

// emailProtocolAddressSQL builds the per-flag address extraction for
// internet_config.email_protocols.
//
// Both stored shapes must be handled. The current shape is a list of detail
// objects; rows written before it became a list stored a single bare object,
// which JSONExtractArrayRaw returns nothing for. The Go reader
// (database.EmailProtocolMap.UnmarshalJSON) accepts both, so the sweep has to
// as well -- otherwise a domain visible on the report would never be checked,
// with nothing logged to say so.
func emailProtocolAddressSQL() string {
	flags := []string{"IMI", "ITX", "ISE", "IUC", "EMA", "EVY"}
	clauses := make([]string, 0, len(flags))
	for _, flag := range flags {
		clauses = append(clauses, fmt.Sprintf(
			`if(JSONType(ic, 'email_protocols', '%[1]s') = 'Array',
				arrayMap(x -> JSONExtractString(x, 'email'), JSONExtractArrayRaw(ic, 'email_protocols', '%[1]s')),
				if(JSONType(ic, 'email_protocols', '%[1]s') = 'Object',
				   [JSONExtractString(ic, 'email_protocols', '%[1]s', 'email')],
				   []))`, flag))
	}
	return strings.Join(clauses, ",\n\t\t\t\t") + ","
}

// GetEmailDomainsToCheck returns the mail domains published in the latest
// nodelist of every network, together with when each was last attempted.
//
// This deliberately does not reuse GetNodesWithInternet: that query requires a
// non-empty protocols object, which excludes a node advertising only IEM --
// exactly the population this check exists for.
//
// The domains are extracted in SQL from both storage shapes, because rows
// written before the parser was made consistent kept addressed IUC/EMA/EVY
// flags verbatim in the flags array rather than in internet_config.
func (s *ClickHouseStorage) GetEmailDomainsToCheck(ctx context.Context, staleAfter time.Duration) ([]string, error) {
	query := `
		WITH latest AS (
			SELECT toString(internet_config) AS ic, flags
			FROM nodes
			WHERE (domain, nodelist_date) IN (SELECT domain, MAX(nodelist_date) FROM nodes GROUP BY domain)
			  AND conflict_sequence = 0
			  AND node_type NOT IN ('Down', 'Hold')
		),
		advertised AS (
			SELECT arrayConcat(
				-- IEM addresses live among the defaults, in either the current
				-- list shape or the older bare-string shape.
				if(JSONType(ic, 'defaults', 'IEM') = 'Array',
				   JSONExtract(ic, 'defaults', 'IEM', 'Array(String)'),
				   if(JSONType(ic, 'defaults', 'IEM') = 'String',
				      [JSONExtractString(ic, 'defaults', 'IEM')],
				      [])),
				%s
				-- Pre-fix rows keep the whole "FLAG:address" token in flags.
				arrayMap(f -> substringUTF8(f, position(f, ':') + 1),
					arrayFilter(f -> match(f, '^(IEM|IMI|ITX|ISE|IUC|EMA|EVY):'), flags))
			) AS addrs
			FROM latest
		)
		SELECT DISTINCT lower(splitByChar('@', addr)[-1]) AS mail_domain
		FROM (SELECT arrayJoin(addrs) AS addr FROM advertised)
		WHERE addr != ''
		  AND position(addr, '@') > 1
		  AND mail_domain != ''
		  AND mail_domain LIKE '%%.%%'
		  AND mail_domain NOT IN (
			-- Fresh verdicts are skipped, but a stored 'error' is not a
			-- verdict -- it is a DNS hiccup. Excluding it here would freeze
			-- one timeout on screen for the whole stale_after window, so
			-- error rows are always retried on the next sweep.
			SELECT domain FROM (
				SELECT domain, status, last_attempt_time
				FROM email_domain_checks
				ORDER BY domain, last_attempt_time DESC
				LIMIT 1 BY domain
			)
			WHERE last_attempt_time >= ? AND status != '%s'
		  )
		ORDER BY mail_domain`

	query = fmt.Sprintf(query, emailProtocolAddressSQL(), emailflags.DomainStatusError)

	cutoff := time.Now().Add(-staleAfter)
	rows, err := s.conn.Query(ctx, query, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var domains []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		domains = append(domains, d)
	}
	return domains, rows.Err()
}
