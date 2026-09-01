package api

import (
	"net/http"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/nodelistdb/internal/config"
	"gopkg.in/yaml.v3"
)

// TestRoutesMatchTheSpec walks the real router and compares it with
// openapi.yaml. Seven live routes were missing from the spec - every optional
// one, plus the three PSTN endpoints the modem tester calls - and nothing
// noticed, because a spec that omits an endpoint still serves and still
// renders.
//
// The router is built with every optional dependency installed. router.go
// registers the cache-stats, FTP-stats and modem routes only when the matching
// field is non-nil, and those are exactly the routes that went missing.
func TestRoutesMatchTheSpec(t *testing.T) {
	body, err := os.ReadFile("openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Paths map[string]map[string]interface{} `yaml:"paths"`
	}
	if err := yaml.Unmarshal(body, &spec); err != nil {
		t.Fatalf("parsing openapi.yaml: %v", err)
	}

	s := New(&fakeOps{})
	s.SetCacheStatsHandler(func(http.ResponseWriter, *http.Request) {})
	s.SetFTPStatsHandler(func(http.ResponseWriter, *http.Request) {})
	s.SetRateLimitStatsHandler(func(http.ResponseWriter, *http.Request) {})
	s.SetModemHandler(NewModemHandler(&config.ModemAPIConfig{MaxBodySizeMB: 1}, nil))
	router := s.SetupRouter()

	live := map[string]bool{}
	err = chi.Walk(router.(chi.Routes), func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		// chi reports a trailing slash for a subrouter's index route.
		if route != "/" {
			route = strings.TrimSuffix(route, "/")
		}
		live[strings.ToLower(method)+" "+route] = true
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	documented := map[string]bool{}
	for path, ops := range spec.Paths {
		for method := range ops {
			switch method {
			case "get", "post", "put", "patch", "delete", "head", "options":
				documented[method+" "+path] = true
			}
		}
	}

	var undocumented, unimplemented []string
	for op := range live {
		if !documented[op] {
			undocumented = append(undocumented, op)
		}
	}
	for op := range documented {
		if !live[op] {
			unimplemented = append(unimplemented, op)
		}
	}
	sort.Strings(undocumented)
	sort.Strings(unimplemented)

	for _, op := range undocumented {
		t.Errorf("%s is routed but missing from openapi.yaml", op)
	}
	for _, op := range unimplemented {
		t.Errorf("%s is in openapi.yaml but not routed", op)
	}
}
