package storage

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// BuildReachabilityStatsQuery has been wrong three times, each time in a way no
// Go test could see: it counted stored rows instead of test cycles, then merged
// two distinct tests into one, then failed to notice the boundary between two
// complete multi-hostname cycles. Every one of those was caught by reading SQL,
// and the last shipped to production. What the other tests here check is the
// query's shape, which catches a deleted rule but not a subtly wrong new one.
//
// So this test feeds known rows through the REAL query string and checks the
// numbers that come out. It runs the actual ClickHouse engine via `clickhouse
// local`, a one-shot mode that needs no server, no port and no stored data, so
// `go test ./...` exercises it wherever that binary exists and skips cleanly
// where it does not.
//
// Cases are written as the daemon would have stored them: one row per hostname
// in the order tested, then the aggregated summary if the run finished.
func TestReachabilityStatsCycleSemantics(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns the ClickHouse engine; skipped under -short")
	}
	bin, err := exec.LookPath("clickhouse")
	if err != nil {
		t.Skip("clickhouse binary not in PATH; cannot execute the query for real")
	}

	tests := []struct {
		name string
		why  string
		rows []fixtureRow
		want statsTiles
	}{
		{
			name: "single hostname, three cycles, one failed",
			why:  "the ordinary case: one row per cycle, counted as written",
			rows: []fixtureRow{
				{ts: "10:00:00", idx: 0, operational: true, ipv6OK: true},
				{ts: "11:00:00", idx: 0, operational: false},
				{ts: "12:00:00", idx: 0, operational: true, ipv6OK: true},
			},
			want: statsTiles{total: 3, fully: 2, partial: 0, failed: 1, rate: 66.7},
		},
		{
			name: "multi-hostname cycle with a broken backup",
			why: "the bug that started all of this: a permanently broken second " +
				"hostname must not make the node look partly unreachable",
			rows: []fixtureRow{
				{ts: "10:00:00", idx: 0, operational: false},
				{ts: "10:00:01", idx: 1, operational: true, ipv6OK: true},
				{ts: "10:00:02", idx: 0, aggregated: true, operational: true, ipv6OK: true},
			},
			want: statsTiles{total: 1, fully: 1, partial: 0, failed: 0, rate: 100},
		},
		{
			name: "whole cycle inside one second",
			why:  "rows commonly share a test_time; ties must not split a cycle",
			rows: []fixtureRow{
				{ts: "10:00:00", idx: 0, operational: false},
				{ts: "10:00:00", idx: 1, operational: true, ipv6OK: true},
				{ts: "10:00:00", idx: 0, aggregated: true, operational: true, ipv6OK: true},
			},
			want: statsTiles{total: 1, fully: 1, partial: 0, failed: 0, rate: 100},
		},
		{
			name: "two complete multi-hostname cycles 61s apart",
			why: "the boundary is aggregate(index 0) -> next primary(index 0); " +
				"before 489496e the whole second cycle vanished and this read 1 at 100%",
			rows: []fixtureRow{
				{ts: "10:00:00", idx: 0, operational: true, ipv6OK: true},
				{ts: "10:00:01", idx: 1, operational: true, ipv6OK: true},
				{ts: "10:00:02", idx: 0, aggregated: true, operational: true, ipv6OK: true},
				{ts: "10:01:03", idx: 0, operational: false},
				{ts: "10:01:04", idx: 1, operational: false},
				{ts: "10:01:05", idx: 0, aggregated: true, operational: false},
			},
			want: statsTiles{total: 2, fully: 1, partial: 0, failed: 1, rate: 50},
		},
		{
			name: "same host re-tested inside the window",
			why: "1:342/806 was re-tested by hand 60s after an automatic run; two " +
				"tests of one host are two cycles, not one",
			rows: []fixtureRow{
				{ts: "10:00:00", idx: 0, operational: false},
				{ts: "10:01:00", idx: 0, operational: true, ipv6OK: true},
			},
			want: statsTiles{total: 2, fully: 1, partial: 0, failed: 1, rate: 50},
		},
		{
			name: "interrupted cycle, aggregate never written",
			why: "the working hostname must not launder another hostname's broken " +
				"IPv6 into a fully successful cycle",
			rows: []fixtureRow{
				{ts: "10:00:00", idx: 0, operational: false, ipv6OK: false},
				{ts: "10:00:01", idx: 1, operational: true, ipv6OK: false, noIPv6: true},
			},
			want: statsTiles{total: 1, fully: 0, partial: 1, failed: 0, rate: 100},
		},
		{
			name: "operational cycle with a failed IPv6 leg",
			why:  "reachable over IPv4 but not IPv6 is the partially-failed tier",
			rows: []fixtureRow{
				{ts: "10:00:00", idx: 0, operational: true, ipv6OK: false},
			},
			want: statsTiles{total: 1, fully: 0, partial: 1, failed: 0, rate: 100},
		},
		{
			name: "three hostnames plus aggregate",
			why: "indices ascend 0,1,2 within one cycle, so the does-not-advance " +
				"rule must not fire mid-cycle for a node with more than two hosts",
			rows: []fixtureRow{
				{ts: "10:00:00", idx: 0, operational: false},
				{ts: "10:00:01", idx: 1, operational: false},
				{ts: "10:00:02", idx: 2, operational: true, ipv6OK: true},
				{ts: "10:00:03", idx: 0, aggregated: true, operational: true, ipv6OK: true},
			},
			want: statsTiles{total: 1, fully: 1, partial: 0, failed: 0, rate: 100},
		},
		{
			name: "AKA-derived result must not absorb this node's failure",
			why: "a derived row is another node's success cloned in for a shared " +
				"host; merging it hid a real failure and read 1 test at 100%",
			rows: []fixtureRow{
				{ts: "10:00:00", idx: 0, operational: false},
				{ts: "10:00:01", idx: 0, aggregated: true, operational: true, ipv6OK: true,
					derivedFrom: "2:5001/100@fidonet"},
			},
			want: statsTiles{total: 2, fully: 1, partial: 0, failed: 1, rate: 50},
		},
		{
			name: "two complete cycles inside one second",
			why: "test_time has second resolution, so index cannot separate them; " +
				"splitting on it shredded 2 cycles into 4",
			rows: []fixtureRow{
				{ts: "10:00:00", idx: 0, operational: true, ipv6OK: true},
				{ts: "10:00:00", idx: 1, operational: true, ipv6OK: true},
				{ts: "10:00:00", idx: 0, aggregated: true, operational: true, ipv6OK: true},
				{ts: "10:00:00", idx: 0, operational: false},
				{ts: "10:00:00", idx: 1, operational: false},
				{ts: "10:00:00", idx: 0, aggregated: true, operational: false},
			},
			want: statsTiles{total: 2, fully: 1, partial: 0, failed: 1, rate: 50},
		},
		{
			name: "legacy no-index row abutting a modern cycle",
			why: "hostname_index -1 is a live sentinel on 3090 production rows; " +
				"-1 then 0 ascends, so the does-not-advance rule alone misses it",
			rows: []fixtureRow{
				{ts: "10:00:00", idx: -1, operational: false},
				{ts: "10:00:30", idx: 0, operational: true, ipv6OK: true},
			},
			want: statsTiles{total: 2, fully: 1, partial: 0, failed: 1, rate: 50},
		},
		{
			name: "two interrupted cycles back to back",
			why: "with no aggregate to end either cycle, the boundary is only " +
				"visible as index 1 followed by index 0",
			rows: []fixtureRow{
				{ts: "10:00:00", idx: 0, operational: false},
				{ts: "10:00:01", idx: 1, operational: true, ipv6OK: true},
				{ts: "10:01:00", idx: 0, operational: false},
				{ts: "10:01:01", idx: 1, operational: false},
			},
			want: statsTiles{total: 2, fully: 1, partial: 0, failed: 1, rate: 50},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := runStatsQuery(t, bin, tc.rows)
			if err != nil {
				t.Fatalf("query failed: %v", err)
			}
			if got != tc.want {
				t.Errorf("%s\n  got  %+v\n  want %+v", tc.why, got, tc.want)
			}
		})
	}
}

