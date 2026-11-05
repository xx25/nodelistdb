package ratelimit

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClientIPWalksAcrossRepeatedForwardedHeaders(t *testing.T) {
	r, err := NewClientIPResolver(nil)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "127.0.0.1:8081"
	// The forgery arrives first; the proxy's own field is last.
	req.Header.Add("X-Forwarded-For", "1.2.3.4")
	req.Header.Add("X-Forwarded-For", "203.0.113.9")

	if got := r.ClientIP(req).String(); got != "203.0.113.9" {
		t.Fatalf("got %s, want the proxy's hop 203.0.113.9", got)
	}
}

// TestClientIPIgnoresRealIPWhenForwardedForSpoke is nodelist.5001.ru's shape:
// its Caddy reaches the server with a chain of only 127.0.0.1, and passes a
// client's X-Real-IP through untouched.
func TestClientIPIgnoresRealIPWhenForwardedForSpoke(t *testing.T) {
	r, err := NewClientIPResolver(nil)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "127.0.0.1:8081"
	req.Header.Set("X-Forwarded-For", "127.0.0.1")
	req.Header.Set("X-Real-IP", "198.51.100.1")

	if got := r.ClientIP(req).String(); got != "127.0.0.1" {
		t.Fatalf("forged X-Real-IP believed: got %s", got)
	}
}

func TestClientIPStillUsesRealIPWithoutForwardedFor(t *testing.T) {
	r, err := NewClientIPResolver(nil)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "127.0.0.1:8081"
	req.Header.Set("X-Real-IP", "203.0.113.9")

	if got := r.ClientIP(req).String(); got != "203.0.113.9" {
		t.Fatalf("got %s, want 203.0.113.9", got)
	}
}

// TestSharedKeyIgnoresForwardingHeaders is the property an anonymity listener
// rests on: a Tor client arrives from loopback, where the resolver believes
// X-Forwarded-For, so without this a forged value per request is a fresh bucket.
func TestSharedKeyIgnoresForwardingHeaders(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SharedKey = "anon:tor"
	cfg.Classes = []Class{{Name: "expensive", Prefixes: []string{"/node/"}, Rate: Rate{Refill: 0.001, Burst: 2}}}
	m, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	h := m.Wrap(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	codes := make([]int, 0, 3)
	for i := range 3 {
		req := httptest.NewRequest("GET", "/node/2/5001/100", nil)
		req.RemoteAddr = "127.0.0.1:40000"
		req.Header.Set("X-Forwarded-For", net.IPv4(198, 51, 100, byte(i+1)).String())
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		codes = append(codes, rec.Code)
	}
	if codes[0] != 200 || codes[1] != 200 || codes[2] != http.StatusTooManyRequests {
		t.Fatalf("codes = %v, want 200 200 429", codes)
	}
}

func testGate(rate int64, slots int) *DownloadGate {
	g := NewDownloadGate(GateConfig{Name: "test", Prefixes: []string{"/download/", "/api/nodelist/latest"}, BytesPerSec: rate, MaxConcurrent: slots})
	g.queueWait = 50 * time.Millisecond
	return g
}

func TestGateLeavesOtherPathsAlone(t *testing.T) {
	g := testGate(1, 1)
	g.slots <- struct{}{} // full, and at one byte a second
	h := g.Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(make([]byte, 1<<20))
	}))

	rec := httptest.NewRecorder()
	start := time.Now()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/node/2/5001/100", nil))
	if rec.Code != 200 || rec.Body.Len() != 1<<20 {
		t.Fatalf("code %d, %d bytes", rec.Code, rec.Body.Len())
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("a page was throttled: %v", d)
	}
}

func TestGateRejectsWhenFullAndReleasesSlots(t *testing.T) {
	g := testGate(1<<30, 1)
	release := make(chan struct{})
	entered := make(chan struct{})
	h := g.Wrap(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		close(entered)
		<-release
	}))

	done := make(chan struct{})
	go func() {
		defer close(done)
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/download/a", nil))
	}()
	<-entered

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/download/b", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("code = %d, want 503", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("headers = %v", rec.Header())
	}
	if g.Rejected() != 1 {
		t.Fatalf("rejected = %d", g.Rejected())
	}

	close(release)
	<-done
	if len(g.slots) != 0 {
		t.Fatalf("%d slots still held", len(g.slots))
	}
}

func TestGateReleasesSlotOnPanic(t *testing.T) {
	g := testGate(1<<30, 1)
	h := g.Wrap(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") }))
	func() {
		defer func() { _ = recover() }()
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/download/a", nil))
	}()
	if len(g.slots) != 0 {
		t.Fatal("a panicking download kept its slot")
	}
}

func TestGateGivesUpQuietlyWhenClientLeavesTheQueue(t *testing.T) {
	g := testGate(1<<30, 1)
	g.queueWait = time.Minute
	g.slots <- struct{}{}
	h := g.Wrap(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("handler ran") }))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/download/a", nil).WithContext(ctx))
	if g.Rejected() != 0 {
		t.Fatal("a client that left counted as a rejection")
	}
}

func TestGatePacesBytes(t *testing.T) {
	g := testGate(256*1024, 4)
	h := g.Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(make([]byte, 128*1024))
	}))

	rec := httptest.NewRecorder()
	start := time.Now()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/download/a", nil))
	elapsed := time.Since(start)

	if rec.Body.Len() != 128*1024 {
		t.Fatalf("sent %d bytes", rec.Body.Len())
	}
	// Half a second, less the first chunk, which goes immediately.
	if elapsed < 400*time.Millisecond {
		t.Fatalf("128 KiB at 256 KiB/s took %v", elapsed)
	}
}

