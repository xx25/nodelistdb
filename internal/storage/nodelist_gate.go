package storage

import (
	"fmt"
	"regexp"
	"strings"
)

// The analytics pages under /analytics are censuses of a live FTN network, but
// they are computed from node_test_results, which is a log of what the daemon
// did — a different population, and one that drifts from the nodelist in three
// ways:
//
//   - a node that leaves the nodelist keeps being tested. The daemon schedules
//     from the latest *imported* issue and that has no staleness bound of its
//     own, so departed nodes stay under test indefinitely (3 such nodes were
//     listed across the reports when this was written);
//   - a node the sysop marked Down or Hold — out of service, by their own
//     declaration — still answers, and still counts (47 such nodes);
//   - a hand-run `testdaemon -test-node <addr> -test-proto <p>` writes a row
//     for a protocol the node never announced, on that protocol's default port
//     (see daemon_testing.go). The scheduled cycle cannot do this: it gates
//     every protocol on node.HasProtocol (test_executor.go). 2:221/1, whose IVM
//     flag has been gone since 2014, was listed on /analytics/vmodem-unavailable
//     this way.
//
// None of the three is an error anywhere. The query just answers a slightly
// different question than the page's title claims, which is why these gates
// exist and why nodelist_gate_test.go guards them.
//
// The helpers below emit WHERE-clause fragments assuming an unaliased `nodes`,
// to follow a WHERE that a leading AND can extend. domainFilter and nodeFilter
// are ready-made clauses, "" meaning no filtering (see domainFilterSQL). All
// values are inlined, so the fragments bind no parameters and callers'
// positional argument lists are unchanged.

// ftnFlagSQLRe validates a nodelist protocol flag before it is inlined into SQL.
var ftnFlagSQLRe = regexp.MustCompile(`^[A-Z]{3}$`)

// protocolAnnouncementFlags maps a tested protocol to the nodelist flag that
// announces it — the same pairing test_executor.go gates its scheduled tests
// on, so that a report and the daemon agree on what a node claims to run.
var protocolAnnouncementFlags = map[string]string{
	"binkp":  "IBN",
	"ifcico": "IFC",
	"telnet": "ITN",
	"ftp":    "IFT",
	"vmodem": "IVM",
}

// announcementGatedProtocols names the protocols whose report page claims the
// node ADVERTISES the protocol, rather than merely that a probe succeeded.
// Only the VModem pages do: "...on their announced IVM port" and "Nodes whose
// announced IVM port was not confirmed...". BinkP, IFCICO, Telnet and FTP all
// say "Nodes that have been successfully tested with X protocol", which is a
// claim about the probe alone — so they get nodelistMembershipPredicateSQL and
// not the flag test.
//
// Both VModem pages therefore resolve "announces IVM" through the same
// predicate, which is what keeps /analytics/vmodem and
// /analytics/vmodem-unavailable complementary: every gated node lands on
// exactly one of them, and the pair cannot drift into double-counting or into
// a gap where a node appears on neither.
//
// The distinction is not academic. Because one host answers for all of its
// AKAs, a successful test in one network is derived onto the same host's
// entries in the others (derived_from_address; see the testdaemon notes in
// CLAUDE.md). Eight fidonet nodes were listed on /analytics/telnet that way,
// their telnet proven by their fsxnet twin while their own fidonet entry
// announces only IBN. Under "successfully tested" they belong on the page;
// under "announced" they would not. Gating them out would have made the page
// contradict its own subtitle.
//
// web/analytics_page_claims_test.go fails if a page's copy drifts across this
// line without the map following.
var announcementGatedProtocols = map[string]bool{
	"vmodem": true,
}

// nodelistMembershipPredicateSQL selects, from `nodes`, the rows of the current
// nodelist of each FTN network that are in service.
//
// Deliberately NOT bounded by nodeIdentityWindowSQL: measured on production a
// date bound saves nothing here (`nodes` is ORDER BY (zone, net, node,
// nodelist_date, ...), so it cannot prune to a single issue), and it would
// silently drop any network whose last import predates the bound.
func nodelistMembershipPredicateSQL(domainFilter, nodeFilter string) string {
	return fmt.Sprintf(`
				AND (domain, nodelist_date) IN (SELECT domain, MAX(nodelist_date) FROM nodes WHERE 1 = 1 %[1]s GROUP BY domain)
				%[1]s
				%[2]s
				AND conflict_sequence = 0
				AND node_type NOT IN ('Down', 'Hold')`,
		domainFilter, nodeFilter)
}

