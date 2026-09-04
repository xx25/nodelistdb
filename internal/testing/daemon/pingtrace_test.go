package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nodelistdb/internal/pingtrace"
)

// fakePingStore is an in-memory ping_tests / ping_replies.
type fakePingStore struct {
	candidates []pingtrace.Candidate
	pings      map[string]pingtrace.Ping // key: address@domain|mode|sent
	replies    map[uint64]pingtrace.Reply
	replyHops  map[uint64][]pingtrace.Hop
}

func newFakePingStore() *fakePingStore {
	return &fakePingStore{pings: map[string]pingtrace.Ping{}, replies: map[uint64]pingtrace.Reply{}, replyHops: map[uint64][]pingtrace.Hop{}}
}

func pingKey(p pingtrace.Ping) string {
	return pingtrace.DueKey(p.Address, p.Domain, p.Mode) + "|" + p.SentTime.UTC().Format(time.RFC3339)
}

func (f *fakePingStore) GetPingCandidates(context.Context, []string) ([]pingtrace.Candidate, error) {
	return f.candidates, nil
}

func (f *fakePingStore) GetLatestPingTimes(context.Context) (map[string]time.Time, error) {
	out := map[string]time.Time{}
	for _, p := range f.pings {
		k := pingtrace.DueKey(p.Address, p.Domain, p.Mode)
		if p.SentTime.After(out[k]) {
			out[k] = p.SentTime
		}
	}
	return out, nil
}

func (f *fakePingStore) GetRecentPings(_ context.Context, since time.Time) ([]pingtrace.Ping, error) {
	var out []pingtrace.Ping
	for _, p := range f.pings {
		if !p.SentTime.Before(since) {
			out = append(out, p)
		}
	}
	return out, nil
}

func (f *fakePingStore) StorePing(_ context.Context, p pingtrace.Ping) error {
	f.pings[pingKey(p)] = p
	return nil
}

func (f *fakePingStore) StorePingReply(_ context.Context, r pingtrace.Reply, hops []pingtrace.Hop) error {
	f.replies[r.FidomailMessageID] = r
	f.replyHops[r.FidomailMessageID] = hops
	return nil
}

func (f *fakePingStore) GetKnownReplyIDs(context.Context, time.Time) (map[uint64]bool, error) {
	out := map[uint64]bool{}
	for id := range f.replies {
		out[id] = true
	}
	return out, nil
}

func (f *fakePingStore) ping(t *testing.T, address, mode string) pingtrace.Ping {
	t.Helper()
	for _, p := range f.pings {
		if p.Address == address && p.Mode == mode {
			return p
		}
	}
	t.Fatalf("no ping to %s (%s) stored", address, mode)
	return pingtrace.Ping{}
}

// fakeMailer records sends and serves a scripted inbox + status table.
type fakeMailer struct {
	sendErr  error
	sent     []SendNetmailRequest
	nextID   uint64
	inbox    []InboxItem
	statuses map[uint64]NetmailStatus
	local    bool
}

func (m *fakeMailer) SendNetmail(_ context.Context, req SendNetmailRequest) (SendNetmailResponse, error) {
	if m.sendErr != nil {
		return SendNetmailResponse{}, m.sendErr
	}
	m.sent = append(m.sent, req)
	m.nextID++
	resp := SendNetmailResponse{MessageID: m.nextID, MSGID: fmt.Sprintf("2:5001/100 %08x", m.nextID),
		Disposition: "queued", RouteVia: "2:5020/715@fidonet", RouteSource: "default-route"}
	if m.local {
		resp.Disposition = "local"
	}
	return resp, nil
}

func (m *fakeMailer) NetmailStatus(_ context.Context, id uint64) (NetmailStatus, error) {
	st, ok := m.statuses[id]
	if !ok {
		return NetmailStatus{}, &APIError{Status: 404, Code: "not_found"}
	}
	return st, nil
}

func (m *fakeMailer) Inbox(_ context.Context, _, toName string, minID uint64, _ time.Time, limit int) (InboxPage, error) {
	var page InboxPage
	for _, it := range m.inbox {
		if it.ID < minID || !strings.EqualFold(it.ToName, toName) {
			continue
		}
		page.Items = append(page.Items, it)
		if it.ID > page.MaxID {
			page.MaxID = it.ID
		}
		if len(page.Items) >= limit {
			break
		}
	}
	return page, nil
}

