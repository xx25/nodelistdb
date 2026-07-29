package api

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/nodelistdb/internal/database"
)

// parseIntParam parses an integer parameter from a query string. It reports
// present separately from ok: an absent parameter is not an error, but a
// malformed one is. Both used to come back as (0, false), so ?zone=two was
// silently dropped and the caller got an unfiltered search instead of a
// complaint.
func parseIntParam(query url.Values, key string) (value int, present bool, err error) {
	raw := query.Get(key)
	if raw == "" {
		return 0, false, nil
	}
	n, cerr := strconv.Atoi(raw)
	if cerr != nil {
		return 0, true, &ParamError{Field: key, Value: raw, Message: key + " must be a whole number"}
	}
	return n, true, nil
}

// maxAnalyticsDays bounds the ?days= look-back window on the analytics
// endpoints. Those endpoints are unauthenticated and the window feeds
// straight into the range of nodelist partitions a query reads, so an
// unbounded value buys an arbitrary full-history scan per request. 3650 days
// covers the entire test-results history several times over.
const maxAnalyticsDays = 3650

// parseDaysParam reads a ?days= look-back window, falling back to
// defaultDays when the parameter is absent or unusable, and clamping the
// result to maxAnalyticsDays.
func parseDaysParam(query url.Values, defaultDays int) int {
	days := defaultDays
	if d, err := strconv.Atoi(query.Get("days")); err == nil && d > 0 {
		days = d
	}
	if days > maxAnalyticsDays {
		days = maxAnalyticsDays
	}
	return days
}

// parseBoolParam parses a boolean parameter from query string.
// Returns the value, whether it was present, and any error.
// Accepts: true/false, 1/0, yes/no (case-insensitive).
func parseBoolParam(query url.Values, key string) (bool, bool) {
	if val := query.Get(key); val != "" {
		lower := strings.ToLower(strings.TrimSpace(val))
		switch lower {
		case "true", "1", "yes":
			return true, true
		case "false", "0", "no":
			return false, true
		default:
			// For backward compatibility, treat invalid values as false
			// but still indicate the parameter was present
			return false, true
		}
	}
	return false, false
}

// parseDateParam parses a YYYY-MM-DD query parameter. Like parseIntParam, an
// absent parameter is not an error and a malformed one is: ?date_from=last
// used to be dropped, which quietly widened the search rather than rejecting
// it.
func parseDateParam(query url.Values, key string) (value time.Time, present bool, err error) {
	raw := query.Get(key)
	if raw == "" {
		return time.Time{}, false, nil
	}
	t, perr := time.Parse("2006-01-02", raw)
	if perr != nil {
		return time.Time{}, true, &ParamError{Field: key, Value: raw, Message: key + " must be a date in YYYY-MM-DD form"}
	}
	return t, true, nil
}

// parseStringParam parses a string parameter from query string with minimum length validation.
// Returns empty string and false if the parameter doesn't meet minimum length.
// The returned value is trimmed of leading/trailing whitespace.
func parseStringParam(query url.Values, key string, minLength int) (string, bool) {
	if val := query.Get(key); val != "" {
		trimmed := strings.TrimSpace(val)
		if len(trimmed) >= minLength {
			return trimmed, true
		}
	}
	return "", false
}

// parsePaginationParams parses limit and offset parameters with defaults and bounds.
func parsePaginationParams(query url.Values, defaultLimit, maxLimit int) (limit, offset int) {
	limit = defaultLimit

	if limitStr := query.Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			if l > maxLimit {
				limit = maxLimit
			} else {
				limit = l
			}
		}
	}

	if offsetStr := query.Get("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	return limit, offset
}