// fixtureRow is one stored test result, described the way the daemon writes it.
type fixtureRow struct {
	ts          string // time of day on the fixed fixture date
	idx         int    // hostname_index; -1 is the legacy/CLI "no index" sentinel
	aggregated  bool
	operational bool
	ipv6OK      bool   // the per-protocol IPv6 legs succeeded
	noIPv6      bool   // this hostname resolved no IPv6 at all
	derivedFrom string // non-empty: cloned from another node's test via a shared host
}

type statsTiles struct {
	total, fully, partial, failed int
	rate                          float64
}

const fixtureDate = "2026-07-26 "

func (r fixtureRow) values() string {
	b := func(v bool) string { return map[bool]string{true: "true", false: "false"}[v] }
	v6 := "['2001:db8::1']"
	if r.noIPv6 {
		v6 = "[]"
	}
	// Protocol legs mirror the row's overall verdict; the IPv6 legs are what the
	// fully/partially-failed tiers read.
	return fmt.Sprintf("('fidonet',2,999,1,'%s%s',%d,%s,%s,'%s',['192.0.2.1'],%s,"+
		"true,%s,true,%s,true,%s,10,"+
		"true,%s,true,%s,true,%s,10,"+
		"true,%s,true,%s,true,%s,10)",
		fixtureDate, r.ts, r.idx, b(r.aggregated), b(r.operational), r.derivedFrom, v6,
		b(r.operational), b(r.operational), b(r.ipv6OK),
		b(r.operational), b(r.operational), b(r.ipv6OK),
		b(r.operational), b(r.operational), b(r.ipv6OK))
}

