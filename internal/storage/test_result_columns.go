package storage

import (
	"strings"
)

// testResultColumnGroups lists, in scan order, the columns
// ResultParser.ParseTestResultRow reads. Every query that feeds that parser
// must project exactly this list, in this order: the parser scans by position,
// so a column added to one query and not another is not a compile error, not a
// runtime error at that query, but a silent off-by-one that fills every field
// after the insertion point with the neighbouring column's value.
//
// The sub-slices are line breaks and nothing else - they exist so a query
// printed to a log stays readable.
var testResultColumnGroups = [][]string{
	{"test_time", "zone", "net", "node", "address", "hostname"},
	{"resolved_ipv4", "resolved_ipv6", "dns_error"},
	{"country", "country_code", "city", "region", "latitude", "longitude", "isp", "org", "asn"},
	{"binkp_tested", "binkp_success", "binkp_response_ms", "binkp_system_name"},
	{"binkp_sysop", "binkp_location", "binkp_version", "binkp_addresses", "binkp_capabilities", "binkp_error"},
	{"ifcico_tested", "ifcico_success", "ifcico_response_ms", "ifcico_mailer_info", "ifcico_system_name"},
	{"ifcico_addresses", "ifcico_response_type", "ifcico_error"},
	{"telnet_tested", "telnet_success", "telnet_response_ms", "telnet_error"},
	{"ftp_tested", "ftp_success", "ftp_response_ms", "ftp_error"},
	{"vmodem_tested", "vmodem_success", "vmodem_response_ms", "vmodem_error"},
	{"vmodem_variant", "vmodem_conformant", "vmodem_software", "vmodem_system_name"},
	{"vmodem_sysop", "vmodem_location", "vmodem_addresses"},
	{"vmodem_detail", "vmodem_call_outcome", "vmodem_banner"},
	{"binkp_ipv4_tested", "binkp_ipv4_success", "binkp_ipv4_response_ms", "binkp_ipv4_address", "binkp_ipv4_error"},
	{"binkp_ipv6_tested", "binkp_ipv6_success", "binkp_ipv6_response_ms", "binkp_ipv6_address", "binkp_ipv6_error"},
	{"ifcico_ipv4_tested", "ifcico_ipv4_success", "ifcico_ipv4_response_ms", "ifcico_ipv4_address", "ifcico_ipv4_error"},
	{"ifcico_ipv6_tested", "ifcico_ipv6_success", "ifcico_ipv6_response_ms", "ifcico_ipv6_address", "ifcico_ipv6_error"},
	{"telnet_ipv4_tested", "telnet_ipv4_success", "telnet_ipv4_response_ms", "telnet_ipv4_address", "telnet_ipv4_error"},
	{"telnet_ipv6_tested", "telnet_ipv6_success", "telnet_ipv6_response_ms", "telnet_ipv6_address", "telnet_ipv6_error"},
	{"ftp_ipv4_tested", "ftp_ipv4_success", "ftp_ipv4_response_ms", "ftp_ipv4_address", "ftp_ipv4_error"},
	{"ftp_ipv6_tested", "ftp_ipv6_success", "ftp_ipv6_response_ms", "ftp_ipv6_address", "ftp_ipv6_error"},
	{"vmodem_ipv4_tested", "vmodem_ipv4_success", "vmodem_ipv4_response_ms", "vmodem_ipv4_address", "vmodem_ipv4_error"},
	{"vmodem_ipv6_tested", "vmodem_ipv6_success", "vmodem_ipv6_response_ms", "vmodem_ipv6_address", "vmodem_ipv6_error"},
	{"is_operational", "has_connectivity_issues", "address_validated"},
	{"tested_hostname", "hostname_index", "is_aggregated"},
	{"total_hostnames", "hostnames_tested", "hostnames_operational"},
	{"ftp_anon_success", "domain", "derived_from_address"},
}

// Markers for the projection above. A query carries one of these where the
// column list would otherwise be spelled out, and applyTestResultColumns
// expands it.
const (
	// testColumnsMarker is the unqualified projection, for a query selecting
	// straight from node_test_results with no alias.
	testColumnsMarker = "{{TEST_RESULT_COLUMNS}}"
	// testColumnsMarkerR is the projection qualified with the "r" alias, used
	// inside the ranked_results CTE that picks one row per node.
	testColumnsMarkerR = "{{TEST_RESULT_COLUMNS_R}}"
	// testColumnsMarkerRRNodeName is the projection qualified with the "rr"
	// alias, where the two handshake-reported system names defer to the
	// nodelist. Requires a LEFT JOIN aliased "n" onto a nodes-derived CTE.
	testColumnsMarkerRRNodeName = "{{TEST_RESULT_COLUMNS_RR_NODENAME}}"
)

// applyTestResultColumns expands whichever projection markers a query carries.
// Continuation lines are indented to the marker's own column, so the generated
// SQL lines up with the query it was spliced into.
func applyTestResultColumns(query string) string {
	for _, m := range []struct {
		marker    string
		alias     string
		overrides map[string]string
	}{
		{testColumnsMarkerRRNodeName, "rr", nodelistSystemNameOverrides("rr", "n")},
		{testColumnsMarkerR, "r", nil},
		{testColumnsMarker, "", nil},
	} {
		for {
			idx := strings.Index(query, m.marker)
			if idx < 0 {
				break
			}
			query = query[:idx] +
				testResultColumnsSQL(m.alias, m.overrides, markerIndent(query, idx)) +
				query[idx+len(m.marker):]
		}
	}
	return query
}

// markerIndent returns the whitespace between the start of the marker's line
// and the marker itself, or "" when anything else precedes it on that line.
func markerIndent(query string, idx int) string {
	start := strings.LastIndexByte(query[:idx], '\n') + 1
	prefix := query[start:idx]
	if strings.TrimLeft(prefix, " \t") != "" {
		return ""
	}
	return prefix
}

// testResultColumnsSQL renders the projection for one query.
//
// alias is the table alias the columns hang off ("" for none). overrides
// replaces named columns with a complete expression; each expression must
// alias back to the column name it replaces, so that the projection's
// position and the scan target it feeds stay paired. indent prefixes every
// line after the first.
func testResultColumnsSQL(alias string, overrides map[string]string, indent string) string {
	qualify := func(col string) string {
		if expr, ok := overrides[col]; ok {
			return expr
		}
		if alias == "" {
			return col
		}
		return alias + "." + col
	}

	lines := make([]string, 0, len(testResultColumnGroups))
	for _, group := range testResultColumnGroups {
		parts := make([]string, len(group))
		for i, col := range group {
			parts[i] = qualify(col)
		}
		lines = append(lines, strings.Join(parts, ", "))
	}
	return strings.Join(lines, ",\n"+indent)
}

// nodelistSystemNameOverrides prefers the nodelist's system name over the one
// the remote mailer reported during the handshake. BinkP and IFCICO announce
// whatever their operator configured, which on a misconfigured host is a
// hostname, a version banner or nothing at all; the nodelist is the name the
// network agreed on. NULLIF is required because a LEFT JOIN miss yields ”
// rather than NULL in ClickHouse (join_use_nulls is off), so COALESCE alone
// would happily return the empty string.
func nodelistSystemNameOverrides(rowAlias, nodeAlias string) map[string]string {
	overrides := make(map[string]string, 2)
	for _, col := range []string{"binkp_system_name", "ifcico_system_name"} {
		overrides[col] = "COALESCE(NULLIF(" + nodeAlias + ".system_name, ''), " +
			rowAlias + "." + col + ") as " + col
	}
	return overrides
}
