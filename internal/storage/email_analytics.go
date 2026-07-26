package storage

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/nodelistdb/internal/database"
	"github.com/nodelistdb/internal/emailflags"
	"github.com/nodelistdb/internal/logging"
)

// emailFlagOrder is the presentation order for the email transport flags:
// IEM first as the address-bearing default, then the remaining FTS-5001
// methods, then the two non-standard flags seen in real nodelists.
var emailFlagOrder = []string{"IEM", "IMI", "ITX", "ISE", "IUC", "EMA", "EVY"}

// emailFlagPresenceSQL builds a predicate that is true when a node advertises
// the given email flag, whichever way the row happens to store it.
//
// Two storage shapes coexist and always will. Rows written before the parser
// was made consistent kept a bare IEM and any addressed IUC/EMA/EVY verbatim
// in the flags array; everything else lives in internet_config. Reading only
// internet_config undercounts historical usage by roughly a fifth.
//
// JSONHas is used rather than JSONExtract because defaults.IEM is a bare
// string in the overwhelming majority of rows and a list only in those
// written since the multi-address change. JSONHas is indifferent to the value
// type; JSONExtract with an Array(String) shape silently returns nothing for
// every scalar row.
func emailFlagPresenceSQL(flag string) string {
	if flag == "IEM" {
		return `(JSONHas(toString(internet_config), 'defaults', 'IEM')
			OR JSONHas(toString(internet_config), 'email_protocols', 'IEM')
			OR has(flags, 'IEM')
			OR arrayExists(f -> startsWith(f, 'IEM:'), flags))`
	}
	return fmt.Sprintf(`(JSONHas(toString(internet_config), 'email_protocols', '%[1]s')
		OR arrayExists(f -> f = '%[1]s' OR startsWith(f, '%[1]s:'), flags))`, flag)
}

// anyEmailPresenceSQL is true when a node advertises any email transport.
func anyEmailPresenceSQL() string {
	clauses := make([]string, 0, len(emailFlagOrder))
	for _, flag := range emailFlagOrder {
		clauses = append(clauses, emailFlagPresenceSQL(flag))
	}
	return "(" + strings.Join(clauses, " OR ") + ")"
}

// GetEmailCapableNodes returns nodes on the latest nodelist that advertise
// mail transport over Internet email (FTS-5001 "Email Flags", plus the
// non-standard EMA and EVY).
//
// Address resolution happens in Go rather than SQL: the rules are ordered
// fallbacks across flags and fields, the stored JSON has two historical
// shapes, and the population is small enough (about a hundred nodes) that
// clarity is worth more than pushing the logic into ClickHouse.
//
// An empty domain searches all FTN networks.
func (ao *AnalyticsOperations) GetEmailCapableNodes(limit int, useFieldFallback bool, domain string) ([]EmailCapableNode, error) {
	ao.mu.RLock()
	defer ao.mu.RUnlock()

	if limit <= 0 {
		limit = DefaultSearchLimit
	}
	if limit > MaxPSTNSearchLimit {
		limit = MaxPSTNSearchLimit
	}

	conn := ao.db.Conn()
	domainFilter := domainFilterSQL(domain, "")

	query := fmt.Sprintf(`
		SELECT
			domain,
			zone,
			net,
			node,
			system_name,
			location,
			sysop_name,
			nodelist_date,
			node_type,
			flags,
			-- Aliased to a distinct name on purpose: the WHERE clause still
			-- references the native internet_config column, and reusing the
			-- column's own name for the stringified copy makes ClickHouse
			-- report a block structure mismatch between String and JSON.
			toString(internet_config) AS internet_config_text
		FROM nodes
		WHERE (domain, nodelist_date) IN (SELECT domain, MAX(nodelist_date) FROM nodes WHERE 1 = 1 %s GROUP BY domain)
		  %s
		  AND conflict_sequence = 0
		  AND node_type NOT IN ('Down', 'Hold')
		  AND %s
		ORDER BY zone, net, node
		LIMIT ?`, domainFilter, domainFilter, anyEmailPresenceSQL())

	rows, err := conn.Query(query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query email capable nodes: %w", err)
	}
	defer rows.Close()

	var results []EmailCapableNode
	for rows.Next() {
		var (
			n          EmailCapableNode
			configJSON string
		)
		if err := rows.Scan(
			&n.Domain,
			&n.Zone,
			&n.Net,
			&n.Node,
			&n.SystemName,
			&n.Location,
			&n.SysopName,
			&n.NodelistDate,
			&n.NodeType,
			&n.Flags,
			&configJSON,
		); err != nil {
			return nil, fmt.Errorf("failed to scan email capable node: %w", err)
		}

		var ic *database.InternetConfiguration
		if configJSON != "" && configJSON != "{}" && configJSON != "null" {
			var parsed database.InternetConfiguration
			// A row whose JSON will not decode still advertised the flags that
			// matched the query, so it is reported with whatever the raw flags
			// array yields rather than dropped.
			if err := json.Unmarshal([]byte(configJSON), &parsed); err == nil {
				ic = &parsed
			}
		}

		n.Capabilities = emailflags.Extract(n.Flags, ic, emailflags.Options{
			Location:         n.Location,
			SystemName:       n.SystemName,
			UseFieldFallback: useFieldFallback,
		})
		n.summarize()

		if len(n.Capabilities) == 0 {
			// The SQL predicate is deliberately broad; if nothing resolved,
			// the row is not actually email capable.
			continue
		}
		results = append(results, n)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating email capable nodes: %w", err)
	}

	ao.attachEndpointStatus(results)
	return results, nil
}

