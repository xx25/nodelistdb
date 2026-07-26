package storage

import (
	"fmt"
	"regexp"
)

// domainSQLRe validates FTN network names before they are inlined into SQL.
var domainSQLRe = regexp.MustCompile(`^[a-z0-9_-]{1,32}$`)

// domainFilterSQL returns an "AND <prefix>domain = '<name>'" clause scoping a
// query to one FTN network, or "" when domain is empty (= all networks).
// A name that fails validation yields a clause matching nothing — the value
// may originate from a request cookie, so it must never reach SQL verbatim.
// Inlining a validated literal (instead of a ? placeholder) keeps the many
// analytics queries' positional argument lists unchanged.
func domainFilterSQL(domain, prefix string) string {
	if domain == "" {
		return ""
	}
	if !domainSQLRe.MatchString(domain) {
		return "AND 1 = 0"
	}
	return fmt.Sprintf("AND %sdomain = '%s'", prefix, domain)
}

// nodeIdentityMarginDays is how much wider the nodes-history window is than the
// report asking for it. The margin matters because a node can still be tested
// after it leaves the nodelist — the daemon schedules from the latest *imported*
// issue, which has no staleness bound of its own, so a stalled import keeps
// nodes under test long after their last nodelist appearance. Without the
// margin, such a node's name would silently fall back to the mailer-reported
// one. It doubles as the floor for callers with no window of their own.
const nodeIdentityMarginDays = 365

// nodeIdentityWindowSQL returns an "AND nodelist_date >= today() - N" clause
// bounding the `nodes` scan behind a latest_nodes CTE.
//
// Without it those CTEs aggregate the entire 31.5M-row history of `nodes` on
// every request, to name a few hundred result rows — a country drill-down
// measured 16.7s / 31.7M rows before this clause and 0.22s after. The bound
// works because `nodes` is ORDER BY (zone, net, node, nodelist_date, ...), so a
// granule holding only long-dead nodes has an old max nodelist_date and the
// idx_nodes_date minmax index skips it. Pruning is per granule (~29 node keys),
// not per node, so the win depends on live addresses staying clustered; if these
// queries ever regress, check read_rows in system.query_log first.
//
// The window is inlined as a validated integer rather than bound, for the same
// reason domainFilterSQL inlines its literal: it keeps the many analytics
// queries' positional argument lists unchanged.
//
// Pass the report's own window in days, or 0 if it has none.
func nodeIdentityWindowSQL(days int) string {
	window := days + nodeIdentityMarginDays
	if days < 0 {
		window = nodeIdentityMarginDays
	}
	return fmt.Sprintf("AND nodelist_date >= today() - %d", window)
}
