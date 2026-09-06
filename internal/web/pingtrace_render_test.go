package web

import (
	"bytes"
	"html/template"
	"strings"
	"testing"
	"time"

	"github.com/nodelistdb/internal/pingtrace"
	"github.com/nodelistdb/internal/storage"
)

func renderPingTemplate(t *testing.T, name string, data any) string {
	t.Helper()
	s := &Server{templates: make(map[string]*template.Template), templatesFS: TemplatesFS}
	if err := s.loadTemplates(); err != nil {
		t.Fatalf("loading templates: %v", err)
	}
	tmpl, ok := s.templates[name]
	if !ok {
		t.Fatalf("%s template not loaded", name)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		t.Fatalf("rendering %s: %v", name, err)
	}
	return buf.String()
}

func samplePing() pingtrace.Ping {
	t0 := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	return pingtrace.Ping{
		Domain: "fidonet", Zone: 2, Net: 280, Node: 5555, Address: "2:280/5555", Mode: "routed",
		SentTime: t0, MSGID: "2:5001/100 68b8a1c2", FirstHop: "2:5020/715@fidonet", RouteSource: "default-route",
		Status: pingtrace.StatusPong, DispatchedTime: t0.Add(5 * time.Minute), ReplyTime: t0.Add(3*time.Hour + 12*time.Minute),
		RTTSeconds: 3*3600 + 12*60, ReplyFromName: "Ping Robot", ReplyFromAddr: "2:280/5555", RobotPID: "FMail 2.3",
		// TimeIsUTC mirrors the raw lines, as pingHops derives it on read:
		// the 715 stamp carries no "UTC" marker and so is that system's
		// local time.
		OutHops: []pingtrace.Hop{
			{Address: "2:5001/100", Time: t0.Add(time.Minute), TimeIsUTC: true, Software: "FidoMail 0.1.3", Raw: "2:5001/100@fidonet @20260903.120100.UTC FidoMail 0.1.3"},
			{Address: "2:5020/715", Time: t0.Add(30 * time.Minute), Software: "hpt/lnx 1.9.0", Raw: "2:5020/715 @20260903.123000 hpt/lnx 1.9.0"},
			{Software: "ZoneGate V8.1", Raw: "ZoneGate V8.1 by Alexey Presniakov, id : 493C"},
			{Address: "2:280/5555", Time: t0.Add(3 * time.Hour), TimeIsUTC: true, Software: "FMail 2.3", Raw: "2:280/5555 @20260903.150000.UTC FMail 2.3"},
		},
		BackHops:   []pingtrace.Hop{{Address: "2:280/5555", Time: t0.Add(3 * time.Hour), Software: "FMail 2.3"}},
		TraceCount: 1,
	}
}

