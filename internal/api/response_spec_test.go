package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nodelistdb/internal/config"
	"github.com/nodelistdb/internal/database"
	"github.com/nodelistdb/internal/pingtrace"
	"github.com/nodelistdb/internal/ratelimit"
	"github.com/nodelistdb/internal/storage"
)

// TestResponsesMatchTheSpec sends one request to every readable endpoint
// through the real router, backed by a fake store that answers each call
// with a fully populated record, and compares the top-level keys of the 200
// body with the properties the spec declares for it. /api/stats used to be
// documented as the bare statistics object while the handler wrapped it in
// {stats, domain, ...}; no test noticed because none read the spec.
//
// Only the top level is compared: nested records are held against their
// structs by TestSchemasMatchTheirTypes, and the two together cover a body
// down to its leaves.
func TestResponsesMatchTheSpec(t *testing.T) {
	spec := loadSpec(t)
	ops := newSpecFake()
	s := New(ops)
	s.SetHealthChecker(&mockHealthChecker{status: &HealthStatus{
		Cache: &CacheHealth{}, FTP: &FTPHealth{},
	}})
	s.SetCacheStatsHandler(func(w http.ResponseWriter, _ *http.Request) { WriteJSONSuccess(w, CacheStats{}) })
	s.SetRateLimitStatsHandler(func(w http.ResponseWriter, _ *http.Request) { WriteJSONSuccess(w, ratelimit.Stats{}) })
	s.SetFTPStatsHandler(func(w http.ResponseWriter, _ *http.Request) { WriteJSONSuccess(w, FTPStats{}) })
	s.SetModemHandler(NewModemHandler(&config.ModemAPIConfig{MaxBodySizeMB: 1}, nil))
	router := s.SetupRouter()

	// route (as the spec spells it) -> a request that makes the handler emit
	// every documented key.
	requests := map[string]string{
		"/api/health":                                     "/api/health",
		"/api/networks":                                   "/api/networks",
		"/api/nodes":                                      "/api/nodes?zone=2",
		"/api/nodes/pstn":                                 "/api/nodes/pstn",
		"/api/nodes/pstn/dead":                            "/api/nodes/pstn/dead",
		"/api/nodes/pstn/recent-success":                  "/api/nodes/pstn/recent-success",
		"/api/nodes/{zone}/{net}/{node}":                  "/api/nodes/2/5001/100",
		"/api/nodes/{zone}/{net}/{node}/history":          "/api/nodes/2/5001/100/history",
		"/api/nodes/{zone}/{net}/{node}/changes":          "/api/nodes/2/5001/100/changes",
		"/api/nodes/{zone}/{net}/{node}/timeline":         "/api/nodes/2/5001/100/timeline",
		"/api/nodes/{zone}/{net}/{node}/points":           "/api/nodes/2/5001/100/points?date=2026-01-01",
		"/api/nodes/{zone}/{net}/{node}/ping":             "/api/nodes/2/5001/100/ping",
		"/api/nodes/{zone}/{net}/{node}/tests":            "/api/nodes/2/5001/100/tests",
		"/api/nodes/{zone}/{net}/{node}/tests/detail":     "/api/nodes/2/5001/100/tests/detail?time=2026-09-10T12:00:00Z",
		"/api/reachability/trends":                        "/api/reachability/trends",
		"/api/reachability/nodes":                         "/api/reachability/nodes",
		"/api/points":                                     "/api/points?zone=2",
		"/api/points/{zone}/{net}/{node}/{point}":         "/api/points/2/5001/100/1",
		"/api/points/{zone}/{net}/{node}/{point}/history": "/api/points/2/5001/100/1/history",
		"/api/pointlists/dates":                           "/api/pointlists/dates",
		"/api/pointlists/sources":                         "/api/pointlists/sources",
		"/api/sysops":                                     "/api/sysops",
		"/api/sysops/{name}/nodes":                        "/api/sysops/A_Sysop/nodes",
		"/api/stats":                                      "/api/stats",
		"/api/stats/dates":                                "/api/stats/dates",
		"/api/flags":                                      "/api/flags",
		"/api/software/binkp":                             "/api/software/binkp",
		"/api/software/ifcico":                            "/api/software/ifcico",
		"/api/software/binkd":                             "/api/software/binkd",
		"/api/analytics/geo-hosting":                      "/api/analytics/geo-hosting",
		"/api/analytics/pingtrace":                        "/api/analytics/pingtrace",
		"/api/cache/stats":                                "/api/cache/stats",
		"/api/ratelimit/stats":                            "/api/ratelimit/stats",
		"/api/ftp/stats":                                  "/api/ftp/stats",
	}
	// Not exercised: the two documentation routes, /api/nodelist/latest
	// (reads the archive on disk) and the authenticated modem writes.
	skipped := map[string]bool{
		"/api/openapi.yaml": true, "/api/docs": true, "/api/nodelist/latest": true,
		"/api/modem/results/direct": true, "/api/modem/pstn-dead": true,
	}
	for route := range spec.Paths {
		if _, ok := requests[route]; !ok && !skipped[route] {
			t.Errorf("%s is in openapi.yaml but this test sends it nothing", route)
		}
	}

	// Second pass, with a store that has nothing: every property the spec
	// types as an array must still be an array, never null. /api/sysops
	// answered "sysops": null on a name that matched nothing. Bodies that
	// are one storage record ($ref at the top) are skipped: there the store
	// builds the arrays, and a fake cannot vouch for it.
	emptyRouter := New(&emptyFake{}).SetupRouter()
	for route, target := range requests {
		op, ok := spec.Paths[route]["get"]
		if !ok || op.Responses["200"].Content["application/json"].Schema.Ref != "" {
			continue
		}
		rec := httptest.NewRecorder()
		emptyRouter.ServeHTTP(rec, httptest.NewRequest("GET", target, nil))
		if rec.Code != http.StatusOK {
			continue // 404 on an absent record is documented; not this pass's concern
		}
		var body map[string]json.RawMessage
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Errorf("%s (empty store): body is not a JSON object: %v", route, err)
			continue
		}
		for prop, ps := range responseSchema(spec, op) {
			if ps.Type != "array" {
				continue
			}
			if raw, ok := body[prop]; ok && strings.TrimSpace(string(raw)) == "null" {
				t.Errorf("%s (empty store): %q is null, the spec says array", route, prop)
			}
		}
	}

	for route, target := range requests {
		op, ok := spec.Paths[route]["get"]
		if !ok {
			t.Errorf("%s: no GET in openapi.yaml", route)
			continue
		}
		want := responseProperties(spec, op)
		if want == nil {
			t.Errorf("%s: the 200 response has no object schema to compare", route)
			continue
		}

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest("GET", target, nil))
		if rec.Code != http.StatusOK {
			t.Errorf("%s: GET %s = %d: %s", route, target, rec.Code, rec.Body.String())
			continue
		}
		var body map[string]json.RawMessage
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Errorf("%s: body is not a JSON object: %v", route, err)
			continue
		}
		got := map[string]bool{}
		for k := range body {
			got[k] = true
		}
		for k := range got {
			if !want[k] {
				t.Errorf("%s: the handler emits %q but the spec's 200 response lacks it", route, k)
			}
		}
		for k := range want {
			if !got[k] {
				t.Errorf("%s: the spec's 200 response lists %q but the handler did not emit it (keys: %s)",
					route, k, strings.Join(sortedKeys(got), " "))
			}
		}
	}
}

