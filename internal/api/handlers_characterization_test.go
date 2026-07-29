package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nodelistdb/internal/database"
	"github.com/nodelistdb/internal/storage"
)

// fakeOps satisfies storage.Operations by embedding it, so only the methods a
// test overrides are usable and every other call panics. That is the point:
// the panic pins exactly which storage calls a handler makes, and a handler
// that grows a new one fails loudly instead of quietly reaching a nil field.
type fakeOps struct {
	storage.Operations

	nodes        []database.Node
	nodesErr     error
	nodesFilters []database.NodeFilter

	history    []database.Node
	historyErr error

	changes    []database.NodeChange
	changesErr error

	nodeDomains []string

	points       []database.Point
	pointsErr    error
	pointHistory []database.Point
	pointDomains []string

	sysops        []storage.SysopInfo
	sysopsErr     error
	sysopsFilters []sysopQuery
	sysopNodes    []database.Node
	sysopNodesErr error
	sysopNodesFor string
}

// sysopQuery records how a sysop listing was asked for.
type sysopQuery struct {
	name          string
	limit, offset int
}

func (f *fakeOps) GetUniqueSysops(nameFilter string, limit, offset int) ([]storage.SysopInfo, error) {
	f.sysopsFilters = append(f.sysopsFilters, sysopQuery{nameFilter, limit, offset})
	return f.sysops, f.sysopsErr
}

func (f *fakeOps) GetNodesBySysop(sysopName string, limit int) ([]database.Node, error) {
	f.sysopNodesFor = sysopName
	return f.sysopNodes, f.sysopNodesErr
}

func (f *fakeOps) GetNodes(filter database.NodeFilter) ([]database.Node, error) {
	f.nodesFilters = append(f.nodesFilters, filter)
	return f.nodes, f.nodesErr
}

func (f *fakeOps) GetNodeHistory(zone, net, node int, domain string) ([]database.Node, error) {
	return f.history, f.historyErr
}

func (f *fakeOps) GetNodeDateRange(zone, net, node int, domain string) (time.Time, time.Time, error) {
	if len(f.history) == 0 {
		return time.Time{}, time.Time{}, nil
	}
	return f.history[0].NodelistDate, f.history[len(f.history)-1].NodelistDate, nil
}

func (f *fakeOps) GetNodeChanges(zone, net, node int, domain string) ([]database.NodeChange, error) {
	return f.changes, f.changesErr
}

func (f *fakeOps) GetNodeDomains(zone, net, node int) ([]string, error) {
	return f.nodeDomains, nil
}

func (f *fakeOps) GetPointDomains(zone, net, node int, point *int) ([]string, error) {
	return f.pointDomains, nil
}

func (f *fakeOps) SearchPoints(filter database.PointFilter) ([]database.Point, error) {
	return f.points, f.pointsErr
}

func (f *fakeOps) GetPointsByBoss(domain string, zone, net, node int, asOf *time.Time) ([]database.Point, error) {
	return f.points, f.pointsErr
}

func (f *fakeOps) GetPointHistory(domain string, zone, net, node, point int) ([]database.Point, error) {
	return f.pointHistory, nil
}

// call routes a request through the real router, so path parameters, method
// matching and middleware are exercised as they are in production.
func call(t *testing.T, ops storage.Operations, method, target string) (*httptest.ResponseRecorder, map[string]interface{}) {
	t.Helper()
	router := New(ops).SetupRouter()
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(method, target, nil))

	var body map[string]interface{}
	if rec.Body.Len() > 0 {
		// Not every response is an object (a node lookup returns one), so a
		// decode failure is not an error here.
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
	}
	return rec, body
}

func sampleNode() database.Node {
	return database.Node{
		Zone: 2, Net: 5001, Node: 100,
		SystemName:   "Test_System",
		Location:     "Moscow",
		SysopName:    "A_Sysop",
		NodelistDate: time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC),
		NodeType:     "Node",
		Domain:       "fidonet",
	}
}

