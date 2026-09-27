package web

import (
	"bytes"
	"fmt"
	"html/template"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nodelistdb/internal/emailflags"
	"github.com/nodelistdb/internal/storage"
)

// renderEmailAnalytics loads the real embedded templates and renders the
// FidoNet-over-email report for the given page data.
func renderEmailAnalytics(t *testing.T, page emailAnalyticsPage) string {
	t.Helper()

	s := &Server{templates: make(map[string]*template.Template), templatesFS: TemplatesFS}
	if err := s.loadTemplates(); err != nil {
		t.Fatalf("loading templates: %v", err)
	}
	tmpl, ok := s.templates["email_analytics"]
	if !ok {
		t.Fatal("email_analytics template not loaded")
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, page); err != nil {
		t.Fatalf("rendering email_analytics: %v", err)
	}
	return buf.String()
}

// samplePage builds a page with one node per interesting shape.
func samplePage() emailAnalyticsPage {
	date := time.Date(2026, 7, 26, 0, 0, 0, 0, time.UTC)

	nodes := []storage.EmailCapableNode{
		{
			Domain: "fidonet", Zone: 2, Net: 5001, Node: 100,
			SystemName: "Test_System", Location: "Moscow", SysopName: "A_Sysop",
			NodelistDate: date, NodeType: "Node",
			Capabilities: []emailflags.Capability{
				{Flag: "IEM", Standard: true, Addresses: []string{"sysop@example.net"}, Source: emailflags.SourceExplicit, Occurrences: 1},
				{Flag: "IMI", Standard: true, Addresses: []string{"sysop@example.net"}, Source: emailflags.SourceIEMDefault, Occurrences: 1},
			},
		},
		{
			Domain: "fidonet", Zone: 2, Net: 5020, Node: 113,
			SystemName: "SEAT_Node", Location: "Berlin", SysopName: "B_Sysop",
			NodelistDate: date, NodeType: "Node",
			Capabilities: []emailflags.Capability{
				{Flag: "ISE", Standard: true, ReceiptRequired: true, WireProtocolSpecified: true,
					Addresses: []string{"seat@example.org"}, Source: emailflags.SourceExplicit, Occurrences: 1},
			},
		},
		{
			Domain: "fidonet", Zone: 2, Net: 450, Node: 1024,
			SystemName: "Unresolved_Node", Location: "Kyiv", SysopName: "C_Sysop",
			NodelistDate: date, NodeType: "Node",
			Capabilities: []emailflags.Capability{
				{Flag: "IMI", Standard: true, Source: emailflags.SourceUnresolved, Occurrences: 1},
			},
		},
		{
			Domain: "fidonet", Zone: 2, Net: 5030, Node: 55,
			SystemName: "NonStandard_Node", Location: "Praha", SysopName: "D_Sysop",
			NodelistDate: date, NodeType: "Node",
			Capabilities: []emailflags.Capability{
				{Flag: "EMA", Addresses: []string{"other@example.com"}, Source: emailflags.SourceExplicit, Occurrences: 1,
					Malformed: []string{"broken@@thing"}},
			},
		},
		{
			// Nothing on this node's flags carried an address; it came from
			// the Location field, so the row must say so.
			Domain: "fidonet", Zone: 2, Net: 460, Node: 58,
			SystemName: "Guessed_Node", Location: "Odesa", SysopName: "E_Sysop",
			NodelistDate: date, NodeType: "Node",
			Capabilities: []emailflags.Capability{
				{Flag: "IUC", Standard: true, Addresses: []string{"guess@example.ua"},
					Source: emailflags.SourceLocationField, Occurrences: 1},
			},
		},
	}
	for i := range nodes {
		// Mirror what the storage layer derives.
		n := &nodes[i]
		for _, c := range n.Capabilities {
			n.FlagNames = append(n.FlagNames, c.Flag)
			if c.Standard {
				n.HasStandardMethod = true
			} else {
				n.HasNonStandardFlag = true
			}
			if c.ReceiptRequired {
				n.ReceiptCapable = true
			}
			if c.WireProtocolSpecified {
				n.WireProtocolSpecified = true
			}
			if len(c.Malformed) > 0 {
				n.HasMalformed = true
			}
			for _, a := range c.Addresses {
				n.Addresses = appendIfMissing(n.Addresses, a)
			}
		}
		n.Resolved = len(n.Addresses) > 0
		for _, a := range n.Addresses {
			if d := emailflags.MailDomain(a); d != "" {
				n.MailDomains = appendIfMissing(n.MailDomains, d)
			}
		}
	}

	// The first two nodes can be reached another way; the rest only by email.
	nodes[0].OtherTransports = []string{"IBN"}
	nodes[1].OtherTransports = []string{"IBN", "PSTN"}
	for i := 2; i < len(nodes); i++ {
		nodes[i].EmailOnly = true
	}

	// Give the first node a DNS verdict, and the SEAT node a dead one.
	nodes[0].Endpoint = []storage.EmailEndpointStatus{
		{Address: "sysop@example.net", MailDomain: "example.net", Status: storage.EmailDomainStatusOK, Detail: "1 MX host", CheckTime: date},
	}
	nodes[1].Endpoint = []storage.EmailEndpointStatus{
		{Address: "seat@example.org", MailDomain: "example.org", Status: storage.EmailDomainStatusNoDomain, Detail: "NXDOMAIN", CheckTime: date, Stale: true},
	}

	return emailAnalyticsPage{
		Title:      "FidoNet over Email",
		ActivePage: "analytics",
		Nodes:      nodes,
		AllCount:   len(nodes),
		Stats:      computeEmailStats(nodes),
		Trend: []storage.EmailFlagTrendPoint{
			{Year: 2001, Flags: map[string]int{"IEM": 114, "IMI": 141, "ITX": 120, "ISE": 19, "IUC": 147, "EMA": 9, "EVY": 5}, AnyEmail: 296, TotalNodes: 13792},
			{Year: 2026, Flags: map[string]int{"IEM": 50, "IMI": 82, "ITX": 3, "ISE": 0, "IUC": 3, "EMA": 4, "EVY": 0}, AnyEmail: 102, TotalNodes: 1229},
		},
		FlagOrder: storage.EmailFlagOrder(),
		Limit:     5000,
	}
}