// attachEndpointStatus fills in each node's DNS verdicts.
//
// Failures here are logged and swallowed: the verification table is optional
// (a server that has not run migration 014 does not have it, and the sweep may
// be disabled), and the report is useful without it.
func (ao *AnalyticsOperations) attachEndpointStatus(nodes []EmailCapableNode) {
	if len(nodes) == 0 {
		return
	}

	wanted := make(map[string]bool)
	for _, n := range nodes {
		for _, addr := range n.Addresses {
			if d := emailflags.MailDomain(addr); d != "" {
				wanted[d] = true
			}
		}
	}
	if len(wanted) == 0 {
		return
	}

	domains := make([]string, 0, len(wanted))
	for d := range wanted {
		domains = append(domains, d)
	}

	// LIMIT 1 BY collapses the ReplacingMergeTree's un-merged duplicates
	// without waiting for a background merge.
	query := `
		SELECT domain, status, detail, check_time, check_error
		FROM email_domain_checks
		WHERE domain IN (?)
		ORDER BY domain, last_attempt_time DESC
		LIMIT 1 BY domain`

	rows, err := ao.db.Conn().Query(query, domains)
	if err != nil {
		logging.Debugf("email endpoint status unavailable: %v", err)
		return
	}
	defer rows.Close()

	verdicts := make(map[string]EmailEndpointStatus, len(domains))
	for rows.Next() {
		var (
			domain, status, detail, checkError string
			checkTime                          time.Time
		)
		if err := rows.Scan(&domain, &status, &detail, &checkTime, &checkError); err != nil {
			logging.Debugf("email endpoint status scan failed: %v", err)
			return
		}
		verdicts[domain] = EmailEndpointStatus{
			MailDomain: domain,
			Status:     status,
			Detail:     detail,
			CheckTime:  checkTime,
			// A recorded error means the newest attempt failed and the
			// verdict being shown was established earlier than it appears.
			Stale: checkError != "",
		}
	}
	if err := rows.Err(); err != nil {
		logging.Debugf("email endpoint status iteration failed: %v", err)
		return
	}
	if len(verdicts) == 0 {
		return
	}

	for i := range nodes {
		n := &nodes[i]
		seen := make(map[string]bool, len(n.Addresses))
		for _, addr := range n.Addresses {
			domain := emailflags.MailDomain(addr)
			if domain == "" || seen[domain] {
				continue
			}
			seen[domain] = true

			// Every domain gets a row, checked or not. Omitting the unchecked
			// ones would make a node with one verified and one never-swept
			// domain look fully verified.
			verdict, ok := verdicts[domain]
			if !ok {
				verdict = EmailEndpointStatus{MailDomain: domain}
			}
			verdict.Address = addr
			n.Endpoint = append(n.Endpoint, verdict)
		}
	}
}

