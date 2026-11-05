package storage

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/nodelistdb/internal/database"
)

// TestNodeTimelineReadsHistoryOnce is a source guard for the same reason as the
// ones in context_errors_test.go: the behaviour needs a live ClickHouse.
//
// A node page used to run the history query twice, once through GetNodeHistory
// and again inside GetNodeChanges, at nine seconds a page.
func TestNodeTimelineReadsHistoryOnce(t *testing.T) {
	body := functionBody(t, "search_operations_changes.go", "GetNodeTimeline")
	if got := strings.Count(body, "GetNodeHistory("); got != 1 {
		t.Errorf("GetNodeTimeline reads history %d times, want 1", got)
	}
	for _, fn := range []string{"GetNodeChanges", "computeNodeChanges"} {
		if strings.Contains(functionBody(t, "search_operations_changes.go", fn), "GetNodeHistory(") {
			t.Errorf("%s reads history itself; it must take the timeline's read", fn)
		}
	}
}

// TestCachedNodeTimelineIsOneRead pins why the cached timeline does not reuse
// the GetNodeHistory cache entry. Changes depend on the newest nodelist date at
// the moment they are computed, so history from before an import paired with a
// computation after it reports a node that is still listed as removed.
func TestCachedNodeTimelineIsOneRead(t *testing.T) {
	body := functionBody(t, "cached_storage_nodes.go", "GetNodeTimeline")
	if strings.Contains(body, "cs.GetNodeHistory(") {
		t.Error("the cached timeline reads the separately cached history")
	}
	if !strings.Contains(body, "SearchOps().GetNodeTimeline(") {
		t.Error("the cached timeline no longer reads through GetNodeTimeline")
	}
}

// TestNodeTimelineSurvivesTheCache: the cache stores JSON, and a page that loses
// either half on a hit would look fine on the miss that populated it.
func TestNodeTimelineSurvivesTheCache(t *testing.T) {
	date := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	node := database.Node{Zone: 2, Net: 5001, Node: 100, NodelistDate: date}
	in := NodeTimeline{
		History: []database.Node{node},
		Changes: []database.NodeChange{{Date: date, ChangeType: "added", NewNode: &node}},
	}

	data, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out NodeTimeline
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.History) != 1 || out.History[0].Node != 100 {
		t.Errorf("history lost: %+v", out.History)
	}
	if len(out.Changes) != 1 || out.Changes[0].NewNode == nil || out.Changes[0].NewNode.Node != 100 {
		t.Errorf("changes lost: %+v", out.Changes)
	}
}