func appendIfMissing(list []string, v string) []string {
	for _, e := range list {
		if e == v {
			return list
		}
	}
	return append(list, v)
}

func TestEmailAnalyticsRenders(t *testing.T) {
	html := renderEmailAnalytics(t, samplePage())

	mustContain := []string{
		"FidoNet over Email",
		"2:5001/100",
		// Domains, not mailboxes: the page must not publish addresses.
		"example.net",
		"example.org",
		// Provenance is shown when the address was not on the flag itself.
		"via location field",
		// Unresolved capability is labelled, not hidden.
		"unresolved",
		// Malformed values stay visible, with the mailbox redacted.
		"…@thing",
		// Endpoint verdicts.
		"routable",
		"no such domain",
		// The reference table explains each flag.
		"FTS-1025",
		"non-standard",
		// The chart's table view exists (required as contrast relief).
		"Show the same data as a table",
	}
	for _, want := range mustContain {
		if !strings.Contains(html, want) {
			t.Errorf("rendered page is missing %q", want)
		}
	}

	// The chart must carry one dataset per flag, coloured consistently.
	for flag, color := range emailSeriesColors {
		if !strings.Contains(html, "label: '"+flag+"'") {
			t.Errorf("chart has no dataset for %s", flag)
		}
		if !strings.Contains(html, color) {
			t.Errorf("colour %s for %s missing from page", color, flag)
		}
	}
}

func TestEmailAnalyticsStats(t *testing.T) {
	stats := samplePage().Stats

	if stats.TotalNodes != 5 {
		t.Errorf("TotalNodes = %d, want 5", stats.TotalNodes)
	}
	if stats.Resolved != 4 {
		t.Errorf("Resolved = %d, want 4", stats.Resolved)
	}
	if stats.Unresolved != 1 {
		t.Errorf("Unresolved = %d, want 1", stats.Unresolved)
	}
	if stats.ReceiptCapable != 1 {
		t.Errorf("ReceiptCapable = %d, want 1 (only the ISE node)", stats.ReceiptCapable)
	}
	if stats.NonStandardOnly != 1 {
		t.Errorf("NonStandardOnly = %d, want 1 (the EMA-only node)", stats.NonStandardOnly)
	}
	if stats.Malformed != 1 {
		t.Errorf("Malformed = %d, want 1", stats.Malformed)
	}
	if stats.DistinctDomains != 4 {
		t.Errorf("DistinctDomains = %d, want 4", stats.DistinctDomains)
	}

	// Every flag gets a reference row, present or not, so the table is stable.
	if len(stats.FlagReferences) != len(storage.EmailFlagOrder()) {
		t.Fatalf("FlagReferences = %d rows, want %d", len(stats.FlagReferences), len(storage.EmailFlagOrder()))
	}
	byFlag := make(map[string]EmailFlagReference, len(stats.FlagReferences))
	for _, ref := range stats.FlagReferences {
		byFlag[ref.Flag] = ref
	}
	if got := byFlag["IMI"].Count; got != 2 {
		t.Errorf("IMI count = %d, want 2", got)
	}
	if got := byFlag["ISE"].Count; got != 1 {
		t.Errorf("ISE count = %d, want 1", got)
	}
	if byFlag["EMA"].Standard || byFlag["EVY"].Standard {
		t.Error("EMA and EVY must not be marked as FTSC standard flags")
	}
	if byFlag["IEM"].ReceiptRequired || byFlag["IMI"].ReceiptRequired || byFlag["IUC"].ReceiptRequired {
		t.Error("only ITX and ISE require receipts")
	}
	if !byFlag["ITX"].ReceiptRequired {
		t.Error("ITX requires receipts per FTS-5001")
	}
}

