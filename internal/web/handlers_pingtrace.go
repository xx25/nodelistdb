package web

import (
	"fmt"
	"net/http"
	"net/url"
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
	Delta    string // "+32m" since the previous UTC-stamped hop, or ""
	Raw      string
	IsOrigin bool
	IsTarget bool
	// NodeURL is the hop's page in the archive, empty when the Via line
	// named no address.
	NodeURL string
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
	// Anchor is the fragment id the node page gives this ping's card, so a
	// reply can link to the ping it answers.
	Anchor string
	// Origin is the node that sent the ping, read from the MSGID. A ping
	// still queued, or one fidomail refused, has no MSGID yet; the prose
	// then says "the sending node" rather than leaving a hole.
	Origin string
	// FirstHopURL and ReplyFromURL are archive links for the first hop and
	// the robot that answered ("" when the address does not parse).
	FirstHopURL  string
	ReplyFromURL string
}

// replyView is one stored reply for the node page.
type replyView struct {
	R         storage.PingReplyRow
	Received  string
	KindClass string
	Hops      []hopView
	Body      string
	FromURL   string
	// PingAnchor is the fragment id of the ping this reply answers, empty
	// when it was never matched to one.
	PingAnchor string
}

// pingAnchor derives a fragment id from a MSGID ("2:5001/100@fidonet
// 6a99d1e3" -> "ping-6a99d1e3"): the serial is unique per origin, and the
// origin is always our own node on this page.
func pingAnchor(msgid string) string {
	f := strings.Fields(msgid)
	if len(f) == 0 {
		return ""
	}
	return "ping-" + f[len(f)-1]
}

func fmtPingTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02 15:04 UTC")
}

// fmtHopTime renders a hop's own stamp. The FTS-4009 "UTC" marker is
// optional, and a line without it carries that system's local time -- which
// is not ours to convert, so it is labelled for what it is instead of being
// relabelled UTC.
// nodeHistoryURL builds the archive link for an FTN address. The route is
// /node/{zone}/{net}/{node}: an address written as one segment
// ("/node/2:5020/715") answers 400, which is what every hop link on this
// page used to do.
func nodeHistoryURL(addr, domain string) string {
	var zone, net, node int
	if _, err := fmt.Sscanf(pingtrace.Node3D(addr), "%d:%d/%d", &zone, &net, &node); err != nil {
		return ""
	}
	u := fmt.Sprintf("/node/%d/%d/%d", zone, net, node)
	if domain != "" {
		u += "?domain=" + url.QueryEscape(domain)
	}
	return u
}

func fmtHopTime(h pingtrace.Hop) string {
	if h.Time.IsZero() {
		return ""
	}
	if h.TimeIsUTC {
		return h.Time.UTC().Format("2006-01-02 15:04 UTC")
	}
	return h.Time.Format("2006-01-02 15:04 local")
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

func hopViews(hops []pingtrace.Hop, origin, target, domain string) []hopView {
	out := make([]hopView, 0, len(hops))
	// Elapsed time is measured between UTC-marked stamps only, skipping
	// the hops in between. Subtracting a local stamp from a UTC one
	// measures that hop's timezone rather than the time it held the
	// message, which is where this page's negative "elapsed" times came
	// from; a mailer that writes local time simply contributes no reading.
	var prevUTC time.Time
	for _, h := range hops {
		v := hopView{Address: h.Address, Software: h.Software, Time: fmtHopTime(h), Raw: h.Raw}
		if !h.Time.IsZero() && h.TimeIsUTC {
			if !prevUTC.IsZero() {
				if d := h.Time.Sub(prevUTC); d > 0 {
					v.Delta = "+" + fmtDurationShort(d)
				} else if d < 0 {
					v.Delta = "-" + fmtDurationShort(-d)
				}
			}
			prevUTC = h.Time
		}
		// The origin is named by the MSGID, not by being first: our own
		// mailer stamps no Via, so hop 0 is normally the first system we
		// routed through.
		v.IsOrigin = origin != "" && h.Address != "" && pingtrace.Node3D(h.Address) == origin
		v.IsTarget = h.Address != "" && pingtrace.Node3D(h.Address) == target
		if h.Address != "" {
			v.NodeURL = nodeHistoryURL(h.Address, domain)
		}
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
		Out:        hopViews(p.OutHops, pingtrace.OriginAddress(p.MSGID), p.Address, p.Domain),
		Back:       hopViews(p.BackHops, pingtrace.OriginAddress(p.MSGID), p.Address, p.Domain),
	}
	if p.Status == pingtrace.StatusPong {
		v.RTT = fmtDurationShort(time.Duration(p.RTTSeconds) * time.Second)
	}
	v.StatusLabel, v.StatusClass = pingStatusLabel(p.Status)
	v.Anchor = pingAnchor(p.MSGID)
	v.Origin = pingtrace.OriginAddress(p.MSGID)
	if v.Origin == "" {
		v.Origin = "the sending node"
	}
	v.FirstHopURL = nodeHistoryURL(p.FirstHop, p.Domain)
	v.ReplyFromURL = nodeHistoryURL(p.ReplyFromAddr, p.Domain)
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
	NodeURL    string
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
		NodeURL:    nodeHistoryURL(address, domain),
		Error:      displayError,
	}
	rendered := make(map[string]bool, len(pings))
	for _, p := range pings {
		v := newPingView(p)
		rendered[v.Anchor] = true
		page.Pings = append(page.Pings, v)
	}
	for _, rep := range replies {
		v := newReplyView(rep, address, domain)
		// A reply may outlive its ping (the two tables expire on their own
		// clocks, and the page reads more replies than pings), so it only
		// links to a card that is actually on the page.
		if !rendered[v.PingAnchor] {
			v.PingAnchor = ""
		}
		page.Replies = append(page.Replies, v)
	}
	s.renderStatus(w, "pingtrace_node", page, statusFor(displayError))
}

// newReplyView formats one stored reply for the node page of `address`.
func newReplyView(rep storage.PingReplyRow, address, domain string) replyView {
	v := replyView{R: rep, Received: fmtPingTime(rep.ReceivedAt),
		Hops:    hopViews(rep.Hops, pingtrace.OriginAddress(rep.PingMSGID), address, domain),
		Body:    rep.Body,
		FromURL: nodeHistoryURL(rep.FromAddr, domain),
	}
	if rep.PingMSGID != "" {
		v.PingAnchor = pingAnchor(rep.PingMSGID)
	}
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
	return v
}

// anyRaw reports whether any hop of a path kept its raw Via line, which
// decides whether the "Raw Via lines" block is worth rendering.
func anyRaw(hops []hopView) bool {
	for _, h := range hops {
		if h.Raw != "" {
			return true
		}
	}
	return false
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
