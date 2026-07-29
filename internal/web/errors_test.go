package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The classifiers only earn their keep if they still recognise a context error
// that has travelled up through a few layers of %w wrapping - which is exactly
// how these errors arrive, since every storage method wraps what the driver
// returned.
func wrapped(err error) error {
	return fmt.Errorf("failed to query nodes: %w", fmt.Errorf("clickhouse: %w", err))
}

func TestStorageFailureClassification(t *testing.T) {
	cases := []struct {
		name        string
		err         error
		wantHandled bool
		wantDisplay string
	}{
		{"client hung up", wrapped(context.Canceled), true, ""},
		{"budget exhausted", wrapped(context.DeadlineExceeded), false, timeoutMessage},
		{"real failure", wrapped(errors.New("connection refused")), false, "Failed to load zones"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			display, handled := storageFailure("Browse zones", "Failed to load zones", tc.err)
			if handled != tc.wantHandled {
				t.Fatalf("handled = %v, want %v", handled, tc.wantHandled)
			}
			if tc.wantHandled {
				if display != nil {
					t.Errorf("display = %v, want nil when the request is already over", display)
				}
				return
			}
			if display == nil || display.Error() != tc.wantDisplay {
				t.Errorf("display = %v, want %q", display, tc.wantDisplay)
			}
		})
	}
}

func TestHTTPStorageError(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantBody   string
	}{
		// Nothing is written at all: the client is gone, and writing a 500 to
		// a closed connection is the noise this whole helper exists to remove.
		{"client hung up", wrapped(context.Canceled), http.StatusOK, ""},
		{"budget exhausted", wrapped(context.DeadlineExceeded), http.StatusServiceUnavailable, timeoutMessage},
		{"real failure", wrapped(errors.New("boom")), http.StatusInternalServerError, "Error retrieving node history"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			httpStorageError(rec, "Node history", "Error retrieving node history", tc.err)
			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if !strings.Contains(rec.Body.String(), tc.wantBody) {
				t.Errorf("body = %q, want it to contain %q", rec.Body.String(), tc.wantBody)
			}
		})
	}
}

func TestClientGone(t *testing.T) {
	if !clientGone("test", wrapped(context.Canceled)) {
		t.Error("a wrapped context.Canceled must be recognised")
	}
	if clientGone("test", wrapped(context.DeadlineExceeded)) {
		t.Error("a timeout is a failure the user should see, not a silent stop")
	}
	if clientGone("test", errors.New("boom")) {
		t.Error("an ordinary error is not a cancelled client")
	}
}
