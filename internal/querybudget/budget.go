// Package querybudget bounds how long one request's database work may run.
//
// It is deliberately separate from the cancellation work in internal/storage.
// Cancellation needs no deadline: it fires when the client goes away, costs
// nothing, and is on for everyone. A deadline is a policy decision with real
// failure modes, so it ships as its own opt-in switch, default off.
//
// # Why not chi's middleware.Timeout or http.TimeoutHandler
//
// chi's middleware.Timeout writes its own 504 from a defer, unconditionally,
// over whatever the handler already did - and there is no flag to turn that
// off. A handler that returns without writing gets a bare empty-body 504
// instead of the error page it meant to render. http.TimeoutHandler is worse:
// it buffers the entire response a second time, on top of the buffer
// web.render already keeps to avoid half-written pages.
//
// So this middleware only sets the context deadline. It never writes, never
// touches the status code, and leaves the response entirely to the handler,
// which learns about the deadline as a context.DeadlineExceeded from storage
// and turns it into a 503 through its own error classifier.
//
// # Why per route group and not one deadline at the mux
//
// context.WithDeadline can only shorten an inherited deadline, never extend
// it: given a parent whose deadline is already earlier, it degrades to a plain
// WithCancel. A single mux-level budget would therefore make "give the heavy
// analytics pages longer" unimplementable downstream - the earliest deadline
// always wins. Because r.Context() carries no deadline of its own, a per-group
// budget can pick any value in either direction, and the composition problem
// disappears.
//
// # Why it is skipped on the HTTP protocol
//
// clickhouse-go turns a context deadline into a max_execution_time setting on
// the query whenever more than a second remains. Readonly HTTP users are not
// allowed to set it, and the query fails outright.
// internal/database/clickhouse.go already carries a comment about this from
// the last time somebody hit it. Cancellation still works there; only the
// budget is withheld.
package querybudget

import (
	"context"
	"net/http"
	"strings"
	"time"
)

// Budget is the deadline applied to the requests it wraps. The zero value is
// "no budget", which is what every route gets until the config turns it on.
type Budget struct {
	d time.Duration
}

// New returns a Budget of d, or the zero Budget when the feature is off, when
// d is not positive, or when the ClickHouse protocol is one whose users cannot
// be given a max_execution_time.
func New(enabled bool, d time.Duration, clickhouseProtocol string) Budget {
	if !enabled || d <= 0 || isHTTPProtocol(clickhouseProtocol) {
		return Budget{}
	}
	return Budget{d: d}
}

func isHTTPProtocol(p string) bool {
	return strings.EqualFold(p, "http")
}

// Duration reports the budget, or zero when there is none. Callers use it to
// size the server's WriteTimeout, which is a TCP-level deadline that does not
// cancel r.Context() and would otherwise cut a long request off before its own
// budget ever expired.
func (b Budget) Duration() time.Duration { return b.d }

// Wrap returns next with the budget applied. A zero Budget returns next
// unchanged, so the wiring can stay in place while the feature is off.
func (b Budget) Wrap(next http.Handler) http.Handler {
	if b.d <= 0 {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), b.d)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
