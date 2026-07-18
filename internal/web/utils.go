package web

import (
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/nodelistdb/internal/database"
)

// networkCookieName holds the globally selected FTN network. It is written by
// the nav switcher JS (static/network-switch.js), so it is deliberately not
// HttpOnly.
const networkCookieName = "ftn_network"

// domainNameRe limits cookie-supplied network names to the same shape as
// nodelist_download.go's networkNameRe; anything else is ignored.
var domainNameRe = regexp.MustCompile(`^[a-z0-9_-]{1,32}$`)

// explicitDomain returns the domain the request itself names (?domain= query
// or form field), or "". The global switcher cookie is deliberately NOT
// consulted — use requestDomain for that.
func explicitDomain(r *http.Request) string {
	if d := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("domain"))); d != "" {
		return d
	}
	return strings.ToLower(strings.TrimSpace(r.FormValue("domain")))
}

// requestDomain returns the FTN network for this request: an explicit
// ?domain= query (or form) parameter wins so pre-multi-network URLs and deep
// links are unchanged, then the global switcher cookie, then fidonet.
func requestDomain(r *http.Request) string {
	if d := explicitDomain(r); d != "" {
		return d
	}
	return cookieDomain(r)
}

// cookieDomain returns the network from the global switcher cookie, or
// fidonet when the cookie is absent or malformed.
func cookieDomain(r *http.Request) string {
	if c, err := r.Cookie(networkCookieName); err == nil {
		if d := strings.ToLower(strings.TrimSpace(c.Value)); domainNameRe.MatchString(d) {
			return d
		}
	}
	return database.DefaultDomain
}

// resolveEntityDomain picks the network for a node/point detail page: an
// explicit ?domain= wins; otherwise the globally selected network when the
// address exists there, then fidonet, then the first network it exists in —
// so cross-network address lookups keep working whatever the switcher says.
func resolveEntityDomain(r *http.Request, availableDomains []string) string {
	if d := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("domain"))); d != "" {
		return d
	}
	if len(availableDomains) == 0 {
		return cookieDomain(r)
	}
	for _, preferred := range []string{cookieDomain(r), database.DefaultDomain} {
		for _, d := range availableDomains {
			if d == preferred {
				return d
			}
		}
	}
	return availableDomains[0]
}

// domainQuerySuffix returns "" for the default network and "?domain=<name>"
// (or "&domain=<name>" when appending) otherwise.
func domainQuerySuffix(domain string, first bool) string {
	if domain == "" || domain == database.DefaultDomain {
		return ""
	}
	if first {
		return "?domain=" + domain
	}
	return "&domain=" + domain
}

// ftnAddressRe matches the FidoNet address format zone:net/node[.point]
var ftnAddressRe = regexp.MustCompile(`^(\d+):(\d+)/(\d+)(?:\.(\d+))?$`)

// parseFullAddress parses a FidoNet address like "2:5001/100" or
// "2:5001/100.7". hasPoint reports whether a point suffix was present.
func parseFullAddress(address string) (zone, net, node, point int, hasPoint bool, err error) {
	matches := ftnAddressRe.FindStringSubmatch(strings.TrimSpace(address))
	if len(matches) < 4 {
		return 0, 0, 0, 0, false, errors.New("invalid node address format")
	}

	for i, dest := range []*int{&zone, &net, &node} {
		v, aerr := strconv.Atoi(matches[i+1])
		if aerr != nil {
			return 0, 0, 0, 0, false, aerr
		}
		*dest = v
	}

	if matches[4] != "" {
		point, err = strconv.Atoi(matches[4])
		if err != nil {
			return 0, 0, 0, 0, false, err
		}
		hasPoint = true
	}

	return zone, net, node, point, hasPoint, nil
}

// parseNodeAddress parses the 3-D part of a FidoNet node address; a ".point"
// suffix is accepted and ignored (4-D input is routed to the point page by
// the search handler before node search runs).
func parseNodeAddress(address string) (zone, net, node int, err error) {
	zone, net, node, _, _, err = parseFullAddress(address)
	return zone, net, node, err
}

// buildNodeFilterFromAddress creates a node filter from a full address string
func buildNodeFilterFromAddress(address string) (database.NodeFilter, error) {
	zone, net, node, err := parseNodeAddress(address)
	if err != nil {
		return database.NodeFilter{}, err
	}

	latestOnly := true
	return database.NodeFilter{
		Zone:       &zone,
		Net:        &net,
		Node:       &node,
		LatestOnly: &latestOnly,
	}, nil
}

// pointTextSearchMinLen is the shortest text term the points search accepts.
// The points table's ngrambf_v1 skip indexes are trigram-based (migration
// 010): a shorter LIKE pattern yields no trigrams, so ClickHouse would fall
// back to a full scan of the 76M-row table — and this query runs on every
// web search.
const pointTextSearchMinLen = 3

// pointTextTermIndexable reports whether a term can be pruned by those
// trigram indexes. ClickHouse's ngram tokenizer counts UTF-8 code points
// (not bytes) and restarts at every unescaped LIKE wildcard, so the term
// needs a run of at least pointTextSearchMinLen literal code points between
// wildcards ("Мо" is 4 bytes but 2 code points; "%zz" has no 3-run).
// underscoreIsWildcard says whether '_' reaches LIKE as a wildcard: true for
// system/location (the raw term is embedded in the pattern), false for the
// sysop field, whose SQL rewrites '_' to a literal space first.
func pointTextTermIndexable(term string, underscoreIsWildcard bool) bool {
	run := 0
	for _, r := range term {
		if r == '%' || (underscoreIsWildcard && r == '_') {
			run = 0
			continue
		}
		run++
		if run >= pointTextSearchMinLen {
			return true
		}
	}
	return false
}

