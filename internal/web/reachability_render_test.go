package web

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nodelistdb/internal/database"
	"github.com/nodelistdb/internal/storage"
)

// renderPage executes one embedded template against a payload and returns the
// HTML, failing the test on any execution error.
//
// html/template writes as it goes, so a payload the template cannot handle
// produces a partial page rather than nothing: by the time Execute returns an
// error the status line and the opening markup are already on the wire. That
// is why these tests assert on Execute's error rather than on the handler's
// status code - the status code is 200 either way.
func renderPage(t *testing.T, name string, data any) string {
	t.Helper()
	s := newTestServer(t, nil)
	tmpl, ok := s.templates[name]
	if !ok {
		t.Fatalf("template %q not loaded", name)
	}
	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		t.Fatalf("rendering %s: %v", name, err)
	}
	return buf.String()
}

func sampleReachabilityNode() storage.NodeTestResult {
	return storage.NodeTestResult{
		Zone: 2, Net: 5001, Node: 100,
		Address:        "2:5001/100",
		Hostname:       "bbs.example.org",
		TestedHostname: "bbs.example.org",
		TestTime:       time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC),
		ResolvedIPv4:   []string{"192.0.2.10"},
		ResolvedIPv6:   []string{"2001:db8::10"},
		IsOperational:  true,
		BinkPTested:    true,
		BinkPSuccess:   true,
		BinkPVersion:   "binkd/1.1a-115",
		Country:        "Russia",
		CountryCode:    "RU",
		Domain:         "fidonet",
		HostnameIndex:  0,
	}
}

func sampleReachabilityTrends() []storage.ReachabilityTrend {
	return []storage.ReachabilityTrend{{
		Date:             time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC),
		TotalNodes:       120,
		OperationalNodes: 100,
		FailedNodes:      20,
		SuccessRate:      83.3,
		AvgResponseMs:    142.5,
	}}
}

