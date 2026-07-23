package web

import (
	"bytes"
	"html/template"
	"strings"
	"testing"
	"time"

	"github.com/nodelistdb/internal/database"
	"github.com/nodelistdb/internal/flags"
)

func phEntry(date string, source string, sysop string, flags ...string) database.Point {
	d, _ := time.Parse("2006-01-02", date)
	return database.Point{
		PointlistDate: d,
		ListSource:    source,
		SystemName:    "System",
		Location:      "Location",
		SysopName:     sysop,
		Phone:         "-Unpublished-",
		MaxSpeed:      9600,
		Flags:         flags,
	}
}

func TestGroupPointHistoryCollapsesIdenticalRuns(t *testing.T) {
	// Newest-first, as GetPointHistory returns: two interleaved sources
	// republishing the same content week after week.
	history := []database.Point{
		phEntry("2003-08-15", "r50", "Oleg", "MO"),
		phEntry("2003-08-15", "z2", "Oleg", "MO"),
		phEntry("2003-08-08", "r50", "Oleg", "MO"),
		phEntry("2003-08-08", "z2", "Oleg", "MO"),
		phEntry("2003-08-01", "r50", "Oleg", "MO"),
	}

	periods := groupPointHistory(history)
	if len(periods) != 1 {
		t.Fatalf("expected 1 period, got %d", len(periods))
	}
	p := periods[0]
	if p.Count != 5 {
		t.Errorf("Count = %d, want 5", p.Count)
	}
	if got := p.FirstDate.Format("2006-01-02"); got != "2003-08-01" {
		t.Errorf("FirstDate = %s, want 2003-08-01", got)
	}
	if got := p.LastDate.Format("2006-01-02"); got != "2003-08-15" {
		t.Errorf("LastDate = %s, want 2003-08-15", got)
	}
	if len(p.Sources) != 2 || p.Sources[0] != "r50" || p.Sources[1] != "z2" {
		t.Errorf("Sources = %v, want [r50 z2]", p.Sources)
	}
}

func TestGroupPointHistorySplitsOnContentChange(t *testing.T) {
	history := []database.Point{
		phEntry("2003-08-15", "r50", "Oleg", "MO", "CM"), // flags changed
		phEntry("2003-08-08", "r50", "Oleg", "MO"),
		phEntry("2003-08-01", "r50", "Oleg", "MO"),
		phEntry("2003-07-25", "r50", "Olga", "MO"), // sysop differs
	}

	periods := groupPointHistory(history)
	if len(periods) != 3 {
		t.Fatalf("expected 3 periods, got %d", len(periods))
	}
	if periods[0].Count != 1 || len(periods[0].Entry.Flags) != 2 {
		t.Errorf("newest period should be the single changed row, got count %d", periods[0].Count)
	}
	if periods[1].Count != 2 {
		t.Errorf("middle period Count = %d, want 2", periods[1].Count)
	}
	if periods[2].Entry.SysopName != "Olga" {
		t.Errorf("oldest period sysop = %s, want Olga", periods[2].Entry.SysopName)
	}
	// A period's Entry is its newest row: the middle run spans 08-01..08-08.
	if got := periods[1].Entry.PointlistDate.Format("2006-01-02"); got != "2003-08-08" {
		t.Errorf("middle period Entry date = %s, want 2003-08-08", got)
	}
}

func TestGroupPointHistoryRevertDoesNotMerge(t *testing.T) {
	history := []database.Point{
		phEntry("2003-08-15", "r50", "Oleg", "MO"),
		phEntry("2003-08-08", "r50", "Oleg", "MO", "CM"),
		phEntry("2003-08-01", "r50", "Oleg", "MO"), // same as newest, but not adjacent
	}

	periods := groupPointHistory(history)
	if len(periods) != 3 {
		t.Fatalf("expected 3 periods (revert must not merge across the change), got %d", len(periods))
	}
}

func TestGroupPointHistorySplitsOnInternetConfigChange(t *testing.T) {
	// IBN/INA tokens are stripped out of Flags into InternetConfig during
	// parsing, so an endpoint change must split via InternetConfig equality.
	newer := phEntry("2003-08-08", "r50", "Oleg", "MO")
	newer.InternetConfig = []byte(`{"protocols":{"IBN":{"address":"host2.example.com"}}}`)
	older := phEntry("2003-08-01", "r50", "Oleg", "MO")
	older.InternetConfig = []byte(`{"protocols":{"IBN":{"address":"host1.example.com"}}}`)

	periods := groupPointHistory([]database.Point{newer, older})
	if len(periods) != 2 {
		t.Fatalf("expected 2 periods for an InternetConfig change, got %d", len(periods))
	}
}

