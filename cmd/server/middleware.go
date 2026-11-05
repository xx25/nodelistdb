package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/nodelistdb/internal/config"
	"github.com/nodelistdb/internal/logging"
	"github.com/nodelistdb/internal/ratelimit"
)

// maxLoggedUserAgent bounds what one header can add to a log line.
const maxLoggedUserAgent = 200

// requestLog says how to describe requests arriving on one listener.
type requestLog struct {
	listener string
	clientIP func(*http.Request) string
}

// mainRequestLog logs the address the limiter would charge, so a line in the
// log and a 429 always agree on who the caller was. The peer goes in alongside
// it: this log used to believe X-Real-IP from anyone, Caddy passes that header
// through, and a request's real origin could not be recovered afterwards.
func mainRequestLog(resolver *ratelimit.ClientIPResolver) requestLog {
	return requestLog{listener: "main", clientIP: func(r *http.Request) string {
		return resolver.ClientIP(r).String()
	}}
}

// anonRequestLog logs the peer alone. An anonymity listener has no proxy in
// front of it, so nothing a request claims about itself means anything.
func anonRequestLog(name string) requestLog {
	return requestLog{listener: name, clientIP: func(r *http.Request) string {
		return r.RemoteAddr
	}}
}

// middleware logs every request, API and web alike.
//
// It wraps the ResponseWriter with chi's WrapResponseWriter rather than a
// local struct. The local one embedded http.ResponseWriter and overrode
// WriteHeader, which silently dropped the optional interfaces the embedded
// writer implements - Flusher, Hijacker, ReaderFrom - from everything
// downstream of it. Nothing needs them today; the next handler that streams a
// response would have found out at runtime.
func (l requestLog) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		wrapped := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		next.ServeHTTP(wrapped, r)

		logging.Info("HTTP request",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", wrapped.Status()),
			slog.Int("bytes", wrapped.BytesWritten()),
			slog.Duration("duration", time.Since(start)),
			slog.String("ip", l.clientIP(r)),
			slog.String("peer", r.RemoteAddr),
			slog.String("listener", l.listener),
			slog.String("user_agent", truncateUserAgent(r.UserAgent())),
		)
	})
}

func truncateUserAgent(ua string) string {
	if len(ua) > maxLoggedUserAgent {
		return ua[:maxLoggedUserAgent]
	}
	return ua
}

// stripForwardingHeaders removes the headers a client could use to name itself.
// The limiter on an anonymity listener already ignores them; dropping them here
// keeps that true of anything downstream that ever reads one.
func stripForwardingHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Header.Del("X-Forwarded-For")
		r.Header.Del("X-Real-IP")
		r.Header.Del("CF-Connecting-IP")
		next.ServeHTTP(w, r)
	})
}

// buildRateLimiter turns the config section into middleware, or nil when the
// limiter is switched off.
//
// The zero values in the config mean "keep the measured default" rather than
// "no allowance", so an operator can raise one number without having to
// restate the whole policy - and cannot accidentally set a limit of zero
// requests per second by mentioning the section at all.
func buildRateLimiter(cfg config.RateLimitConfig) (*ratelimit.Middleware, error) {
	if !cfg.On() {
		logging.Info("Rate limiting disabled by configuration")
		return nil, nil
	}

	rl, err := rateLimitPolicy(cfg)
	if err != nil {
		return nil, err
	}

	m, err := ratelimit.New(rl)
	if err != nil {
		return nil, err
	}
	logging.Info("Rate limiting enabled",
		slog.Float64("expensive_rps", rl.Classes[0].Rate.Refill),
		slog.Float64("expensive_burst", rl.Classes[0].Rate.Burst),
		slog.Float64("default_rps", rl.Default.Refill),
		slog.Int("max_keys", rl.MaxKeys),
		slog.Any("trusted_proxies", trustedProxyNames(cfg.TrustedProxies)),
	)
	return m, nil
}

// buildAnonLimiter is the limiter for one anonymity listener. It is built even
// when rate_limit.enabled is false: bounding this listener is the reason it
// exists, and switching the public limiter off must not open it.
func buildAnonLimiter(cfg config.RateLimitConfig, l config.AnonListener) (*ratelimit.Middleware, error) {
	rl, err := rateLimitPolicy(cfg)
	if err != nil {
		return nil, err
	}
	rl.SharedKey = "anon:" + l.Name
	for i := range rl.Classes {
		if rl.Classes[i].Name == "expensive" {
			rl.Classes[i].Rate = ratelimit.Rate{Refill: l.ExpensiveRPS, Burst: l.ExpensiveBurst}
		}
	}
	return ratelimit.New(rl)
}

// rateLimitPolicy applies the config section to the measured defaults.
func rateLimitPolicy(cfg config.RateLimitConfig) (ratelimit.Config, error) {
	rl := ratelimit.DefaultConfig()
	rl.TrustedProxies = cfg.TrustedProxies

	idle, err := cfg.IdleDuration()
	if err != nil {
		return ratelimit.Config{}, err
	}
	rl.Idle = idle
	if cfg.MaxKeys > 0 {
		rl.MaxKeys = cfg.MaxKeys
	}
	if cfg.DefaultRPS > 0 {
		rl.Default.Refill = cfg.DefaultRPS
	}
	if cfg.DefaultBurst > 0 {
		rl.Default.Burst = cfg.DefaultBurst
	}
	for i := range rl.Classes {
		switch rl.Classes[i].Name {
		case "expensive":
			if cfg.ExpensiveRPS > 0 {
				rl.Classes[i].Rate.Refill = cfg.ExpensiveRPS
			}
			if cfg.ExpensiveBurst > 0 {
				rl.Classes[i].Rate.Burst = cfg.ExpensiveBurst
			}
		case "download":
			if cfg.DownloadRPS > 0 {
				rl.Classes[i].Rate.Refill = cfg.DownloadRPS
			}
			if cfg.DownloadBurst > 0 {
				rl.Classes[i].Rate.Burst = cfg.DownloadBurst
			}
		}
	}
	return rl, nil
}

// trustedProxyNames renders the trusted set for the startup log, naming the
// default explicitly so the log answers "whose headers does this believe"
// without the reader having to know what an empty list means.
func trustedProxyNames(cidrs []string) []string {
	if len(cidrs) == 0 {
		return []string{"loopback (default)"}
	}
	return cidrs
}

// rateLimitStatsHandler exposes the limiter's counters alongside the cache's.
func rateLimitStatsHandler(m *ratelimit.Middleware) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(m.Stats()); err != nil {
			logging.Warn("failed to encode rate limit stats", slog.Any("error", err))
		}
	}
}