func TestPingTraceAnalyticsRenders(t *testing.T) {
	p := samplePing()
	summary := &storage.PingTraceSummary{
		Domain: "fidonet", Days: 90,
		Nodes: []storage.PingNodeSummary{
			{Domain: "fidonet", Zone: 2, Net: 280, Node: 5555, Address: "2:280/5555", SystemName: "Michiel's_Node", SysopName: "Michiel_van_der_Vlist", HasPing: true, HasTrace: true, Latest: &p, Pings: 2, Pongs: 2, TraceVerdict: "unobserved"},
			{Domain: "fidonet", Zone: 2, Net: 5020, Node: 715, Address: "2:5020/715", HasTrace: true, TraceVerdict: "confirmed", TraceSeen: 3, TraceNotices: 2},
			{Domain: "fidonet", Zone: 1, Net: 1, Node: 19, Address: "1:1/19", HasPing: true},
		},
		PingNodes: 2, Answered: 1, NeverPinged: 1, MedianRTTSeconds: p.RTTSeconds, MedianHops: 4,
		TraceNodes: 2, TraceConfirmed: 1, TraceUnobserved: 1,
		Tracers: []storage.TracerStat{{Address: "2:5020/715", Notices: 2, Flagged: true}},
		Robots:  []storage.RobotStat{{Software: "FMail 2.3", Nodes: 1}},
	}
	rows := make([]pingNodeRow, 0)
	for _, n := range summary.Nodes {
		row := pingNodeRow{N: n, TraceClass: traceVerdictClass(n.TraceVerdict)}
		if n.Latest != nil {
			row.Latest = newPingView(*n.Latest)
			row.StatusLabel, row.StatusClass = row.Latest.StatusLabel, row.Latest.StatusClass
		} else if n.HasPing {
			row.StatusLabel, row.StatusClass = pingStatusLabel("")
		}
		rows = append(rows, row)
	}
	html := renderPingTemplate(t, "pingtrace_analytics", pingtraceAnalyticsPage{
		Title: "Netmail PING/TRACE", ActivePage: "analytics", Version: "test", Summary: summary, Rows: rows, Days: 90,
	})
	for _, want := range []string{
		"2:280/5555", "Answered", "3h 12m", "Never pinged", "confirmed", "2/3",
		"/analytics/pingtrace/node?address=2%3a280%2f5555", "FMail 2.3", "ZoneGate",
		// The elapsed reading skips the local-time 715 stamp and spans
		// 12:01 UTC -> 15:00 UTC.
		`class="hop origin"`, `class="hop target"`, "&#43;2h 59m",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("rendered report lacks %q", want)
		}
	}
}

func TestPingTraceNodeRenders(t *testing.T) {
	p := samplePing()
	page := pingtraceNodePage{
		Title: "x", ActivePage: "analytics", Version: "test", Address: "2:280/5555", Domain: "fidonet",
		Pings: []*pingView{newPingView(p)},
		Replies: []replyView{newReplyView(storage.PingReplyRow{Reply: pingtrace.Reply{FidomailMessageID: 9, Kind: pingtrace.KindTrace, FromName: "Trace Robot", FromAddr: "2:5020/715",
			Subject: "Trace: your message to PING", PingMSGID: p.MSGID, Vias: []string{"2:5020/715 @20260903.123000 hpt/lnx 1.9.0"}, PID: "hpt/lnx 1.9.0",
			ReceivedAt: time.Date(2026, 9, 3, 12, 31, 0, 0, time.UTC), Body: "passed through"}}, "2:280/5555", "fidonet")},
	}
	html := renderPingTemplate(t, "pingtrace_node", page)
	for _, want := range []string{"Outbound path", "Return path", "2:5020/715", "hpt/lnx 1.9.0", "Trace Robot", "passed through", "68b8a1c2", "Transit notices",
		// A stamp without the UTC marker is labelled for what it is, and
		// contributes no elapsed reading of its own.
		"2026-09-03 12:30 local", "2026-09-03 12:01 UTC",
		// The reply links to the card of the ping it answers.
		`id="ping-68b8a1c2"`, `href="#ping-68b8a1c2"`,
		// Hop and robot addresses link to the archive, not to a 400.
		`href="/node/2/5020/715?domain=fidonet"`, `href="/node/2/280/5555?domain=fidonet"`} {
		if !strings.Contains(html, want) {
			t.Errorf("rendered node page lacks %q", want)
		}
	}
	if strings.Contains(html, "&#43;29m") {
		t.Error("elapsed time must not be measured across a local-time stamp")
	}
	// A ping with no MSGID yet (still queued, or refused by fidomail)
	// must not leave a hole in the prose.
	queued := samplePing()
	queued.MSGID, queued.Status, queued.OutHops, queued.BackHops = "", pingtrace.StatusFailed, nil, nil
	failed := renderPingTemplate(t, "pingtrace_node", pingtraceNodePage{Title: "x", Address: "2:280/5555", Version: "test", Pings: []*pingView{newPingView(queued)}})
	if !strings.Contains(failed, "never left the sending node") {
		t.Error("a ping without a MSGID must fall back to a generic origin")
	}
	empty := renderPingTemplate(t, "pingtrace_node", pingtraceNodePage{Title: "x", Address: "1:1/19", Version: "test"})
	if !strings.Contains(empty, "No ping has been sent") {
		t.Error("empty node page must say so")
	}
}

