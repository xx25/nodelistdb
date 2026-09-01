package ratelimit

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestClientIPIgnoresHeadersFromUntrustedPeer is the security property the
// whole limiter rests on. cmd/server's clientIP() trusts these headers
// unconditionally, which is why it is not reused here: if it were, any caller
// could send a fresh X-Real-IP per request and never share a bucket.
func TestClientIPIgnoresHeadersFromUntrustedPeer(t *testing.T) {
	r, err := NewClientIPResolver(nil) // loopback only
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/node/1/2/3", nil)
	req.RemoteAddr = "203.0.113.7:41234"
	req.Header.Set("X-Real-IP", "198.51.100.1")
	req.Header.Set("X-Forwarded-For", "198.51.100.2")

	if got := r.ClientIP(req).String(); got != "203.0.113.7" {
		t.Fatalf("forged header believed from untrusted peer: got %s, want 203.0.113.7", got)
	}
}

func TestClientIPUsesForwardedForFromTrustedProxy(t *testing.T) {
	r, err := NewClientIPResolver(nil)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/node/1/2/3", nil)
	req.RemoteAddr = "127.0.0.1:8081"
	req.Header.Set("X-Forwarded-For", "203.0.113.9")

	if got := r.ClientIP(req).String(); got != "203.0.113.9" {
		t.Fatalf("got %s, want 203.0.113.9", got)
	}
}

// TestClientIPWalksChainFromRight pins the direction of the walk. A client
// that prepends its own hops must not be able to push the real address out of
// view: only the rightmost non-trusted entry is believed.
func TestClientIPWalksChainFromRight(t *testing.T) {
	r, err := NewClientIPResolver([]string{"127.0.0.0/8", "10.0.0.0/8"})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "127.0.0.1:8081"
	// "1.2.3.4" is the forgery; 203.0.113.9 is what our own proxy appended.
	req.Header.Set("X-Forwarded-For", "1.2.3.4, 203.0.113.9, 10.0.0.18")

	if got := r.ClientIP(req).String(); got != "203.0.113.9" {
		t.Fatalf("got %s, want 203.0.113.9", got)
	}
}

// TestKeyGroupsIPv6BySlash64 guards the evasion route that per-address
// bucketing would leave open for a host holding a routed prefix.
func TestKeyGroupsIPv6BySlash64(t *testing.T) {
	a := Key(net.ParseIP("2a00:801:7cf:1273:e190:2125:3892:e6fc"))
	b := Key(net.ParseIP("2a00:801:7cf:1273:ffff:ffff:ffff:1"))
	if a != b {
		t.Fatalf("addresses in one /64 got different keys: %s vs %s", a, b)
	}
	if c := Key(net.ParseIP("2a00:801:7cf:9999::1")); c == a {
		t.Fatal("different /64s share a key")
	}
}

func TestAllowConsumesBurstThenRefills(t *testing.T) {
	l := NewLimiter(100, time.Minute)
	now := time.Now()
	l.now = func() time.Time { return now }

	rate := Rate{Refill: 0.2, Burst: 3}
	for i := 0; i < 3; i++ {
		if ok, _ := l.Allow("k", rate); !ok {
			t.Fatalf("request %d inside burst was rejected", i)
		}
	}

	ok, wait := l.Allow("k", rate)
	if ok {
		t.Fatal("burst exhausted but request allowed")
	}
	if wait < 4*time.Second || wait > 6*time.Second {
		t.Fatalf("Retry-After %v, want ~5s at 0.2/s", wait)
	}

	now = now.Add(5 * time.Second)
	if ok, _ := l.Allow("k", rate); !ok {
		t.Fatal("token should have refilled after 5s")
	}
}

// TestRejectedRequestDoesNotStarveCaller pins that hammering a limit does not
// push the recovery time out. A crawler that keeps knocking must still be
// admitted at the sustained rate, not permanently locked out.
func TestRejectedRequestDoesNotStarveCaller(t *testing.T) {
	l := NewLimiter(100, time.Minute)
	now := time.Now()
	l.now = func() time.Time { return now }
	rate := Rate{Refill: 1, Burst: 1}

	l.Allow("k", rate) // consume the single token
	for i := 0; i < 20; i++ {
		now = now.Add(50 * time.Millisecond)
		l.Allow("k", rate) // rejected, repeatedly
	}

	// One second after the last grant, a token is owed regardless of how many
	// rejections happened in between.
	now = now.Add(time.Second)
	if ok, _ := l.Allow("k", rate); !ok {
		t.Fatal("caller starved by its own rejected retries")
	}
}