func testTracer(store *fakePingStore, mailer *fakeMailer, now time.Time) *PingTracer {
	cfg := PingTraceConfig{
		FromName: "NodelistDB", Networks: []string{"fidonet"},
		Interval: 14 * 24 * time.Hour, ReplyTimeout: 7 * 24 * time.Hour,
		PollInterval: 10 * time.Minute, MaxPerPoll: 2, Mode: "routed",
		ResultsURL: "https://example.test/pingtrace",
	}
	t := NewPingTracer(cfg, store, mailer, false)
	t.now = func() time.Time { return now }
	return t
}

func TestSendDueRespectsFlagsIntervalAndCap(t *testing.T) {
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	store := newFakePingStore()
	store.candidates = []pingtrace.Candidate{
		{Domain: "fidonet", Zone: 2, Net: 280, Node: 5555, Address: "2:280/5555", HasPing: true, HasTrace: true, HasIBN: true},
		{Domain: "fidonet", Zone: 1, Net: 1, Node: 19, Address: "1:1/19", HasPing: true},
		{Domain: "fidonet", Zone: 2, Net: 221, Node: 0, Address: "2:221/0", HasTrace: true}, // TRACE only: never pinged
		{Domain: "fidonet", Zone: 2, Net: 5020, Node: 1, Address: "2:5020/1", HasPing: true, HasIBN: true},
	}
	// 2:5020/1 was pinged a day ago: not due. 1:1/19 was pinged 20 days ago: due, but after never-pinged nodes.
	store.pings["a"] = pingtrace.Ping{Domain: "fidonet", Address: "2:5020/1", Mode: "routed", SentTime: now.Add(-24 * time.Hour), Status: "pong"}
	store.pings["b"] = pingtrace.Ping{Domain: "fidonet", Address: "1:1/19", Mode: "routed", SentTime: now.Add(-20 * 24 * time.Hour), Status: "timeout"}
	mailer := &fakeMailer{}
	tr := testTracer(store, mailer, now)

	if err := tr.sendDue(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(mailer.sent) != 2 {
		t.Fatalf("cap of 2 per pass, sent %d: %+v", len(mailer.sent), mailer.sent)
	}
	if mailer.sent[0].ToAddr != "2:280/5555@fidonet" {
		t.Errorf("never-pinged node goes first, got %s", mailer.sent[0].ToAddr)
	}
	if mailer.sent[1].ToAddr != "1:1/19@fidonet" {
		t.Errorf("stalest node next, got %s", mailer.sent[1].ToAddr)
	}
	req := mailer.sent[0]
	if req.ToName != "PING" || req.FromName != "NodelistDB" || req.Direct {
		t.Errorf("bad request shape: %+v", req)
	}
	if req.Subject != "PING" {
		t.Errorf("subject must be exactly PING for the sloppiest robot, got %q", req.Subject)
	}
	p := store.ping(t, "2:280/5555", "routed")
	if !strings.Contains(req.Body, "https://example.test/pingtrace") || !strings.Contains(req.Body, "Ref:     "+p.Token) {
		t.Errorf("body must name the results page and the token: %q", req.Body)
	}
	if p.Status != pingtrace.StatusQueued || p.FirstHop != "2:5020/715@fidonet" || p.FidomailMessageID != 1 || p.Token == "" {
		t.Errorf("queued row not recorded properly: %+v", p)
	}

	// A second pass in the same instant sends the remaining due node only.
	if err := tr.sendDue(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(mailer.sent) != 2 {
		t.Fatalf("nothing else is due, but %d sent", len(mailer.sent))
	}
}

func TestSendDueBothModeSkipsDirectWithoutIBN(t *testing.T) {
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	store := newFakePingStore()
	store.candidates = []pingtrace.Candidate{
		{Domain: "fidonet", Zone: 1, Net: 1, Node: 19, Address: "1:1/19", HasPing: true},
		{Domain: "fidonet", Zone: 2, Net: 280, Node: 5555, Address: "2:280/5555", HasPing: true, HasIBN: true},
	}
	mailer := &fakeMailer{}
	tr := testTracer(store, mailer, now)
	tr.cfg.Mode = "both"
	tr.cfg.MaxPerPoll = 10
	if err := tr.sendDue(context.Background()); err != nil {
		t.Fatal(err)
	}
	var direct, routed int
	for _, s := range mailer.sent {
		if s.Direct {
			direct++
			if s.ToAddr != "2:280/5555@fidonet" {
				t.Errorf("DIR ping to a node without IBN: %s", s.ToAddr)
			}
		} else {
			routed++
		}
	}
	if routed != 2 || direct != 1 {
		t.Errorf("want 2 routed + 1 direct, got %d + %d", routed, direct)
	}
}

func TestPollRepliesFoldsPongTraceAndNDR(t *testing.T) {
	now := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	sent := now.Add(-26 * time.Hour)
	store := newFakePingStore()
	target := pingtrace.Ping{Domain: "fidonet", Zone: 2, Net: 280, Node: 5555, Address: "2:280/5555", Mode: "routed",
		SentTime: sent, Token: "a1b2c3d4", MSGID: "2:5001/100 68b8a1c2", FidomailMessageID: 7, Status: pingtrace.StatusSent}
	other := pingtrace.Ping{Domain: "fidonet", Zone: 1, Net: 1, Node: 19, Address: "1:1/19", Mode: "routed",
		SentTime: sent, Token: "e5f6a7b8", MSGID: "2:5001/100 68b8a1c3", FidomailMessageID: 8, Status: pingtrace.StatusSent}
	store.StorePing(context.Background(), target)
	store.StorePing(context.Background(), other)

	pongBody := "Your PING message arrived at its destination 2:280/5555.\r\n\r\nIt travelled via:\r\n" +
		"@Via 2:5001/100@fidonet @20260904.100000.UTC FidoMail 0.1\r\n" +
		"@Via 2:5020/715 @20260904.103000 hpt/lnx 1.9.0\r\n" +
		"@Via 2:280/5555 @20260905.090000.UTC FMail 2.3\r\n"
	mailer := &fakeMailer{inbox: []InboxItem{
		{ID: 101, ToName: "NodelistDB", FromName: "Trace Robot", FromAddr: "2:5020/715", Subject: "Trace: your message to PING at 2:280/5555",
			Body: "  MSGID: 2:5001/100 68b8a1c2\r\n", ReceivedAt: sent.Add(time.Hour)},
		{ID: 102, ToName: "NodelistDB", FromName: "Ping Robot", FromAddr: "2:280/5555", Subject: "Pong: PING 2:280/5555 a1b2c3d4",
			ReplyID: "2:5001/100 68b8a1c2", Body: pongBody, PID: "FMail 2.3", Tearline: "--- FMail 2.3",
			Vias:       []string{"2:280/5555 @20260905.090100.UTC FMail 2.3", "2:5020/715 @20260905.093000 hpt/lnx 1.9.0"},
			ReceivedAt: sent.Add(25 * time.Hour)},
		{ID: 103, ToName: "NodelistDB", FromName: "NDR Robot", FromAddr: "2:5001/100", Subject: "Undeliverable: PING 1:1/19 e5f6a7b8",
			Body: "could not be delivered", ReceivedAt: sent.Add(2 * time.Hour)},
		{ID: 104, ToName: "NodelistDB", FromName: "Someone", FromAddr: "2:9999/1", Subject: "hi", Body: "unrelated", ReceivedAt: now},
		{ID: 105, ToName: "Sysop", FromName: "Someone", FromAddr: "2:9999/1", Subject: "not ours", Body: "", ReceivedAt: now},
	}}
	tr := testTracer(store, mailer, now)

	recent, _ := store.GetRecentPings(context.Background(), tr.lookback())
	if err := tr.pollReplies(context.Background(), recent); err != nil {
		t.Fatal(err)
	}

	p := store.ping(t, "2:280/5555", "routed")
	if p.Status != pingtrace.StatusPong {
		t.Fatalf("status = %s, want pong (%+v)", p.Status, p)
	}
	if p.RTTSeconds != 25*3600 {
		t.Errorf("rtt = %d, want 25h", p.RTTSeconds)
	}
	if p.TraceCount != 1 {
		t.Errorf("trace_count = %d, want 1", p.TraceCount)
	}
	if got := pingtrace.Addresses(p.OutHops); strings.Join(got, ",") != "2:5001/100,2:5020/715,2:280/5555" {
		t.Errorf("out hops = %v", got)
	}
	if got := pingtrace.Addresses(p.BackHops); strings.Join(got, ",") != "2:280/5555,2:5020/715" {
		t.Errorf("back hops = %v", got)
	}
	if p.RobotPID != "FMail 2.3" || p.ReplyMessageID != 102 || p.ReplyFromAddr != "2:280/5555" {
		t.Errorf("robot fields: %+v", p)
	}

	o := store.ping(t, "1:1/19", "routed")
	if o.Status != pingtrace.StatusNDR || !strings.Contains(o.Error, "Undeliverable") {
		t.Errorf("NDR must mark the ping: %+v", o)
	}

	if len(store.replies) != 4 {
		t.Fatalf("every reply to our name is kept, got %d", len(store.replies))
	}
	if store.replies[104].Kind != pingtrace.KindUnmatched {
		t.Errorf("stranger's mail is unmatched, got %s", store.replies[104].Kind)
	}
	if store.replies[101].Kind != pingtrace.KindTrace || store.replies[101].PingMSGID != target.MSGID {
		t.Errorf("trace reply not linked: %+v", store.replies[101])
	}
	if tr.watermark != 104 {
		t.Errorf("watermark = %d, want 104", tr.watermark)
	}

	// A second pass re-reads nothing and changes nothing.
	pongBefore := store.ping(t, "2:280/5555", "routed")
	recent, _ = store.GetRecentPings(context.Background(), tr.lookback())
	if err := tr.pollReplies(context.Background(), recent); err != nil {
		t.Fatal(err)
	}
	if after := store.ping(t, "2:280/5555", "routed"); after.TraceCount != pongBefore.TraceCount || after.ReplyMessageID != pongBefore.ReplyMessageID {
		t.Errorf("second pass must be idempotent: %+v vs %+v", after, pongBefore)
	}
}

func TestRefreshDispatchAndExpire(t *testing.T) {
	now := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	store := newFakePingStore()
	old := now.Add(-8 * 24 * time.Hour)
	fresh := now.Add(-time.Hour)
	store.StorePing(context.Background(), pingtrace.Ping{Domain: "fidonet", Address: "1:1/19", Mode: "routed", SentTime: old, FidomailMessageID: 1, Status: pingtrace.StatusSent})
	store.StorePing(context.Background(), pingtrace.Ping{Domain: "fidonet", Address: "2:2/0", Mode: "routed", SentTime: old, FidomailMessageID: 2, Status: pingtrace.StatusQueued})
	store.StorePing(context.Background(), pingtrace.Ping{Domain: "fidonet", Address: "2:280/5555", Mode: "routed", SentTime: fresh, FidomailMessageID: 3, Status: pingtrace.StatusQueued})
	store.StorePing(context.Background(), pingtrace.Ping{Domain: "fidonet", Address: "2:221/1", Mode: "routed", SentTime: fresh, FidomailMessageID: 4, Status: pingtrace.StatusQueued})
	mailer := &fakeMailer{statuses: map[uint64]NetmailStatus{
		2: {ID: 2, Status: "queued"},
		3: {ID: 3, Status: "sent", UpdatedAt: now.Add(-30 * time.Minute)},
		4: {ID: 4, Status: "failed"},
	}}
	tr := testTracer(store, mailer, now)

	recent, _ := store.GetRecentPings(context.Background(), tr.lookback())
	if err := tr.refreshDispatch(context.Background(), recent); err != nil {
		t.Fatal(err)
	}
	if p := store.ping(t, "2:280/5555", "routed"); p.Status != pingtrace.StatusSent || !p.DispatchedTime.Equal(now.Add(-30*time.Minute)) {
		t.Errorf("sent state not recorded: %+v", p)
	}
	if p := store.ping(t, "2:221/1", "routed"); p.Status != pingtrace.StatusFailed {
		t.Errorf("failed state not recorded: %+v", p)
	}
	if err := tr.expire(context.Background(), recent); err != nil {
		t.Fatal(err)
	}
	if p := store.ping(t, "1:1/19", "routed"); p.Status != pingtrace.StatusTimeout {
		t.Errorf("old sent ping must time out: %+v", p)
	}
	if p := store.ping(t, "2:2/0", "routed"); p.Status != pingtrace.StatusTimeout || !strings.Contains(p.Error, "never handed") {
		t.Errorf("old queued ping must time out with its own reason: %+v", p)
	}
	if p := store.ping(t, "2:280/5555", "routed"); p.Status != pingtrace.StatusSent {
		t.Errorf("fresh ping must keep waiting: %+v", p)
	}
}

func TestPingNodeLocalAKAIsFailedNotQueued(t *testing.T) {
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	store := newFakePingStore()
	mailer := &fakeMailer{local: true}
	tr := testTracer(store, mailer, now)
	p, err := tr.PingNode(context.Background(), "2:5001/5001", "")
	if err != nil {
		t.Fatal(err)
	}
	if p.Status != pingtrace.StatusFailed || p.Address != "2:5001/5001" || p.Domain != "fidonet" {
		t.Errorf("%+v", p)
	}
}

func TestFidomailClientRoundTrip(t *testing.T) {
	var gotAuth, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/netmail":
			var req SendNetmailRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ToName != "PING" {
				w.WriteHeader(400)
				return
			}
			w.WriteHeader(http.StatusAccepted)
			json.NewEncoder(w).Encode(SendNetmailResponse{MessageID: 42, MSGID: "2:5001/100 abc", Disposition: "queued", RouteVia: "2:5020/715@fidonet"})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/netmail/inbox":
			gotQuery = r.URL.RawQuery
			json.NewEncoder(w).Encode(InboxPage{Items: []InboxItem{{ID: 5, FromAddr: "2:280/5555"}}, MaxID: 5})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/netmail/42":
			json.NewEncoder(w).Encode(NetmailStatus{ID: 42, Status: "sent"})
		default:
			w.WriteHeader(404)
			w.Write([]byte(`{"error":{"code":"not_found","message":"no such endpoint"}}`))
		}
	}))
	defer srv.Close()

	c := NewFidomailClient(srv.URL+"/", "secret", time.Second)
	ctx := context.Background()
	resp, err := c.SendNetmail(ctx, SendNetmailRequest{ToAddr: "2:280/5555@fidonet", ToName: "PING", Subject: "x", Body: "y"})
	if err != nil || resp.MessageID != 42 || resp.RouteVia != "2:5020/715@fidonet" {
		t.Fatalf("send: %v %+v", err, resp)
	}
	if gotAuth != "Bearer secret" {
		t.Errorf("auth header = %q", gotAuth)
	}
	page, err := c.Inbox(ctx, "fidonet", "NodelistDB", 3, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), 50)
	if err != nil || len(page.Items) != 1 || page.MaxID != 5 {
		t.Fatalf("inbox: %v %+v", err, page)
	}
	for _, want := range []string{"to_name=NodelistDB", "min_id=3", "since=2026-09-01T00%3A00%3A00Z", "limit=50", "network=fidonet"} {
		if !strings.Contains(gotQuery, want) {
			t.Errorf("query %q lacks %s", gotQuery, want)
		}
	}
	st, err := c.NetmailStatus(ctx, 42)
	if err != nil || st.Status != "sent" {
		t.Fatalf("status: %v %+v", err, st)
	}
	_, err = c.NetmailStatus(ctx, 43)
	if !IsNotFound(err) {
		t.Errorf("404 must surface as not-found, got %v", err)
	}
}

