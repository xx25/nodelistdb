package daemon

import (
	"testing"
	"time"

	"github.com/nodelistdb/internal/testing/models"
)

func TestParseAKA(t *testing.T) {
	tests := []struct {
		input      string
		wantOK     bool
		wantZone   int
		wantNet    int
		wantNode   int
		wantDomain string
	}{
		{"2:5001/100", true, 2, 5001, 100, ""},
		{"21:1/100@fsxnet", true, 21, 1, 100, "fsxnet"},
		{"21:1/100@FSXNET", true, 21, 1, 100, "fsxnet"},
		{" 2:5001/100 ", true, 2, 5001, 100, ""},
		{"2:5001/100.0@fidonet", true, 2, 5001, 100, "fidonet"},
		{"2:5001/100.5", false, 0, 0, 0, ""}, // points are skipped
		{"garbage", false, 0, 0, 0, ""},
		{"2:5001", false, 0, 0, 0, ""},
		{"", false, 0, 0, 0, ""},
	}

	for _, tt := range tests {
		aka, ok := parseAKA(tt.input)
		if ok != tt.wantOK {
			t.Errorf("parseAKA(%q) ok = %v, want %v", tt.input, ok, tt.wantOK)
			continue
		}
		if !ok {
			continue
		}
		if aka.Zone != tt.wantZone || aka.Net != tt.wantNet || aka.Node != tt.wantNode || aka.Domain != tt.wantDomain {
			t.Errorf("parseAKA(%q) = %+v, want %d:%d/%d@%q", tt.input, aka, tt.wantZone, tt.wantNet, tt.wantNode, tt.wantDomain)
		}
	}
}

func TestHostnamesOverlap(t *testing.T) {
	a := &models.Node{InternetHostnames: []string{"bbs.example.com", "backup.example.com"}}
	b := &models.Node{InternetHostnames: []string{"BBS.EXAMPLE.COM"}}
	c := &models.Node{InternetHostnames: []string{"other.example.net"}}
	empty := &models.Node{}

	if !hostnamesOverlap(a, b) {
		t.Error("expected case-insensitive overlap between a and b")
	}
	if hostnamesOverlap(a, c) {
		t.Error("expected no overlap between a and c")
	}
	if hostnamesOverlap(empty, b) || hostnamesOverlap(a, empty) {
		t.Error("empty hostname sets must never overlap")
	}
}

func TestNodeKeyIncludesDomain(t *testing.T) {
	fido := &models.Node{Zone: 21, Net: 1, Node: 100}
	fsx := &models.Node{Zone: 21, Net: 1, Node: 100, Domain: "fsxnet"}

	if fido.Key() == fsx.Key() {
		t.Errorf("keys must differ across domains: %q vs %q", fido.Key(), fsx.Key())
	}
	if fido.Key() != "21:1/100@fidonet" {
		t.Errorf("default domain key = %q, want 21:1/100@fidonet", fido.Key())
	}
}

func TestAkaEquivalenceGroup(t *testing.T) {
	e := NewAkaEquivalence()
	e.Link("a", "b")
	e.Link("b", "c")

	group := e.Group("a")
	if len(group) != 3 {
		t.Fatalf("expected transitive group of 3, got %v", group)
	}
	if e.Group("unknown") != nil {
		t.Error("unknown key must return nil group")
	}
}

func TestSeedFromNodes(t *testing.T) {
	fido := &models.Node{Zone: 2, Net: 5001, Node: 100, Domain: "fidonet", InternetHostnames: []string{"bbs.example.com"}}
	fsx := &models.Node{Zone: 21, Net: 1, Node: 100, Domain: "fsxnet", InternetHostnames: []string{"BBS.example.com"}}
	unrelated := &models.Node{Zone: 2, Net: 5001, Node: 300, Domain: "fidonet", InternetHostnames: []string{"else.example.org"}}

	e := NewAkaEquivalence()
	e.SeedFromNodes([]*models.Node{fido, fsx, unrelated})

	if group := e.Group(fido.Key()); len(group) != 2 {
		t.Errorf("expected fidonet/fsxnet pair linked, got %v", group)
	}
	if group := e.Group(unrelated.Key()); group != nil {
		t.Errorf("unrelated node must not be linked, got %v", group)
	}
}