// TestNodePathParamsAreValidated pins the status and message for a malformed
// address component on each of the four node endpoints. They parse it with
// four copies of the same block, so this is what keeps them saying the same
// thing.
func TestNodePathParamsAreValidated(t *testing.T) {
	ops := &fakeOps{}

	for _, tc := range []struct {
		target  string
		wantMsg string
	}{
		{"/api/nodes/x/5001/100", "Invalid zone parameter"},
		{"/api/nodes/2/x/100", "Invalid net parameter"},
		{"/api/nodes/2/5001/x", "Invalid node parameter"},
		{"/api/nodes/x/5001/100/history", "Invalid zone parameter"},
		{"/api/nodes/2/5001/x/history", "Invalid node parameter"},
		{"/api/nodes/x/5001/100/changes", "Invalid zone parameter"},
		{"/api/nodes/2/x/100/changes", "Invalid net parameter"},
		{"/api/nodes/x/5001/100/timeline", "Invalid zone parameter"},
		{"/api/nodes/2/5001/x/timeline", "Invalid node parameter"},
	} {
		rec, body := call(t, ops, "GET", tc.target)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", tc.target, rec.Code)
		}
		if got := body["error"]; got != tc.wantMsg {
			t.Errorf("%s: error = %v, want %q", tc.target, got, tc.wantMsg)
		}
	}
}

// TestNodeEnvelopesCarryTheirAddress pins the shared envelope on the three
// node endpoints that return one, including which fields it has.
func TestNodeEnvelopesCarryTheirAddress(t *testing.T) {
	node := sampleNode()
	ops := &fakeOps{
		nodes:       []database.Node{node},
		history:     []database.Node{node},
		changes:     []database.NodeChange{{Date: node.NodelistDate, ChangeType: "added"}},
		nodeDomains: []string{"fidonet", "fsxnet"},
	}

	for _, target := range []string{
		"/api/nodes/2/5001/100/history",
		"/api/nodes/2/5001/100/changes",
		"/api/nodes/2/5001/100/timeline",
	} {
		rec, body := call(t, ops, "GET", target)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, want 200: %s", target, rec.Code, rec.Body.String())
		}
		if got := body["address"]; got != "2:5001/100" {
			t.Errorf("%s: address = %v, want 2:5001/100", target, got)
		}
		if got := body["domain"]; got != "fidonet" {
			t.Errorf("%s: domain = %v, want fidonet (the default wins when an address is in several)", target, got)
		}
		domains, _ := body["available_domains"].([]interface{})
		if len(domains) != 2 {
			t.Errorf("%s: available_domains = %v, want both networks", target, body["available_domains"])
		}
		if _, ok := body["count"]; !ok {
			t.Errorf("%s: response has no count", target)
		}
	}
}

// TestDomainQueryParameterWins pins that ?domain= overrides the resolution
// heuristic on both the node and the point endpoints.
func TestDomainQueryParameterWins(t *testing.T) {
	node := sampleNode()
	ops := &fakeOps{
		history:      []database.Node{node},
		nodeDomains:  []string{"fidonet", "fsxnet"},
		pointDomains: []string{"fidonet", "fsxnet"},
		pointHistory: []database.Point{{Zone: 2, Net: 5001, Node: 100, PointNum: 7, Domain: "fsxnet"}},
	}

	_, body := call(t, ops, "GET", "/api/nodes/2/5001/100/history?domain=fsxnet")
	if got := body["domain"]; got != "fsxnet" {
		t.Errorf("node history: domain = %v, want fsxnet", got)
	}

	_, body = call(t, ops, "GET", "/api/points/2/5001/100/7/history?domain=fsxnet")
	if got := body["domain"]; got != "fsxnet" {
		t.Errorf("point history: domain = %v, want fsxnet", got)
	}
	// The point history envelope carries a 4-D address; the node ones do not.
	if got := body["address"]; got != "2:5001/100.7" {
		t.Errorf("point history: address = %v, want 2:5001/100.7", got)
	}
}

// TestNotFoundIs404 pins that an empty result is a 404 rather than an empty
// 200, on every endpoint that looks up one entity.
func TestNotFoundIs404(t *testing.T) {
	ops := &fakeOps{nodeDomains: []string{"fidonet"}, pointDomains: []string{"fidonet"}}

	for _, target := range []string{
		"/api/nodes/2/5001/100",
		"/api/nodes/2/5001/100/history",
		"/api/nodes/2/5001/100/timeline",
		"/api/points/2/5001/100/7",
		"/api/points/2/5001/100/7/history",
	} {
		rec, _ := call(t, ops, "GET", target)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: status = %d, want 404", target, rec.Code)
		}
	}

	// Changes is deliberately not in that list: no changes is a valid answer.
	rec, body := call(t, ops, "GET", "/api/nodes/2/5001/100/changes")
	if rec.Code != http.StatusOK {
		t.Errorf("changes: status = %d, want 200 (no changes is an answer, not a miss)", rec.Code)
	}
	if got := body["count"]; got != float64(0) {
		t.Errorf("changes: count = %v, want 0", got)
	}
}

