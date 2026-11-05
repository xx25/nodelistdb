package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func robotsGroups(t *testing.T) map[string][]string {
	t.Helper()
	groups := map[string][]string{}
	var agents []string
	inRules := false
	for _, raw := range strings.Split(string(robotsTxt), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		field, value, ok := strings.Cut(line, ":")
		if !ok {
			t.Fatalf("bad line %q", line)
		}
		value = strings.TrimSpace(value)
		if strings.EqualFold(field, "User-agent") {
			if inRules {
				agents, inRules = nil, false
			}
			agents = append(agents, value)
			continue
		}
		inRules = true
		if strings.EqualFold(field, "Disallow") {
			for _, a := range agents {
				groups[a] = append(groups[a], value)
			}
		}
	}
	return groups
}

// TestRobotsGroupsRepeatTheCommonRules: a crawler obeys only the most specific
// group naming it and groups do not inherit from "*" (RFC 9309), so a group that
// only adds /download/ would hand that crawler every report on the site.
func TestRobotsGroupsRepeatTheCommonRules(t *testing.T) {
	groups := robotsGroups(t)
	common := []string{"/reachability", "/analytics", "/api/", "/search"}
	if len(groups["*"]) == 0 {
		t.Fatal("no * group")
	}
	for agent, rules := range groups {
		if len(rules) == 1 && rules[0] == "/" {
			continue
		}
		have := strings.Join(rules, " ")
		for _, want := range common {
			if !strings.Contains(" "+have+" ", " "+want+" ") {
				t.Errorf("%s: missing Disallow %s", agent, want)
			}
		}
	}
}

// TestRobotsLeavesTheArchiveOpen: distributing the archive is the site's job.
func TestRobotsLeavesTheArchiveOpen(t *testing.T) {
	for _, rule := range robotsGroups(t)["*"] {
		if strings.HasPrefix("/download/nodelist/fidonet/2026/nodelist.309", rule) || rule == "/" {
			t.Errorf("* group blocks downloads with %q", rule)
		}
	}
}

// TestRobotsRouteBeatsTheCatchAll: "/" is a catch-all, and /robots.txt used to
// answer 200 with the search page.
func TestRobotsRouteBeatsTheCatchAll(t *testing.T) {
	s := &Server{}
	mux := http.NewServeMux()
	s.SetupRoutes(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/robots.txt", nil))
	if rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("Content-Type = %q", ct)
	}
	if !strings.Contains(rec.Body.String(), "User-agent: *") {
		t.Fatal("body is not the robots file")
	}
}

// TestUnknownPathsAreNotSearches: "/" is the mux's catch-all, so every unknown
// path rendered the search page with a 200, and /favicon.ico, being exempt from
// the rate limiter, ran searches with no limit. The nil storage panics if the
// handler reaches it.
func TestUnknownPathsAreNotSearches(t *testing.T) {
	s := &Server{}
	mux := http.NewServeMux()
	s.SetupRoutes(mux)

	for _, path := range []string{"/favicon.ico", "/.env", "/robots.txt/anything", "/wp-login.php"} {
		for _, method := range []string{"GET", "POST"} {
			req := httptest.NewRequest(method, path+"?sysop_name=Smith", strings.NewReader("sysop_name=Smith"))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			if rec.Code != http.StatusNotFound {
				t.Errorf("%s %s = %d, want 404", method, path, rec.Code)
			}
		}
	}
}
