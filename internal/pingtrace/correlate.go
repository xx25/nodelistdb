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

	// InboundTier and InboundAuth record HOW this reply reached us, not
	// what it says. On an unauthenticated session (fidomail tier B or C)
	// the From address is an unverified claim -- worth keeping beside the
	// evidence, because the whole output of this measurement is "node X
	// answered". Tier A is no proof of the ORIGINATOR either: it says a
	// contracted link relayed the mail to us.
	//
	// InboundAuth is fidomail's verdict copied verbatim ("secure" /
	// "unsecure"), empty when it did not report one, so the rule for which
	// tiers count as authenticated is never restated here and cannot drift
	// from fidomail's own Unsecure view.
	InboundTier uint8
	InboundAuth string
}

// Inbound auth verdicts, as stored in ping_replies.inbound_auth.
const (
	AuthUnreported = ""
	AuthSecure     = "secure"
	AuthUnsecure   = "unsecure"
)

// inboundTierA is fidomail's tier for a configured link that authenticated
// with a session password -- the only receipt that positively evidences an
// authenticated wire session. Tier 0 is "no session was recorded", which
// is not the same thing and is not treated as one.
const inboundTierA = 1

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
// coming from the pinged node itself over a receipt that evidences an
// authenticated session, the only open ping to that node.
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
	//
	// This branch rests on NOTHING but the From line, so it takes POSITIVE
	// evidence of an authenticated session, not merely the absence of a
	// warning. A ping is open against most monitored nodes for most of
	// every cycle and nothing above needs to be known to forge one, so a
	// stranger could otherwise have any node recorded as "answered"
	// without ever seeing its ping.
	//
	// Tier A -- a configured link that passed its session password -- is
	// the only receipt that evidences one. Tier B and C are strangers.
	// Tier 0 means fidomail recorded NO session at all, which for mail
	// claiming to come from another node should not happen: it is what
	// this node's own robot mail carries, and our own mail can never be
	// the answer to our own ping. Trusting it would also mean that any
	// future receive path that skipped classification silently reopened
	// this hole -- which has happened before, in the quarantine retoss.
	//
	// An unreported receipt still passes: a fidomail predating the field
	// says nothing about the session, and refusing it there would drop
	// real answers (3 of the first 57 matched on this branch alone --
	// 4:80/1 and 2:240/1120 quote neither our token nor a REPLY kludge)
	// and publish them as timeouts.
	//
	// The evidence branches above stay ungated on purpose: quoting our
	// token or MSGID back proves the sender saw the ping, which is why
	// the real unauthenticated answers (2:221/0 and 2:221/1 deposit
	// directly, and quote the token) still match.
	//
	// A refused reply is not discarded: it is stored unmatched, with its
	// sender and body, so a claim we would not credit is still visible.
	if r.InboundAuth != AuthUnreported && r.InboundTier != inboundTierA {
		return nil
	}
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