// parseNodeFilter builds a NodeFilter from query parameters.
// Returns the filter and a boolean indicating if any specific constraint was provided.
func parseNodeFilter(r *http.Request) (database.NodeFilter, bool, error) {
	filter := database.NodeFilter{}
	query := r.URL.Query()
	hasConstraint := false

	// FTN network filter (does not count as a constraint on its own)
	if domain, ok := parseStringParam(query, "domain", 1); ok {
		domain = strings.ToLower(domain)
		filter.Domain = &domain
	}

	// Zone, Net, Node
	zone, present, err := parseIntParam(query, "zone")
	if err != nil {
		return filter, false, err
	}
	if present {
		filter.Zone = &zone
		hasConstraint = true
	}

	net, present, err := parseIntParam(query, "net")
	if err != nil {
		return filter, false, err
	}
	if present {
		filter.Net = &net
		hasConstraint = true
	}

	node, present, err := parseIntParam(query, "node")
	if err != nil {
		return filter, false, err
	}
	if present {
		filter.Node = &node
		hasConstraint = true
	}

	// String fields with minimum length validation
	if systemName, ok := parseStringParam(query, "system_name", 2); ok {
		filter.SystemName = &systemName
		hasConstraint = true
	} else if query.Get("system_name") != "" {
		return filter, false, &ParamError{
			Field:   "system_name",
			Value:   query.Get("system_name"),
			Message: "system_name must be at least 2 characters long",
		}
	}

	if location, ok := parseStringParam(query, "location", 2); ok {
		filter.Location = &location
		hasConstraint = true
	} else if query.Get("location") != "" {
		return filter, false, &ParamError{
			Field:   "location",
			Value:   query.Get("location"),
			Message: "location must be at least 2 characters long",
		}
	}

	if sysopName, ok := parseStringParam(query, "sysop_name", 2); ok {
		filter.SysopName = &sysopName
		hasConstraint = true
	} else if query.Get("sysop_name") != "" {
		return filter, false, &ParamError{
			Field:   "sysop_name",
			Value:   query.Get("sysop_name"),
			Message: "sysop_name must be at least 2 characters long",
		}
	}

	// Node type
	if nodeType := query.Get("node_type"); nodeType != "" {
		filter.NodeType = &nodeType
		hasConstraint = true
	}

	// Boolean flags
	if isCM, ok := parseBoolParam(query, "is_cm"); ok {
		filter.IsCM = &isCM
		hasConstraint = true
	}

	if isMO, ok := parseBoolParam(query, "is_mo"); ok {
		filter.IsMO = &isMO
		hasConstraint = true
	}

	if hasInet, ok := parseBoolParam(query, "has_inet"); ok {
		filter.HasInet = &hasInet
		hasConstraint = true
	}

	if hasBinkp, ok := parseBoolParam(query, "has_binkp"); ok {
		filter.HasBinkp = &hasBinkp
		hasConstraint = true
	}

	// Date range
	dateFrom, present, err := parseDateParam(query, "date_from")
	if err != nil {
		return filter, false, err
	}
	if present {
		filter.DateFrom = &dateFrom
		hasConstraint = true
	}

	dateTo, present, err := parseDateParam(query, "date_to")
	if err != nil {
		return filter, false, err
	}
	if present {
		filter.DateTo = &dateTo
		hasConstraint = true
	}

	// Latest only filter
	if latestOnly, ok := parseBoolParam(query, "latest_only"); ok {
		filter.LatestOnly = &latestOnly
	}

	// Pagination with defaults
	filter.Limit, filter.Offset = parsePaginationParams(query, 100, 500)

	return filter, hasConstraint, nil
}