// Rows stored before INA became a list hold {"INA":"host"}; newer imports of
// the same unchanged entry hold {"INA":["host"]}. That reshape alone must not
// split a continuous period.
func TestGroupPointHistoryIgnoresINAShapeChange(t *testing.T) {
	newer := phEntry("2003-08-08", "r50", "Oleg", "MO")
	newer.InternetConfig = []byte(`{"protocols":{"IBN":[{"port":24554}]},"defaults":{"INA":["host.example.com"]}}`)
	older := phEntry("2003-08-01", "r50", "Oleg", "MO")
	older.InternetConfig = []byte(`{"protocols":{"IBN":[{"port":24554}]},"defaults":{"INA":"host.example.com"}}`)

	periods := groupPointHistory([]database.Point{newer, older})
	if len(periods) != 1 {
		t.Fatalf("expected 1 period for an unchanged entry, got %d", len(periods))
	}
}

// A genuine second INA is a real endpoint change and must still split.
func TestGroupPointHistorySplitsOnAddedINA(t *testing.T) {
	newer := phEntry("2003-08-08", "r50", "Oleg", "MO")
	newer.InternetConfig = []byte(`{"defaults":{"INA":["host.example.com","backup.example.com"]}}`)
	older := phEntry("2003-08-01", "r50", "Oleg", "MO")
	older.InternetConfig = []byte(`{"defaults":{"INA":"host.example.com"}}`)

	periods := groupPointHistory([]database.Point{newer, older})
	if len(periods) != 2 {
		t.Fatalf("expected 2 periods for an added INA, got %d", len(periods))
	}
}

func TestGroupPointHistoryConflictRowsCountOnce(t *testing.T) {
	// A duplicate line inside one file (conflict_sequence > 0) is one
	// publication, not two issues.
	dup := phEntry("2003-08-08", "r50", "Oleg", "MO")
	dup.ConflictSequence = 1
	history := []database.Point{
		phEntry("2003-08-08", "r50", "Oleg", "MO"),
		dup,
		phEntry("2003-08-01", "r50", "Oleg", "MO"),
	}

	periods := groupPointHistory(history)
	if len(periods) != 1 {
		t.Fatalf("expected 1 period, got %d", len(periods))
	}
	if periods[0].Count != 2 {
		t.Errorf("Count = %d, want 2 (conflict duplicate must not inflate)", periods[0].Count)
	}
}

func TestGroupPointHistoryEmpty(t *testing.T) {
	if got := groupPointHistory(nil); len(got) != 0 {
		t.Fatalf("expected no periods for empty history, got %d", len(got))
	}
}

func TestPointHistoryPageRendersPeriods(t *testing.T) {
	s := &Server{templates: make(map[string]*template.Template), templatesFS: TemplatesFS}
	s.loadTemplates()
	tmpl, ok := s.templates["point_history"]
	if !ok {
		t.Fatal("point_history template not loaded")
	}

	history := []database.Point{
		phEntry("2003-08-15", "r50", "Oleg", "MO"),
		phEntry("2003-08-08", "z2", "Oleg", "MO"),
		phEntry("2003-08-01", "r50", "Olga", "MO"),
	}
	data := map[string]interface{}{
		"Title":            "Point History",
		"Address":          "2:50/700.1",
		"BossAddress":      "2:50/700",
		"Domain":           "fidonet",
		"AvailableDomains": []string{"fidonet"},
		"History":          history,
		"HistoryPeriods":   groupPointHistory(history),
		"Latest":           history[0],
		"FirstDate":        history[2].PointlistDate,
		"LastDate":         history[0].PointlistDate,
		"CurrentlyActive":  false,
		"FlagDescriptions": flags.GetFlagDescriptions(),
		"Version":          "test",
		"ActivePage":       "",
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		t.Fatalf("point_history Execute error: %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, "2003-08-08 &ndash; 2003-08-15") {
		t.Error("render missing collapsed period range for the unchanged run")
	}
	if !strings.Contains(out, "2003-08-01") {
		t.Error("render missing single-issue period date")
	}
	// One table row per period, not per history entry.
	if got := strings.Count(out, "Olga"); got != 1 {
		t.Errorf("expected the older sysop once in the table, got %d occurrences", got)
	}

	// Flags in the history table carry description tooltips, same as the
	// node page (MO has a static description).
	start, end := strings.Index(out, "<tbody>"), strings.Index(out, "</tbody>")
	if start < 0 || end < start {
		t.Fatal("history table body not found in render")
	}
	if !strings.Contains(out[start:end], "flag-tooltip") {
		t.Error("history table flags missing tooltip markup")
	}
}