// TestEmailAnalyticsEndpointColumn covers a node publishing two mail domains
// where only one has been swept. The unchecked one must be visible and both
// must be labelled, so a partly-verified node cannot read as fully verified.
func TestEmailAnalyticsEndpointColumn(t *testing.T) {
	date := time.Date(2026, 7, 26, 0, 0, 0, 0, time.UTC)

	node := storage.EmailCapableNode{
		Domain: "fidonet", Zone: 2, Net: 5001, Node: 100,
		SystemName: "Two_Domains", Location: "Moscow", SysopName: "A_Sysop",
		NodelistDate: date, NodeType: "Node",
		Addresses:   []string{"a@checked.example", "b@unswept.example"},
		MailDomains: []string{"checked.example", "unswept.example"},
		Resolved:    true,
		FlagNames:   []string{"IEM"},
		Capabilities: []emailflags.Capability{
			{Flag: "IEM", Standard: true, Occurrences: 1,
				Addresses: []string{"a@checked.example", "b@unswept.example"},
				Source:    emailflags.SourceExplicit},
		},
		HasStandardMethod: true,
		Endpoint: []storage.EmailEndpointStatus{
			{Address: "a@checked.example", MailDomain: "checked.example",
				Status: storage.EmailDomainStatusOK, Detail: "1 MX host", CheckTime: date},
			// Never swept: no status at all.
			{Address: "b@unswept.example", MailDomain: "unswept.example"},
			// A transient DNS failure: not a verdict about the domain.
			{Address: "c@flaky.example", MailDomain: "flaky.example",
				Status: storage.EmailDomainStatusError, Detail: "DNS lookup failed", Stale: true},
		},
	}

	nodes := []storage.EmailCapableNode{node}
	html := renderEmailAnalytics(t, emailAnalyticsPage{
		Title:      "FidoNet over Email",
		ActivePage: "analytics",
		Nodes:      nodes,
		AllCount:   len(nodes),
		Stats:      computeEmailStats(nodes),
		FlagOrder:  storage.EmailFlagOrder(),
	})

	for _, want := range []string{
		"routable",
		"not checked",
		// Both domains are named, because the node has more than one.
		"checked.example",
		"unswept.example",
		// A transient failure must not render as a bare "error".
		"check failed",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("endpoint column is missing %q", want)
		}
	}
}

func TestEmailAnalyticsEmptyState(t *testing.T) {
	html := renderEmailAnalytics(t, emailAnalyticsPage{
		Title:      "FidoNet over Email",
		ActivePage: "analytics",
		Stats:      computeEmailStats(nil),
		FlagOrder:  storage.EmailFlagOrder(),
	})

	if !strings.Contains(html, "No nodes advertising email transport were found") {
		t.Error("empty state message missing")
	}
}

// TestEmailAnalyticsFlagsTable pins the reference table's shape: four columns,
// with the two per-flag properties folded into one Notes cell.
func TestEmailAnalyticsFlagsTable(t *testing.T) {
	html := renderEmailAnalytics(t, samplePage())

	if strings.Contains(html, "Complete wire spec") {
		t.Error("the wire-spec column should be gone; the fact belongs in the Meaning text")
	}
	if !strings.Contains(html, "<th>Notes</th>") {
		t.Error("the merged Notes column header is missing")
	}
	// The wire-spec fact still has to reach the reader, via ISE's description.
	if !strings.Contains(html, "FTS-1025") {
		t.Error("ISE's description should still name FTS-1025")
	}

	// Every flag keeps a row, including those with no nodes on this nodelist:
	// ISE reading zero is itself the report's headline finding.
	for _, flag := range storage.EmailFlagOrder() {
		if !strings.Contains(html, ">"+flag+"</span>") {
			t.Errorf("flag %s has no row in the reference table", flag)
		}
	}

	// Notes carries the defining document, and receipts only where owed.
	if !strings.Contains(html, "FTS-5001") || !strings.Contains(html, "non-standard") {
		t.Error("Notes should distinguish FTS-5001 flags from non-standard ones")
	}
	if got := strings.Count(html, ">receipts</span>"); got != 2 {
		t.Errorf("receipts marked on %d flags, want 2 (ITX and ISE only)", got)
	}
}

