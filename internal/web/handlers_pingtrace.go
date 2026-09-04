package web

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/nodelistdb/internal/logging"
	"github.com/nodelistdb/internal/pingtrace"
	"github.com/nodelistdb/internal/storage"
	"github.com/nodelistdb/internal/version"
)

// hopView is one hop of a rendered path.
type hopView struct {
	Address  string
	Software string
	Time     string // "2026-09-03 12:00 UTC" or ""
	Delta    string // "+32m" since the previous timed hop, or ""
	Raw      string
	IsOrigin bool
	IsTarget bool
}

// pingView is one ping with its fields pre-formatted for a template.
type pingView struct {
	P           pingtrace.Ping
	Sent        string
	Dispatched  string
	Reply       string
	RTT         string
	StatusLabel string
	StatusClass string
	Out         []hopView
	Back        []hopView
	OutCount    int
	BackCount   int
}

// replyView is one stored reply for the node page.
type replyView struct {
	R         storage.PingReplyRow
	Received  string
	KindClass string
	Hops      []hopView
	Body      string
}

func fmtPingTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02 15:04 UTC")
}

// fmtDurationShort renders a duration as "3d 4h", "2h 15m", "45m" or "30s".
func fmtDurationShort(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	d = d.Round(time.Second)
	days := int(d / (24 * time.Hour))
	d -= time.Duration(days) * 24 * time.Hour
	hours := int(d / time.Hour)
	d -= time.Duration(hours) * time.Hour
	mins := int(d / time.Minute)
	secs := int((d - time.Duration(mins)*time.Minute) / time.Second)
	switch {
	case days > 0:
		return fmt.Sprintf("%dd %dh", days, hours)
	case hours > 0:
		return fmt.Sprintf("%dh %dm", hours, mins)
	case mins > 0:
		return fmt.Sprintf("%dm", mins)
	default:
		return fmt.Sprintf("%ds", secs)
	}
}

func hopViews(hops []pingtrace.Hop, target string) []hopView {
	out := make([]hopView, 0, len(hops))
	var prev time.Time
	for i, h := range hops {
		v := hopView{Address: h.Address, Software: h.Software, Time: fmtPingTime(h.Time), Raw: h.Raw}
		if !h.Time.IsZero() {
			if !prev.IsZero() {
				if d := h.Time.Sub(prev); d > 0 {
					v.Delta = "+" + fmtDurationShort(d)
				} else if d < 0 {
					v.Delta = "-" + fmtDurationShort(-d)
				}
			}
			prev = h.Time
		}
		v.IsOrigin = i == 0
		v.IsTarget = h.Address != "" && pingtrace.Node3D(h.Address) == target
		out = append(out, v)
	}
	return out
}

func pingStatusLabel(status string) (string, string) {
	switch status {
	case pingtrace.StatusPong:
		return "Answered", "badge-success"
	case pingtrace.StatusSent:
		return "Waiting", "badge-info"
	case pingtrace.StatusQueued:
		return "Queued", "badge-info"
	case pingtrace.StatusTimeout:
		return "No answer", "badge-warning"
	case pingtrace.StatusNDR:
		return "Bounced", "badge-danger"
	case pingtrace.StatusFailed:
		return "Not sent", "badge-danger"
	case "":
		return "Never pinged", "badge-secondary"
	default:
		return status, "badge-secondary"
	}
}

func newPingView(p pingtrace.Ping) *pingView {
	v := &pingView{
		P:          p,
		Sent:       fmtPingTime(p.SentTime),
		Dispatched: fmtPingTime(p.DispatchedTime),
		Reply:      fmtPingTime(p.ReplyTime),
		Out:        hopViews(p.OutHops, p.Address),
		Back:       hopViews(p.BackHops, p.Address),
	}
	if p.Status == pingtrace.StatusPong {
		v.RTT = fmtDurationShort(time.Duration(p.RTTSeconds) * time.Second)
	}
	v.StatusLabel, v.StatusClass = pingStatusLabel(p.Status)
	v.OutCount = len(pingtrace.Addresses(p.OutHops))
	v.BackCount = len(pingtrace.Addresses(p.BackHops))
	return v
}

// pingNodeRow is one row of the report table.
type pingNodeRow struct {
	N            storage.PingNodeSummary
	Latest       *pingView
	LatestDirect *pingView
	StatusLabel  string
	StatusClass  string
	TraceClass   string
}

type pingtraceAnalyticsPage struct {
	Title      string
	ActivePage string
	Version    string
	Summary    *storage.PingTraceSummary
	Rows       []pingNodeRow
	Days       int
	Error      error
}

