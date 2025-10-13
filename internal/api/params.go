package api

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/nodelistdb/internal/database"
)

// parseIntParam parses an integer parameter from query string.
// Returns 0 and false if the parameter doesn't exist or is invalid.
func parseIntParam(query url.Values, key string) (int, bool) {
	if val := query.Get(key); val != "" {
		if n, err := strconv.Atoi(val); err == nil {
			return n, true
		}
	}
	return 0, false
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

// parseDateParam parses a date parameter from query string in YYYY-MM-DD format.
// Returns zero time and false if the parameter doesn't exist or is invalid.
func parseDateParam(query url.Values, key string) (time.Time, bool) {
	if val := query.Get(key); val != "" {
		if t, err := time.Parse("2006-01-02", val); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
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

// parsePathInt extracts an integer from URL path.
// Returns 0 and error if parsing fails.
func parsePathInt(pathPart string, fieldName string) (int, error) {
	n, err := strconv.Atoi(pathPart)
	if err != nil {
		return 0, &ParamError{
			Field:   fieldName,
			Value:   pathPart,
			Message: "invalid " + fieldName + " number",
		}
	}
	return n, nil
}

// parseNodeFilter builds a NodeFilter from query parameters.
// Returns the filter and a boolean indicating if any specific constraint was provided.
func parseNodeFilter(r *http.Request) (database.NodeFilter, bool, error) {
	filter := database.NodeFilter{}
	query := r.URL.Query()
	hasConstraint := false

	// Zone, Net, Node
	if zone, ok := parseIntParam(query, "zone"); ok {
		filter.Zone = &zone
		hasConstraint = true
	}

	if net, ok := parseIntParam(query, "net"); ok {
		filter.Net = &net
		hasConstraint = true
	}

	if node, ok := parseIntParam(query, "node"); ok {
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
	if dateFrom, ok := parseDateParam(query, "date_from"); ok {
		filter.DateFrom = &dateFrom
		hasConstraint = true
	}

	if dateTo, ok := parseDateParam(query, "date_to"); ok {
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

// ParamError represents a parameter parsing error.
type ParamError struct {
	Field   string
	Value   string
	Message string
}

func (e *ParamError) Error() string {
	return e.Message
}