// runStatsQuery creates a throwaway in-process table, inserts the fixture and
// runs the production query against it.
func runStatsQuery(t *testing.T, bin string, rows []fixtureRow) (statsTiles, error) {
	t.Helper()

	const ddl = `CREATE TABLE node_test_results (
	domain String, zone Int32, net Int32, node Int32,
	test_time DateTime, hostname_index Int32,
	is_aggregated Bool, is_operational Bool, derived_from_address String,
	resolved_ipv4 Array(String), resolved_ipv6 Array(String),
	binkp_tested Bool, binkp_success Bool, binkp_ipv4_tested Bool, binkp_ipv4_success Bool,
	binkp_ipv6_tested Bool, binkp_ipv6_success Bool, binkp_response_ms UInt32,
	ifcico_tested Bool, ifcico_success Bool, ifcico_ipv4_tested Bool, ifcico_ipv4_success Bool,
	ifcico_ipv6_tested Bool, ifcico_ipv6_success Bool, ifcico_response_ms UInt32,
	telnet_tested Bool, telnet_success Bool, telnet_ipv4_tested Bool, telnet_ipv4_success Bool,
	telnet_ipv6_tested Bool, telnet_ipv6_success Bool, telnet_response_ms UInt32
) ENGINE = Memory;`

	values := make([]string, 0, len(rows))
	for _, r := range rows {
		values = append(values, r.values())
	}

	// The real query, with its placeholders filled. Everything else - the cycle
	// boundaries, the folds, the tiers - is exactly what production runs.
	query := (&TestQueryBuilder{}).BuildReachabilityStatsQuery()
	query = strings.Replace(query, "zone = ? AND net = ? AND node = ?", "zone = 2 AND net = 999 AND node = 1", 1)
	query = strings.ReplaceAll(query, "now() - INTERVAL ? DAY", "toDateTime('2026-07-01 00:00:00')")
	query = strings.ReplaceAll(query, "(? = '' OR domain = ?)", "1 = 1")

	script := fmt.Sprintf("%s\nINSERT INTO node_test_results VALUES %s;\n%s;\n",
		ddl, strings.Join(values, ","), query)

	path := filepath.Join(t.TempDir(), "case.sql")
	if err := os.WriteFile(path, []byte(script), 0o600); err != nil {
		return statsTiles{}, err
	}

	out, err := exec.Command(bin, "local", "--queries-file", path, "--format", "TSV").CombinedOutput()
	if err != nil {
		return statsTiles{}, fmt.Errorf("%v: %s", err, out)
	}

	fields := strings.Split(strings.TrimSpace(string(out)), "\t")
	// zone, net, node, total, fully, partial, failed, successful, rate, ...
	if len(fields) < 9 {
		return statsTiles{}, fmt.Errorf("unexpected output %q", out)
	}
	num := func(i int) int { n, _ := strconv.Atoi(fields[i]); return n }
	rate, _ := strconv.ParseFloat(fields[8], 64)
	return statsTiles{
		total:   num(3),
		fully:   num(4),
		partial: num(5),
		failed:  num(6),
		rate:    float64(int(rate*10+0.5)) / 10,
	}, nil
}
