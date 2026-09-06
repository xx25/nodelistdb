package storage

import (
	"testing"
	"time"

	"github.com/nodelistdb/internal/pingtrace"
)

func TestFoldPingTraceSummary(t *testing.T) {
	t0 := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	summary := &PingTraceSummary{Domain: "fidonet", Days: 90, Nodes: []PingNodeSummary{
		{Domain: "fidonet", Address: "2:280/5555", HasPing: true},
		{Domain: "fidonet", Address: "2:5020/715", HasTrace: true}, // transit, notices sent
		{Domain: "fidonet", Address: "2:5020/0", HasTrace: true},   // transit, silent
		{Domain: "fidonet", Address: "2:221/0", HasTrace: true},    // never on a path
		{Domain: "fidonet", Address: "2:5001/100", HasTrace: true}, // us: on the path, never crossed
		{Domain: "fidonet", Address: "1:1/19", HasPing: true},      // timed out
		{Domain: "fidonet", Address: "2:33/0", HasPing: true},      // never pinged
	}}
	index := map[string]int{}
	for i, n := range summary.Nodes {
		index[n.Address+"@"+n.Domain] = i
	}
	hop := func(a string) pingtrace.Hop { return pingtrace.Hop{Address: a} }
	// The first ping quotes a path that includes our own Via stamp; the
	// second is the shape production actually produces -- no stamp of our
	// own, and the target quoting the path as it stood before its own toss
	// stamp, so 2:5020/0 is the last hop without being the destination.
	// Both crossings of 715 and the one of 5020/0 must be counted.
	pings := []pingtrace.Ping{
		{Domain: "fidonet", Address: "2:280/5555", Mode: "routed", SentTime: t0, Status: pingtrace.StatusPong, RTTSeconds: 3600, RobotPID: "FMail 2.3",
			MSGID:   "2:5001/100@fidonet 68b8a1c2",
			OutHops: []pingtrace.Hop{hop("2:5001/100"), hop("2:5020/715"), hop("2:5020/0"), hop("2:280/5555")}},
		{Domain: "fidonet", Address: "2:280/5555", Mode: "routed", SentTime: t0.Add(-14 * 24 * time.Hour), Status: pingtrace.StatusPong, RTTSeconds: 7200, RobotPID: "FMail 2.3",
			MSGID:   "2:5001/100@fidonet 4f1c9a30",
			OutHops: []pingtrace.Hop{hop("2:5020/715"), hop("2:5020/0")}},
		{Domain: "fidonet", Address: "2:280/5555", Mode: "direct", SentTime: t0, Status: pingtrace.StatusSent},
		{Domain: "fidonet", Address: "1:1/19", Mode: "routed", SentTime: t0, Status: pingtrace.StatusTimeout},
	}
	notices := []traceNotice{
		{from: "2:5020/715", pingDomain: "fidonet", pingAddress: "2:280/5555", pingSentTime: t0},
		{from: "2:9999/1", pingDomain: "fidonet", pingAddress: "2:280/5555", pingSentTime: t0}, // unflagged tracer
	}
	foldPingTraceSummary(summary, index, pings, notices)

	if summary.PingNodes != 3 || summary.Answered != 1 || summary.Timeouts != 1 || summary.NeverPinged != 1 {
		t.Errorf("ping counts: %+v", summary)
	}
	if summary.TraceNodes != 4 || summary.TraceConfirmed != 1 || summary.TraceSilent != 1 || summary.TraceUnobserved != 2 {
		t.Errorf("trace counts: %+v", summary)
	}
	n := summary.Nodes[index["2:280/5555@fidonet"]]
	if n.Latest == nil || n.Latest.RTTSeconds != 3600 || n.LatestDirect == nil || n.Pings != 3 || n.Pongs != 2 {
		t.Errorf("node summary: %+v", n)
	}
	if v := summary.Nodes[index["2:5020/715@fidonet"]]; v.TraceVerdict != "confirmed" || v.TraceSeen != 2 || v.TraceNotices != 1 {
		t.Errorf("715 verdict: %+v", v)
	}
	if v := summary.Nodes[index["2:5020/0@fidonet"]]; v.TraceVerdict != "silent" || v.TraceSeen != 2 {
		t.Errorf("5020/0 verdict: %+v", v)
	}
	if v := summary.Nodes[index["2:221/0@fidonet"]]; v.TraceVerdict != "unobserved" {
		t.Errorf("221/0 verdict: %+v", v)
	}
	// Our own Via stamp is an endpoint, not a hop we crossed.
	if v := summary.Nodes[index["2:5001/100@fidonet"]]; v.TraceVerdict != "unobserved" || v.TraceSeen != 0 {
		t.Errorf("the origin must never count as a transit hop: %+v", v)
	}
	if summary.MedianRTTSeconds != 7200 || summary.MedianHops != 4 {
		t.Errorf("medians: rtt=%d hops=%d", summary.MedianRTTSeconds, summary.MedianHops)
	}
	if len(summary.Tracers) != 2 || summary.Tracers[0].Address != "2:5020/715" || !summary.Tracers[0].Flagged || summary.Tracers[1].Flagged {
		t.Errorf("tracers: %+v", summary.Tracers)
	}
	if len(summary.Robots) != 1 || summary.Robots[0].Software != "FMail 2.3" || summary.Robots[0].Nodes != 1 {
		t.Errorf("robots: %+v", summary.Robots)
	}
}