// responseSchema resolves the 200 application/json schema of an operation
// to its top-level properties, following $ref and allOf.
func responseSchema(spec *specDoc, op specOperation) map[string]specSchema {
	content, ok := op.Responses["200"].Content["application/json"]
	if !ok {
		return nil
	}
	props := map[string]specSchema{}
	var collect func(s specSchema)
	collect = func(s specSchema) {
		if s.Ref != "" {
			name := s.Ref[strings.LastIndex(s.Ref, "/")+1:]
			collect(spec.Components.Schemas[name])
			return
		}
		for _, part := range s.AllOf {
			collect(part)
		}
		for name, ps := range s.Properties {
			props[name] = ps
		}
	}
	collect(content.Schema)
	return props
}

// responseProperties is responseSchema reduced to the property names.
func responseProperties(spec *specDoc, op specOperation) map[string]bool {
	schema := responseSchema(spec, op)
	if len(schema) == 0 {
		return nil
	}
	props := map[string]bool{}
	for name := range schema {
		props[name] = true
	}
	return props
}

// emptyFake is a store with nothing in it: every list is nil, every lookup
// misses. fakeOps with no fields set is exactly that for the node and point
// readers; the rest are spelled out.
type emptyFake struct {
	fakeOps
}

func (f *emptyFake) GetDomains(context.Context) ([]storage.DomainInfo, error) { return nil, nil }
func (f *emptyFake) GetStats(context.Context, time.Time, string) (*database.NetworkStats, error) {
	return &database.NetworkStats{}, nil
}
func (f *emptyFake) GetLatestStatsDate(context.Context, string) (time.Time, error) {
	return time.Time{}, nil
}
func (f *emptyFake) GetAvailableDates(context.Context, string) ([]time.Time, error) { return nil, nil }
func (f *emptyFake) GetNearestAvailableDate(_ context.Context, d time.Time, _ string) (time.Time, error) {
	return d, nil
}
func (f *emptyFake) GetPointlistDates(context.Context, string, string) ([]database.PointlistFile, error) {
	return nil, nil
}
func (f *emptyFake) GetPointlistSources(context.Context, string) ([]storage.PointlistSourceInfo, error) {
	return nil, nil
}
func (f *emptyFake) GetBinkPSoftwareDistribution(context.Context, int, string) (*storage.SoftwareDistribution, error) {
	return &storage.SoftwareDistribution{}, nil
}
func (f *emptyFake) GetIFCICOSoftwareDistribution(context.Context, int, string) (*storage.SoftwareDistribution, error) {
	return &storage.SoftwareDistribution{}, nil
}
func (f *emptyFake) GetBinkdDetailedStats(context.Context, int, string) (*storage.SoftwareDistribution, error) {
	return &storage.SoftwareDistribution{}, nil
}
func (f *emptyFake) GetGeoHostingDistribution(context.Context, int, string) (*storage.GeoHostingDistribution, error) {
	return &storage.GeoHostingDistribution{}, nil
}
func (f *emptyFake) GetPingTraceSummary(context.Context, string, int) (*storage.PingTraceSummary, error) {
	return &storage.PingTraceSummary{}, nil
}
func (f *emptyFake) GetNodePings(context.Context, string, int, int, int, int) ([]pingtrace.Ping, error) {
	return nil, nil
}
func (f *emptyFake) GetNodePingReplies(context.Context, string, int, int, int, int) ([]storage.PingReplyRow, error) {
	return nil, nil
}
func (f *emptyFake) GetNodeTestHistory(context.Context, int, int, int, int, string) ([]storage.NodeTestResult, error) {
	return nil, nil
}
func (f *emptyFake) GetDetailedTestResult(context.Context, int, int, int, string, string) (*storage.NodeTestResult, error) {
	return nil, nil
}
func (f *emptyFake) GetNodeReachabilityStats(context.Context, int, int, int, int, string) (*storage.NodeReachabilityStats, error) {
	return nil, nil
}
func (f *emptyFake) GetReachabilityTrends(context.Context, int, string) ([]storage.ReachabilityTrend, error) {
	return nil, nil
}
func (f *emptyFake) GetReachabilityTrendsAllTime(context.Context, string) ([]storage.ReachabilityTrend, error) {
	return nil, nil
}
func (f *emptyFake) SearchNodesByReachability(context.Context, storage.ReachabilityFilter) ([]storage.NodeTestResult, error) {
	return nil, nil
}
func (f *emptyFake) GetPSTNNodes(context.Context, int, int, string) ([]storage.PSTNNode, error) {
	return nil, nil
}
func (f *emptyFake) GetPSTNDeadNodes(context.Context) ([]storage.PSTNDeadNode, error) {
	return nil, nil
}
func (f *emptyFake) GetRecentModemSuccessPhones(context.Context, int) ([]string, error) {
	return nil, nil
}

