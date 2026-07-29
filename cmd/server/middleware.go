package main

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/nodelistdb/internal/logging"
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
