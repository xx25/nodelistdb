package web

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/nodelistdb/internal/storage"
)

// TestHostnameCellFallsBackWhenHostnameEmpty covers the AKA-mismatch table's
// hostname column. Aggregated multi-hostname rows are stored with an empty
// `hostname` (only `tested_hostname` is set), and a row can be left with
// neither, so the cell must name the host that actually answered instead of
// rendering a bare N/A.
func TestHostnameCellFallsBackWhenHostnameEmpty(t *testing.T) {
	s, err := New(nil, TemplatesFS, StaticFS)
	if err != nil {
		t.Fatalf("loading templates: %v", err)
	}

	tmpl := s.templates["aka_mismatch_analytics"]
	if tmpl == nil {
		t.Fatal("aka_mismatch_analytics template was not loaded")
	}

	base := storage.NodeTestResult{
		TestTime: time.Date(2026, 7, 24, 21, 46, 2, 0, time.UTC),
		Zone:     2, Net: 455, Node: 19,
		Address:        "2:455/19",
		BinkPTested:    true,
		BinkPSuccess:   true,
		BinkPAddresses: []string{"2:455/20"},
		IsOperational:  true,
	}

	cases := []struct {
		name     string
		mutate   func(*storage.NodeTestResult)
		want     string
		notWant  string
		wantNoIP bool
	}{
		{
			name: "hostname present",
			mutate: func(r *storage.NodeTestResult) {
				r.Hostname = "unnamed.by"
				r.TestedHostname = "unnamed.by"
				r.ResolvedIPv4 = []string{"82.209.239.18"}
			},
			want:    "unnamed.by",
			notWant: "N/A",
		},
		{
			name: "aggregated row keeps only tested_hostname",
			mutate: func(r *storage.NodeTestResult) {
				r.TestedHostname = "unnamed.by"
				r.ResolvedIPv4 = []string{"82.209.239.18"}
				r.IsAggregated = true
			},
			want:    "unnamed.by",
			notWant: "N/A",
		},
		{
			name: "no hostname at all falls back to the resolved IPv4",
			mutate: func(r *storage.NodeTestResult) {
				r.ResolvedIPv4 = []string{"82.209.239.18"}
			},
			want:    "82.209.239.18",
			notWant: "N/A",
		},
		{
			name: "IPv6-only node falls back to the resolved IPv6",
			mutate: func(r *storage.NodeTestResult) {
				r.ResolvedIPv6 = []string{"2001:db8::1"}
			},
			want:    "2001:db8::1",
			notWant: "N/A",
		},
		{
			name:   "nothing to show still renders N/A",
			mutate: func(r *storage.NodeTestResult) {},
			want:   "N/A",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			row := base
			tc.mutate(&row)

			var buf bytes.Buffer
			data := akaMismatchAnalyticsData{
				Title:         "AKA Mismatch",
				MismatchNodes: []storage.NodeTestResult{row},
				Days:          30,
				Limit:         100,
			}
			if err := tmpl.Execute(&buf, data); err != nil {
				t.Fatalf("render failed: %v", err)
			}

			got := buf.String()
			if !strings.Contains(got, tc.want) {
				t.Errorf("expected hostname cell to contain %q", tc.want)
			}
			if tc.notWant != "" && strings.Contains(got, tc.notWant) {
				t.Errorf("expected hostname cell not to contain %q", tc.notWant)
			}
		})
	}
}

// TestReachabilityPagesNameTheTestedHost covers the same fallback on the two
// reachability surfaces, which read the aggregated row directly
// (BuildDetailedTestResultQuery prefers it) and so used to print nothing at all
// for a multi-hostname node.
func TestReachabilityPagesNameTheTestedHost(t *testing.T) {
	s, err := New(nil, TemplatesFS, StaticFS)
	if err != nil {
		t.Fatalf("loading templates: %v", err)
	}

	aggregated := storage.NodeTestResult{
		TestTime: time.Date(2026, 7, 24, 21, 46, 2, 0, time.UTC),
		Zone:     2, Net: 455, Node: 19,
		Address:        "2:455/19",
		TestedHostname: "unnamed.by",
		ResolvedIPv4:   []string{"82.209.239.18"},
		IsAggregated:   true,
		IsOperational:  true,
		BinkPTested:    true,
		BinkPSuccess:   true,
	}

	t.Run("test_detail", func(t *testing.T) {
		var buf bytes.Buffer
		data := map[string]interface{}{
			"Title":      "Test Result Details",
			"TestResult": &aggregated,
			"Address":    "2:455/19",
		}
		if err := s.templates["test_detail"].Execute(&buf, data); err != nil {
			t.Fatalf("render failed: %v", err)
		}
		if !strings.Contains(buf.String(), "unnamed.by") {
			t.Error("expected the tested hostname in the DNS Resolution section")
		}
	})

	t.Run("reachability", func(t *testing.T) {
		var buf bytes.Buffer
		data := map[string]interface{}{
			"Title":             "Reachability History",
			"FilteredNodes":     []storage.NodeTestResult{aggregated},
			"NodesPeriodFilter": 1,
		}
		if err := s.templates["reachability"].Execute(&buf, data); err != nil {
			t.Fatalf("render failed: %v", err)
		}
		if !strings.Contains(buf.String(), "unnamed.by") {
			t.Error("expected the tested hostname next to the node address")
		}
	})
}
