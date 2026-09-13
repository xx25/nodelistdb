package storage

import (
	"fmt"
	"strings"
)

// telnetSuccessSQL is the condition under which a test row's telnet result
// counts as a working ITN mailer. column is one of telnet_success,
// telnet_ipv4_success or telnet_ipv6_success; prefix is the table alias ("" or
// "r.").
//
// Until 2026-09-10 the ITN tester reported success for any accepted TCP
// connection, so a BBS login prompt or an ssh banner on the announced port
// counted as a working mailer, and made the node operational. Since then
// success means a completed EMSI handshake, and what the remote said in
// EMSI_DAT is stored alongside. Requiring that evidence keeps the old
// bare-connect rows out of every report that reads the flag — which matters
// most where a report keeps a node on the strength of *any* success in its
// window, since such a node would otherwise stay listed for the whole window
// after its first honest failure. The address list is the primary witness:
// EMSI_DAT's first field is the remote's AKAs and is never empty on a real
// handshake, while the mailer name/version fields further along may
// legitimately be blank.
//
// The evidence columns are per row, not per address family, but a
// per-family success flag is only ever set by a handshake that produced
// them, so the same witness serves all three columns.
func telnetSuccessSQL(prefix, column string) string {
	return fmt.Sprintf("(%[1]s%[2]s = true AND (notEmpty(%[1]stelnet_addresses) OR %[1]stelnet_mailer_info != ''))", prefix, column)
}

// applyTelnetEvidence substitutes the {{TELNET_OK}}, {{TELNET_IPV4_OK}} and
// {{TELNET_IPV6_OK}} placeholders (and their _R variants for queries that
// alias node_test_results as r) with telnetSuccessSQL.
func applyTelnetEvidence(query string) string {
	r := strings.NewReplacer(
		"{{TELNET_OK}}", telnetSuccessSQL("", "telnet_success"),
		"{{TELNET_OK_R}}", telnetSuccessSQL("r.", "telnet_success"),
		"{{TELNET_IPV4_OK}}", telnetSuccessSQL("", "telnet_ipv4_success"),
		"{{TELNET_IPV4_OK_R}}", telnetSuccessSQL("r.", "telnet_ipv4_success"),
		"{{TELNET_IPV6_OK}}", telnetSuccessSQL("", "telnet_ipv6_success"),
		"{{TELNET_IPV6_OK_R}}", telnetSuccessSQL("r.", "telnet_ipv6_success"),
	)
	return r.Replace(query)
}
