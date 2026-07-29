package web

import (
	"bytes"
	"html/template"
	"strings"
	"testing"
	"time"

	"github.com/nodelistdb/internal/storage"
)

func TestVModemUnavailableRender(t *testing.T) {
	s := &Server{templates: make(map[string]*template.Template), templatesFS: TemplatesFS}
	if err := s.loadTemplates(); err != nil {
		t.Fatalf("loading templates: %v", err)
	}
	tmpl, ok := s.templates["vmodem_unavailable_analytics"]
	if !ok {
		t.Fatal("vmodem_unavailable_analytics template not loaded")
	}

	config := VModemUnavailablePageConfig{
		basePageConfig: basePageConfig{
			PageTitle:       "VMODEM Unavailable",
			PageSubtitle:    template.HTML(`<p class="subtitle">test</p>`),
			StatsHeading:    "Not Confirmed VMODEM",
			InfoText:        []string{"Over the last %d days"},
			EmptyStateTitle: "empty title",
			EmptyStateDesc:  "empty desc",
		},
	}

	data := vmodemUnavailableAnalyticsData{
		Title:      "VMODEM Unavailable",
		ActivePage: "analytics",
		Version:    "test",
		UnconfirmedNodes: []storage.NodeTestResult{
			{
				Zone: 2, Net: 5001, Node: 100, Address: "2:5001/100", Hostname: "example.org",
				TestTime: time.Now(), VModemTested: true, VModemSuccess: false,
				VModemVariant: "down", VModemError: "connection failed: dial tcp: timeout",
			},
			{
				Zone: 3, Net: 54, Node: 0, Address: "3:54/0", Hostname: "bbs.example.org",
				TestTime: time.Now(), VModemTested: true, VModemSuccess: true,
				VModemVariant: "emsi-telnet", VModemSoftware: "Platinum Xpress/WINServer",
				VModemDetail: "IVM announced, actual: emsi-telnet (Platinum Xpress/WINServer)",
			},
			{
				Zone: 1, Net: 1, Node: 1, Address: "1:1/1", Hostname: "legacy.example.org",
				TestTime: time.Now(), VModemTested: true, VModemSuccess: true,
				VModemVariant: "",
			},
			{
				Zone: 4, Net: 1, Node: 1, Address: "4:1/1", Hostname: "noport.example.org",
				TestTime: time.Now(), VModemTested: true, VModemSuccess: false,
				VModemVariant: "vmp", VModemConformant: false, VModemCallOutcome: "no-local-port",
			},
		},
		Days:          30,
		Limit:         1000,
		Config:        config,
		ProcessedInfo: []template.HTML{"Over the last 30 days"},
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	out := buf.String()

	for _, want := range []string{
		"2:5001/100", "Down / unreachable",
		"emsi-telnet", "Platinum Xpress/WINServer",
		"Not classified",
		"Call not attempted (local)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("render missing %q", want)
		}
	}
	if strings.Contains(out, "badge-warning\">vmp<") {
		t.Errorf("no-local-port row must not render as a plain 'vmp' badge")
	}
	// The page answers one question — "announced IVM, answered as something
	// else" — and a second table of nodes nobody probed made it read as "IVM
	// nodes with a problem" when half of it was "IVM nodes we have not looked
	// at". That is a coverage question, not a node question.
	for _, gone := range []string{"Never Tested", "no VModem test recorded"} {
		if strings.Contains(out, gone) {
			t.Errorf("page still renders the untested section (%q); it asks a different question", gone)
		}
	}
}
