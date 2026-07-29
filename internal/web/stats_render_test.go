package web

import (
	"errors"
	"html/template"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nodelistdb/internal/database"
	"github.com/nodelistdb/internal/storage"
)

// stubStorage satisfies storage.Operations by embedding it. Only the methods a
// test explicitly overrides are usable; everything else panics on call, so a
// test pins exactly which storage calls the handler under test makes.
type stubStorage struct {
	storage.Operations

	availableDates    []time.Time
	availableDatesErr error
	latestDate        time.Time
	latestDateErr     error
	nearestDate       time.Time
	nearestDateErr    error
	stats             *database.NetworkStats
	statsErr          error
	nodeHistory       []storage.NodeCountByDate
	pointStats        *storage.PointStats
}

func (s *stubStorage) GetAvailableDates(domain string) ([]time.Time, error) {
	return s.availableDates, s.availableDatesErr
}

func (s *stubStorage) GetLatestStatsDate(domain string) (time.Time, error) {
	return s.latestDate, s.latestDateErr
}

func (s *stubStorage) GetNearestAvailableDate(requested time.Time, domain string) (time.Time, error) {
	return s.nearestDate, s.nearestDateErr
}

func (s *stubStorage) GetStats(date time.Time, domain string) (*database.NetworkStats, error) {
	return s.stats, s.statsErr
}

func (s *stubStorage) GetNodeCountHistory(domain string) ([]storage.NodeCountByDate, error) {
	return s.nodeHistory, nil
}

func (s *stubStorage) GetPointStats(domain string, asOf *time.Time) (*storage.PointStats, error) {
	return s.pointStats, nil
}

// newTestServer builds a Server with the real embedded templates and the given
// storage stub.
func newTestServer(t *testing.T, ops storage.Operations) *Server {
	t.Helper()
	s := &Server{
		storage:     ops,
		templates:   make(map[string]*template.Template),
		templatesFS: TemplatesFS,
	}
	s.loadTemplates()
	return s
}

// TestStatsHandlerErrorPathsRenderFully covers every early return in
// StatsHandler. Each one renders stats.html with its own payload, and the
// template dereferences fields (.NodeHistory, .PointStats) unconditionally —
// a payload missing one fails mid-write, after the 200 and the first bytes of
// the body have already been flushed, so the client silently gets truncated
// HTML instead of an error.
func TestStatsHandlerErrorPathsRenderFully(t *testing.T) {
	dates := []time.Time{time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)}
	boom := errors.New("clickhouse unavailable")

	cases := []struct {
		name    string
		query   string
		ops     *stubStorage
		wantMsg string
	}{
		{
			name:    "available dates fail",
			ops:     &stubStorage{availableDatesErr: boom},
			wantMsg: "Failed to get available dates",
		},
		{
			name:    "unparseable date and latest fails",
			query:   "?date=not-a-date",
			ops:     &stubStorage{availableDates: dates, latestDateErr: boom},
			wantMsg: "Invalid date format and failed to get latest date",
		},
		{
			name:    "nearest date lookup fails",
			query:   "?date=2026-07-20",
			ops:     &stubStorage{availableDates: dates, nearestDateErr: boom},
			wantMsg: "Failed to find available date",
		},
		{
			name:    "latest date lookup fails",
			ops:     &stubStorage{availableDates: dates, latestDateErr: boom},
			wantMsg: "Failed to find latest nodelist date",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestServer(t, tc.ops)
			rec := httptest.NewRecorder()
			s.StatsHandler(rec, httptest.NewRequest("GET", "/stats"+tc.query, nil))

			body := rec.Body.String()
			if rec.Code != 200 {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			if !strings.Contains(body, tc.wantMsg) {
				t.Errorf("body does not carry the error message %q", tc.wantMsg)
			}
			// A template that aborted mid-execution never reaches the closing
			// tags, which is the only symptom visible to a browser.
			if !strings.Contains(body, "</html>") {
				t.Errorf("page was truncated (no </html>); rendered %d bytes", len(body))
			}
		})
	}
}

// TestStatsHandlerRendersStats covers the success path so the error-path
// assertions above cannot pass on a page that renders nothing at all.
func TestStatsHandlerRendersStats(t *testing.T) {
	date := time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)
	ops := &stubStorage{
		availableDates: []time.Time{date},
		latestDate:     date,
		stats: &database.NetworkStats{
			Date:          date,
			TotalNodes:    1234,
			ActiveNodes:   1200,
			CMNodes:       42,
			BinkpNodes:    900,
			InternetNodes: 1000,
		},
		nodeHistory: []storage.NodeCountByDate{{Date: date, TotalNodes: 1234}},
	}

	s := newTestServer(t, ops)
	rec := httptest.NewRecorder()
	s.StatsHandler(rec, httptest.NewRequest("GET", "/stats", nil))

	body := rec.Body.String()
	for _, want := range []string{"Network Statistics", "1234", "2026-07-20", "</html>"} {
		if !strings.Contains(body, want) {
			t.Errorf("rendered stats page is missing %q", want)
		}
	}
}