func TestCollapseAKAGroupsKeepsSameDomainMembers(t *testing.T) {
	// One physical host with two fidonet entries and one fsxnet entry: a
	// derived result can only cover the fsxnet entry, so the second fidonet
	// entry must never be deferred.
	fidoA := &models.Node{Zone: 2, Net: 5001, Node: 100, Domain: "fidonet", InternetHostnames: []string{"bbs.example.com"}}
	fidoB := &models.Node{Zone: 2, Net: 5001, Node: 200, Domain: "fidonet", InternetHostnames: []string{"bbs.example.com"}}
	fsx := &models.Node{Zone: 21, Net: 1, Node: 100, Domain: "fsxnet", InternetHostnames: []string{"bbs.example.com"}}

	d := newTestDaemon(t, []*models.Node{fidoA, fidoB, fsx})
	d.akaEquiv.SeedFromNodes([]*models.Node{fidoA, fidoB, fsx})

	now := time.Now()
	d.scheduler.schedules[fidoA.Key()].LastTestTime = now.Add(-10 * time.Hour) // stalest -> representative
	d.scheduler.schedules[fidoB.Key()].LastTestTime = now.Add(-2 * time.Hour)
	d.scheduler.schedules[fsx.Key()].LastTestTime = now.Add(-1 * time.Hour)

	collapsed := d.collapseAKAGroups([]*models.Node{fidoA, fidoB, fsx})

	keys := map[string]bool{}
	for _, n := range collapsed {
		keys[n.Key()] = true
	}
	if !keys[fidoA.Key()] || !keys[fidoB.Key()] {
		t.Errorf("same-domain members must both stay, got %v", keys)
	}
	if keys[fsx.Key()] {
		t.Error("cross-domain member must be deferred (covered by derivation)")
	}
}

// newTestDaemon builds a minimal daemon good enough for AKA derivation tests.
func newTestDaemon(t *testing.T, nodes []*models.Node) *Daemon {
	t.Helper()

	scheduler := NewScheduler(SchedulerConfig{}, nil)
	for _, n := range nodes {
		scheduler.schedules[n.Key()] = &NodeSchedule{Node: n}
	}

	return &Daemon{
		scheduler: scheduler,
		akaEquiv:  NewAkaEquivalence(),
	}
}

func makeOperationalResult(node *models.Node, announced []string) *models.TestResult {
	r := models.NewTestResult(node)
	r.IsOperational = true
	r.SetBinkPResult(true, 10, &models.BinkPTestDetails{Addresses: announced}, "")
	return r
}

func TestDeriveAKAResults(t *testing.T) {
	tested := &models.Node{Zone: 2, Net: 5001, Node: 100, Domain: "fidonet", InternetHostnames: []string{"bbs.example.com"}}
	fsxTwin := &models.Node{Zone: 21, Net: 1, Node: 100, Domain: "fsxnet", InternetHostnames: []string{"bbs.example.com"}}
	fsxOtherHost := &models.Node{Zone: 21, Net: 1, Node: 200, Domain: "fsxnet", InternetHostnames: []string{"different.example.net"}}

	d := newTestDaemon(t, []*models.Node{tested, fsxTwin, fsxOtherHost})

	announced := []string{"2:5001/100", "21:1/100@fsxnet", "21:1/200@fsxnet"}
	result := makeOperationalResult(tested, announced)

	derived := d.deriveAKAResults(tested, result, nil, nil)

	if len(derived) != 1 {
		t.Fatalf("expected exactly 1 derived result (hostname-overlap gate), got %d", len(derived))
	}

	dr := derived[0]
	if dr.Zone != 21 || dr.Net != 1 || dr.Node != 100 {
		t.Errorf("derived identity = %d:%d/%d, want 21:1/100", dr.Zone, dr.Net, dr.Node)
	}
	if dr.Domain != "fsxnet" {
		t.Errorf("derived domain = %q, want fsxnet", dr.Domain)
	}
	if dr.Address != "21:1/100" {
		t.Errorf("derived address = %q, want 21:1/100", dr.Address)
	}
	if dr.DerivedFromAddress != "2:5001/100@fidonet" {
		t.Errorf("derived_from = %q, want 2:5001/100@fidonet", dr.DerivedFromAddress)
	}
	if !dr.AddressValidated {
		t.Error("derived result must be address-validated: 21:1/100@fsxnet was announced")
	}
	if !dr.IsOperational {
		t.Error("derived result must inherit operational state")
	}

	// The covered entry's schedule must have been pushed out
	sched := d.scheduler.schedules[fsxTwin.Key()]
	if sched.LastTestTime.IsZero() {
		t.Error("covered entry's LastTestTime must be updated")
	}
	if sched.TestReason != "aka_derived" {
		t.Errorf("covered entry's TestReason = %q, want aka_derived", sched.TestReason)
	}

	// The equivalence index must have learned the link
	if group := d.akaEquiv.Group(tested.Key()); len(group) != 2 {
		t.Errorf("expected equivalence link after derivation, got %v", group)
	}

	// Derived results must never trigger further derivation
	if again := d.deriveAKAResults(fsxTwin, dr, nil, nil); again != nil {
		t.Errorf("derivation chain detected: %v", again)
	}
}