// announcedProtocolPredicateSQL narrows nodelistMembershipPredicateSQL to nodes
// announcing one protocol flag.
//
// A flag, bare or valued, is always routed into internet_config.protocols.<FLAG>
// and never into the flags array (see internal/parser/parser_flags.go), so
// detecting it needs JSON_EXISTS over internet_config rather than
// hasAny(flags, ...). toString() avoids the clickhouse-go SharedVariant decode
// panic on raw internet_config reads (see internetConfigSelectSQL in types.go);
// JSON_EXISTS never materializes the value at all.
//
// hasAny(json_protocols, ...) in front of it is a pure accelerator. That
// MATERIALIZED column is extractAll(toString(internet_config), '([A-Z]{3})') —
// every three-letter run in the JSON, so a strict superset of the protocol keys
// (verified on the current nodelist: 0 misses across IBN/IFC/ITN/IFT/IVM) — and
// it carries a bloom_filter index, so granules holding no such node are skipped
// instead of having their internet_config parsed. Measured on production: 1.24s
// exact-only vs 0.18s with the prefilter, same 17 rows. The exact test stays
// because both the superset column and the bloom filter admit false positives.
func announcedProtocolPredicateSQL(flag, domainFilter, nodeFilter string) string {
	if !ftnFlagSQLRe.MatchString(flag) {
		// Unreachable via protocolAnnouncementFlags; a clause matching nothing
		// is still safer than inlining an unvalidated value, per domainFilterSQL.
		return "\n\t\t\t\tAND 1 = 0"
	}
	return fmt.Sprintf(`%s
				AND hasAny(json_protocols, ['%[2]s'])
				AND JSON_EXISTS(toString(internet_config), '$.protocols.%[2]s')`,
		nodelistMembershipPredicateSQL(domainFilter, nodeFilter), flag)
}

// currentNodesSQL wraps nodelistMembershipPredicateSQL as a key-only subquery,
// for gating a report whose own SELECT comes from node_test_results.
func currentNodesSQL(domainFilter, nodeFilter string) string {
	return nodeKeysSQL(nodelistMembershipPredicateSQL(domainFilter, nodeFilter))
}

// announcedProtocolNodesSQL wraps announcedProtocolPredicateSQL the same way.
func announcedProtocolNodesSQL(flag, domainFilter, nodeFilter string) string {
	return nodeKeysSQL(announcedProtocolPredicateSQL(flag, domainFilter, nodeFilter))
}

func nodeKeysSQL(predicate string) string {
	return fmt.Sprintf(`
			SELECT domain, zone, net, node
			FROM nodes
			WHERE 1 = 1%s`, predicate)
}

// nodelistGateMarker marks the one place in a report's anchor CTE where
// candidates are narrowed to nodes still in the nodelist and in service. It is
// a marker rather than a directly interpolated clause for the same reason
// {{DOMAIN_FILTER}} and {{NODE_WINDOW}} are: these queries are long, their
// argument lists are positional, and threading one more %s through each is how
// a filter ends up in the wrong CTE.
//
// Put it in the CTE that fixes the candidate set — the one every later stage
// joins back to — so the gate runs once against the smallest input. Repeating
// it downstream costs a second scan of `nodes` and buys nothing.
const nodelistGateMarker = "{{NODELIST_GATE}}"

// applyNodelistGate expands nodelistGateMarker. prefix is the table alias the
// surrounding CTE uses for node_test_results ("" when unaliased, "r." inside a
// join). A query with no marker is returned unchanged, which is what makes it
// safe to call unconditionally.
func applyNodelistGate(query, prefix, domainFilter, nodeFilter string) string {
	if !strings.Contains(query, nodelistGateMarker) {
		return query
	}
	gate := fmt.Sprintf("AND (%[1]sdomain, %[1]szone, %[1]snet, %[1]snode) IN (%[2]s\n\t\t\t)",
		prefix, currentNodesSQL(domainFilter, nodeFilter))
	return strings.ReplaceAll(query, nodelistGateMarker, gate)
}
