package web

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/nodelistdb/internal/logging"
)

// timeoutMessage is what a page says when its own query budget ran out.
// "Try again later" would be wrong advice: the same query will time out again
// at the same width, so the useful suggestion is to ask for less.
const timeoutMessage = "This query exceeded its time budget. Try a shorter period or a smaller limit."

// storageFailure classifies a failed storage read for a page handler and logs
// it at the level it deserves.
//
// handled == true means the request is already over and the handler should
// return without writing anything. The only case is context.Canceled, which
// says the client closed the tab: rendering a page for it wastes the work, and
// logging it at ERROR buries the failures that matter - one abandoned
// analytics page would otherwise look identical to ClickHouse being down.
//
// Otherwise display is what belongs in the page's error banner. msg is the
// wording the site already used, so the pages read exactly as they did; only
// the two context cases are new.
//
// A budget expiry gets a real 503, not just a banner on a 200 page: a page
// that timed out has to be distinguishable from a page that rendered, or
// nothing upstream - load balancer, uptime monitor, log aggregator - can tell
// the difference. The display error carries that in its type rather than in a
// second return value, so each handler threads it through one call
// (renderStatus with statusFor) instead of carrying a status variable around.
//
// Like the API's writeStorageError this cannot live inside render(): the web
// handlers replace the real error with a generic one before render() is
// reached, so the classification has to happen at the call site while
// errors.Is still has something to inspect.
func storageFailure(op, msg string, err error) (display error, handled bool) {
	switch {
	case errors.Is(err, context.Canceled):
		logging.Debug("Request cancelled by client", slog.String("op", op))
		return nil, true
	case errors.Is(err, context.DeadlineExceeded):
		logging.Warn("Query exceeded its time budget", slog.String("op", op))
		return &timeoutError{}, false
	default:
		logging.Error(op, slog.Any("error", err))
		return errors.New(msg), false
	}
}

// timeoutError is the display error storageFailure returns when a query budget
// ran out. It exists to be recognised by statusFor: a page carrying it answers
// 503 rather than 200. Everything else a handler puts in its banner - a real
// storage failure, a bad query parameter - stays a 200.
type timeoutError struct{}

func (*timeoutError) Error() string { return timeoutMessage }

// statusFor is the status a page carrying this display error should answer
// with. A nil error, or any error other than a budget expiry, is a 200: that
// is what every error banner in this package has always meant.
func statusFor(display error) int {
	var t *timeoutError
	if errors.As(display, &t) {
		return http.StatusServiceUnavailable
	}
	return http.StatusOK
}

// httpStorageError is storageFailure for the handlers that answer with
// http.Error rather than a rendered page - the node, point and history views,
// which have no error banner to put a message in.
func httpStorageError(w http.ResponseWriter, op, msg string, err error) {
	switch {
	case errors.Is(err, context.Canceled):
		logging.Debug("Request cancelled by client", slog.String("op", op))
	case errors.Is(err, context.DeadlineExceeded):
		logging.Warn("Query exceeded its time budget", slog.String("op", op))
		http.Error(w, timeoutMessage, http.StatusServiceUnavailable)
	default:
		logging.Error(op, slog.Any("error", err))
		http.Error(w, msg, http.StatusInternalServerError)
	}
}

// clientGone reports whether an error that has already travelled up through a
// helper is the request context ending rather than a storage failure.
//
// It exists for the three helpers that return their error to a caller
// (resolveBrowseDate, searchNodes, getFilteredReachabilityNodes) instead of
// handling it where it happened. Wrapping preserves the cause, so errors.Is
// still works at the top - but only if the top asks.
func clientGone(op string, err error) bool {
	if errors.Is(err, context.Canceled) {
		logging.Debug("Request cancelled by client", slog.String("op", op))
		return true
	}
	return false
}
