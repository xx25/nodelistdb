package daemon

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/nodelistdb/internal/logging"
	"github.com/nodelistdb/internal/pingtrace"
)

// pingStore is the slice of storage the tracer needs.
type pingStore interface {
	GetPingCandidates(ctx context.Context, domains []string) ([]pingtrace.Candidate, error)
	GetLatestPingTimes(ctx context.Context) (map[string]time.Time, error)
	GetRecentPings(ctx context.Context, since time.Time) ([]pingtrace.Ping, error)
	StorePing(ctx context.Context, p pingtrace.Ping) error
	StorePingReply(ctx context.Context, r pingtrace.Reply, hops []pingtrace.Hop) error
	GetKnownReplyIDs(ctx context.Context, since time.Time) (map[uint64]bool, error)
	// SameSystem says whether two addresses on domain's latest nodelist
	// belong to one sysop, i.e. are AKAs of one system.
	SameSystem(ctx context.Context, domain, a, b string) (bool, error)
}

// pingMailer is the slice of the fidomail control API the tracer needs.
type pingMailer interface {
	SendNetmail(ctx context.Context, req SendNetmailRequest) (SendNetmailResponse, error)
	NetmailStatus(ctx context.Context, id uint64) (NetmailStatus, error)
	Inbox(ctx context.Context, network, toName string, minID uint64, since time.Time, limit int) (InboxPage, error)
}

// PingTracer runs the FTS-4010 PING/TRACE measurement.
//
// It is a separate loop beside the connectivity test cycle because a
// netmail answer arrives hours or days later: every pass reads the
// replies that came in, advances the delivery state of what was sent,
// times out what has waited too long, and sends the pings that are due.
// All state lives in ClickHouse (ping_tests / ping_replies); the only
// thing kept in memory is the inbox id watermark, and losing it costs a
// re-read that the stored reply ids make idempotent.
type PingTracer struct {
	cfg    PingTraceConfig
	store  pingStore
	mailer pingMailer
	dryRun bool
	now    func() time.Time

	mu        sync.Mutex
	watermark uint64 // highest inbox id consumed

	stopOnce sync.Once
	done     chan struct{}
}

// NewPingTracer builds a tracer from config.
func NewPingTracer(cfg PingTraceConfig, store pingStore, mailer pingMailer, dryRun bool) *PingTracer {
	return &PingTracer{
		cfg:    cfg,
		store:  store,
		mailer: mailer,
		dryRun: dryRun,
		now:    time.Now,
		done:   make(chan struct{}),
	}
}

// Start runs a pass shortly after startup and then every poll interval.
func (t *PingTracer) Start(ctx context.Context) {
	go func() {
		select {
		case <-time.After(45 * time.Second):
		case <-ctx.Done():
			return
		case <-t.done:
			return
		}
		if err := t.RunOnce(ctx); err != nil {
			logging.Errorf("PING/TRACE pass failed: %v", err)
		}
		ticker := time.NewTicker(t.cfg.PollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.done:
				return
			case <-ticker.C:
				if err := t.RunOnce(ctx); err != nil {
					logging.Errorf("PING/TRACE pass failed: %v", err)
				}
			}
		}
	}()
}

// Stop ends the loop.
func (t *PingTracer) Stop() {
	t.stopOnce.Do(func() { close(t.done) })
}