// GetEmailFlagTrend returns per-year counts of each email flag, sampled from
// the last nodelist of each year.
//
// An empty domain aggregates all FTN networks.
func (ao *AnalyticsOperations) GetEmailFlagTrend(domain string) ([]EmailFlagTrendPoint, error) {
	ao.mu.RLock()
	defer ao.mu.RUnlock()

	conn := ao.db.Conn()
	domainFilter := domainFilterSQL(domain, "")

	counts := make([]string, 0, len(emailFlagOrder))
	for _, flag := range emailFlagOrder {
		counts = append(counts, fmt.Sprintf("countIf(%s) AS count_%s", emailFlagPresenceSQL(flag), flag))
	}

	// The trend is computed straight from nodes rather than from
	// flag_statistics on purpose. That table harvests flag names with a
	// three-uppercase-letter regex over the JSON text, so it counts pre-fix
	// rows as distinct per-address pseudo-flags ("IUC:user@host") and would
	// show a false step change on the day the parser fix shipped.
	query := fmt.Sprintf(`
		SELECT
			toYear(nodelist_date) AS year,
			%s,
			countIf(%s) AS count_any,
			count() AS total_nodes
		FROM nodes
		WHERE (domain, nodelist_date) IN (
				SELECT domain, MAX(nodelist_date)
				FROM nodes
				WHERE 1 = 1 %s
				GROUP BY domain, toYear(nodelist_date)
			)
		  %s
		  AND conflict_sequence = 0
		  -- Matches GetEmailCapableNodes, so the chart's last point agrees
		  -- with the headline count instead of quietly exceeding it.
		  AND node_type NOT IN ('Down', 'Hold')
		GROUP BY year
		ORDER BY year`,
		strings.Join(counts, ",\n\t\t\t"), anyEmailPresenceSQL(), domainFilter, domainFilter)

	rows, err := conn.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query email flag trend: %w", err)
	}
	defer rows.Close()

	var results []EmailFlagTrendPoint
	for rows.Next() {
		var (
			year       uint16
			flagCounts = make([]uint64, len(emailFlagOrder))
			anyEmail   uint64
			total      uint64
		)

		dest := make([]any, 0, len(emailFlagOrder)+3)
		dest = append(dest, &year)
		for i := range flagCounts {
			dest = append(dest, &flagCounts[i])
		}
		dest = append(dest, &anyEmail, &total)

		if err := rows.Scan(dest...); err != nil {
			return nil, fmt.Errorf("failed to scan email flag trend: %w", err)
		}

		point := EmailFlagTrendPoint{
			Year:       int(year),
			Flags:      make(map[string]int, len(emailFlagOrder)),
			AnyEmail:   int(anyEmail),
			TotalNodes: int(total),
		}
		for i, flag := range emailFlagOrder {
			point.Flags[flag] = int(flagCounts[i])
		}
		results = append(results, point)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating email flag trend: %w", err)
	}
	return results, nil
}

// summarize fills the display-oriented fields derived from Capabilities.
func (n *EmailCapableNode) summarize() {
	seen := make(map[string]bool)
	for _, c := range n.Capabilities {
		n.FlagNames = append(n.FlagNames, c.Flag)
		if c.Standard {
			n.HasStandardMethod = true
		} else {
			n.HasNonStandardFlag = true
		}
		if c.ReceiptRequired {
			n.ReceiptCapable = true
		}
		if c.WireProtocolSpecified {
			n.WireProtocolSpecified = true
		}
		if len(c.Malformed) > 0 {
			n.HasMalformed = true
		}
		for _, addr := range c.Addresses {
			key := strings.ToLower(addr)
			if seen[key] {
				continue
			}
			seen[key] = true
			n.Addresses = append(n.Addresses, addr)
		}
	}
	sort.Strings(n.Addresses)
	n.Resolved = len(n.Addresses) > 0
}