// TestEmailAnalyticsTableWidths pins the column sizing: the three token columns
// shrink to their content so Meaning, the only one holding prose, gets the rest.
func TestEmailAnalyticsTableWidths(t *testing.T) {
	html := renderEmailAnalytics(t, samplePage())

	for _, want := range []string{
		`<col class="col-flag">`,
		`<col class="col-nodes">`,
		`<col class="col-notes">`,
		`<col class="col-meaning">`,
		".flags-table .col-meaning { width: 97%; }",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("column sizing is missing %q", want)
		}
	}

	// The ISE stat tile is gone; the flag row still carries the fact.
	if strings.Contains(html, "Fully specified wire format") {
		t.Error("the ISE wire-format stat tile should be removed")
	}
	if !strings.Contains(html, "FTS-1025") {
		t.Error("ISE's description must still name FTS-1025")
	}
}

// TestEmailAnalyticsListsEveryMailDomain pins that the domain list is complete
// rather than a top-N slice, and that it holds bare domains, never addresses.
func TestEmailAnalyticsListsEveryMailDomain(t *testing.T) {
	date := time.Date(2026, 7, 26, 0, 0, 0, 0, time.UTC)

	// More domains than the old 20-item cap allowed.
	var nodes []storage.EmailCapableNode
	for i := 0; i < 25; i++ {
		addr := fmt.Sprintf("sysop@domain%02d.example", i)
		nodes = append(nodes, storage.EmailCapableNode{
			Domain: "fidonet", Zone: 2, Net: 5001, Node: 100 + i,
			SystemName: "S", Location: "L", SysopName: "P",
			NodelistDate: date, NodeType: "Node",
			Addresses: []string{addr}, MailDomains: []string{emailflags.MailDomain(addr)},
			Resolved: true, FlagNames: []string{"IEM"},
			HasStandardMethod: true,
			Capabilities: []emailflags.Capability{
				{Flag: "IEM", Standard: true, Occurrences: 1,
					Addresses: []string{addr}, Source: emailflags.SourceExplicit},
			},
		})
	}

	stats := computeEmailStats(nodes)
	if stats.DistinctDomains != 25 {
		t.Fatalf("DistinctDomains = %d, want 25", stats.DistinctDomains)
	}
	if len(stats.DomainCounts) != 25 {
		t.Errorf("DomainCounts holds %d entries, want all 25 (no top-N truncation)", len(stats.DomainCounts))
	}
	for _, dc := range stats.DomainCounts {
		if strings.Contains(dc.Domain, "@") {
			t.Errorf("domain list entry %q is an email address, not a domain", dc.Domain)
		}
	}

	html := renderEmailAnalytics(t, emailAnalyticsPage{
		Title: "FidoNet over Email", ActivePage: "analytics",
		Nodes: nodes, AllCount: len(nodes), Stats: stats, FlagOrder: storage.EmailFlagOrder(),
	})
	if strings.Contains(html, "top 20") {
		t.Error("the heading should no longer advertise a truncated list")
	}
	for i := 0; i < 25; i++ {
		if want := fmt.Sprintf("domain%02d.example", i); !strings.Contains(html, want) {
			t.Errorf("domain %s is missing from the rendered list", want)
		}
	}
}

// TestEmailAnalyticsPublishesNoMailboxes pins that the report shows mail
// domains and never a full address. The nodelist publishes the address, but a
// web page is far easier to harvest, and every question this page answers
// turns on the domain alone.
func TestEmailAnalyticsPublishesNoMailboxes(t *testing.T) {
	html := renderEmailAnalytics(t, samplePage())

	if !strings.Contains(html, "<th>Mail domain</th>") {
		t.Error("the column should be headed Mail domain, not Email address")
	}

	// Every mailbox in the fixtures must be absent, while its domain is shown.
	for _, addr := range []string{
		"sysop@example.net", "seat@example.org",
		"other@example.com", "guess@example.ua",
	} {
		if strings.Contains(html, addr) {
			t.Errorf("the page published the full address %q", addr)
		}
		if d := emailflags.MailDomain(addr); !strings.Contains(html, d) {
			t.Errorf("domain %q of %q is missing", d, addr)
		}
	}

	// A malformed value still has to be diagnosable, but with the mailbox hidden.
	if strings.Contains(html, "broken@@thing") {
		t.Error("a malformed value was published with its mailbox intact")
	}
	if !strings.Contains(html, "…@thing") {
		t.Error("the malformed value should appear with its local part redacted")
	}
}