func TestGatePacingIsSharedAcrossDownloads(t *testing.T) {
	p := &bytePacer{rate: 1000}
	now := time.Now()
	if got := p.reserve(500, now); got != 0 {
		t.Fatalf("first reservation waited %v", got)
	}
	if got := p.reserve(500, now); got != 500*time.Millisecond {
		t.Fatalf("second reservation waited %v, want 500ms", got)
	}
	// An idle pacer must not bank unused time as a burst.
	if got := p.reserve(1, now.Add(time.Hour)); got != 0 {
		t.Fatalf("after idle waited %v", got)
	}
	if got := p.reserve(1, now.Add(time.Hour)); got != time.Millisecond {
		t.Fatalf("idle time became a burst: waited %v", got)
	}
}

// TestGateCoversEveryPrefix: /api/nodelist/latest streams a whole file when the
// newest issue is uncompressed, and escaped the gate by not being /download/.
func TestGateCoversEveryPrefix(t *testing.T) {
	g := testGate(1<<30, 1)
	g.slots <- struct{}{}
	h := g.Wrap(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("handler ran") }))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/nodelist/latest?domain=fsxnet", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("code = %d, want 503", rec.Code)
	}
}

// TestAbandonedReservationsLeaveNoDebt: at 16 KiB/s every cancelled chunk would
// otherwise book a second that nobody uses, and connecting and hanging up is
// free, so an honest download behind them would wait for bytes never sent.
func TestAbandonedReservationsLeaveNoDebt(t *testing.T) {
	g := testGate(16*1024, 1)
	h := g.Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(make([]byte, gateChunk*2))
	}))
	for range 20 {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/download/a", nil).WithContext(ctx))
		cancel()
	}

	g.pace.mu.Lock()
	debt := time.Until(g.pace.next)
	g.pace.mu.Unlock()
	if debt > 2*time.Second { // 20 abandoned chunks would be 20 s
		t.Fatalf("%v of debt left by abandoned downloads", debt)
	}
}

func TestPacedWriteStopsWhenClientLeaves(t *testing.T) {
	g := testGate(1024, 1)
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	h := g.Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, err := w.Write(make([]byte, 1<<20))
		result <- err
	}))

	go h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/download/a", nil).WithContext(ctx))
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("write reported success after the client left")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("write kept waiting after the client left")
	}
}

// liveServer runs h on a real listener: a recorder has no deadlines, and every
// test below is about what a deadline does to a real connection.
func liveServer(t *testing.T, h http.Handler, writeTimeout time.Duration) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: h, WriteTimeout: writeTimeout}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return "http://" + ln.Addr().String()
}

// TestThrottledDownloadOutlivesWriteTimeout: the server's WriteTimeout is one
// deadline for a whole response, so a throttle that took a tarball past it would
// cut the file off at a length the throttle itself caused.
func TestThrottledDownloadOutlivesWriteTimeout(t *testing.T) {
	const size = 128 * 1024
	g := testGate(256*1024, 1)
	base := liveServer(t, g.Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(make([]byte, size))
	})), 100*time.Millisecond)

	resp, err := http.Get(base + "/download/a")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read after %d bytes: %v", len(body), err)
	}
	if len(body) != size {
		t.Fatalf("got %d bytes, want %d", len(body), size)
	}
}

// TestQueueWaitOutlivesWriteTimeout is the case extending the deadline after the
// wait would miss: by then it has already expired and cannot be revived.
func TestQueueWaitOutlivesWriteTimeout(t *testing.T) {
	g := testGate(1<<30, 1)
	g.queueWait = 5 * time.Second
	first := make(chan struct{})
	release := make(chan struct{})
	base := liveServer(t, g.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/download/first" {
			close(first)
			<-release
		}
		_, _ = io.WriteString(w, "payload")
	})), 100*time.Millisecond)

	go func() {
		if resp, err := http.Get(base + "/download/first"); err == nil {
			_ = resp.Body.Close()
		}
	}()
	<-first

	second := make(chan string, 1)
	go func() {
		resp, err := http.Get(base + "/download/second")
		if err != nil {
			second <- "error: " + err.Error()
			return
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		second <- string(b)
	}()

	time.Sleep(300 * time.Millisecond) // well past WriteTimeout, still queued
	close(release)
	select {
	case got := <-second:
		if got != "payload" {
			t.Fatalf("got %q", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("queued download never finished")
	}
}

// TestStalledClientFreesItsSlot: taking the server's deadline away must not
// leave a client that stops reading holding a slot for ever.
func TestStalledClientFreesItsSlot(t *testing.T) {
	g := testGate(1<<40, 1)
	g.writeWindow = 200 * time.Millisecond
	finished := make(chan error, 1)
	base := liveServer(t, g.Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		chunk := make([]byte, 1<<20)
		for {
			if _, err := w.Write(chunk); err != nil {
				finished <- err
				return
			}
		}
	})), 0)

	conn, err := net.Dial("tcp", strings.TrimPrefix(base, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := io.WriteString(conn, "GET /download/a HTTP/1.1\r\nHost: x\r\n\r\n"); err != nil {
		t.Fatal(err)
	}

	select {
	case <-finished: // the client read nothing, and the write failed
	case <-time.After(20 * time.Second):
		t.Fatal("a stalled client held the write open")
	}
	deadline := time.Now().Add(2 * time.Second)
	for len(g.slots) != 0 {
		if time.Now().After(deadline) {
			t.Fatal("slot never released")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
