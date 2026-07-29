package main

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nodelistdb/internal/cache"
)

// TestServeReturnsOnListenFailure is the reason run() returns errors instead
// of calling logging.Fatalf. A late startup failure - the listen port already
// taken, which happens on every restart that races the old process - used to
// reach os.Exit(1) from inside a goroutine, so none of run's defers fired and
// the Badger cache was closed by process death rather than by Close().
//
// A failure before any defer is registered (a bad ClickHouse host) proves
// nothing about this; the port has to be occupied after the cache exists.
func TestServeReturnsOnListenFailure(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()

	closed := make(chan struct{})
	server := &http.Server{Addr: occupied.Addr().String(), Handler: http.NewServeMux()}

	done := make(chan error, 1)
	go func() {
		defer close(closed) // stands in for run()'s defers
		done <- serve(server, nil)
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("serve returned nil for an occupied listen address")
		}
		if !strings.Contains(err.Error(), "HTTP server") {
			t.Errorf("error = %v, want it to name the HTTP server", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not return after ListenAndServe failed")
	}

	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("the caller's deferred cleanup never ran")
	}
}

// metricsOnlyCache is the smallest thing cacheStatsHandler needs: the
// interface, embedded so every unused method panics if it is ever called, plus
// the counters the handler reads.
type metricsOnlyCache struct {
	cache.Cache
	metrics cache.Metrics
}

func (c *metricsOnlyCache) GetMetrics() *cache.Metrics { return &c.metrics }

// TestCacheStatsHandlerShape pins the JSON the endpoint emits. It used to be
// hand-formatted with Fprintf, where one of the six counters was read without
// the atomic load its neighbours used.
func TestCacheStatsHandlerShape(t *testing.T) {
	rec := httptest.NewRecorder()
	handler := cacheStatsHandler(&metricsOnlyCache{metrics: cache.Metrics{
		Hits: 30, Misses: 10, Sets: 40, Keys: 7, Size: 1024,
	}})
	handler(rec, httptest.NewRequest("GET", "/api/cache/stats", nil))

	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want JSON", ct)
	}

	var got cacheStats
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("response is not JSON: %s", rec.Body.String())
	}
	if got.Hits != 30 || got.Misses != 10 || got.Sets != 40 || got.Keys != 7 || got.Size != 1024 {
		t.Errorf("counters did not survive the round trip: %+v", got)
	}
	// 30/(30+10+1) as a percentage.
	if got.HitRate < 73 || got.HitRate > 74 {
		t.Errorf("hit_rate = %v, want ~73.2", got.HitRate)
	}
}