// TestReachabilityTemplateRendersEveryPayload pins reachability.html against
// all three payloads its two handlers build.
//
// The template is the largest in the tree and the only one with no render
// coverage, and it is reached by three payloads that share no keys beyond
// Title/Version/ActivePage: the trends+lists page, the same page under
// filters, and the bare lookup form served when no address was supplied. Each
// payload is a map[string]interface{}, so a key one branch sets and another
// omits is not a compile error anywhere - it surfaces only here.
func TestReachabilityTemplateRendersEveryPayload(t *testing.T) {
	nodes := []storage.NodeTestResult{sampleReachabilityNode()}

	cases := []struct {
		name        string
		data        map[string]interface{}
		mustContain []string
	}{
		{
			// ReachabilityHandler, no filters: trends chart plus the recently
			// tested list built from the handler's own defaults. The node list
			// is asserted through the sample's hostname rather than its address,
			// because the address also appears in the lookup form's placeholder
			// and so cannot tell a rendered list from an empty one.
			name: "overview with default list",
			data: map[string]interface{}{
				"Title":              "Reachability History",
				"Version":            "test",
				"ActivePage":         "reachability",
				"Trends":             sampleReachabilityTrends(),
				"StatusFilter":       "",
				"TrendsPeriodFilter": 0,
				"NodesPeriodFilter":  1,
				"ProtocolFilter":     "",
				"LimitFilter":        10,
				"FilteredNodes":      nodes,
			},
			mustContain: []string{"2:5001/100", "bbs.example.org", "2026-07-28"},
		},
		{
			// The same payload with no tests inside the window: the list is
			// empty rather than absent, so the no-results panel must show.
			name: "overview with an empty default list",
			data: map[string]interface{}{
				"Title":              "Reachability History",
				"Version":            "test",
				"ActivePage":         "reachability",
				"Trends":             sampleReachabilityTrends(),
				"StatusFilter":       "",
				"TrendsPeriodFilter": 0,
				"NodesPeriodFilter":  1,
				"ProtocolFilter":     "",
				"LimitFilter":        10,
				"FilteredNodes":      []storage.NodeTestResult{},
			},
			mustContain: []string{"No nodes found"},
		},
		{
			// ReachabilityHandler with filters applied: same key, different
			// filter values echoed back into the form's selects.
			name: "overview with filters applied",
			data: map[string]interface{}{
				"Title":              "Reachability History",
				"Version":            "test",
				"ActivePage":         "reachability",
				"Trends":             sampleReachabilityTrends(),
				"StatusFilter":       "operational",
				"TrendsPeriodFilter": 90,
				"NodesPeriodFilter":  7,
				"ProtocolFilter":     "binkp",
				"LimitFilter":        50,
				"FilteredNodes":      nodes,
			},
			mustContain: []string{"bbs.example.org"},
		},
		{
			// ReachabilityNodeHandler with no address: the lookup form. This
			// payload carries three keys, but the template gates the whole
			// filter form on `not .Address` - so this three-key payload also
			// renders the four selects that read StatusFilter,
			// NodesPeriodFilter, ProtocolFilter and LimitFilter, none of which
			// it sets. Asserting on the selects keeps that path covered rather
			// than merely reached.
			name: "lookup form with no address",
			data: map[string]interface{}{
				"Title":      "Node Reachability History",
				"Version":    "test",
				"ActivePage": "reachability",
			},
			mustContain: []string{"Operational Only", "Last 24 hours", "BinkP Only", "25 nodes"},
		},
		{
			// ReachabilityNodeHandler with an address: per-node history.
			name: "node history",
			data: map[string]interface{}{
				"Title":      "Node Reachability History",
				"Version":    "test",
				"ActivePage": "reachability",
				"Zone":       2,
				"Net":        5001,
				"Node":       100,
				"Address":    "2:5001/100",
				"Days":       30,
				"History":    nodes,
				"Stats":      &storage.NodeReachabilityStats{Zone: 2, Net: 5001, Node: 100, TotalTests: 10, FullySuccessfulTests: 8, FailedTests: 1, PartiallyFailedTests: 1, SuccessRate: 80, AverageResponseMs: 142.5},
				"NodeInfo":   &database.Node{Zone: 2, Net: 5001, Node: 100, SystemName: "Example BBS", Location: "Moscow", SysopName: "Test Sysop"},
				"HasResults": true,
			},
			mustContain: []string{"2:5001/100", "Example BBS"},
		},
		{
			// Same handler, node with no test history at all: Stats is nil
			// because GetNodeReachabilityStats returns nil when the daemon has
			// not tested the node inside the window, and NodeInfo is nil when
			// the address is not in the nodelist.
			name: "node history with no results",
			data: map[string]interface{}{
				"Title":      "Node Reachability History",
				"Version":    "test",
				"ActivePage": "reachability",
				"Zone":       2,
				"Net":        5001,
				"Node":       100,
				"Address":    "2:5001/100",
				"Days":       30,
				"History":    []storage.NodeTestResult{},
				"Stats":      (*storage.NodeReachabilityStats)(nil),
				"NodeInfo":   (*database.Node)(nil),
				"HasResults": false,
			},
			mustContain: []string{"2:5001/100"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			html := renderPage(t, "reachability", tc.data)
			for _, want := range tc.mustContain {
				if !strings.Contains(html, want) {
					t.Errorf("rendered page missing %q", want)
				}
			}
		})
	}
}

// reachabilityStub serves ReachabilityHandler's three storage calls and records
// the arguments of each node search.
type reachabilityStub struct {
	Storage

	trends    []storage.ReachabilityTrend
	nodes     []storage.NodeTestResult
	searchLog []reachabilitySearch
}

type reachabilitySearch struct {
	operational bool
	limit       int
	days        int
	domain      string
}

func (s *reachabilityStub) GetReachabilityTrends(ctx context.Context, days int, domain string) ([]storage.ReachabilityTrend, error) {
	return s.trends, nil
}

func (s *reachabilityStub) GetReachabilityTrendsAllTime(ctx context.Context, domain string) ([]storage.ReachabilityTrend, error) {
	return s.trends, nil
}

func (s *reachabilityStub) SearchNodesByReachability(ctx context.Context, operational bool, limit int, days int, domain string) ([]storage.NodeTestResult, error) {
	s.searchLog = append(s.searchLog, reachabilitySearch{operational: operational, limit: limit, days: days, domain: domain})
	if !operational {
		return nil, nil
	}
	return s.nodes, nil
}

