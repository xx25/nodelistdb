package web

import (
	"html/template"
	"strings"
	"testing"
	"time"

	"github.com/nodelistdb/internal/storage"
)

// renderAnalyticsPage renders one config-driven analytics template through the
// real embedded templates.
func renderAnalyticsPage(t *testing.T, name string, data analyticsPageData) string {
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

func sampleAnalyticsNodes() []storage.NodeTestResult {
	return []storage.NodeTestResult{{
		Zone: 2, Net: 5001, Node: 100,
		Address:        "2:5001/100",
		Hostname:       "bbs.example.org",
		TestTime:       time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC),
		ResolvedIPv4:   []string{"192.0.2.10"},
		ResolvedIPv6:   []string{"2001:db8::10"},
		IsOperational:  true,
		BinkPTested:    true,
		BinkPSuccess:   true,
		BinkPVersion:   "binkd/1.1a-115",
		TestedHostname: "bbs.example.org",
		Domain:         "fidonet",
	}}
}

// TestConfigDrivenAnalyticsPagesRender covers the two templates behind the ten
// config-driven node listings. Both read their rows from .Nodes and their page
// copy from .Config, whose six shared fields are promoted from an embedded
// basePageConfig - a promotion that only resolves at render time, so nothing
// but a render catches it breaking.
func TestConfigDrivenAnalyticsPagesRender(t *testing.T) {
	nodes := sampleAnalyticsNodes()

	cases := []struct {
		name        string
		template    string
		config      analyticsPageConfig
		mustContain []string
	}{
		{
			name:     "protocol page",
			template: "unified_analytics",
			config: ProtocolPageConfig{
				basePageConfig: basePageConfig{
					PageTitle:    "BinkP Enabled Nodes",
					PageSubtitle: template.HTML(`<p class="subtitle">Nodes answering BinkP</p>`),
					StatsHeading: "BinkP Enabled",
					InfoText:     []string{`Tested over the last %d days.`},
				},
				ShowVersion:  true,
				VersionField: "BinkPVersion",
			},
			mustContain: []string{
				"BinkP Enabled Nodes",
				"Nodes answering BinkP",
				"Found 1 BinkP Enabled Nodes",
				"Tested over the last 30 days.",
				"2:5001/100",
				// ShowVersion is the one field this config adds on top of the
				// promoted six.
				"binkd/1.1a-115",
			},
		},
		{
			name:     "ipv6 page",
			template: "ipv6_analytics_generic",
			config: IPv6PageConfig{
				basePageConfig: basePageConfig{
					PageTitle:    "IPv6 Enabled Nodes",
					PageSubtitle: template.HTML(`<p class="subtitle">Nodes reachable over IPv6</p>`),
					StatsHeading: "IPv6 Enabled",
					InfoText:     []string{`Resolved over the last %d days.`},
				},
				TableLayout: "standard",
			},
			mustContain: []string{
				"IPv6 Enabled Nodes",
				"Nodes reachable over IPv6",
				"Found 1 IPv6 Enabled Nodes",
				"Resolved over the last 30 days.",
				"2001:db8::10",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			html := renderAnalyticsPage(t, tc.template, analyticsPageData{
				Title:         tc.config.base().PageTitle,
				ActivePage:    "analytics",
				Nodes:         nodes,
				Days:          30,
				Limit:         500,
				Config:        tc.config,
				ProcessedInfo: tc.config.base().processInfoText(30),
			})
			for _, want := range tc.mustContain {
				if !strings.Contains(html, want) {
					t.Errorf("rendered page is missing %q", want)
				}
			}
			if !strings.Contains(html, "</html>") {
				t.Errorf("page was truncated; rendered %d bytes", len(html))
			}
		})
	}
}

// TestAnalyticsEmptyStateRenders pins that the empty state - also promoted from
// basePageConfig - reaches the page.
func TestAnalyticsEmptyStateRenders(t *testing.T) {
	config := ProtocolPageConfig{
		basePageConfig: basePageConfig{
			PageTitle:       "FTP Enabled Nodes",
			StatsHeading:    "FTP Enabled",
			EmptyStateTitle: "No FTP nodes found for the selected period.",
			EmptyStateDesc:  "No node answered on port 21 during this window.",
		},
	}
	html := renderAnalyticsPage(t, "unified_analytics", analyticsPageData{
		Title:      config.PageTitle,
		ActivePage: "analytics",
		Days:       30,
		Config:     config,
	})

	for _, want := range []string{
		"No FTP nodes found for the selected period.",
		"No node answered on port 21 during this window.",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("empty state is missing %q", want)
		}
	}
}
