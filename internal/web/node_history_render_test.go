package web

import (
	"bytes"
	"encoding/json"
	"html/template"
	"strings"
	"testing"
	"time"

	"github.com/nodelistdb/internal/database"
	"github.com/nodelistdb/internal/flags"
)

// renderNodeHistory loads the real embedded templates and renders the node page
// for a node whose first nodelist entry carries the given internet config.
func renderNodeHistory(t *testing.T, internetConfig string) string {
	t.Helper()

	s := &Server{templates: make(map[string]*template.Template), templatesFS: TemplatesFS}
	s.loadTemplates()
	tmpl, ok := s.templates["node_history"]
	if !ok {
		t.Fatal("node_history template not loaded")
	}

	date := time.Date(2026, 7, 23, 0, 0, 0, 0, time.UTC)
	node := database.Node{
		Zone: 2, Net: 5020, Node: 113,
		Domain:         "fidonet",
		NodelistDate:   date,
		SystemName:     "Minas_Anor",
		Location:       "Moscow",
		SysopName:      "Boris_Paleev",
		Phone:          "-Unpublished-",
		NodeType:       "Node",
		IsCM:           true,
		HasInet:        true,
		InternetConfig: json.RawMessage(internetConfig),
	}

	data := struct {
		Title            string
		Address          string
		Domain           string
		AvailableDomains []string
		History          []database.Node
		Changes          []database.NodeChange
		Points           []database.Point
		FirstDate        time.Time
		LastDate         time.Time
		CurrentlyActive  bool
		FlagDescriptions map[string]flags.FlagInfo
		Version          string
		ActivePage       string
	}{
		Title:            "Node History",
		Address:          "2:5020/113",
		Domain:           "fidonet",
		History:          []database.Node{node},
		Changes:          []database.NodeChange{{Date: date, ChangeType: "added", NewNode: &node}},
		FirstDate:        date,
		LastDate:         date,
		CurrentlyActive:  true,
		FlagDescriptions: flags.GetFlagDescriptions(),
		Version:          "test",
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		t.Fatalf("node_history Execute error: %v", err)
	}
	return buf.String()
}

// A node reachable at several INA addresses must show all of them; only the
// last one used to survive parsing, so the page could not have shown more.
func TestNodeHistoryRendersEveryINA(t *testing.T) {
	out := renderNodeHistory(t, `{"protocols":{"IBN":[{"port":24554}]},`+
		`"defaults":{"INA":["horris.now.im","horris.privatedns.org"]},`+
		`"email_protocols":{"IMI":[{"email":"horris77@mail.ru"}]}}`)

	for _, want := range []string{"Internet Addresses", "horris.now.im", "horris.privatedns.org"} {
		if !strings.Contains(out, want) {
			t.Errorf("render missing %q", want)
		}
	}

	// The node's current addresses belong in the summary card, not only in
	// the timeline entry for the day it was added.
	summary, _, found := strings.Cut(out, "Change History")
	if !found {
		t.Fatal("render has no Change History section to split on")
	}
	for _, want := range []string{"horris.now.im", "horris.privatedns.org"} {
		if !strings.Contains(summary, want) {
			t.Errorf("Node Information card missing %q", want)
		}
	}
}

// Rows written before INA became a list hold a bare string.
func TestNodeHistoryRendersLegacyScalarINA(t *testing.T) {
	out := renderNodeHistory(t, `{"protocols":{"IBN":[{"port":24554}]},"defaults":{"INA":"legacy.example.org"}}`)

	if !strings.Contains(out, "legacy.example.org") {
		t.Error("render missing legacy INA hostname")
	}
}

// IEM shares the defaults map with INA but is an email address, so it belongs
// with the emails, not under "Internet Addresses".
func TestNodeHistoryRendersIEMAsEmail(t *testing.T) {
	out := renderNodeHistory(t, `{"defaults":{"IEM":["sysop@example.org"]}}`)

	if !strings.Contains(out, "sysop@example.org") {
		t.Error("render missing IEM address")
	}
	if strings.Contains(out, "Internet Addresses") {
		t.Error("IEM must not render under the Internet Addresses heading")
	}
	if !strings.Contains(out, "Email Addresses") {
		t.Error("IEM should render under Email Addresses")
	}
}

func TestNodeHistoryHidesAddressesWhenAbsent(t *testing.T) {
	out := renderNodeHistory(t, `{"protocols":{"IBN":[{"address":"bbs.example.org","port":24554}]}}`)

	if strings.Contains(out, "Internet Addresses") {
		t.Error("Internet Addresses section should be hidden when the node has no INA/IEM")
	}
}
