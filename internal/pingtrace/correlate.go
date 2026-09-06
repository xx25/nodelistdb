package pingtrace

import (
	"regexp"
	"strings"
	"time"
)

// Ping modes.
const (
	ModeRouted = "routed" // normal netmail routing: the path is the measurement
	ModeDirect = "direct" // FSC-0053 DIR: dialed straight from the nodelist, robot only
)

// Ping statuses, in lifecycle order.
const (
	StatusQueued  = "queued"  // accepted by fidomail, waiting for the dialer
	StatusSent    = "sent"    // handed to the first hop
	StatusFailed  = "failed"  // fidomail could not deliver it to the first hop
	StatusPong    = "pong"    // the destination's robot answered
	StatusNDR     = "ndr"     // bounced as undeliverable
	StatusTimeout = "timeout" // no answer within the reply window
)

// Reply kinds.
const (
	KindPong      = "pong"
	KindTrace     = "trace"
	KindNDR       = "ndr"
	KindUnmatched = "unmatched"
)

// Ping is one netmail PING sent to one node, and everything learned about
// it since. It is the row of the ping_tests table.
type Ping struct {
	Domain   string
	Zone     int
	Net      int
	Node     int
	Address  string // "zone:net/node"
	Mode     string
	SentTime time.Time
	// Token is the per-ping random tag carried in the subject, so a robot
	// that quotes neither MSGID nor REPLY can still be matched.
	Token string
	MSGID string
	// FidomailMessageID is the sender's row id, used to poll delivery state.
	FidomailMessageID uint64
	FirstHop          string
	RouteSource       string
	Status            string
	DispatchedTime    time.Time
	ReplyTime         time.Time
	RTTSeconds        uint32
	ReplyMessageID    uint64
	ReplyMSGID        string
	ReplyFromName     string
	ReplyFromAddr     string
	RobotPID          string
	RobotTearline     string
	OutHops           []Hop
	BackHops          []Hop
	TraceCount        uint32
	Error             string
	UpdatedAt         time.Time
}

// Key identifies the node a ping targets.
func (p Ping) Key() string {
	return p.Address + "@" + p.Domain
}

// Reply is one inbound netmail the poller read from fidomail's inbox, as
// stored in ping_replies together with its classification.
type Reply struct {
	FidomailMessageID uint64
	Kind              string
	// The ping it answers (zero values when unmatched).
	PingDomain   string
	PingZone     int
	PingNet      int
	PingNode     int
	PingSentTime time.Time
	PingMSGID    string

	MSGID      string
	ReplyID    string
	FromName   string
	FromAddr   string
	ToName     string
	Subject    string
	Body       string
	Date       time.Time
	ReceivedAt time.Time
	PID        string
	Tearline   string
	Vias       []string
	UpdatedAt  time.Time
}

var (
	ndrRe = regexp.MustCompile(`(?i)\bNDR\b|undeliverable|non-?delivery|could not be delivered|delivery (failure|failed)`)
	// "traceroute" is 3:712/848's spelling ("ping/traceroute report",
	// From "mailer-daemon", sent for a ping merely passing through it).
	traceRe = regexp.MustCompile(`(?i)\btrace\b|trace-?route|in[ -]transit|pass(ed|ing) through|transit`)
)

// Match finds the ping a reply answers, or nil.
//
// Evidence in order of strength: the REPLY kludge naming our MSGID; the
// subject token; our MSGID quoted anywhere in the body; and, for a reply
// coming from the pinged node itself, the only open ping to that node.
// Flag order or wording is never consulted here -- that is Classify's job.
func Match(r Reply, open []Ping) *Ping {
	if rid := normalizeMSGID(r.ReplyID); rid != "" {
		for i := range open {
			if normalizeMSGID(open[i].MSGID) == rid {
				return &open[i]
			}
		}
	}
	haystack := strings.ToLower(r.Subject + "\n" + r.Body)
	for i := range open {
		if open[i].Token != "" && strings.Contains(haystack, strings.ToLower(open[i].Token)) {
			return &open[i]
		}
	}
	for i := range open {
		if m := strings.ToLower(open[i].MSGID); m != "" && strings.Contains(haystack, m) {
			return &open[i]
		}
	}
	// Last resort: the sender is the pinged node itself. With several
	// pings to it in the window (mode "both", or a re-ping while an
	// earlier one is still open) the one still waiting for an answer is
	// the most plausible target, and failing that the newest.
	from := Node3D(r.FromAddr)
	var best *Ping
	for i := range open {
		p := &open[i]
		if p.Address != from {
			continue
		}
		switch {
		case best == nil:
			best = p
		case best.Status == StatusPong && p.Status != StatusPong:
			best = p
		case (best.Status == StatusPong) == (p.Status == StatusPong) && p.SentTime.After(best.SentTime):
			best = p
		}
	}
	return best
}

