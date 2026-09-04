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
		OutHops: []pingtrace.Hop{
			{Address: "2:5001/100", Time: t0.Add(time.Minute), Software: "FidoMail 0.1.3", Raw: "2:5001/100@fidonet @20260903.120100.UTC FidoMail 0.1.3"},
			{Address: "2:5020/715", Time: t0.Add(30 * time.Minute), Software: "hpt/lnx 1.9.0", Raw: "2:5020/715 @20260903.123000 hpt/lnx 1.9.0"},
			{Software: "ZoneGate V8.1", Raw: "ZoneGate V8.1 by Alexey Presniakov, id : 493C"},
			{Address: "2:280/5555", Time: t0.Add(3 * time.Hour), Software: "FMail 2.3", Raw: "2:280/5555 @20260903.150000.UTC FMail 2.3"},
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
		`class="hop origin"`, `class="hop target"`, "&#43;29m",
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
		Replies: []replyView{{
			R: storage.PingReplyRow{Reply: pingtrace.Reply{FidomailMessageID: 9, Kind: pingtrace.KindTrace, FromName: "Trace Robot", FromAddr: "2:5020/715",
				Subject: "Trace: your message to PING", PingMSGID: p.MSGID, Vias: []string{"2:5020/715 @20260903.123000 hpt/lnx 1.9.0"}, PID: "hpt/lnx 1.9.0"}},
			Received: "2026-09-03 12:31 UTC", KindClass: "badge-info", Body: "passed through",
		}},
	}
	html := renderPingTemplate(t, "pingtrace_node", page)
	for _, want := range []string{"Outbound path", "Return path", "2:5020/715", "hpt/lnx 1.9.0", "Trace Robot", "passed through", "68b8a1c2", "Transit notices"} {
		if !strings.Contains(html, want) {
			t.Errorf("rendered node page lacks %q", want)
		}
	}
	empty := renderPingTemplate(t, "pingtrace_node", pingtraceNodePage{Title: "x", Address: "1:1/19", Version: "test"})
	if !strings.Contains(empty, "No ping has been sent") {
		t.Error("empty node page must say so")
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
