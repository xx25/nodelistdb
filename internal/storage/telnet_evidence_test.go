package storage

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestTelnetSuccessSQL(t *testing.T) {
	got := telnetSuccessSQL("r.", "telnet_ipv6_success")
	want := "(r.telnet_ipv6_success = true AND (notEmpty(r.telnet_addresses) OR r.telnet_mailer_info != ''))"
	if got != want {
		t.Errorf("telnetSuccessSQL:\n got %s\nwant %s", got, want)
	}

	// The protocol predicate is the same condition with the alias left as a
	// format verb for BuildProtocolEnabledQuery to fill in.
	if p := protocolSuccessPredicates["telnet"]; !strings.Contains(p, "%[1]stelnet_addresses") {
		t.Errorf("telnet predicate does not carry the EMSI evidence: %s", p)
	}
}

func TestApplyTelnetEvidenceReplacesEveryPlaceholder(t *testing.T) {
	q := "{{TELNET_OK}} {{TELNET_OK_R}} {{TELNET_IPV4_OK}} {{TELNET_IPV4_OK_R}} {{TELNET_IPV6_OK}} {{TELNET_IPV6_OK_R}}"
	out := applyTelnetEvidence(q)
	if strings.Contains(out, "{{TELNET") {
		t.Errorf("placeholder survived: %s", out)
	}
	for _, col := range []string{"telnet_success", "r.telnet_success", "telnet_ipv4_success", "r.telnet_ipv4_success", "telnet_ipv6_success", "r.telnet_ipv6_success"} {
		if !strings.Contains(out, "("+col+" = true AND") {
			t.Errorf("missing gated %s in %s", col, out)
		}
	}

	// Every marker used in the package's queries must be one
	// applyTelnetEvidence knows about, and every file that uses one must call
	// it - a forgotten call leaves the marker in the SQL, which fails only when
	// the query runs.
	known := map[string]bool{}
	for _, m := range []string{"{{TELNET_OK}}", "{{TELNET_OK_R}}", "{{TELNET_IPV4_OK}}", "{{TELNET_IPV4_OK_R}}", "{{TELNET_IPV6_OK}}", "{{TELNET_IPV6_OK_R}}"} {
		known[m] = true
	}
	used := regexp.MustCompile(`\{\{TELNET_\w+\}\}`)
	sources, _ := filepath.Glob("*.go")
	for _, src := range sources {
		if strings.HasSuffix(src, "_test.go") || src == "telnet_evidence.go" {
			continue
		}
		body, err := os.ReadFile(src)
		if err != nil {
			t.Fatal(err)
		}
		text := string(body)
		markers := used.FindAllString(text, -1)
		for _, marker := range markers {
			if !known[marker] {
				t.Errorf("%s uses unknown marker %s - add it to applyTelnetEvidence", src, marker)
			}
		}
		if len(markers) > 0 && !strings.Contains(text, "applyTelnetEvidence(") {
			t.Errorf("%s contains %d telnet marker(s) but never calls applyTelnetEvidence - the marker would reach ClickHouse verbatim",
				src, len(markers))
		}
	}
}

// A telnet success flag read bare in a report query is the bug this package
// fixed on 2026-09-13: rows written before the ITN tester spoke EMSI carry the
// flag with nothing behind it. Every reader must go through telnetSuccessSQL
// (or the {{TELNET_*_OK}} placeholders), so a bare `telnet_*_success = true`
// in any non-test source file here is a regression, whichever query it is in.
func TestNoBareTelnetSuccessPredicateInQueries(t *testing.T) {
	bare := regexp.MustCompile(`\btelnet(_ipv[46])?_success = true`)
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") || f == "telnet_evidence.go" {
			continue
		}
		body, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(body), "\n") {
			if bare.MatchString(line) {
				t.Errorf("%s:%d reads a telnet success flag without EMSI evidence; use telnetSuccessSQL or a {{TELNET_*_OK}} placeholder:\n%s", f, i+1, strings.TrimSpace(line))
			}
		}
	}
}