// buildPointFilterFromForm creates a point filter from the search form fields,
// mirroring the node search's branch semantics exactly so the two result
// sections answer the same question: a full address uses ONLY the address
// (text fields ignored, like buildNodeFilterFromAddress); a sysop search uses
// ONLY the sysop (like SearchNodesBySysop); otherwise the individual fields
// combine. The result is always lifetime-aggregated — the node side's
// lifetime search ignores the "include historical" checkbox too.
// ok is false when no usable constraint was given (including any provided
// text term pointTextTermIndexable rejects).
func buildPointFilterFromForm(r *http.Request) (database.PointFilter, bool) {
	filter := database.PointFilter{Limit: 100}
	hasConstraint := false

	// Scope to the globally selected network (explicit ?domain= still wins)
	domain := requestDomain(r)
	filter.Domain = &domain

	// Sysop mode searches by operator, nothing else — checked FIRST because
	// the node side routes to SearchNodesBySysop whenever the sysop field is
	// filled, even alongside a full address.
	if sysop := r.FormValue("sysop_name"); strings.TrimSpace(sysop) != "" {
		if !pointTextTermIndexable(strings.TrimSpace(sysop), false) {
			return filter, false
		}
		filter.SysopName = &sysop
		return filter, true
	}

	// A full 3-D address searches the points under that boss, nothing else.
	// Like the node side, a full address spans ALL networks unless the
	// request names one explicitly — the switcher cookie does not scope it.
	if fullAddress := r.FormValue("full_address"); fullAddress != "" {
		zone, net, node, _, _, err := parseFullAddress(fullAddress)
		if err != nil {
			return filter, false
		}
		if explicitDomain(r) == "" {
			filter.Domain = nil
		}
		filter.Zone, filter.Net, filter.Node = &zone, &net, &node
		return filter, true
	}

	if v := r.FormValue("zone"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			filter.Zone = &n
			hasConstraint = true
		}
	}
	if v := r.FormValue("net"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			filter.Net = &n
			hasConstraint = true
		}
	}
	if v := r.FormValue("node"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			filter.Node = &n
			// Same guard as the node search: a bare node number without
			// zone or net would sweep the whole table
			if filter.Zone != nil || filter.Net != nil {
				hasConstraint = true
			}
		}
	}

	for _, tf := range []struct {
		field string
		dest  **string
	}{
		{"system_name", &filter.SystemName},
		{"location", &filter.Location},
	} {
		if v := r.FormValue(tf.field); strings.TrimSpace(v) != "" {
			// A provided-but-unindexable text term skips the points search
			// entirely rather than being dropped from the filter — running
			// the remaining constraints would show points that ignore the
			// user's text, diverging from the node section above it.
			if !pointTextTermIndexable(strings.TrimSpace(v), true) {
				return filter, false
			}
			val := v
			*tf.dest = &val
			hasConstraint = true
		}
	}

	return filter, hasConstraint
}

// buildNodeFilterFromForm creates a node filter from individual form fields
func buildNodeFilterFromForm(r *http.Request) database.NodeFilter {
	// Check if historical search is requested
	// Checkbox sends "1" when checked, empty string when unchecked
	includeHistorical := r.FormValue("include_historical") == "1"
	latestOnly := !includeHistorical

	filter := database.NodeFilter{
		LatestOnly: &latestOnly,
		Limit:      100, // Default limit
	}

	// Track if we have any specific constraints to prevent overly broad searches
	hasSpecificConstraint := false

	if zone := r.FormValue("zone"); zone != "" {
		if z, err := strconv.Atoi(zone); err == nil {
			filter.Zone = &z
			hasSpecificConstraint = true
		}
	}

	if net := r.FormValue("net"); net != "" {
		if n, err := strconv.Atoi(net); err == nil {
			filter.Net = &n
			hasSpecificConstraint = true
		}
	}

	if node := r.FormValue("node"); node != "" {
		if n, err := strconv.Atoi(node); err == nil {
			filter.Node = &n
			// Only consider node alone as sufficient constraint if zone or net is also specified
			// This prevents resource-intensive queries like searching for node=0 across all zones/nets
			if filter.Zone != nil || filter.Net != nil {
				hasSpecificConstraint = true
			}
		}
	}

	if systemName := r.FormValue("system_name"); systemName != "" {
		// Prevent memory exhaustion from very short search strings
		if len(strings.TrimSpace(systemName)) >= 2 {
			filter.SystemName = &systemName
			hasSpecificConstraint = true
		}
	}

	if location := r.FormValue("location"); location != "" {
		// Prevent memory exhaustion from very short search strings
		if len(strings.TrimSpace(location)) >= 2 {
			filter.Location = &location
			hasSpecificConstraint = true
		}
	}

	// Scope to the globally selected network (explicit ?domain= still wins)
	domain := requestDomain(r)
	filter.Domain = &domain

	// Return empty filter for resource-intensive searches to prevent OOM
	if !hasSpecificConstraint {
		return database.NodeFilter{LatestOnly: &latestOnly, Limit: 0} // Limit 0 = no results
	}

	return filter
}