// EmailCapableNode is a node advertising mail transport over Internet email.
type EmailCapableNode struct {
	Domain       string    `json:"domain"`
	Zone         int       `json:"zone"`
	Net          int       `json:"net"`
	Node         int       `json:"node"`
	SystemName   string    `json:"system_name"`
	Location     string    `json:"location"`
	SysopName    string    `json:"sysop_name"`
	NodelistDate time.Time `json:"nodelist_date"`
	NodeType     string    `json:"node_type"`
	Flags        []string  `json:"flags"`

	// Capabilities is the resolved per-flag view.
	Capabilities []emailflags.Capability `json:"capabilities"`

	// The remaining fields are derived from Capabilities for display.
	FlagNames []string `json:"flag_names"`
	Addresses []string `json:"addresses"`
	// Resolved is true when at least one usable address was found.
	Resolved bool `json:"resolved"`
	// ReceiptCapable is true when ITX or ISE is advertised, the only flags
	// FTS-5001 requires to answer with a receipt.
	ReceiptCapable bool `json:"receipt_capable"`
	// WireProtocolSpecified is true when ISE is advertised: SEAT is the only
	// method in this group with a complete FTSC wire specification.
	WireProtocolSpecified bool `json:"wire_protocol_specified"`
	HasStandardMethod     bool `json:"has_standard_method"`
	HasNonStandardFlag    bool `json:"has_non_standard_flag"`
	HasMalformed          bool `json:"has_malformed"`

	// Endpoint carries the DNS verification verdict for this node's
	// addresses. It is nil when verification is disabled or has not run.
	Endpoint []EmailEndpointStatus `json:"endpoint,omitempty"`
}

// Address renders the node's FidoNet address.
func (n EmailCapableNode) Address() string {
	return fmt.Sprintf("%d:%d/%d", n.Zone, n.Net, n.Node)
}

// Verdicts produced by DNS verification of a published mail domain. The
// canonical definitions live in emailflags so that the testdaemon, which
// writes them, and the web layer, which reads them, share one vocabulary
// without either importing the other.
const (
	EmailDomainStatusOK             = emailflags.DomainStatusOK
	EmailDomainStatusImplicitMX     = emailflags.DomainStatusImplicitMX
	EmailDomainStatusDegraded       = emailflags.DomainStatusDegraded
	EmailDomainStatusNoDomain       = emailflags.DomainStatusNoDomain
	EmailDomainStatusNullMX         = emailflags.DomainStatusNullMX
	EmailDomainStatusNoMX           = emailflags.DomainStatusNoMX
	EmailDomainStatusMXUnresolvable = emailflags.DomainStatusMXUnresolvable
	EmailDomainStatusInvalid        = emailflags.DomainStatusInvalid
	EmailDomainStatusError          = emailflags.DomainStatusError
)

// EmailEndpointStatus pairs one advertised address with its DNS verdict.
type EmailEndpointStatus struct {
	Address string `json:"address"`
	// MailDomain is the part after the last "@", lower-cased.
	MailDomain string `json:"mail_domain"`
	// Status is one of the EmailDomainStatus constants, or empty when the
	// domain has not been checked yet.
	Status string `json:"status"`
	// Detail is a short human-readable explanation.
	Detail string `json:"detail"`
	// CheckTime is when the verdict was produced.
	CheckTime time.Time `json:"check_time"`
	// Stale is true when the last check attempt failed transiently and this
	// verdict is therefore older than it looks.
	Stale bool `json:"stale"`
}

// EmailFlagTrendPoint is one year's email flag counts.
type EmailFlagTrendPoint struct {
	Year int `json:"year"`
	// Flags maps each email flag to the number of nodes advertising it.
	Flags map[string]int `json:"flags"`
	// AnyEmail counts nodes advertising at least one email flag.
	AnyEmail int `json:"any_email"`
	// TotalNodes is the size of the sampled nodelist, for context.
	TotalNodes int `json:"total_nodes"`
}

// Percent returns the share of the nodelist advertising email transport.
func (p EmailFlagTrendPoint) Percent() float64 {
	if p.TotalNodes == 0 {
		return 0
	}
	return float64(p.AnyEmail) / float64(p.TotalNodes) * 100
}

// EmailFlagOrder exposes the canonical flag ordering to callers that render
// per-flag columns.
func EmailFlagOrder() []string {
	out := make([]string, len(emailFlagOrder))
	copy(out, emailFlagOrder)
	return out
}