// specFake answers every reader the API has with one populated record, so a
// handler emits each key its response can carry.
type specFake struct {
	fakeOps
}

func newSpecFake() *specFake {
	region := 50
	node := sampleNode()
	node.Region = &region
	node.InternetConfig = json.RawMessage(`{}`)
	node.RawLine = ",100,Test_System,Moscow,A_Sysop,-Unpublished-,300,CM"
	point := database.Point{Zone: 2, Net: 5001, Node: 100, PointNum: 1, Domain: "fidonet", InternetConfig: json.RawMessage(`{}`), RawLine: ",1,x,y,z,-,300"}
	f := &specFake{}
	f.nodes = []database.Node{node}
	f.history = []database.Node{node, node}
	f.changes = []database.NodeChange{{Date: node.NodelistDate, ChangeType: "added"}}
	f.nodeDomains = []string{"fidonet"}
	f.points = []database.Point{point}
	f.pointHistory = []database.Point{point}
	f.pointDomains = []string{"fidonet"}
	f.sysops = []storage.SysopInfo{{Name: "A_Sysop"}}
	f.sysopNodes = []database.Node{node}
	return f
}

func (f *specFake) GetDomains(context.Context) ([]storage.DomainInfo, error) {
	return []storage.DomainInfo{{Domain: "fidonet"}}, nil
}

