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
		{Domain: "fidonet", Address: "1:1/19", HasPing: true},      // timed out
		{Domain: "fidonet", Address: "2:33/0", HasPing: true},      // never pinged
	}}
	index := map[string]int{}
	for i, n := range summary.Nodes {
		index[n.Address+"@"+n.Domain] = i
	}
	hop := func(a string) pingtrace.Hop { return pingtrace.Hop{Address: a} }
	pings := []pingtrace.Ping{
		{Domain: "fidonet", Address: "2:280/5555", Mode: "routed", SentTime: t0, Status: pingtrace.StatusPong, RTTSeconds: 3600, RobotPID: "FMail 2.3",
			OutHops: []pingtrace.Hop{hop("2:5001/100"), hop("2:5020/715"), hop("2:5020/0"), hop("2:280/5555")}},
		{Domain: "fidonet", Address: "2:280/5555", Mode: "routed", SentTime: t0.Add(-14 * 24 * time.Hour), Status: pingtrace.StatusPong, RTTSeconds: 7200, RobotPID: "FMail 2.3",
			OutHops: []pingtrace.Hop{hop("2:5001/100"), hop("2:5020/715"), hop("2:280/5555")}},
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
	if summary.TraceNodes != 3 || summary.TraceConfirmed != 1 || summary.TraceSilent != 1 || summary.TraceUnobserved != 1 {
		t.Errorf("trace counts: %+v", summary)
	}
	n := summary.Nodes[index["2:280/5555@fidonet"]]
	if n.Latest == nil || n.Latest.RTTSeconds != 3600 || n.LatestDirect == nil || n.Pings != 3 || n.Pongs != 2 {
		t.Errorf("node summary: %+v", n)
	}
	if v := summary.Nodes[index["2:5020/715@fidonet"]]; v.TraceVerdict != "confirmed" || v.TraceSeen != 2 || v.TraceNotices != 1 {
		t.Errorf("715 verdict: %+v", v)
	}
	if v := summary.Nodes[index["2:5020/0@fidonet"]]; v.TraceVerdict != "silent" || v.TraceSeen != 1 {
		t.Errorf("5020/0 verdict: %+v", v)
	}
	if v := summary.Nodes[index["2:221/0@fidonet"]]; v.TraceVerdict != "unobserved" {
		t.Errorf("221/0 verdict: %+v", v)
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
