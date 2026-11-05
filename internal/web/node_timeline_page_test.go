package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nodelistdb/internal/database"
	"github.com/nodelistdb/internal/pingtrace"
	"github.com/nodelistdb/internal/storage"
)

// timelineStorage answers a node page from GetNodeTimeline alone; anything else
// the handler asks of the nil embedded Storage panics.
type timelineStorage struct {
	Storage
	timeline storage.NodeTimeline
	calls    int
}

func (s *timelineStorage) GetNodeDomains(context.Context, int, int, int) ([]string, error) {
	return []string{"fidonet"}, nil
}

func (s *timelineStorage) GetNodeTimeline(context.Context, int, int, int, string) (storage.NodeTimeline, error) {
	s.calls++
	return s.timeline, nil
}

func (s *timelineStorage) GetPointsByBoss(context.Context, string, int, int, int, *time.Time) ([]database.Point, error) {
	return nil, nil
}

func (s *timelineStorage) GetNodePings(context.Context, string, int, int, int, int) ([]pingtrace.Ping, error) {
	return nil, nil
}

// TestNodePageUsesOneTimelineRead: the page takes history and changes from one
// read, so it cannot pair a cached history with changes computed later. An
// import landing inside that read is a separate, narrower race, not covered.
func TestNodePageUsesOneTimelineRead(t *testing.T) {
	date := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	node := database.Node{Zone: 2, Net: 5001, Node: 100, NodelistDate: date, SystemName: "Timeline Test BBS", Domain: "fidonet"}
	ops := &timelineStorage{timeline: storage.NodeTimeline{
		History: []database.Node{node},
		Changes: []database.NodeChange{{Date: date, ChangeType: "added", NewNode: &node}},
	}}

	rec := httptest.NewRecorder()
	newTestServer(t, ops).NodeHistoryHandler(rec, httptest.NewRequest("GET", "/node/2/5001/100", nil))
	if rec.Code != 200 {
		t.Fatalf("code = %d: %s", rec.Code, rec.Body.String())
	}
	if ops.calls != 1 {
		t.Fatalf("GetNodeTimeline called %d times", ops.calls)
	}
	if !strings.Contains(rec.Body.String(), "Timeline Test BBS") {
		t.Fatal("the page does not show the timeline's history")
	}
}

func TestNodePageNotFoundOnEmptyTimeline(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestServer(t, &timelineStorage{}).NodeHistoryHandler(rec, httptest.NewRequest("GET", "/node/2/5001/100", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d", rec.Code)
	}
}
