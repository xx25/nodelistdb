package main

import (
	"net/http"
	"sync/atomic"

	"github.com/nodelistdb/internal/api"
	"github.com/nodelistdb/internal/cache"
	"github.com/nodelistdb/internal/ftp"
)

// cacheStats is the /api/cache/stats body.
type cacheStats struct {
	Hits    uint64  `json:"hits"`
	Misses  uint64  `json:"misses"`
	Sets    uint64  `json:"sets"`
	Deletes uint64  `json:"deletes"`
	Size    uint64  `json:"size"`
	Keys    uint64  `json:"keys"`
	HitRate float64 `json:"hit_rate"`
}

// cacheStatsHandler serves the cache counters.
//
// It used to hand-format the JSON with Fprintf, which meant a %d against a
// uint64 read without the atomic load its neighbours used, and no way for the
// encoder to escape anything. Marshalling a struct removes both concerns.
func cacheStatsHandler(c cache.Cache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		m := c.GetMetrics()
		hits, misses := atomic.LoadUint64(&m.Hits), atomic.LoadUint64(&m.Misses)
		api.WriteJSONSuccess(w, cacheStats{
			Hits:    hits,
			Misses:  misses,
			Sets:    atomic.LoadUint64(&m.Sets),
			Deletes: atomic.LoadUint64(&m.Deletes),
			Size:    atomic.LoadUint64(&m.Size),
			Keys:    atomic.LoadUint64(&m.Keys),
			// The +1 keeps a cold cache from dividing by zero; it costs a
			// fraction of a percent on the first few requests and nothing after.
			HitRate: float64(hits) / float64(hits+misses+1) * 100,
		})
	}
}

// ftpStats is the /api/ftp/stats body.
type ftpStats struct {
	Enabled        bool   `json:"enabled"`
	Host           string `json:"host"`
	Port           int    `json:"port"`
	MaxConnections int    `json:"max_connections"`
}

// ftpStatsHandler serves the FTP server's configuration counters. The previous
// version asserted four types out of a map[string]any without checking any of
// them, so a missing or retyped key was a panic in the request goroutine
// rather than a field reading zero.
func ftpStatsHandler(s *ftp.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw := s.GetStats()
		enabled, _ := raw["enabled"].(bool)
		host, _ := raw["host"].(string)
		port, _ := raw["port"].(int)
		maxConns, _ := raw["max_connections"].(int)
		api.WriteJSONSuccess(w, ftpStats{
			Enabled:        enabled,
			Host:           host,
			Port:           port,
			MaxConnections: maxConns,
		})
	}
}