func (f *specFake) GetStats(context.Context, time.Time, string) (*database.NetworkStats, error) {
	return &database.NetworkStats{}, nil
}

func (f *specFake) GetLatestStatsDate(context.Context, string) (time.Time, error) {
	return time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC), nil
}

func (f *specFake) GetAvailableDates(context.Context, string) ([]time.Time, error) {
	return []time.Time{time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC)}, nil
}

func (f *specFake) GetNearestAvailableDate(_ context.Context, d time.Time, _ string) (time.Time, error) {
	return d, nil
}

func (f *specFake) GetPointlistDates(context.Context, string, string) ([]database.PointlistFile, error) {
	return []database.PointlistFile{{}}, nil
}

func (f *specFake) GetPointlistSources(context.Context, string) ([]storage.PointlistSourceInfo, error) {
	return []storage.PointlistSourceInfo{{}}, nil
}

func (f *specFake) GetBinkPSoftwareDistribution(context.Context, int, string) (*storage.SoftwareDistribution, error) {
	return &storage.SoftwareDistribution{}, nil
}

func (f *specFake) GetIFCICOSoftwareDistribution(context.Context, int, string) (*storage.SoftwareDistribution, error) {
	return &storage.SoftwareDistribution{}, nil
}

func (f *specFake) GetBinkdDetailedStats(context.Context, int, string) (*storage.SoftwareDistribution, error) {
	return &storage.SoftwareDistribution{}, nil
}

func (f *specFake) GetGeoHostingDistribution(context.Context, int, string) (*storage.GeoHostingDistribution, error) {
	return &storage.GeoHostingDistribution{}, nil
}

func (f *specFake) GetPingTraceSummary(context.Context, string, int) (*storage.PingTraceSummary, error) {
	return &storage.PingTraceSummary{}, nil
}

func (f *specFake) GetNodePings(context.Context, string, int, int, int, int) ([]pingtrace.Ping, error) {
	return []pingtrace.Ping{{}}, nil
}

func (f *specFake) GetNodePingReplies(context.Context, string, int, int, int, int) ([]storage.PingReplyRow, error) {
	return []storage.PingReplyRow{{}}, nil
}

func (f *specFake) GetNodeTestHistory(context.Context, int, int, int, int, string) ([]storage.NodeTestResult, error) {
	return []storage.NodeTestResult{{Domain: "fidonet"}}, nil
}

func (f *specFake) GetDetailedTestResult(context.Context, int, int, int, string, string) (*storage.NodeTestResult, error) {
	return &storage.NodeTestResult{Domain: "fidonet", DerivedFromAddress: "2:5001/100"}, nil
}

func (f *specFake) GetNodeReachabilityStats(context.Context, int, int, int, int, string) (*storage.NodeReachabilityStats, error) {
	return &storage.NodeReachabilityStats{}, nil
}

func (f *specFake) GetReachabilityTrends(context.Context, int, string) ([]storage.ReachabilityTrend, error) {
	return []storage.ReachabilityTrend{{}}, nil
}

func (f *specFake) GetReachabilityTrendsAllTime(context.Context, string) ([]storage.ReachabilityTrend, error) {
	return []storage.ReachabilityTrend{{}}, nil
}

func (f *specFake) SearchNodesByReachability(context.Context, storage.ReachabilityFilter) ([]storage.NodeTestResult, error) {
	return []storage.NodeTestResult{{}}, nil
}

func (f *specFake) GetPSTNNodes(context.Context, int, int, string) ([]storage.PSTNNode, error) {
	return []storage.PSTNNode{{}}, nil
}

func (f *specFake) GetPSTNDeadNodes(context.Context) ([]storage.PSTNDeadNode, error) {
	return []storage.PSTNDeadNode{{}}, nil
}

func (f *specFake) GetRecentModemSuccessPhones(context.Context, int) ([]string, error) {
	return []string{"+7"}, nil
}