func TestRedactLocalPart(t *testing.T) {
	tests := []struct{ in, want string }{
		{"user@example.net:extra", "…@example.net:extra"},
		{"user@example.net", "…@example.net"},
		{"weird@user@example.net", "…@example.net"},
		{"no-at-sign", "no-at-sign"},
		{"@leading", "@leading"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := emailflags.RedactLocalPart(tt.in); got != tt.want {
			t.Errorf("RedactLocalPart(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestEmailAnalyticsEmailOnlyToggle covers the unfiltered view: both toggle
// links with their counts, the Also via column, and the summary tile.
func TestEmailAnalyticsEmailOnlyToggle(t *testing.T) {
	page := samplePage()
	page.EmailOnlyCount = countEmailOnly(page.Nodes)
	page.AllURL = "/analytics/email"
	page.EmailOnlyURL = "/analytics/email?only=email"
	html := renderEmailAnalytics(t, page)

	for _, want := range []string{
		`href="/analytics/email?only=email"`,
		"All nodes with email flags (5)",
		"Email-only nodes (3)",
		"Also via",
		"Email-only (no IP, no PSTN)",
		`<span class="badge badge-secondary">PSTN</span>`,
		"email only",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("unfiltered page is missing %q", want)
		}
	}
	if page.Stats.EmailOnly != 3 {
		t.Errorf("Stats.EmailOnly = %d, want 3", page.Stats.EmailOnly)
	}
}

// TestEmailAnalyticsEmailOnlyFiltered covers the filtered view, including the
// case where the network has email nodes but none that are email-only: that
// must not read as "no nodes advertise email".
func TestEmailAnalyticsEmailOnlyFiltered(t *testing.T) {
	all := samplePage().Nodes
	shown := filterEmailOnly(all)
	if len(shown) != 3 {
		t.Fatalf("filterEmailOnly kept %d nodes, want 3", len(shown))
	}

	page := samplePage()
	page.Nodes = shown
	page.Stats = computeEmailStats(shown)
	page.AllCount = len(all)
	page.EmailOnlyCount = len(shown)
	page.EmailOnly = true
	html := renderEmailAnalytics(t, page)

	if strings.Contains(html, "Also via") {
		t.Error("the Also via column is empty by definition in the email-only view and should be hidden")
	}
	if strings.Contains(html, "2:5001/100") {
		t.Error("a node reachable over IBN is listed in the email-only view")
	}
	if !strings.Contains(html, "not only the email-only ones") {
		t.Error("the trend should say it does not follow the filter")
	}

	page.Nodes = nil
	page.Stats = computeEmailStats(nil)
	page.EmailOnlyCount = 0
	html = renderEmailAnalytics(t, page)
	if strings.Contains(html, "No nodes advertising email transport were found") {
		t.Error("an empty email-only view must not claim the network has no email nodes")
	}
	if !strings.Contains(html, "can also be reached another way") {
		t.Error("empty email-only view message missing")
	}
}

// TestEmailPageURL pins that each toggle keeps the other parameters.
func TestEmailPageURL(t *testing.T) {
	r := httptest.NewRequest("GET", "/analytics/email?fields=1&domain=fsxnet", nil)
	if got := emailPageURL(r, "only", "email"); got != "/analytics/email?domain=fsxnet&fields=1&only=email" {
		t.Errorf("setting only: got %q", got)
	}

	r = httptest.NewRequest("GET", "/analytics/email?only=email&fields=1", nil)
	if got := emailPageURL(r, "fields", ""); got != "/analytics/email?only=email" {
		t.Errorf("clearing fields: got %q", got)
	}
	if got := emailPageURL(r, "only", ""); got != "/analytics/email?fields=1" {
		t.Errorf("clearing only: got %q", got)
	}

	r = httptest.NewRequest("GET", "/analytics/email?only=email", nil)
	if got := emailPageURL(r, "only", ""); got != "/analytics/email" {
		t.Errorf("clearing the last parameter: got %q", got)
	}
}