func traceVerdictClass(v string) string {
	switch v {
	case "confirmed":
		return "badge-success"
	case "silent":
		return "badge-warning"
	case "unobserved":
		return "badge-secondary"
	}
	return ""
}

// PingTraceAnalyticsHandler renders /analytics/pingtrace.
func (s *Server) PingTraceAnalyticsHandler(w http.ResponseWriter, r *http.Request) {
	domain := requestDomain(r)
	days := parseLimitParam(r, "days", 90, 730)

	var displayError error
	summary, err := s.storage.GetPingTraceSummary(r.Context(), domain, days)
	if err != nil {
		var handled bool
		if displayError, handled = storageFailure("PING/TRACE Analytics", "Failed to fetch PING/TRACE data. Please try again later", err); handled {
			return
		}
		summary = &storage.PingTraceSummary{Domain: domain, Days: days}
	}

	rows := make([]pingNodeRow, 0, len(summary.Nodes))
	for _, n := range summary.Nodes {
		row := pingNodeRow{N: n, TraceClass: traceVerdictClass(n.TraceVerdict)}
		if n.Latest != nil {
			row.Latest = newPingView(*n.Latest)
			row.StatusLabel, row.StatusClass = row.Latest.StatusLabel, row.Latest.StatusClass
		} else if n.HasPing {
			row.StatusLabel, row.StatusClass = pingStatusLabel("")
		}
		if n.LatestDirect != nil {
			row.LatestDirect = newPingView(*n.LatestDirect)
		}
		rows = append(rows, row)
	}

	s.renderStatus(w, "pingtrace_analytics", pingtraceAnalyticsPage{
		Title:      "Netmail PING/TRACE",
		ActivePage: "analytics",
		Version:    version.GetVersionInfo(),
		Summary:    summary,
		Rows:       rows,
		Days:       days,
		Error:      displayError,
	}, statusFor(displayError))
}

type pingtraceNodePage struct {
	Title      string
	ActivePage string
	Version    string
	Address    string
	Domain     string
	Pings      []*pingView
	Replies    []replyView
	Error      error
}

// PingTraceNodeHandler renders /analytics/pingtrace/node?address=z:n/n.
func (s *Server) PingTraceNodeHandler(w http.ResponseWriter, r *http.Request) {
	address := strings.TrimSpace(r.URL.Query().Get("address"))
	var zone, net, node int
	if _, err := fmt.Sscanf(address, "%d:%d/%d", &zone, &net, &node); err != nil {
		http.Error(w, "address must be zone:net/node", http.StatusBadRequest)
		return
	}
	address = fmt.Sprintf("%d:%d/%d", zone, net, node)
	availableDomains, _ := s.storage.GetNodeDomains(r.Context(), zone, net, node)
	domain := resolveEntityDomain(r, availableDomains)

	var displayError error
	pings, err := s.storage.GetNodePings(r.Context(), domain, zone, net, node, 200)
	if err != nil {
		var handled bool
		if displayError, handled = storageFailure("PING/TRACE node", "Failed to fetch ping history. Please try again later", err); handled {
			return
		}
	}
	replies, err := s.storage.GetNodePingReplies(r.Context(), domain, zone, net, node, 500)
	if err != nil {
		if clientGone("PING/TRACE node: replies", err) {
			return
		}
		logging.Errorf("PING/TRACE node: error fetching replies: %v", err)
	}

	page := pingtraceNodePage{
		Title:      "Netmail PING " + address,
		ActivePage: "analytics",
		Version:    version.GetVersionInfo(),
		Address:    address,
		Domain:     domain,
		Error:      displayError,
	}
	for _, p := range pings {
		page.Pings = append(page.Pings, newPingView(p))
	}
	for _, rep := range replies {
		v := replyView{R: rep, Received: fmtPingTime(rep.ReceivedAt), Hops: hopViews(rep.Hops, address), Body: rep.Body}
		switch rep.Kind {
		case pingtrace.KindPong:
			v.KindClass = "badge-success"
		case pingtrace.KindTrace:
			v.KindClass = "badge-info"
		case pingtrace.KindNDR:
			v.KindClass = "badge-danger"
		default:
			v.KindClass = "badge-secondary"
		}
		page.Replies = append(page.Replies, v)
	}
	s.renderStatus(w, "pingtrace_node", page, statusFor(displayError))
}

// parseLimitParam reads a bounded positive integer query parameter.
func parseLimitParam(r *http.Request, name string, def, max int) int {
	v := 0
	if _, err := fmt.Sscanf(r.URL.Query().Get(name), "%d", &v); err != nil || v <= 0 {
		return def
	}
	if v > max {
		return max
	}
	return v
}
