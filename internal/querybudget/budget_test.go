package querybudget

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewWithholdsBudget(t *testing.T) {
	cases := []struct {
		name     string
		enabled  bool
		d        time.Duration
		protocol string
		want     time.Duration
	}{
		{"off by default", false, 30 * time.Second, "native", 0},
		{"enabled with a duration", true, 30 * time.Second, "native", 30 * time.Second},
		{"enabled with no duration", true, 0, "native", 0},
		{"negative duration", true, -time.Second, "native", 0},
		// The driver turns a deadline into max_execution_time, which readonly
		// HTTP users are not allowed to set - so the budget is withheld there
		// however the config is written.
		{"http protocol", true, 30 * time.Second, "http", 0},
		{"HTTP protocol, any case", true, 30 * time.Second, "HTTP", 0},
		{"empty protocol means native", true, 30 * time.Second, "", 30 * time.Second},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := New(tc.enabled, tc.d, tc.protocol).Duration(); got != tc.want {
				t.Errorf("Duration() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestWrapSetsDeadlineWithoutWriting is the property that rules out chi's
// middleware.Timeout: the wrapper must leave the response entirely to the
// handler. A middleware that writes its own 504 would overwrite whatever the
// handler's error classifier had already sent.
func TestWrapSetsDeadlineWithoutWriting(t *testing.T) {
	var (
		sawDeadline bool
		remaining   time.Duration
	)
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dl, ok := r.Context().Deadline()
		sawDeadline = ok
		if ok {
			remaining = time.Until(dl)
		}
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("handler output"))
	})

	rec := httptest.NewRecorder()
	New(true, time.Minute, "native").Wrap(inner).
		ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if !sawDeadline {
		t.Fatal("handler saw no deadline")
	}
	if remaining <= 0 || remaining > time.Minute {
		t.Errorf("remaining budget %v, want (0, 1m]", remaining)
	}
	if rec.Code != http.StatusTeapot {
		t.Errorf("status = %d, want %d - the middleware must not write", rec.Code, http.StatusTeapot)
	}
	if rec.Body.String() != "handler output" {
		t.Errorf("body = %q, want the handler's own output", rec.Body.String())
	}
}

// TestZeroBudgetIsPassThrough keeps the wiring safe to leave in place while the
// feature is off: the handler must come back unchanged, deadline and all.
func TestZeroBudgetIsPassThrough(t *testing.T) {
	var hadDeadline bool
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, hadDeadline = r.Context().Deadline()
	})

	wrapped := Budget{}.Wrap(inner)
	wrapped.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	if hadDeadline {
		t.Error("zero budget added a deadline")
	}
}

// TestWrapShortensButNeverExtends pins the stdlib behaviour the per-route
// design exists to work around: a child deadline later than its parent's is
// silently ignored. This is why there is no single budget at the mux.
func TestWrapShortensButNeverExtends(t *testing.T) {
	var remaining time.Duration
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dl, _ := r.Context().Deadline()
		remaining = time.Until(dl)
	})

	// Parent already has 1s; the route asks for 10 minutes.
	parent, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	r := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(parent)

	New(true, 10*time.Minute, "native").Wrap(inner).ServeHTTP(httptest.NewRecorder(), r)

	if remaining > 2*time.Second {
		t.Errorf("remaining = %v; a child cannot extend its parent's deadline, "+
			"so the outer 1s must win", remaining)
	}
}
