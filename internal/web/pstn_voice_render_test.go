package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nodelistdb/internal/database"
	"github.com/nodelistdb/internal/storage"
)

func TestApplyPSTNMarks(t *testing.T) {
	nodes := []storage.ModemNoAnswerNode{
		{Zone: 2, Net: 5020, Node: 100, ModemPhoneDialed: "74951234567"}, // voice mark for this number
		{Zone: 2, Net: 5020, Node: 200, ModemPhoneDialed: "74957654321"}, // voice mark for the node's OLD number
		{Zone: 2, Net: 5020, Node: 300, ModemPhoneDialed: "74950000000"}, // dead mark without a phone
		{Zone: 2, Net: 5020, Node: 400},                                  // dialled number unknown
		{Zone: 2, Net: 5020, Node: 500, ModemPhoneDialed: "74959999999"}, // not marked
	}
	marks := []storage.PSTNDeadNode{
		{Zone: 2, Net: 5020, Node: 100, Kind: storage.PSTNDeadKindVoice, Phone: "74951234567", Reason: "voice answered"},
		{Zone: 2, Net: 5020, Node: 200, Kind: storage.PSTNDeadKindVoice, Phone: "74951111111", Reason: "voice answered"},
		{Zone: 2, Net: 5020, Node: 300, Kind: storage.PSTNDeadKindDead, Reason: "disconnected"},
		{Zone: 2, Net: 5020, Node: 400, Kind: storage.PSTNDeadKindVoice, Phone: "74952222222"},
	}

	applyPSTNMarks(nodes, marks)

	want := []struct {
		dead bool
		kind string
	}{
		{true, "voice"},
		{false, ""}, // the mark is about a number these attempts never rang
		{true, "dead"},
		{true, "voice"}, // nothing to compare against: show the mark
		{false, ""},
	}
	for i, w := range want {
		if nodes[i].IsPSTNDead != w.dead || nodes[i].PSTNDeadKind != w.kind {
			t.Errorf("node %d: dead=%v kind=%q, want dead=%v kind=%q",
				nodes[i].Node, nodes[i].IsPSTNDead, nodes[i].PSTNDeadKind, w.dead, w.kind)
		}
	}

	stats := computeModemNoAnswerStats(nodes)
	if stats.DeadCount != 3 || stats.VoiceCount != 2 {
		t.Errorf("DeadCount = %d, VoiceCount = %d; want 3 and 2", stats.DeadCount, stats.VoiceCount)
	}
}

// pstnVoiceStorage serves the two pages that show a voice line.
type pstnVoiceStorage struct {
	Storage
}

func (pstnVoiceStorage) GetModemNoAnswerNodes(context.Context, int, int, bool, string) ([]storage.ModemNoAnswerNode, error) {
	when := time.Date(2026, 11, 23, 22, 15, 0, 0, time.UTC)
	return []storage.ModemNoAnswerNode{
		{Zone: 2, Net: 5020, Node: 100, ModemPhoneDialed: "74951234567", TestTime: when, AttemptCount: 1},
		{Zone: 2, Net: 5020, Node: 300, ModemPhoneDialed: "74950000000", TestTime: when, AttemptCount: 4},
	}, nil
}

func (pstnVoiceStorage) GetPSTNDeadNodes(context.Context) ([]storage.PSTNDeadNode, error) {
	return []storage.PSTNDeadNode{
		{Zone: 2, Net: 5020, Node: 100, Kind: storage.PSTNDeadKindVoice, Phone: "74951234567", Reason: "voice answered"},
		{Zone: 2, Net: 5020, Node: 300, Kind: storage.PSTNDeadKindDead, Reason: "disconnected"},
	}, nil
}

func (pstnVoiceStorage) GetDetailedModemTestResult(context.Context, int, int, int, string) (*storage.ModemTestDetail, error) {
	return &storage.ModemTestDetail{
		Zone: 2, Net: 5020, Node: 100, Address: "2:5020/100",
		TestTime:      time.Date(2026, 11, 23, 22, 15, 0, 0, time.UTC),
		PhoneDialed:   "74951234567",
		VoiceAnswered: true,
		FailureDetail: "voice answered",
	}, nil
}

func (pstnVoiceStorage) GetNodeHistory(context.Context, int, int, int, string) ([]database.Node, error) {
	return nil, nil
}

// TestVoiceLineIsShownAsSuch renders both pages through their handlers: the
// no-answer report must tell a voice line from a dead number, and the test
// detail page must say a person answered and show the modem's reason.
func TestVoiceLineIsShownAsSuch(t *testing.T) {
	s := newTestServer(t, pstnVoiceStorage{})

	t.Run("no-answer report", func(t *testing.T) {
		rec := httptest.NewRecorder()
		s.ModemNoAnswerAnalyticsHandler(rec, httptest.NewRequest(http.MethodGet, "/analytics/pstn-no-answer", nil))
		html := rec.Body.String()
		if rec.Code != http.StatusOK || !strings.Contains(html, "</html>") {
			t.Fatalf("status %d, %d bytes rendered", rec.Code, len(html))
		}
		for _, want := range []string{
			`title="voice answered">Voice line, do not call</span>`,
			`title="disconnected">PSTN Dead</span>`,
			"Of Those: Voice Line, Do Not Call",
		} {
			if !strings.Contains(html, want) {
				t.Errorf("page is missing %q", want)
			}
		}
		if got := strings.Count(html, ">Voice line, do not call</span>"); got != 1 {
			t.Errorf("voice badge rendered %d times, want 1 (the dead node must not get it)", got)
		}
	})

	t.Run("modem test detail", func(t *testing.T) {
		rec := httptest.NewRecorder()
		s.ModemTestDetailHandler(rec, httptest.NewRequest(http.MethodGet,
			"/reachability/modem-test?zone=2&net=5020&node=100&time=2026-11-23T22:15:00Z", nil))
		html := rec.Body.String()
		if rec.Code != http.StatusOK || !strings.Contains(html, "</html>") {
			t.Fatalf("status %d, %d bytes rendered", rec.Code, len(html))
		}
		for _, want := range []string{"A person: voice line, do not call", "Failure Detail:", "voice answered"} {
			if !strings.Contains(html, want) {
				t.Errorf("page is missing %q", want)
			}
		}
	})
}
