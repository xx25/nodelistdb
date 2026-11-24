package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nodelistdb/internal/storage"
)

// markRecorder captures what the mark handler hands to storage.
type markRecorder struct {
	fakeOps
	marks []storage.PSTNDeadNode
}

func (f *markRecorder) MarkPSTNDead(_ context.Context, mark storage.PSTNDeadNode) error {
	f.marks = append(f.marks, mark)
	return nil
}

// postMark calls the handler directly: the route is only registered with the
// modem API enabled and sits behind its bearer-key middleware, neither of
// which is what is under test here.
func postMark(t *testing.T, body string) (*httptest.ResponseRecorder, *markRecorder) {
	t.Helper()
	ops := &markRecorder{}
	rec := httptest.NewRecorder()
	New(ops).MarkPSTNDeadHandler(rec, httptest.NewRequest(http.MethodPost, "/api/modem/pstn-dead", strings.NewReader(body)))
	return rec, ops
}

func TestMarkPSTNDeadKind(t *testing.T) {
	cases := []struct {
		name        string
		body        string
		wantStatus  int
		wantKind    string
		wantPhone   string
		wantMessage string
	}{
		{
			// Testers that predate kinds send none.
			name:        "no kind means dead",
			body:        `{"zone":2,"net":5020,"node":100,"reason":"disconnected"}`,
			wantStatus:  http.StatusOK,
			wantKind:    "dead",
			wantMessage: "marked as PSTN dead",
		},
		{
			name:        "explicit dead",
			body:        `{"zone":2,"net":5020,"node":100,"kind":"dead"}`,
			wantStatus:  http.StatusOK,
			wantKind:    "dead",
			wantMessage: "marked as PSTN dead",
		},
		{
			name:        "voice with the phone it was heard on",
			body:        `{"zone":2,"net":5020,"node":100,"kind":"voice","phone":"+7-495-123-45-67","reason":"voice answered"}`,
			wantStatus:  http.StatusOK,
			wantKind:    "voice",
			wantPhone:   "74951234567",
			wantMessage: "voice line, do not call",
		},
		{
			name:       "voice without a phone",
			body:       `{"zone":2,"net":5020,"node":100,"kind":"voice"}`,
			wantStatus: http.StatusOK,
			wantKind:   "voice",
		},
		{
			name:       "unknown kind",
			body:       `{"zone":2,"net":5020,"node":100,"kind":"fax"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			// Not folded to "voice": a kind is a fixed vocabulary.
			name:       "kind is case-sensitive",
			body:       `{"zone":2,"net":5020,"node":100,"kind":"Voice"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing address",
			body:       `{"kind":"voice"}`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec, ops := postMark(t, tc.body)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tc.wantStatus, rec.Body)
			}
			if tc.wantStatus != http.StatusOK {
				if len(ops.marks) != 0 {
					t.Errorf("a refused request reached storage: %+v", ops.marks)
				}
				return
			}
			if len(ops.marks) != 1 {
				t.Fatalf("storage received %d marks, want 1", len(ops.marks))
			}
			mark := ops.marks[0]
			if mark.Zone != 2 || mark.Net != 5020 || mark.Node != 100 {
				t.Errorf("mark address = %d:%d/%d", mark.Zone, mark.Net, mark.Node)
			}
			if mark.Kind != tc.wantKind {
				t.Errorf("kind = %q, want %q", mark.Kind, tc.wantKind)
			}
			if mark.Phone != tc.wantPhone {
				t.Errorf("phone = %q, want %q", mark.Phone, tc.wantPhone)
			}
			if !strings.Contains(rec.Body.String(), tc.wantMessage) {
				t.Errorf("response %s does not contain %q", rec.Body, tc.wantMessage)
			}
		})
	}
}

// markedPSTN serves one node of each mark kind, as storage enriches them.
type markedPSTN struct {
	fakeOps
}

func (f *markedPSTN) GetPSTNNodes(context.Context, int, int, string) ([]storage.PSTNNode, error) {
	return []storage.PSTNNode{
		{Zone: 2, Net: 5020, Node: 100, Phone: "7-495-123-45-67", IsPSTNDead: true, PSTNDeadKind: storage.PSTNDeadKindVoice, PSTNDeadReason: "voice answered"},
		{Zone: 2, Net: 5020, Node: 200, Phone: "7-495-765-43-21", IsPSTNDead: true, PSTNDeadKind: storage.PSTNDeadKindDead},
		{Zone: 2, Net: 5020, Node: 300, Phone: "7-495-000-00-00"},
	}, nil
}

// TestPSTNNodesCarryMarkKind pins the contract older testers rely on: a voice
// line is is_pstn_dead like any other mark, so a caller that only knows that
// flag skips it too, and the kind is there for callers that can tell.
func TestPSTNNodesCarryMarkKind(t *testing.T) {
	_, body := call(t, &markedPSTN{}, http.MethodGet, "/api/nodes/pstn")
	nodes, _ := body["nodes"].([]interface{})
	if len(nodes) != 3 {
		t.Fatalf("got %d nodes, want 3 (body %v)", len(nodes), body)
	}

	want := []struct {
		dead bool
		kind interface{}
	}{{true, "voice"}, {true, "dead"}, {false, nil}}
	for i, w := range want {
		node := nodes[i].(map[string]interface{})
		if node["is_pstn_dead"] != w.dead {
			t.Errorf("node %d: is_pstn_dead = %v, want %v", i, node["is_pstn_dead"], w.dead)
		}
		if node["pstn_dead_kind"] != w.kind {
			t.Errorf("node %d: pstn_dead_kind = %v, want %v", i, node["pstn_dead_kind"], w.kind)
		}
	}
}