// TestSearchRequiresAConstraint pins the 400 the real handler returns for an
// unconstrained search - the copied suite this replaced asserted a 500.
func TestSearchRequiresAConstraint(t *testing.T) {
	rec, body := call(t, &fakeOps{}, "GET", "/api/nodes")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if body["error"] == nil {
		t.Error("response carries no error message")
	}
}

// TestSearchPassesItsFilterThrough pins that query parameters reach storage
// rather than being dropped on the way.
func TestSearchPassesItsFilterThrough(t *testing.T) {
	ops := &fakeOps{nodes: []database.Node{sampleNode()}}
	rec, body := call(t, ops, "GET", "/api/nodes?zone=2&net=5001&system_name=Test&limit=7")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if got := body["count"]; got != float64(1) {
		t.Errorf("count = %v, want 1", got)
	}
	if len(ops.nodesFilters) != 1 {
		t.Fatalf("storage saw %d filters, want 1", len(ops.nodesFilters))
	}
	f := ops.nodesFilters[0]
	if f.Zone == nil || *f.Zone != 2 || f.Net == nil || *f.Net != 5001 {
		t.Errorf("zone/net did not reach the filter: %+v", f)
	}
	if f.SystemName == nil || *f.SystemName != "Test" {
		t.Errorf("system_name did not reach the filter: %+v", f)
	}
	if f.Limit != 7 {
		t.Errorf("limit = %d, want 7", f.Limit)
	}
}

// TestStorageFailureIs500 pins that a storage error becomes a 500 with a
// message, on the handlers about to be edited.
func TestStorageFailureIs500(t *testing.T) {
	boom := errNotAvailable{}
	ops := &fakeOps{
		nodesErr:    boom,
		historyErr:  boom,
		changesErr:  boom,
		nodeDomains: []string{"fidonet"},
	}

	for _, target := range []string{
		"/api/nodes?zone=2",
		"/api/nodes/2/5001/100",
		"/api/nodes/2/5001/100/history",
		"/api/nodes/2/5001/100/changes",
		"/api/nodes/2/5001/100/timeline",
	} {
		rec, body := call(t, ops, "GET", target)
		if rec.Code != http.StatusInternalServerError {
			t.Errorf("%s: status = %d, want 500", target, rec.Code)
		}
		if body["error"] == nil {
			t.Errorf("%s: response carries no error message", target)
		}
	}
}

type errNotAvailable struct{}

func (errNotAvailable) Error() string { return "clickhouse unavailable" }

// TestMalformedFilterParamsAre400 covers the change from silently dropping a
// malformed query parameter to rejecting it. ?zone=two used to produce an
// unfiltered search - the widest possible answer to a question the caller did
// not ask.
func TestMalformedFilterParamsAre400(t *testing.T) {
	ops := &fakeOps{nodes: []database.Node{sampleNode()}}

	for _, tc := range []struct {
		target  string
		wantMsg string
	}{
		{"/api/nodes?zone=two", "zone must be a whole number"},
		{"/api/nodes?net=5001&node=1e2", "node must be a whole number"},
		{"/api/nodes?zone=2&date_from=last-week", "date_from must be a date in YYYY-MM-DD form"},
		{"/api/nodes?zone=2&date_to=2026-13-45", "date_to must be a date in YYYY-MM-DD form"},
		{"/api/points?zone=2&point=all", "point must be a whole number"},
		{"/api/points?zone=2&date_from=yesterday", "date_from must be a date in YYYY-MM-DD form"},
	} {
		rec, body := call(t, ops, "GET", tc.target)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", tc.target, rec.Code)
		}
		if got := body["error"]; got != tc.wantMsg {
			t.Errorf("%s: error = %v, want %q", tc.target, got, tc.wantMsg)
		}
	}

	// An absent parameter is still not an error.
	rec, _ := call(t, ops, "GET", "/api/nodes?zone=2")
	if rec.Code != http.StatusOK {
		t.Errorf("absent parameters: status = %d, want 200", rec.Code)
	}

	// parseBoolParam stays permissive: an unrecognised value reads as false,
	// which is documented backward-compatible behaviour.
	rec, _ = call(t, ops, "GET", "/api/nodes?zone=2&is_cm=maybe")
	if rec.Code != http.StatusOK {
		t.Errorf("is_cm=maybe: status = %d, want 200 (booleans stay permissive)", rec.Code)
	}
}

