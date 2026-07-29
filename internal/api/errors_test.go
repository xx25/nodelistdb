package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Context errors reach a handler wrapped by every storage layer they passed
// through, so the classifier has to survive %w nesting - matching on the
// concrete error would silently stop working the first time a wrapper was
// added.
func wrapped(err error) error {
	return fmt.Errorf("failed to search nodes: %w", fmt.Errorf("clickhouse: %w", err))
}

func TestWriteStorageError(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantErrMsg string
	}{
		// Nothing at all is written: there is no client left to receive it.
		{"client hung up", wrapped(context.Canceled), http.StatusOK, ""},
		{"budget exhausted", wrapped(context.DeadlineExceeded), http.StatusServiceUnavailable,
			"Query exceeded its time budget, please narrow it or retry"},
		{"real failure", wrapped(errors.New("connection refused")), http.StatusInternalServerError,
			"Failed to get sysops"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			writeStorageError(rec, "Failed to get sysops", tc.err)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if tc.wantErrMsg == "" {
				if rec.Body.Len() != 0 {
					t.Errorf("body = %q, want nothing written", rec.Body.String())
				}
				return
			}
			var payload map[string]interface{}
			if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
				t.Fatalf("decoding body %q: %v", rec.Body.String(), err)
			}
			if payload["error"] != tc.wantErrMsg {
				t.Errorf("error = %v, want %q", payload["error"], tc.wantErrMsg)
			}
		})
	}
}

// writeStorageErrorf is the variant for the sites that have always echoed the
// underlying error into the response. The 500 body must stay what it was
// before the classifier was introduced.
func TestWriteStorageErrorfKeepsTheOldBody(t *testing.T) {
	cause := errors.New("connection refused")
	rec := httptest.NewRecorder()
	writeStorageErrorf(rec, "Failed to get sysops", cause)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decoding body: %v", err)
	}
	want := "Failed to get sysops: connection refused"
	if payload["error"] != want {
		t.Errorf("error = %v, want %q", payload["error"], want)
	}
}