func TestFirstTimeCallerStartsFull(t *testing.T) {
	l := NewLimiter(100, time.Minute)
	if ok, _ := l.Allow("new", Rate{Refill: 0.01, Burst: 5}); !ok {
		t.Fatal("a caller we have never seen was rejected")
	}
}

// TestBucketMapStaysBounded is the regression guard for the failure this
// server already hit once elsewhere: an unbounded map keyed by something the
// caller chooses. Here the key is the client address, so a flood of distinct
// sources must not grow the map without limit.
func TestBucketMapStaysBounded(t *testing.T) {
	const maxKeys = 64
	l := NewLimiter(maxKeys, time.Minute)
	rate := Rate{Refill: 1, Burst: 1}

	for i := 0; i < 10000; i++ {
		l.Allow(fmt.Sprintf("10.%d.%d.%d", i>>16&0xff, i>>8&0xff, i&0xff), rate)
	}

	if got := l.Len(); got > maxKeys {
		t.Fatalf("map grew to %d buckets, cap is %d", got, maxKeys)
	}
}

func TestIdleBucketsAreForgotten(t *testing.T) {
	l := NewLimiter(2, time.Minute)
	now := time.Now()
	l.now = func() time.Time { return now }
	rate := Rate{Refill: 1, Burst: 1}

	l.Allow("a", rate)
	l.Allow("b", rate)
	now = now.Add(2 * time.Minute)
	l.Allow("c", rate) // insert triggers eviction of the two idle buckets

	if got := l.Len(); got != 1 {
		t.Fatalf("idle buckets survived: %d live, want 1", got)
	}
}

func TestUnlimitedRateAlwaysAllows(t *testing.T) {
	l := NewLimiter(10, time.Minute)
	for i := 0; i < 100; i++ {
		if ok, _ := l.Allow("k", Rate{}); !ok {
			t.Fatal("zero rate should mean exempt, not blocked")
		}
	}
	if l.Len() != 0 {
		t.Fatal("exempt requests should not allocate buckets")
	}
}

// TestClassificationMatchesMeasuredCost pins each measured-expensive path to
// the tight class. Misfiling /api/networks in particular would undo 18% of the
// saving, since it costs a GROUP BY over the whole nodes table but sits under
// the otherwise-ordinary /api/ prefix.
func TestClassificationMatchesMeasuredCost(t *testing.T) {
	m, err := New(DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct{ path, want string }{
		{"/node/2/5001/100", "expensive"},
		{"/reachability", "expensive"},
		{"/reachability/node", "expensive"},
		{"/points/2/6055/7/71", "expensive"},
		{"/analytics/geo-hosting/country", "expensive"},
		{"/stats", "expensive"},
		{"/api/networks", "expensive"},
		{"/api/nodes/2/5001/100/history", "expensive"},
		{"/download/nodelist/fidonet/2026/nodelist.244.gz", "download"},
		{"/static/style.css", "exempt"},
		{"/robots.txt", "exempt"},
		{"/api/health", "exempt"},
		{"/api/modem/results/direct", "exempt"},
		{"/", "default"},
		{"/browse", "default"},
	} {
		got, _, _ := m.rateFor(tc.path)
		if got != tc.want {
			t.Errorf("%s classified %q, want %q", tc.path, got, tc.want)
		}
	}
}

func TestMiddlewareRejectsWith429AndRetryAfter(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Classes[0].Rate = Rate{Refill: 0.2, Burst: 1}
	m, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	h := m.Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	call := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "/node/1/2/3", nil)
		req.RemoteAddr = "203.0.113.7:1234"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	if rec := call(); rec.Code != http.StatusOK {
		t.Fatalf("first request got %d", rec.Code)
	}
	rec := call()
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("second request got %d, want 429", rec.Code)
	}
	if ra := rec.Header().Get("Retry-After"); ra == "" || ra == "0" {
		t.Fatalf("Retry-After = %q, want a positive whole number of seconds", ra)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("Cache-Control = %q; a 429 must not be cached for other clients", cc)
	}
}