// TestHopViewsOriginAndZones pins the two rules a quoted path must be read
// by: the origin is the MSGID's author, not whoever stamped first, and only
// UTC-marked stamps may be subtracted from one another.
func TestHopViewsOriginAndZones(t *testing.T) {
	t0 := time.Date(2026, 9, 3, 23, 41, 55, 0, time.UTC)
	// The real shape: our own mailer stamped nothing, so the first Via is
	// our uplink, and the last is a transit node rather than the target.
	hops := []pingtrace.Hop{
		{Address: "2:5020/715", Time: t0, TimeIsUTC: true, Raw: "2:5020/715 @20260903.234155.UTC RNtrack"},
		{Address: "2:292/854", Time: time.Date(2026, 9, 4, 1, 46, 4, 0, time.UTC), Raw: "2:292/854 @20260904.014604 D'Bridge 4"},
		{Address: "2:292/854", Time: time.Date(2026, 9, 4, 0, 49, 0, 0, time.UTC), TimeIsUTC: true, Raw: "2:292/854@Ward_Dossche @20260904.004900.UTC O/T-Track+"},
	}
	views := hopViews(hops, pingtrace.OriginAddress("2:5001/100@fidonet 6a99d1e1"), "1:1/19", "fidonet")
	for i, v := range views {
		if v.IsOrigin {
			t.Errorf("hop %d (%s) is a transit node, not the origin", i, v.Address)
		}
		if v.IsTarget {
			t.Errorf("hop %d (%s) is not the target", i, v.Address)
		}
	}
	if views[1].Delta != "" {
		t.Errorf("no elapsed time may be read off a local stamp, got %q", views[1].Delta)
	}
	if views[1].Time != "2026-09-04 01:46 local" {
		t.Errorf("unmarked stamp must not be labelled UTC, got %q", views[1].Time)
	}
	if views[2].Delta != "+1h 7m" {
		t.Errorf("UTC stamps span 23:41:55 -> 00:49:00, got %q", views[2].Delta)
	}
	// The archive route is /node/{zone}/{net}/{node}; the address as one
	// path segment answers 400.
	if views[0].NodeURL != "/node/2/5020/715?domain=fidonet" {
		t.Errorf("hop link = %q", views[0].NodeURL)
	}
	if u := nodeHistoryURL("2:5020/715.12", ""); u != "/node/2/5020/715" {
		t.Errorf("a point's link goes to its boss without a domain: %q", u)
	}
	if u := nodeHistoryURL("ZoneGate V8.1", "fidonet"); u != "" {
		t.Errorf("an addressless hop has no link: %q", u)
	}

	// And when we did stamp our own Via, that hop is the origin.
	own := hopViews([]pingtrace.Hop{{Address: "2:5001/100"}, {Address: "2:5020/715"}},
		pingtrace.OriginAddress("2:5001/100@fidonet 6a99d1e1"), "1:1/19", "fidonet")
	if !own[0].IsOrigin || own[1].IsOrigin {
		t.Errorf("origin must follow the MSGID: %+v", own)
	}
}

func TestFmtDurationShort(t *testing.T) {
	cases := map[time.Duration]string{
		0: "", 30 * time.Second: "30s", 5 * time.Minute: "5m", 3*time.Hour + 12*time.Minute: "3h 12m", 49 * time.Hour: "2d 1h",
	}
	for d, want := range cases {
		if got := fmtDurationShort(d); got != want {
			t.Errorf("fmtDurationShort(%s) = %q, want %q", d, got, want)
		}
	}
}