// TestFoldPingTraceSummaryPerNetwork pins that transit evidence never
// crosses FTN networks. /api/analytics/pingtrace answers with no domain
// filter when ?domain= is omitted, so the same zone:net/node number can
// appear once per network in one fold.
func TestFoldPingTraceSummaryPerNetwork(t *testing.T) {
	t0 := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	summary := &PingTraceSummary{Days: 90, Nodes: []PingNodeSummary{
		{Domain: "fidonet", Address: "21:1/100", HasTrace: true},
		{Domain: "fsxnet", Address: "21:1/100", HasTrace: true},
	}}
	index := map[string]int{}
	for i, n := range summary.Nodes {
		index[n.Address+"@"+n.Domain] = i
	}
	hop := func(a string) pingtrace.Hop { return pingtrace.Hop{Address: a} }
	pings := []pingtrace.Ping{
		{Domain: "fidonet", Address: "2:280/5555", Mode: "routed", SentTime: t0, Status: pingtrace.StatusPong,
			MSGID: "2:5001/100@fidonet 68b8a1c2", OutHops: []pingtrace.Hop{hop("21:1/100"), hop("2:280/5555")}},
	}
	notices := []traceNotice{{from: "21:1/100", pingDomain: "fidonet", pingAddress: "2:280/5555", pingSentTime: t0}}
	foldPingTraceSummary(summary, index, pings, notices)

	if v := summary.Nodes[index["21:1/100@fidonet"]]; v.TraceVerdict != "confirmed" || v.TraceSeen != 1 {
		t.Errorf("fidonet 21:1/100: %+v", v)
	}
	if v := summary.Nodes[index["21:1/100@fsxnet"]]; v.TraceVerdict != "unobserved" || v.TraceSeen != 0 {
		t.Errorf("fsxnet 21:1/100 must not inherit fidonet evidence: %+v", v)
	}
	if len(summary.Tracers) != 1 || summary.Tracers[0].Domain != "fidonet" || !summary.Tracers[0].Flagged {
		t.Errorf("tracers: %+v", summary.Tracers)
	}
}

// TestReplyAndPingHopsRestoreTheRawLine covers the read paths that
// reconstruct hops: ping_tests keeps each raw Via line, ping_replies keeps
// only the body, and both must come back with the FTS-4009 "UTC" marker
// the line carried -- there is no column for it.
func TestReplyAndPingHopsRestoreTheRawLine(t *testing.T) {
	utcRaw := "2:5020/715 @20260903.234155.UTC RNtrack 2.3.0/Lnx/Perl"
	localRaw := "2:292/854 @20260904.014604 D'Bridge 4"
	addrs := []string{"2:5020/715", "2:292/854"}
	times := []time.Time{
		time.Date(2026, 9, 3, 23, 41, 55, 0, time.UTC),
		time.Date(2026, 9, 4, 1, 46, 4, 0, time.UTC),
	}
	soft := []string{"RNtrack 2.3.0/Lnx/Perl", "D'Bridge 4"}

	hops := pingHops(addrs, times, soft, []string{utcRaw, localRaw})
	if !hops[0].TimeIsUTC || hops[1].TimeIsUTC {
		t.Errorf("pingHops lost the UTC marker: %+v", hops)
	}

	body := "Here are all the detected VIA lines from your message:\n" +
		"Via " + utcRaw + "\n" +
		"Via " + localRaw + "\n"
	hops = replyHops(body, addrs, times, soft)
	if len(hops) != 2 || !hops[0].TimeIsUTC || hops[1].TimeIsUTC {
		t.Errorf("replyHops lost the UTC marker: %+v", hops)
	}
	if hops[1].Raw != localRaw {
		t.Errorf("replyHops lost the raw line: %q", hops[1].Raw)
	}

	// A body that no longer yields the same path falls back to the
	// stored columns rather than rewriting history.
	hops = replyHops("no via lines here", addrs, times, soft)
	if len(hops) != 2 || hops[0].Address != "2:5020/715" || hops[0].Raw != "" {
		t.Errorf("fallback to the stored columns failed: %+v", hops)
	}
}

// TestPingHopsRereadRawLine pins that a stored hop is shown as the current
// parser reads its raw line, not as the daemon's parser stored it: the rows
// written before FMail's millisecond stamps were understood carry
// ".188.UTC FMail-W32(Toss) ..." in the software column and a local-time
// stamp, and re-importing them is not an option.
func TestPingHopsRereadRawLine(t *testing.T) {
	stored := time.Date(2026, 9, 3, 20, 0, 54, 0, time.UTC)
	hops := pingHops(
		[]string{"2:280/5555"},
		[]time.Time{stored},
		[]string{".188.UTC FMail-W32(Toss) 2.3.0.1-B20240319"},
		[]string{"2:280/5555 @20260903.200054.188.UTC FMail-W32(Toss) 2.3.0.1-B20240319"},
	)
	if len(hops) != 1 {
		t.Fatalf("got %d hops, want 1", len(hops))
	}
	h := hops[0]
	if h.Software != "FMail-W32(Toss) 2.3.0.1-B20240319" {
		t.Errorf("software = %q, want the stamp tail stripped", h.Software)
	}
	if !h.TimeIsUTC || !h.Time.Equal(stored) {
		t.Errorf("time = %v utc=%v, want %v utc=true", h.Time, h.TimeIsUTC, stored)
	}
	// A hop whose raw line is missing keeps its stored columns.
	bare := pingHops([]string{"2:5020/715"}, []time.Time{stored}, []string{"hpt"}, []string{""})
	if bare[0].Software != "hpt" || !bare[0].Time.Equal(stored) {
		t.Errorf("bare hop = %+v, want stored columns kept", bare[0])
	}
}