func TestSendPingRecordsTheRowBeforeAndAfterTheSend(t *testing.T) {
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	store := newFakePingStore()
	c := pingtrace.Candidate{Domain: "fidonet", Zone: 2, Net: 280, Node: 5555, Address: "2:280/5555", HasPing: true}
	mailer := &fakeMailer{sendErr: context.DeadlineExceeded}
	tr := testTracer(store, mailer, now)
	if _, err := tr.sendPing(context.Background(), c, "routed"); err == nil {
		t.Fatal("send error must surface")
	}
	if p := store.ping(t, "2:280/5555", "routed"); p.Status != pingtrace.StatusFailed || p.Token == "" {
		t.Errorf("a failed send must leave a failed row so the node is not re-pinged at once: %+v", p)
	}
}

func TestSendDueHonoursNodeAllowlist(t *testing.T) {
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	store := newFakePingStore()
	store.candidates = []pingtrace.Candidate{
		{Domain: "fidonet", Zone: 2, Net: 280, Node: 5555, Address: "2:280/5555", HasPing: true},
		{Domain: "fidonet", Zone: 1, Net: 1, Node: 19, Address: "1:1/19", HasPing: true},
		{Domain: "fidonet", Zone: 2, Net: 221, Node: 1, Address: "2:221/1", HasPing: true},
	}
	mailer := &fakeMailer{}
	tr := testTracer(store, mailer, now)
	tr.cfg.MaxPerPoll = 10
	tr.cfg.Nodes = []string{"2:280/5555@fidonet", " 1:1/19 "}
	if err := tr.sendDue(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(mailer.sent) != 2 {
		t.Fatalf("allowlist must narrow the due list to 2, sent %d", len(mailer.sent))
	}
	for _, s := range mailer.sent {
		if s.ToAddr == "2:221/1@fidonet" {
			t.Error("node outside the allowlist was pinged")
		}
	}
}