// TestExemptPathSurvivesFloodedNeighbour checks that exhausting the expensive
// class does not take the site's assets down with it - a page that renders but
// cannot load its stylesheet would look broken rather than throttled.
func TestExemptPathSurvivesFloodedNeighbour(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Classes[0].Rate = Rate{Refill: 0.01, Burst: 1}
	m, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	h := m.Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/node/1/2/3", nil)
		req.RemoteAddr = "203.0.113.7:1234"
		h.ServeHTTP(httptest.NewRecorder(), req)
	}

	req := httptest.NewRequest("GET", "/static/style.css", nil)
	req.RemoteAddr = "203.0.113.7:1234"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("exempt asset got %d while the caller was throttled elsewhere", rec.Code)
	}
}

func TestSeparateCallersHaveSeparateBuckets(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Classes[0].Rate = Rate{Refill: 0.01, Burst: 1}
	m, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	h := m.Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))

	send := func(addr string) int {
		req := httptest.NewRequest("GET", "/node/1/2/3", nil)
		req.RemoteAddr = addr
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	send("203.0.113.7:1")
	if code := send("203.0.113.7:2"); code != http.StatusTooManyRequests {
		t.Fatalf("same IP, different port got %d; the port must not split the bucket", code)
	}
	if code := send("203.0.113.8:1"); code != http.StatusOK {
		t.Fatalf("a different caller got %d; buckets leaked across IPs", code)
	}
}

func TestInvalidTrustedProxyIsAStartupError(t *testing.T) {
	if _, err := NewClientIPResolver([]string{"not-an-address"}); err == nil {
		t.Fatal("a malformed trusted proxy entry must not be silently ignored")
	}
}

// TestRejectionLoggingIsSampled guards the log against being written by the
// attacker: the counters must stay exact while the WARN lines are capped.
func TestRejectionLoggingIsSampled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Classes[0].Rate = Rate{Refill: 0.01, Burst: 1}
	m, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	h := m.Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))

	for i := 0; i < 500; i++ {
		req := httptest.NewRequest("GET", "/node/1/2/3", nil)
		req.RemoteAddr = "203.0.113.7:1234"
		h.ServeHTTP(httptest.NewRecorder(), req)
	}

	if got := m.Stats()["rejected"].(uint64); got != 499 {
		t.Fatalf("rejected count = %d, want 499: sampling must not lose counts", got)
	}
	// The log bucket holds logSampleRate.Burst tokens; anything past that is
	// dropped rather than written.
	if left := m.logged.Len(); left != 1 {
		t.Fatalf("log sampler tracked %d keys, want 1", left)
	}
}

// TestClassesDoNotShareABucket is a regression test for a bug found by running
// the limiter against the live site rather than by reading it: the bucket was
// keyed on the caller alone, so spending the expensive-page burst also blocked
// that caller's cheap requests, and the Retry-After returned for the cheap one
// was computed from the wrong class's refill rate.
func TestClassesDoNotShareABucket(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Classes[0].Rate = Rate{Refill: 0.01, Burst: 2} // expensive, tiny
	cfg.Default = Rate{Refill: 2, Burst: 40}
	m, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	h := m.Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))

	send := func(path string) int {
		req := httptest.NewRequest("GET", path, nil)
		req.RemoteAddr = "203.0.113.7:1234"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	for i := 0; i < 5; i++ {
		send("/node/1/2/3") // drain the expensive class
	}
	if code := send("/node/1/2/3"); code != http.StatusTooManyRequests {
		t.Fatalf("expensive class should be exhausted, got %d", code)
	}
	if code := send("/browse"); code != http.StatusOK {
		t.Fatalf("cheap request got %d; the classes are sharing a bucket", code)
	}
	if code := send("/download/nodelist/fidonet/2026/nodelist.244.gz"); code != http.StatusOK {
		t.Fatalf("download got %d; the classes are sharing a bucket", code)
	}
}