// parsePointFilter builds a PointFilter from query parameters.
// Returns the filter and a boolean indicating if any specific constraint was provided.
func parsePointFilter(r *http.Request) (database.PointFilter, bool, error) {
	filter := database.PointFilter{}
	query := r.URL.Query()
	hasConstraint := false

	// FTN network filter (does not count as a constraint on its own)
	if domain, ok := parseStringParam(query, "domain", 1); ok {
		domain = strings.ToLower(domain)
		filter.Domain = &domain
	}

	// Zone, Net, Node, Point
	zone, present, err := parseIntParam(query, "zone")
	if err != nil {
		return filter, false, err
	}
	if present {
		filter.Zone = &zone
		hasConstraint = true
	}

	net, present, err := parseIntParam(query, "net")
	if err != nil {
		return filter, false, err
	}
	if present {
		filter.Net = &net
		hasConstraint = true
	}

	node, present, err := parseIntParam(query, "node")
	if err != nil {
		return filter, false, err
	}
	if present {
		filter.Node = &node
		hasConstraint = true
	}

	point, present, err := parseIntParam(query, "point")
	if err != nil {
		return filter, false, err
	}
	if present {
		filter.PointNum = &point
		hasConstraint = true
	}

	// Pointlist series filter (r24, z2, ...)
	if source, ok := parseStringParam(query, "list_source", 1); ok {
		source = strings.ToLower(source)
		filter.ListSource = &source
		hasConstraint = true
	}

	// String fields with minimum length validation
	for _, sf := range []struct {
		key  string
		dest **string
	}{
		{"system_name", &filter.SystemName},
		{"location", &filter.Location},
		{"sysop_name", &filter.SysopName},
	} {
		if val, ok := parseStringParam(query, sf.key, 2); ok {
			*sf.dest = &val
			hasConstraint = true
		} else if query.Get(sf.key) != "" {
			return filter, false, &ParamError{
				Field:   sf.key,
				Value:   query.Get(sf.key),
				Message: sf.key + " must be at least 2 characters long",
			}
		}
	}

	// Date range
	dateFrom, present, err := parseDateParam(query, "date_from")
	if err != nil {
		return filter, false, err
	}
	if present {
		filter.DateFrom = &dateFrom
		hasConstraint = true
	}

	dateTo, present, err := parseDateParam(query, "date_to")
	if err != nil {
		return filter, false, err
	}
	if present {
		filter.DateTo = &dateTo
		hasConstraint = true
	}

	// Latest only filter (snapshot semantics)
	if latestOnly, ok := parseBoolParam(query, "latest_only"); ok {
		filter.LatestOnly = &latestOnly
	}

	// Pagination with defaults
	filter.Limit, filter.Offset = parsePaginationParams(query, 100, 500)

	return filter, hasConstraint, nil
}

// ParamError represents a parameter parsing error.
type ParamError struct {
	Field   string
	Value   string
	Message string
}

func (e *ParamError) Error() string {
	return e.Message
}

// addressEnvelope builds the {address, domain, available_domains} preamble the
// node and point endpoints wrap their payloads in. Pass point < 0 for a 3-D
// address; any other value renders the 4-D form.
//
// available_domains is always an array, never null: a client reading it as a
// list should not have to special-case an address that exists in one network.
func addressEnvelope(zone, net, node, point int, domain string, availableDomains []string) map[string]interface{} {
	address := fmt.Sprintf("%d:%d/%d", zone, net, node)
	if point >= 0 {
		address = fmt.Sprintf("%d:%d/%d.%d", zone, net, node, point)
	}
	if availableDomains == nil {
		availableDomains = []string{}
	}
	return map[string]interface{}{
		"address":           address,
		"domain":            domain,
		"available_domains": availableDomains,
	}
}

// explicitDomain returns the FTN network the request itself names via
// ?domain=, normalized, or "" when it names none.
func explicitDomain(r *http.Request) string {
	return strings.ToLower(strings.TrimSpace(r.URL.Query().Get("domain")))
}

// domainOrDefault is explicitDomain with fidonet as the fallback. Endpoints
// that answer about one network use it, so a pre-multi-network URL keeps its
// original meaning.
func domainOrDefault(r *http.Request) string {
	if d := explicitDomain(r); d != "" {
		return d
	}
	return database.DefaultDomain
}

// domainOrAll is explicitDomain with "" - all networks - as the fallback.
// The software and geo analytics endpoints use it: aggregating every network
// is what they did before networks existed, and narrowing that silently would
// change every existing caller's numbers.
func domainOrAll(r *http.Request) string {
	return explicitDomain(r)
}

// preferDomain resolves which of an address's networks an endpoint answers
// about. An explicit request wins outright, even for a network the address is
// not in - the caller asked a specific question and deserves its real answer,
// which may be "not found". Otherwise: the only network it exists in, or
// fidonet when it exists in several, so pre-multi-network URLs keep pointing
// at the same node.
func preferDomain(explicit string, available []string) string {
	if explicit != "" {
		return explicit
	}
	switch len(available) {
	case 0:
		return database.DefaultDomain
	case 1:
		return available[0]
	default:
		for _, d := range available {
			if d == database.DefaultDomain {
				return d
			}
		}
		return available[0]
	}
}