// TestSysopEndpoints covers what the deleted copied suite claimed to: the
// listing, its name filter and pagination bounds, and the per-sysop node
// lookup including how it decodes the name out of the path.
func TestSysopEndpoints(t *testing.T) {
	ops := &fakeOps{
		sysops:     []storage.SysopInfo{{Name: "A_Sysop", NodeCount: 3}},
		sysopNodes: []database.Node{sampleNode()},
	}

	rec, body := call(t, ops, "GET", "/api/sysops?name=Sysop&limit=5&offset=10")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if got := body["count"]; got != float64(1) {
		t.Errorf("count = %v, want 1", got)
	}
	if len(ops.sysopsFilters) != 1 || ops.sysopsFilters[0] != (sysopQuery{"Sysop", 5, 10}) {
		t.Errorf("storage saw %+v, want the name, limit and offset from the query", ops.sysopsFilters)
	}

	// The listing's limit is capped rather than rejected.
	call(t, ops, "GET", "/api/sysops?limit=99999")
	if last := ops.sysopsFilters[len(ops.sysopsFilters)-1]; last.limit != 200 {
		t.Errorf("limit = %d, want it capped at 200", last.limit)
	}

	// A percent-encoded name reaches storage decoded.
	rec, body = call(t, ops, "GET", "/api/sysops/John%20Smith/nodes")
	if rec.Code != http.StatusOK {
		t.Fatalf("sysop nodes: status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if ops.sysopNodesFor != "John Smith" {
		t.Errorf("storage saw sysop %q, want %q", ops.sysopNodesFor, "John Smith")
	}
	if got := body["count"]; got != float64(1) {
		t.Errorf("sysop nodes: count = %v, want 1", got)
	}
}

// TestResponsesAreJSON pins the content type across a representative handler
// from each family, plus the two error shapes.
func TestResponsesAreJSON(t *testing.T) {
	ops := &fakeOps{
		nodes:       []database.Node{sampleNode()},
		sysops:      []storage.SysopInfo{{Name: "A_Sysop"}},
		nodeDomains: []string{"fidonet"},
	}

	for _, target := range []string{
		"/api/health",
		"/api/nodes?zone=2",
		"/api/nodes/2/5001/100",
		"/api/sysops",
		"/api/nodes/x/5001/100", // 400
		"/api/nodes/2/5001/999", // 200 with the same fake, but exercises the path
	} {
		rec, _ := call(t, ops, "GET", target)
		if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
			t.Errorf("%s: Content-Type = %q, want application/json", target, ct)
		}
	}
}

// TestNonGetMethodsAre405 pins that chi answers 405 rather than 404 for a
// wrong method on a real route. Twenty handlers used to carry their own
// CheckMethod guard for this; the router already did it.
func TestNonGetMethodsAre405(t *testing.T) {
	ops := &fakeOps{}
	for _, tc := range []struct{ method, target string }{
		{"POST", "/api/nodes"},
		{"POST", "/api/nodes/2/5001/100"},
		{"DELETE", "/api/stats"},
		{"PUT", "/api/sysops"},
	} {
		rec, _ := call(t, ops, tc.method, tc.target)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s %s: status = %d, want 405", tc.method, tc.target, rec.Code)
		}
	}
}

// TestLargeResultSetsSurviveEncoding covers what the old integration suite
// checked with a copied handler: a full page of results encodes and comes back
// counted.
func TestLargeResultSetsSurviveEncoding(t *testing.T) {
	nodes := make([]database.Node, 100)
	for i := range nodes {
		n := sampleNode()
		n.Net = 5000 + i
		nodes[i] = n
	}

	rec, body := call(t, &fakeOps{nodes: nodes}, "GET", "/api/nodes?zone=2")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := body["count"]; got != float64(100) {
		t.Errorf("count = %v, want 100", got)
	}
	if got, _ := body["nodes"].([]interface{}); len(got) != 100 {
		t.Errorf("nodes = %d entries, want 100", len(got))
	}
}