// RunOnce is one full pass: replies, dispatch state, timeouts, due pings.
// Replies are read before timeouts are applied so an answer that arrived
// late still wins over the timeout it would otherwise have earned.
func (t *PingTracer) RunOnce(ctx context.Context) error {
	var firstErr error
	note := func(err error) {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	recent, err := t.store.GetRecentPings(ctx, t.lookback())
	if err != nil {
		return fmt.Errorf("pingtrace: read recent pings: %w", err)
	}
	note(t.pollReplies(ctx, recent))
	note(t.refreshDispatch(ctx, recent))
	note(t.expire(ctx, recent))
	note(t.sendDue(ctx))
	return firstErr
}

// lookback is how far back pings stay eligible for matching: one full
// interval plus the reply window, so a robot answering after the next
// ping to the same node has already gone out still finds its own ping.
func (t *PingTracer) lookback() time.Time {
	return t.now().Add(-(t.cfg.Interval + t.cfg.ReplyTimeout))
}

// pollReplies reads new inbox items and folds each into the ping it
// answers. recent is updated in place so a later reply in the same page
// sees the earlier one's effect.
func (t *PingTracer) pollReplies(ctx context.Context, recent []pingtrace.Ping) error {
	known, err := t.store.GetKnownReplyIDs(ctx, t.lookback())
	if err != nil {
		return fmt.Errorf("pingtrace: read known replies: %w", err)
	}
	t.mu.Lock()
	minID := t.watermark
	t.mu.Unlock()
	if minID > 0 {
		minID++
	}
	const pageSize = 200
	for {
		page, err := t.mailer.Inbox(ctx, t.cfg.Networks[0], t.cfg.FromName, minID, t.lookback(), pageSize)
		if err != nil {
			return fmt.Errorf("pingtrace: read inbox: %w", err)
		}
		for _, item := range page.Items {
			if item.ID > minID {
				minID = item.ID
			}
			if known[item.ID] {
				continue
			}
			if err := t.absorbReply(ctx, item, recent); err != nil {
				logging.Errorf("PING/TRACE: reply %d from %s not recorded: %v", item.ID, item.FromAddr, err)
				continue
			}
			known[item.ID] = true
		}
		if page.MaxID > 0 {
			t.mu.Lock()
			if page.MaxID > t.watermark {
				t.watermark = page.MaxID
			}
			t.mu.Unlock()
		}
		if len(page.Items) < pageSize {
			return nil
		}
		minID++
	}
}

// absorbReply classifies one inbound message, stores it, and updates the
// ping it answers.
func (t *PingTracer) absorbReply(ctx context.Context, item InboxItem, recent []pingtrace.Ping) error {
	reply := pingtrace.Reply{
		FidomailMessageID: item.ID,
		MSGID:             item.MSGID,
		ReplyID:           item.ReplyID,
		FromName:          item.FromName,
		FromAddr:          item.FromAddr,
		ToName:            item.ToName,
		Subject:           item.Subject,
		Body:              item.Body,
		Date:              item.Date,
		ReceivedAt:        item.ReceivedAt,
		PID:               item.PID,
		Tearline:          item.Tearline,
		Vias:              item.Vias,
		UpdatedAt:         t.now(),
	}
	if reply.ReceivedAt.IsZero() {
		reply.ReceivedAt = t.now()
	}
	quoted := pingtrace.ExtractPath(item.Body)
	p := pingtrace.Match(reply, recent)
	// Only consulted for a matched ping answered from another address,
	// so p is set whenever this runs.
	sameSystem := func(from, target string) bool {
		ok, err := t.store.SameSystem(ctx, p.Domain, from, target)
		if err != nil {
			logging.Errorf("PING/TRACE: cannot tell whether %s and %s are one system, treating reply %d as transit: %v", from, target, item.ID, err)
			return false
		}
		return ok
	}
	reply.Kind = pingtrace.Classify(reply, p, quoted, sameSystem)
	if p != nil {
		reply.PingDomain, reply.PingZone, reply.PingNet, reply.PingNode = p.Domain, p.Zone, p.Net, p.Node
		reply.PingSentTime, reply.PingMSGID = p.SentTime, p.MSGID
	}
	if t.dryRun {
		logging.Infof("PING/TRACE (dry-run): would record %s reply %d from %s (%s)", reply.Kind, item.ID, item.FromAddr, item.Subject)
		return nil
	}
	if p == nil {
		logging.Infof("PING/TRACE: unmatched reply %d from %s %q", item.ID, item.FromAddr, item.Subject)
		return t.store.StorePingReply(ctx, reply, quoted)
	}
	// Order of the two writes: the ping's verdict first, the evidence row
	// second. Dedup keys off the evidence row, so a crash between them
	// re-processes the reply next pass, which is harmless for a pong (the
	// verdict is already set) and at worst counts a trace notice twice.
	// The other order would leave a real answer recorded as evidence while
	// its ping silently timed out, and never look at it again.

	changed := false
	switch reply.Kind {
	case pingtrace.KindPong:
		if p.Status != pingtrace.StatusPong {
			p.Status = pingtrace.StatusPong
			p.ReplyTime = reply.ReceivedAt
			if d := p.ReplyTime.Sub(p.SentTime); d > 0 {
				p.RTTSeconds = uint32(d / time.Second)
			}
			p.ReplyMessageID = item.ID
			p.ReplyMSGID = item.MSGID
			p.ReplyFromName = item.FromName
			p.ReplyFromAddr = item.FromAddr
			p.RobotPID = item.PID
			p.RobotTearline = item.Tearline
			p.OutHops = quoted
			p.BackHops = pingtrace.ParseVias(item.Vias)
			p.Error = ""
			changed = true
			logging.Infof("PING/TRACE: pong from %s for %s after %s via %d hop(s)",
				item.FromAddr, p.Address, time.Duration(p.RTTSeconds)*time.Second, len(quoted))
		}
	case pingtrace.KindTrace:
		p.TraceCount++
		changed = true
		logging.Infof("PING/TRACE: trace notice from %s for ping to %s", item.FromAddr, p.Address)
	case pingtrace.KindNDR:
		if p.Status != pingtrace.StatusPong {
			p.Status = pingtrace.StatusNDR
			p.Error = strings.TrimSpace(item.Subject)
			changed = true
			logging.Infof("PING/TRACE: ping to %s bounced: %s", p.Address, p.Error)
		}
	}
	if changed {
		p.UpdatedAt = t.now()
		if err := t.store.StorePing(ctx, *p); err != nil {
			return err
		}
	}
	return t.store.StorePingReply(ctx, reply, quoted)
}

// refreshDispatch asks fidomail what became of every ping still queued.
func (t *PingTracer) refreshDispatch(ctx context.Context, recent []pingtrace.Ping) error {
	var firstErr error
	for i := range recent {
		p := &recent[i]
		if p.Status != pingtrace.StatusQueued || p.FidomailMessageID == 0 {
			continue
		}
		st, err := t.mailer.NetmailStatus(ctx, p.FidomailMessageID)
		if err != nil {
			if IsNotFound(err) {
				p.Status = pingtrace.StatusFailed
				p.Error = "message vanished from fidomail"
			} else {
				if firstErr == nil {
					firstErr = fmt.Errorf("pingtrace: status of %d: %w", p.FidomailMessageID, err)
				}
				continue
			}
		} else {
			switch strings.ToLower(st.Status) {
			case "sent", "delivered":
				p.Status = pingtrace.StatusSent
				p.DispatchedTime = st.UpdatedAt
				if p.DispatchedTime.IsZero() {
					p.DispatchedTime = t.now()
				}
			case "failed", "canceled", "cancelled", "unroutable", "killed", "rejected":
				p.Status = pingtrace.StatusFailed
				p.Error = "fidomail: " + st.Status
			default:
				continue
			}
		}
		if t.dryRun {
			continue
		}
		p.UpdatedAt = t.now()
		if err := t.store.StorePing(ctx, *p); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// expire marks pings that have waited longer than the reply window.
func (t *PingTracer) expire(ctx context.Context, recent []pingtrace.Ping) error {
	cutoff := t.now().Add(-t.cfg.ReplyTimeout)
	var firstErr error
	for i := range recent {
		p := &recent[i]
		if p.SentTime.After(cutoff) {
			continue
		}
		switch p.Status {
		case pingtrace.StatusQueued:
			p.Error = "never handed to the first hop"
		case pingtrace.StatusSent:
			p.Error = fmt.Sprintf("no answer within %s", t.cfg.ReplyTimeout)
		default:
			continue
		}
		p.Status = pingtrace.StatusTimeout
		if t.dryRun {
			continue
		}
		p.UpdatedAt = t.now()
		if err := t.store.StorePing(ctx, *p); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// modes lists what one node receives per interval.
func (t *PingTracer) modes() []string {
	switch t.cfg.Mode {
	case "direct":
		return []string{pingtrace.ModeDirect}
	case "both":
		return []string{pingtrace.ModeRouted, pingtrace.ModeDirect}
	default:
		return []string{pingtrace.ModeRouted}
	}
}

type dueEntry struct {
	c    pingtrace.Candidate
	mode string
	last time.Time
}

// sendDue sends the pings whose interval has elapsed, oldest first, up to
// the per-pass cap.
func (t *PingTracer) sendDue(ctx context.Context) error {
	candidates, err := t.store.GetPingCandidates(ctx, t.cfg.Networks)
	if err != nil {
		return fmt.Errorf("pingtrace: read candidates: %w", err)
	}
	latest, err := t.store.GetLatestPingTimes(ctx)
	if err != nil {
		return fmt.Errorf("pingtrace: read latest ping times: %w", err)
	}
	now := t.now()
	allow := map[string]bool{}
	for _, a := range t.cfg.Nodes {
		allow[strings.ToLower(strings.TrimSpace(a))] = true
	}
	var due []dueEntry
	for _, c := range candidates {
		if !c.HasPing {
			continue // a TRACE-only node is evaluated from transit, never pinged
		}
		if len(allow) > 0 && !allow[c.Address] && !allow[strings.ToLower(c.Address+"@"+c.Domain)] {
			continue
		}
		for _, mode := range t.modes() {
			if mode == pingtrace.ModeDirect && !c.HasIBN {
				continue // nothing to dial; a DIR ping would only park and bounce
			}
			last := latest[pingtrace.DueKey(c.Address, c.Domain, mode)]
			if !last.IsZero() && now.Sub(last) < t.cfg.Interval {
				continue
			}
			due = append(due, dueEntry{c: c, mode: mode, last: last})
		}
	}
	if len(due) == 0 {
		return nil
	}
	sort.SliceStable(due, func(i, j int) bool {
		if due[i].last.Equal(due[j].last) {
			return due[i].c.Address < due[j].c.Address
		}
		return due[i].last.Before(due[j].last)
	})
	if len(due) > t.cfg.MaxPerPoll {
		due = due[:t.cfg.MaxPerPoll]
	}
	logging.Infof("PING/TRACE: %d ping(s) due, sending %d", len(due), len(due))
	var firstErr error
	for _, d := range due {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := t.sendPing(ctx, d.c, d.mode); err != nil {
			logging.Errorf("PING/TRACE: ping to %s (%s) not sent: %v", d.c.Address, d.mode, err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// sendPing asks fidomail to queue one PING and records the queued row.
func (t *PingTracer) sendPing(ctx context.Context, c pingtrace.Candidate, mode string) (pingtrace.Ping, error) {
	token, err := newToken()
	if err != nil {
		return pingtrace.Ping{}, err
	}
	now := t.now()
	p := pingtrace.Ping{
		Domain: c.Domain, Zone: c.Zone, Net: c.Net, Node: c.Node, Address: c.Address,
		Mode: mode, SentTime: now, Token: token, Status: pingtrace.StatusQueued, UpdatedAt: now,
	}
	req := SendNetmailRequest{
		ToAddr:   c.Address + "@" + c.Domain,
		ToName:   "PING",
		FromAddr: t.cfg.FromAddr,
		FromName: t.cfg.FromName,
		// Exactly "PING": every FTSC text keys the robot on the To name
		// alone, but a script that also compares the subject would silently
		// drop anything else. The correlation token rides in the body.
		Subject: "PING",
		Body:    t.pingBody(c, mode, now, token),
		Direct:  mode == pingtrace.ModeDirect,
	}
	if t.dryRun {
		logging.Infof("PING/TRACE (dry-run): would send %s ping to %s (%s)", mode, c.Address, c.SystemName)
		return p, nil
	}
	// Reserve the row BEFORE asking fidomail to send. A crash between the
	// send and the record would otherwise leave no trace of a netmail that
	// really left, and the next pass would ping the same sysop twice; a
	// reserved row that never gets its message id merely times out as
	// "never handed over", which fails toward too few pings, not too many.
	if err := t.store.StorePing(ctx, p); err != nil {
		return p, fmt.Errorf("pingtrace: reserve ping row: %w", err)
	}
	resp, err := t.mailer.SendNetmail(ctx, req)
	if err != nil {
		p.Status = pingtrace.StatusFailed
		p.Error = "send: " + err.Error()
		p.UpdatedAt = t.now()
		if serr := t.store.StorePing(ctx, p); serr != nil {
			logging.Errorf("PING/TRACE: could not record failed send to %s: %v", c.Address, serr)
		}
		return p, err
	}
	p.MSGID = resp.MSGID
	p.FidomailMessageID = resp.MessageID
	p.FirstHop = resp.RouteVia
	p.RouteSource = resp.RouteSource
	if resp.Disposition == "local" {
		// Our own AKA: nothing left the node, and our own robot never
		// answers our own mail.
		p.Status = pingtrace.StatusFailed
		p.Error = "destination is a local AKA"
	}
	if err := t.store.StorePing(ctx, p); err != nil {
		return p, fmt.Errorf("pingtrace: record ping %s: %w", p.MSGID, err)
	}
	logging.Infof("PING/TRACE: %s ping to %s queued as %s via %s (%s)", mode, c.Address, p.MSGID, p.FirstHop, p.RouteSource)
	return p, nil
}

func (t *PingTracer) pingBody(c pingtrace.Candidate, mode string, now time.Time, token string) string {
	var b strings.Builder
	b.WriteString("This is an automatic FTS-4010 PING from NodelistDB, sent about once every\n")
	b.WriteString("two weeks to every node that advertises the PING flag in the nodelist.\n")
	b.WriteString("Your PING robot should bounce it back quoting the Via lines; nothing else\n")
	b.WriteString("is expected of you.\n\n")
	fmt.Fprintf(&b, "Target:  %s (%s)\n", c.Address, strings.ReplaceAll(c.SystemName, "_", " "))
	fmt.Fprintf(&b, "Sent:    %s UTC\n", now.UTC().Format("2006-01-02 15:04:05"))
	fmt.Fprintf(&b, "Mode:    %s\n", mode)
	fmt.Fprintf(&b, "Ref:     %s\n", token)
	if t.cfg.ResultsURL != "" {
		fmt.Fprintf(&b, "Results: %s\n", t.cfg.ResultsURL)
	}
	return b.String()
}

// PingNode sends one ping to the given node now, regardless of schedule.
func (t *PingTracer) PingNode(ctx context.Context, address, mode string) (pingtrace.Ping, error) {
	if mode == "" {
		mode = pingtrace.ModeRouted
	}
	domain := t.cfg.Networks[0]
	if i := strings.IndexByte(address, '@'); i >= 0 {
		domain = address[i+1:]
		address = address[:i]
	}
	var zone, net, node int
	if _, err := fmt.Sscanf(address, "%d:%d/%d", &zone, &net, &node); err != nil {
		return pingtrace.Ping{}, fmt.Errorf("pingtrace: bad address %q: want zone:net/node", address)
	}
	c := pingtrace.Candidate{Domain: domain, Zone: zone, Net: net, Node: node,
		Address: fmt.Sprintf("%d:%d/%d", zone, net, node), HasPing: true, HasIBN: true}
	if candidates, err := t.store.GetPingCandidates(ctx, []string{domain}); err == nil {
		for _, k := range candidates {
			if k.Address == c.Address {
				c = k
				break
			}
		}
	}
	return t.sendPing(ctx, c, mode)
}

func newToken() (string, error) {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// PingNodeNow is the -ping-node entry point: one ping, sent immediately.
func (d *Daemon) PingNodeNow(ctx context.Context, address string, direct bool) (pingtrace.Ping, error) {
	if d.pingTracer == nil {
		return pingtrace.Ping{}, fmt.Errorf("services.pingtrace is not enabled in the config")
	}
	mode := pingtrace.ModeRouted
	if direct {
		mode = pingtrace.ModeDirect
	}
	return d.pingTracer.PingNode(ctx, address, mode)
}

// PingTracePass is the -ping-poll entry point: one full tracer pass.
func (d *Daemon) PingTracePass(ctx context.Context) error {
	if d.pingTracer == nil {
		return fmt.Errorf("services.pingtrace is not enabled in the config")
	}
	return d.pingTracer.RunOnce(ctx)
}