// TestReachabilityHandlerOpensOnRecentNodes covers the whole handler for a bare
// GET, which the render tests above cannot: they build the payload themselves,
// so they pass whether or not the handler produces it.
//
// That is precisely how the page shipped broken - the handler filled
// OperationalNodes/FailedNodes on the unfiltered path while the template read
// only FilteredNodes, and nothing failed, because the payload is a
// map[string]interface{} and the mismatch is invisible to the compiler and to
// any test that supplies its own map. The assertion is therefore on the handler's
// own output: a visitor who applies no filters sees nodes, not the empty panel.
func TestReachabilityHandlerOpensOnRecentNodes(t *testing.T) {
	ops := &reachabilityStub{
		trends: sampleReachabilityTrends(),
		nodes:  []storage.NodeTestResult{sampleReachabilityNode()},
	}
	s := newTestServer(t, ops)

	rec := httptest.NewRecorder()
	s.ReachabilityHandler(rec, httptest.NewRequest("GET", "/reachability", nil))

	body := rec.Body.String()
	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(body, "bbs.example.org") {
		t.Error("a bare /reachability visit did not render the recently tested node")
	}
	if strings.Contains(body, "No nodes found") {
		t.Error("a bare /reachability visit rendered the empty-results panel")
	}

	// Both statuses are searched, so the default list can mix them, and each
	// search asks for more rows than the page shows - the protocol filter runs
	// in Go over whatever came back.
	if len(ops.searchLog) != 2 {
		t.Fatalf("node searches = %d, want 2 (operational and failed)", len(ops.searchLog))
	}
	if ops.searchLog[0].operational == ops.searchLog[1].operational {
		t.Error("both node searches asked for the same status")
	}
	for _, got := range ops.searchLog {
		if got.limit < reachabilityFetchFloor {
			t.Errorf("search(operational=%v) limit = %d, want at least the %d-row pre-filter floor", got.operational, got.limit, reachabilityFetchFloor)
		}
		if got.days != 1 {
			t.Errorf("search(operational=%v) days = %d, want the 1-day default window", got.operational, got.days)
		}
	}
}

// TestTestDetailTemplatesRender pins the two detail pages renderTestDetail
// serves. Both take .TestResult as an `any` the handler filled from a
// different store, so the template is the only thing that says which concrete
// type each one expects.
func TestTestDetailTemplatesRender(t *testing.T) {
	nodeInfo := &database.Node{Zone: 2, Net: 5001, Node: 100, SystemName: "Example BBS", Location: "Moscow", SysopName: "Test Sysop"}

	t.Run("test_detail", func(t *testing.T) {
		result := sampleReachabilityNode()
		html := renderPage(t, "test_detail", map[string]interface{}{
			"Title":      "Test Result Details",
			"Version":    "test",
			"ActivePage": "reachability",
			"TestResult": &result,
			"NodeInfo":   nodeInfo,
			"Address":    "2:5001/100",
		})
		for _, want := range []string{"2:5001/100", "bbs.example.org", "binkd/1.1a-115"} {
			if !strings.Contains(html, want) {
				t.Errorf("rendered page missing %q", want)
			}
		}
	})

	t.Run("test_detail without node info", func(t *testing.T) {
		result := sampleReachabilityNode()
		renderPage(t, "test_detail", map[string]interface{}{
			"Title":      "Test Result Details",
			"Version":    "test",
			"ActivePage": "reachability",
			"TestResult": &result,
			"NodeInfo":   (*database.Node)(nil),
			"Address":    "2:5001/100",
		})
	})

	t.Run("modem_test_detail", func(t *testing.T) {
		html := renderPage(t, "modem_test_detail", map[string]interface{}{
			"Title":      "Modem Test Details",
			"Version":    "test",
			"ActivePage": "reachability",
			"TestResult": &storage.ModemTestDetail{
				Zone: 2, Net: 5001, Node: 100,
				Address:      "2:5001/100",
				TestTime:     time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC),
				PhoneDialed:  "+79001234567",
				ConnectSpeed: 33600,
				ResponseType: "EMSI",
				MailerInfo:   "binkd/1.1a",
			},
			"NodeInfo": nodeInfo,
			"Address":  "2:5001/100",
		})
		if !strings.Contains(html, "2:5001/100") {
			t.Error("rendered page missing the node address")
		}
	})
}
