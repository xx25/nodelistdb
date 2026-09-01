package ratelimit

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/nodelistdb/internal/logging"
)

// Class is one cost tier. Requests are sorted into a tier by path prefix and
// each tier has its own allowance, because the paths differ in cost by three
// orders of magnitude and one rate cannot serve both. Measured on the public
// front end over 18 hours: /node/ averages 12.3s of request time and /static/
// averages under a millisecond.
type Class struct {
	Name     string
	Prefixes []string
	Rate     Rate
}

// Config is the middleware's policy.
type Config struct {
	// Exempt paths bypass the limiter entirely: assets, health checks and the
	// authenticated modem API, whose callers are known and whose submissions
	// must not be dropped.
	Exempt []string

	// Classes are tested in order; the first prefix match wins, so put the
	// specific paths ahead of the general ones.
	Classes []Class

	// Default applies to any path no class claims.
	Default Rate

	// TrustedProxies are the CIDRs whose forwarding headers are believed.
	TrustedProxies []string

	MaxKeys int
	Idle    time.Duration
}

// DefaultConfig is the policy derived from the front end's measured traffic.
//
// The expensive tier allows 6 requests a minute sustained with a burst of 10.
// The burst is what people actually use - a reader opens a node page, its
// history and a neighbour in a clump, then stops to read - while the sustained
// rate is what no human keeps up and every crawler does.
//
// Replaying 18.5 hours of production log against this rule blocks 5.8% of
// expensive requests and 4.2% of the CPU they cost. That is worth having and
// it is NOT a fix on its own: the load is spread over many clients each
// behaving moderately, so even a punitive 1 request a minute only removes
// 12.3%. What this bounds is the worst single client - the heaviest one was
// peaking at 43 expensive requests a minute, about three cores' worth at the
// congested per-request cost. Getting the total down needs the per-request
// cost down (an uncached /api/networks is a GROUP BY over 31.5M rows, and a
// node page fetches its full history twice), which is a separate change.
//
// The cheap tier is deliberately loose. /download/ served 16,932 requests in
// the same window for under 1% of the CPU, and someone mirroring the archive
// is doing exactly what the archive is for.
func DefaultConfig() Config {
	return Config{
		Exempt: []string{"/static/", "/favicon.ico", "/robots.txt", "/api/health", "/api/modem/"},
		Classes: []Class{{
			Name: "expensive",
			// Every prefix here was measured at or above one second of mean
			// request time. /api/networks is named explicitly because it sits
			// under /api/ but costs a GROUP BY over 31.5M rows.
			Prefixes: []string{
				"/node/", "/reachability", "/points/", "/analytics/",
				"/stats", "/api/networks", "/api/nodes/", "/api/points/",
				"/api/stats", "/api/analytics/", "/api/software/", "/api/sysops",
			},
			Rate: Rate{Refill: 0.1, Burst: 10},
		}, {
			Name:     "download",
			Prefixes: []string{"/download/", "/nodelists", "/pointlists"},
			Rate:     Rate{Refill: 5, Burst: 60},
		}},
		Default:        Rate{Refill: 2, Burst: 40},
		TrustedProxies: nil, // loopback
		MaxKeys:        20000,
		Idle:           15 * time.Minute,
	}
}

// logSampleRate caps rejection logging at one line per caller and class per
// minute, with a small burst so a short spike is still visible in full. The
// aggregate counts stay exact - only the log lines are sampled.
var logSampleRate = Rate{Refill: 1.0 / 60.0, Burst: 3}

// Middleware applies Config to a handler.
type Middleware struct {
	cfg      Config
	resolver *ClientIPResolver
	limiter  *Limiter

	allowed  atomic.Uint64
	rejected atomic.Uint64

	// logged throttles the rejection log to one line per caller per window.
	// Without it the log is written by whoever is being blocked: the traffic
	// that triggered this feature ran to hundreds of thousands of requests a
	// day, and one WARN each would fill the disk of the very host the limiter
	// is protecting. It reuses the token bucket, so it is bounded too.
	logged *Limiter
}

// New builds the middleware, failing on an unparseable trusted-proxy entry so
// a typo is a startup error rather than a silently disabled limiter.
func New(cfg Config) (*Middleware, error) {
	resolver, err := NewClientIPResolver(cfg.TrustedProxies)
	if err != nil {
		return nil, err
	}
	return &Middleware{
		cfg:      cfg,
		resolver: resolver,
		limiter:  NewLimiter(cfg.MaxKeys, cfg.Idle),
		logged:   NewLimiter(cfg.MaxKeys, cfg.Idle),
	}, nil
}

// rateFor sorts a path into its class.
func (m *Middleware) rateFor(path string) (string, Rate, bool) {
	for _, p := range m.cfg.Exempt {
		if strings.HasPrefix(path, p) {
			return "exempt", Rate{}, false
		}
	}
	for _, c := range m.cfg.Classes {
		for _, p := range c.Prefixes {
			if strings.HasPrefix(path, p) {
				return c.Name, c.Rate, true
			}
		}
	}
	return "default", m.cfg.Default, true
}

// Wrap returns next guarded by the limiter.
func (m *Middleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		class, rate, limited := m.rateFor(r.URL.Path)
		if !limited {
			next.ServeHTTP(w, r)
			return
		}

		key := Key(m.resolver.ClientIP(r))
		// The bucket is per caller AND per class. Sharing one bucket across
		// classes silently mixes the policies: a reader who spends their
		// expensive-page burst would then be refused a download too, and the
		// wait handed back would be computed from whichever class asked last.
		ok, wait := m.limiter.Allow(key+"|"+class, rate)
		if ok {
			m.allowed.Add(1)
			next.ServeHTTP(w, r)
			return
		}

		m.rejected.Add(1)
		if noisy, _ := m.logged.Allow(key+"|"+class, logSampleRate); noisy {
			logging.Warn("rate limited",
				slog.String("ip", key),
				slog.String("class", class),
				slog.String("path", r.URL.Path),
				slog.Duration("retry_after", wait),
			)
		}

		// Round up: Retry-After is whole seconds, and a rounded-down 0 would
		// invite an immediate retry that is certain to be rejected again.
		seconds := int(wait.Seconds())
		if wait > time.Duration(seconds)*time.Second {
			seconds++
		}
		if seconds < 1 {
			seconds = 1
		}
		w.Header().Set("Retry-After", strconv.Itoa(seconds))
		// This response varies per caller and must never be cached by a proxy
		// on the way out, or one client's 429 becomes everyone's.
		w.Header().Set("Cache-Control", "no-store")
		http.Error(w, "Too Many Requests: this archive is served from a small host; please slow down.", http.StatusTooManyRequests)
	})
}

// Stats reports the limiter's counters for the health and stats endpoints.
func (m *Middleware) Stats() map[string]any {
	return map[string]any{
		"allowed":  m.allowed.Load(),
		"rejected": m.rejected.Load(),
		"keys":     m.limiter.Len(),
	}
}