func TestDeriveAKAResultsSkipsFailedTests(t *testing.T) {
	tested := &models.Node{Zone: 2, Net: 5001, Node: 100, Domain: "fidonet", InternetHostnames: []string{"bbs.example.com"}}
	fsxTwin := &models.Node{Zone: 21, Net: 1, Node: 100, Domain: "fsxnet", InternetHostnames: []string{"bbs.example.com"}}

	d := newTestDaemon(t, []*models.Node{tested, fsxTwin})

	result := makeOperationalResult(tested, []string{"21:1/100@fsxnet"})
	result.IsOperational = false

	if derived := d.deriveAKAResults(tested, result, nil, nil); derived != nil {
		t.Errorf("failed tests must not derive results, got %v", derived)
	}
}

func TestDeriveAKAResultsUsesPartials(t *testing.T) {
	// Aggregation keeps only the first successful protocol result; a later
	// hostname's announced AKAs must still be honored via the partials.
	tested := &models.Node{Zone: 2, Net: 5001, Node: 100, Domain: "fidonet",
		InternetHostnames: []string{"a.example.com", "b.example.com"}}
	fsxTwin := &models.Node{Zone: 21, Net: 1, Node: 100, Domain: "fsxnet", InternetHostnames: []string{"b.example.com"}}

	d := newTestDaemon(t, []*models.Node{tested, fsxTwin})

	aggregated := makeOperationalResult(tested, []string{"2:5001/100"})
	partial := makeOperationalResult(tested, []string{"2:5001/100", "21:1/100@fsxnet"})

	derived := d.deriveAKAResults(tested, aggregated, []*models.TestResult{partial}, nil)
	if len(derived) != 1 {
		t.Fatalf("expected AKA from partial result to be honored, got %d derived", len(derived))
	}
}

func TestDeriveAKAResultsCycleCoverage(t *testing.T) {
	// Two same-domain siblings of one physical host are both direct-tested in
	// one cycle and both announce the same fsxnet AKA: only the first may
	// produce a derived row for it.
	fidoA := &models.Node{Zone: 2, Net: 5001, Node: 100, Domain: "fidonet", InternetHostnames: []string{"bbs.example.com"}}
	fidoB := &models.Node{Zone: 2, Net: 5001, Node: 200, Domain: "fidonet", InternetHostnames: []string{"bbs.example.com"}}
	fsx := &models.Node{Zone: 21, Net: 1, Node: 100, Domain: "fsxnet", InternetHostnames: []string{"bbs.example.com"}}

	d := newTestDaemon(t, []*models.Node{fidoA, fidoB, fsx})
	cycle := newCycleCoverage()

	first := d.deriveAKAResults(fidoA, makeOperationalResult(fidoA, []string{"21:1/100@fsxnet"}), nil, cycle)
	second := d.deriveAKAResults(fidoB, makeOperationalResult(fidoB, []string{"21:1/100@fsxnet"}), nil, cycle)

	if len(first) != 1 {
		t.Fatalf("first test must derive the fsxnet entry, got %d", len(first))
	}
	if len(second) != 0 {
		t.Fatalf("second test in the same cycle must not derive a duplicate, got %d", len(second))
	}
}

func TestCollapseAKAGroups(t *testing.T) {
	fido := &models.Node{Zone: 2, Net: 5001, Node: 100, Domain: "fidonet", InternetHostnames: []string{"bbs.example.com"}}
	fsx := &models.Node{Zone: 21, Net: 1, Node: 100, Domain: "fsxnet", InternetHostnames: []string{"bbs.example.com"}}
	lone := &models.Node{Zone: 2, Net: 5001, Node: 200, Domain: "fidonet", InternetHostnames: []string{"solo.example.net"}}

	d := newTestDaemon(t, []*models.Node{fido, fsx, lone})
	d.akaEquiv.Link(fido.Key(), fsx.Key())

	// fsx is staler than fido
	d.scheduler.schedules[fido.Key()].LastTestTime = time.Now().Add(-1 * time.Hour)
	d.scheduler.schedules[fsx.Key()].LastTestTime = time.Now().Add(-10 * time.Hour)

	collapsed := d.collapseAKAGroups([]*models.Node{fido, fsx, lone})

	if len(collapsed) != 2 {
		t.Fatalf("expected 2 nodes after collapse, got %d", len(collapsed))
	}

	keys := map[string]bool{}
	for _, n := range collapsed {
		keys[n.Key()] = true
	}
	if !keys[fsx.Key()] {
		t.Error("the stalest group member must be kept as representative")
	}
	if keys[fido.Key()] {
		t.Error("the fresher group member must be deferred")
	}
	if !keys[lone.Key()] {
		t.Error("nodes outside any group must be untouched")
	}
}
