package storage

import (
	"context"
	"errors"
	"log/slog"

	"github.com/nodelistdb/internal/logging"
)

// contextErr returns err when it is the caller's context ending, and nil for
// everything else.
//
// Several readers deliberately degrade when a *secondary* query fails: a
// missing dead-node set still lets GetPSTNNodes return its rows, a missing
// hostname list still lets the IPv6 pages render. That is the right call for a
// transient database hiccup, and it predates this file.
//
// A context error is different in kind. It does not mean "this one extra fact
// is unavailable" - it means the whole request is over, so every later query
// in the same method will fail the same way and the partial result is not an
// answer to anything. Worse, these methods sit behind CachedStorage: returning
// the degraded result as a success stores it, and the next visitor gets the
// half-built answer from cache for the rest of the TTL. For GetPSTNNodes that
// is an operator-marked-dead phone number looking callable to the modem tester
// for an hour.
//
// So: degrade on ordinary failures, abort on a context error.
func contextErr(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return nil
}

// logQueryFailure reports a failed query at the level it deserves.
//
// The storage layer logs a good many failures at ERROR before returning them,
// and the handler layer classifies the same error again on the way out. Without
// this, a closed browser tab still produces an ERROR line here - which defeats
// the point of classifying it at the handler, since the noise the migration was
// supposed to remove is emitted one layer below.
func logQueryFailure(op string, err error, args ...any) {
	args = append(args, slog.Any("error", err))
	if contextErr(err) != nil {
		logging.Debug(op+" (request ended)", args...)
		return
	}
	logging.Error(op, args...)
}
