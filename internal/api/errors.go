package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/nodelistdb/internal/logging"
)

// writeStorageError answers one failed storage read.
//
// Now that reads carry the request context, two of the errors a handler can
// see are not failures of this server, and treating them as if they were is
// what makes cancellation expensive to operate:
//
//   - context.Canceled means the client hung up. There is nobody to send a 500
//     to and nothing went wrong here, so it is logged at debug and no response
//     is written. Left alone it would be an ERROR line and a 500 for every
//     closed browser tab and every load-balancer retry.
//   - context.DeadlineExceeded means the query outran the budget a handler set
//     for it (see withQueryBudget). That is a 503: the request is well-formed
//     and worth retrying, and the client should be told to back off rather than
//     told it broke the server.
//
// Anything else is a real storage failure and keeps the caller's message and
// its 500.
//
// This has to be called per site rather than wired into WriteJSONError,
// because every call site stringifies the error into its message before the
// shared code sees it - by then errors.Is has nothing left to match on.
//
// writeStorageError sends op alone as the client message; writeStorageErrorf
// sends "op: err". The split is not a style choice - the handlers were already
// divided that way, and the 500 bodies stay byte-for-byte what they were so
// that only the two context cases are new behaviour.
func writeStorageError(w http.ResponseWriter, op string, err error) {
	writeStorageMessage(w, op, op, err)
}

func writeStorageErrorf(w http.ResponseWriter, op string, err error) {
	writeStorageMessage(w, op, fmt.Sprintf("%s: %v", op, err), err)
}

func writeStorageMessage(w http.ResponseWriter, op, msg string, err error) {
	switch {
	case errors.Is(err, context.Canceled):
		logging.Debug("Request cancelled by client", slog.String("op", op))
	case errors.Is(err, context.DeadlineExceeded):
		logging.Warn("Query exceeded its time budget", slog.String("op", op))
		WriteJSONError(w, "Query exceeded its time budget, please narrow it or retry", http.StatusServiceUnavailable)
	default:
		logging.Error(op, slog.Any("error", err))
		WriteJSONError(w, msg, http.StatusInternalServerError)
	}
}
