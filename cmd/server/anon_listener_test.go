package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nodelistdb/internal/config"
	"github.com/nodelistdb/internal/logging"
	"github.com/nodelistdb/internal/ratelimit"
)

func anonConfig() config.AnonListener {
	return config.AnonListener{
		Name: "tor", Address: "127.0.0.1:0", ExpensiveBurst: 2, ExpensiveRPS: 0.001,
	}.WithDefaults()
}

// TestAnonListenerCannotForgeAFreshBucket is the bypass this listener closes. On
// the main port a Tor client is a loopback peer, so X-Forwarded-For is believed.
func TestAnonListenerCannotForgeAFreshBucket(t *testing.T) {
	srv, err := buildAnonServer(config.RateLimitConfig{}, anonConfig(),
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	var codes []int
	for i := range 3 {
		req := httptest.NewRequest("GET", "/node/2/5001/100", nil)
		req.RemoteAddr = "127.0.0.1:40000"
		req.Header.Set("X-Forwarded-For", net.IPv4(198, 51, 100, byte(i+1)).String())
		rec := httptest.NewRecorder()
		srv.Handler.ServeHTTP(rec, req)
		codes = append(codes, rec.Code)
	}
	if codes[0] != 200 || codes[1] != 200 || codes[2] != 429 {
		t.Fatalf("codes = %v, want 200 200 429", codes)
	}
}

func TestAnonListenerStaysLimitedWhenPublicLimiterIsOff(t *testing.T) {
	off := false
	srv, err := buildAnonServer(config.RateLimitConfig{Enabled: &off}, anonConfig(),
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	var last int
	for range 3 {
		rec := httptest.NewRecorder()
		srv.Handler.ServeHTTP(rec, httptest.NewRequest("GET", "/node/2/5001/100", nil))
		last = rec.Code
	}
	if last != 429 {
		t.Fatalf("last code = %d, want 429", last)
	}
}

func TestAnonListenerDropsForwardingHeaders(t *testing.T) {
	var seen http.Header
	srv, err := buildAnonServer(config.RateLimitConfig{}, anonConfig(),
		http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { seen = r.Header.Clone() }), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/", nil)
	for _, h := range []string{"X-Forwarded-For", "X-Real-IP", "CF-Connecting-IP"} {
		req.Header.Set(h, "198.51.100.1")
	}
	srv.Handler.ServeHTTP(httptest.NewRecorder(), req)
	for _, h := range []string{"X-Forwarded-For", "X-Real-IP", "CF-Connecting-IP"} {
		if seen.Get(h) != "" {
			t.Errorf("%s reached the handler", h)
		}
	}
}

// TestGateWorksThroughTheRealChain runs the production order on a real socket:
// paced past the server's WriteTimeout, so the ResponseController has to reach
// the connection through every wrapper, with and without compression. The timing
// matters as much as the bytes: without the gate both bodies would still arrive.
func TestGateWorksThroughTheRealChain(t *testing.T) {
	const rawSize = 256 * 1024
	random := make([]byte, rawSize)
	if _, err := rand.Read(random); err != nil {
		t.Fatal(err)
	}
	// Hex of random bytes gzips to about half: real compression, and a stream
	// long enough to outlast the timeout at the gate's rate.
	text := hex.EncodeToString(random)[:rawSize]

	mux := http.NewServeMux()
	mux.HandleFunc("/download/text", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, text)
	})
	mux.HandleFunc("/download/bin", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		_, _ = w.Write(random)
	})

	const writeTimeout = 100 * time.Millisecond
	srv, err := buildAnonServer(config.RateLimitConfig{}, anonConfig(), mux, writeTimeout)
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(ln) }()
	defer srv.Close()

	client := &http.Client{Transport: &http.Transport{DisableCompression: true}}
	get := func(path string) (*http.Response, []byte, time.Duration) {
		t.Helper()
		req, err := http.NewRequest("GET", "http://"+ln.Addr().String()+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Accept-Encoding", "gzip")
		start := time.Now()
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("%s: read after %d bytes: %v", path, len(body), err)
		}
		return resp, body, time.Since(start)
	}

	resp, body, took := get("/download/bin")
	if !bytes.Equal(body, random) {
		t.Fatalf("bin: got %d bytes, want %d", len(body), rawSize)
	}
	if resp.Header.Get("Content-Encoding") != "" {
		t.Fatalf("bin was compressed: %q", resp.Header.Get("Content-Encoding"))
	}
	if took < 800*time.Millisecond { // 256 KiB at 256 KiB/s
		t.Fatalf("bin took %v: not throttled", took)
	}

	resp, body, took = get("/download/text")
	if resp.Header.Get("Content-Encoding") != "gzip" {
		t.Fatalf("text: Content-Encoding = %q, want gzip", resp.Header.Get("Content-Encoding"))
	}
	zr, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	plain, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	if string(plain) != text {
		t.Fatalf("text: got %d bytes, want %d", len(plain), len(text))
	}
	if took < writeTimeout*2 {
		t.Fatalf("text took %v: never outlasted WriteTimeout", took)
	}
	// Paced by what leaves, not by what the handler wrote: at the raw size this
	// would take a full second.
	if want := time.Duration(float64(len(body)) / float64(config.DefaultAnonDownloadBytesPerSec) * float64(time.Second)); took < want*3/4 {
		t.Fatalf("text: %d wire bytes took %v, want about %v", len(body), took, want)
	}
	if took > 900*time.Millisecond {
		t.Fatalf("text took %v for %d wire bytes: metered before compression", took, len(body))
	}
}

func TestRequestLogLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.log")
	if err := logging.Initialize(&logging.Config{Level: "info", File: path, MaxSize: 1}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = logging.Initialize(&logging.Config{Level: "error", Console: true}) })

	resolver, err := ratelimit.NewClientIPResolver(nil)
	if err != nil {
		t.Fatal(err)
	}
	ok := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})

	// Through a proxy: a client's X-Real-IP is passed along by Caddy, and must
	// not become the logged address.
	req := httptest.NewRequest("GET", "/x", nil)
	req.RemoteAddr = "127.0.0.1:50000"
	req.Header.Set("X-Forwarded-For", "203.0.113.9")
	req.Header.Set("X-Real-IP", "198.51.100.1")
	req.Header.Set("User-Agent", "Reflectionbot/1.0")
	mainRequestLog(resolver).middleware(ok).ServeHTTP(httptest.NewRecorder(), req)

	req = httptest.NewRequest("GET", "/y", nil)
	req.RemoteAddr = "127.8.195.234:41000"
	req.Header.Set("User-Agent", strings.Repeat("a", 500))
	anonRequestLog("i2p").middleware(ok).ServeHTTP(httptest.NewRecorder(), req)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	log := string(data)
	for _, want := range []string{
		"ip=203.0.113.9", "peer=127.0.0.1:50000", "listener=main", "user_agent=Reflectionbot/1.0",
		"ip=127.8.195.234:41000", "listener=i2p", "user_agent=" + strings.Repeat("a", maxLoggedUserAgent) + "\n",
	} {
		if !strings.Contains(log, want) {
			t.Errorf("log lacks %q:\n%s", want, log)
		}
	}
	if strings.Contains(log, "198.51.100.1") {
		t.Errorf("forged X-Real-IP reached the log:\n%s", log)
	}
}

// TestServeStopsEveryListenerWhenOneFails: an anonymity listener that cannot
// bind must take the server down, not leave it half up, and serve must not leak
// a goroutine blocked on sending the second failure.
func TestServeStopsEveryListenerWhenOneFails(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()

	free, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	freeAddr := free.Addr().String()
	_ = free.Close()

	good := &http.Server{Addr: freeAddr, Handler: http.NewServeMux()}
	bad := &http.Server{Addr: occupied.Addr().String(), Handler: http.NewServeMux()}
	alsoBad := &http.Server{Addr: occupied.Addr().String(), Handler: http.NewServeMux()}

	done := make(chan error, 1)
	go func() { done <- serve(nil, good, bad, alsoBad) }()

	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), occupied.Addr().String()) {
			t.Fatalf("err = %v, want it to name the occupied address", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("serve did not return")
	}

	if conn, err := net.DialTimeout("tcp", freeAddr, 200*time.Millisecond); err == nil {
		_ = conn.Close()
		t.Fatal("the healthy listener was left accepting")
	}
}

// TestShutdownAllStopsAcceptingEverywhereAtOnce: with a slow request on one
// server, the other must not keep accepting while the first drains.
func TestShutdownAllStopsAcceptingEverywhereAtOnce(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	defer close(release)

	busy := &http.Server{Handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		close(entered)
		<-release
	})}
	idle := &http.Server{Handler: http.NewServeMux()}

	listen := func(s *http.Server) string {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		s.Addr = ln.Addr().String()
		go func() { _ = s.Serve(ln) }()
		return s.Addr
	}
	busyAddr := listen(busy)
	idleAddr := listen(idle)

	go func() {
		client := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}
		if resp, err := client.Get("http://" + busyAddr + "/"); err == nil {
			_ = resp.Body.Close()
		}
	}()
	<-entered

	done := make(chan struct{})
	go func() {
		defer close(done)
		shutdownAll([]*http.Server{busy, idle}, 500*time.Millisecond)
	}()

	time.Sleep(100 * time.Millisecond) // busy is still draining
	var d net.Dialer
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if conn, err := d.DialContext(ctx, "tcp", idleAddr); err == nil {
		_ = conn.Close()
		t.Error("a listener kept accepting while another drained")
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("shutdownAll did not force-close the stuck request")
	}
}
