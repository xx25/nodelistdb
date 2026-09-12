package api

import (
	"sort"
	"strings"
	"testing"
)

// TestQueryParamsMatchTheSpec compares the query parameters each handler
// reads with the ones openapi.yaml documents for its route. The handler side
// is this table, kept by hand: chi does not know which query keys a handler
// touches, and several are read through helpers (parseDaysParam,
// domainOrAll) that name the key internally. It is still the cheapest way
// to catch a parameter added to a handler and never written down, which is
// how ?domain= went undocumented on nine routes and is_mo/has_inet/has_binkp
// on the node search.
func TestQueryParamsMatchTheSpec(t *testing.T) {
	handlerParams := map[string][]string{
		"get /api/health":                                     {},
		"get /api/networks":                                   {},
		"get /api/nodes":                                      {"domain", "zone", "net", "node", "system_name", "location", "sysop_name", "node_type", "is_cm", "is_mo", "has_inet", "has_binkp", "date_from", "date_to", "latest_only", "limit", "offset"},
		"get /api/nodes/pstn":                                 {"domain", "zone", "limit"},
		"get /api/nodes/pstn/dead":                            {},
		"get /api/nodes/pstn/recent-success":                  {"days"},
		"get /api/nodes/{zone}/{net}/{node}":                  {"domain"},
		"get /api/nodes/{zone}/{net}/{node}/history":          {"domain"},
		"get /api/nodes/{zone}/{net}/{node}/changes":          {"domain"},
		"get /api/nodes/{zone}/{net}/{node}/timeline":         {"domain"},
		"get /api/nodes/{zone}/{net}/{node}/points":           {"domain", "date"},
		"get /api/nodes/{zone}/{net}/{node}/ping":             {"domain", "limit"},
		"get /api/nodes/{zone}/{net}/{node}/tests":            {"domain", "days"},
		"get /api/nodes/{zone}/{net}/{node}/tests/detail":     {"domain", "time"},
		"get /api/reachability/trends":                        {"domain", "days"},
		"get /api/reachability/nodes":                         {"domain", "status", "protocol", "days", "limit"},
		"get /api/points":                                     {"domain", "zone", "net", "node", "point", "list_source", "system_name", "location", "sysop_name", "date_from", "date_to", "latest_only", "limit", "offset"},
		"get /api/points/{zone}/{net}/{node}/{point}":         {"domain"},
		"get /api/points/{zone}/{net}/{node}/{point}/history": {"domain"},
		"get /api/pointlists/dates":                           {"domain", "source"},
		"get /api/pointlists/sources":                         {"domain"},
		"get /api/sysops":                                     {"name", "limit", "offset"},
		"get /api/sysops/{name}/nodes":                        {"limit"},
		"get /api/stats":                                      {"domain", "date"},
		"get /api/stats/dates":                                {"domain"},
		"get /api/flags":                                      {"category", "flag"},
		"get /api/software/binkp":                             {"domain", "days"},
		"get /api/software/ifcico":                            {"domain", "days"},
		"get /api/software/binkd":                             {"domain", "days"},
		"get /api/analytics/geo-hosting":                      {"domain", "days"},
		"get /api/analytics/pingtrace":                        {"domain", "days"},
		"get /api/nodelist/latest":                            {"domain"},
		"get /api/cache/stats":                                {},
		"get /api/ratelimit/stats":                            {},
		"get /api/ftp/stats":                                  {},
		"get /api/openapi.yaml":                               {},
		"get /api/docs":                                       {},
		"post /api/modem/results/direct":                      {},
		"post /api/modem/pstn-dead":                           {},
		"delete /api/modem/pstn-dead":                         {},
	}

	spec := loadSpec(t)
	seen := map[string]bool{}
	for path, ops := range spec.Paths {
		for method, op := range ops {
			key := method + " " + path
			seen[key] = true
			want, ok := handlerParams[key]
			if !ok {
				t.Errorf("%s: not in this test's table", key)
				continue
			}
			var got []string
			for _, p := range op.Parameters {
				name, in := p.Name, p.In
				if p.Ref != "" {
					name, in = referencedParam(p.Ref)
					if in == "" {
						t.Errorf("%s: parameter %s is not known to referencedParam", key, p.Ref)
					}
				}
				if in == "query" {
					got = append(got, name)
				}
			}
			sort.Strings(got)
			w := append([]string(nil), want...)
			sort.Strings(w)
			if strings.Join(got, ",") != strings.Join(w, ",") {
				t.Errorf("%s: spec documents query params [%s], handler reads [%s]", key, strings.Join(got, " "), strings.Join(w, " "))
			}
		}
	}
	for key := range handlerParams {
		if !seen[key] {
			t.Errorf("%s: in this test's table but not in openapi.yaml", key)
		}
	}
}

// referencedParam maps a #/components/parameters/X reference to its query
// name and location. The shared parameters are few enough to list.
func referencedParam(ref string) (name, in string) {
	switch ref[strings.LastIndex(ref, "/")+1:] {
	case "Zone", "Net", "Node", "PointNumber":
		return "", "path"
	case "DomainResolved", "DomainDefaultFidonet", "DomainDefaultAll":
		return "domain", "query"
	case "AnalyticsDays":
		return "days", "query"
	}
	return "", ""
}
