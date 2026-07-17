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
