package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/nodelistdb/internal/config"
	"github.com/nodelistdb/internal/logging"
	"github.com/nodelistdb/internal/ratelimit"
)

// loggingMiddleware logs every request, API and web alike.
//
// It wraps the ResponseWriter with chi's WrapResponseWriter rather than a
// local struct. The local one embedded http.ResponseWriter and overrode
// WriteHeader, which silently dropped the optional interfaces the embedded
// writer implements - Flusher, Hijacker, ReaderFrom - from everything
// downstream of it. Nothing needs them today; the next handler that streams a
// response would have found out at runtime.
func loggingMiddleware(next http.Handler) http.Handler {
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
			slog.String("ip", clientIP(r)),
		)
	})
}

// clientIP returns the address to log for this request.
//
// This is the server's single answer to "who is calling", and it is used for
// log lines only. Every header it reads is client-supplied and trivially
// forged; nothing here may be used for authorization or rate limiting without
// first pinning which proxy is trusted. chi's middleware.RealIP used to run on
// the API router as well, giving the two halves of the server different
// answers, and it rewrote r.RemoteAddr in place so handlers could not tell the
// forged value from the real one. It is gone; this reads headers and leaves
// the request alone.
func clientIP(r *http.Request) string {
	// X-Real-IP is set by many reverse proxies, including Caddy by default.
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}

	// X-Forwarded-For may carry a chain; the original client is first.
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if first, _, found := strings.Cut(xff, ","); found {
			return strings.TrimSpace(first)
		}
		return strings.TrimSpace(xff)
	}

	if ip := r.Header.Get("CF-Connecting-IP"); ip != "" {
		return ip
	}

	return r.RemoteAddr
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

	rl := ratelimit.DefaultConfig()
	rl.TrustedProxies = cfg.TrustedProxies

	idle, err := cfg.IdleDuration()
	if err != nil {
		return nil, err
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