// Classify decides what a matched reply is. p is the ping Match returned
// (nil for an unmatched reply). outPath is the path quoted in the reply.
// sameSystem says whether two addresses are one system -- the same sysop
// on the nodelist -- which is what tells the destination answering from
// another AKA apart from a robot on the way; nil means "cannot tell",
// which counts as "not the same".
func Classify(r Reply, p *Ping, outPath []Hop, sameSystem func(from, target string) bool) string {
	text := r.FromName + "\n" + r.Subject
	if ndrRe.MatchString(text) {
		return KindNDR
	}
	if p == nil {
		return KindUnmatched
	}
	from := Node3D(r.FromAddr)
	if from == p.Address {
		return KindPong
	}
	if traceRe.MatchString(text) {
		return KindTrace
	}
	// Under DIR the message was dialed straight at the node and nothing
	// can answer it in transit: an unexpected sender is the destination
	// answering from an AKA, whatever the path says.
	if p.Mode == ModeDirect {
		return KindPong
	}
	if sameSystem != nil && sameSystem(from, p.Address) {
		return KindPong
	}
	// Any other sender is a system the mail passed through, whatever its
	// wording says. Wording used to decide here, until 2:5080/102's
	// pong.pl -- which answers every netmail to PING that it routes,
	// "PONG: PING" in the subject -- was credited as the answer to a ping
	// for 3:770/1 three minutes after it left, with New Zealand never
	// heard from. A transit robot's answer is evidence that the ping
	// crossed that node and says nothing about the destination, so it is
	// a transit notice. The price: a robot answering from an AKA the
	// nodelist does not tie to the target reads as transit and its ping
	// times out. That error sits next to its evidence on the node page; a
	// three-minute round trip to zone 3 does not.
	//
	// Position in the path decides nothing: robots differ on whether they
	// quote the chain as of arrival or after adding their own stamp, and
	// pong.pl quoted a chain it was not in at all.
	return KindTrace
}

// OriginAddress is the address that authored a MSGID ("2:5001/100@fidonet
// 6a99d1e1" -> "2:5001/100"), i.e. us for our own pings. A quoted path
// holds a Via line only for systems that stamped one, and a sending
// system normally does not stamp its own, so the first hop of a path is
// a transit node as often as it is the origin. The MSGID is what names
// the origin; the path must not be read positionally.
func OriginAddress(msgid string) string {
	f := strings.Fields(msgid)
	if len(f) == 0 {
		return ""
	}
	addr := f[0]
	if i := strings.IndexByte(addr, '@'); i >= 0 {
		addr = addr[:i]
	}
	return Node3D(addr)
}

// normalizeMSGID folds a MSGID for comparison: whitespace collapsed,
// case-insensitive (some tossers upper-case the serial).
func normalizeMSGID(s string) string {
	return strings.ToLower(spaceRe.ReplaceAllString(strings.TrimSpace(s), " "))
}

// Candidate is a node on the current nodelist that flies PING and/or TRACE.
type Candidate struct {
	Domain     string
	Zone       int
	Net        int
	Node       int
	Address    string
	SystemName string
	SysopName  string
	HasPing    bool
	HasTrace   bool
	HasIBN     bool
}

// DueKey identifies one (node, mode) series for scheduling.
func DueKey(address, domain, mode string) string {
	return address + "@" + domain + "|" + mode
}
