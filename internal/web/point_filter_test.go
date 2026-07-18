package web

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// buildPointFilterFromForm gates the points query that runs on every search:
// too-short text terms must SKIP the search (the trigram indexes can't prune
// them, and dropping the term from the filter would show points that ignore
// the user's text), never broaden it.
func TestBuildPointFilterFromFormTextMinLength(t *testing.T) {
	cases := []struct {
		name   string
		vals   url.Values
		wantOK bool
	}{
		{"zone only is a usable constraint", url.Values{"zone": {"2"}}, true},
		{"bare node number is refused", url.Values{"node": {"100"}}, false},
		{"2-char system name skips points search", url.Values{"zone": {"2"}, "system_name": {"Jo"}}, false},
		{"3-char system name searches", url.Values{"system_name": {"Joe"}}, true},
		{"2-char location skips even with zone", url.Values{"zone": {"2"}, "location": {"NY"}}, false},
		{"3-char location searches", url.Values{"location": {"NYC"}}, true},
		{"2-char sysop skips points search", url.Values{"sysop_name": {"Jo"}}, false},
		{"3-char sysop searches", url.Values{"sysop_name": {"Joe"}}, true},
		{"2-char Cyrillic term passes the byte-length floor", url.Values{"location": {"Мо"}}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/", strings.NewReader(tc.vals.Encode()))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			if err := r.ParseForm(); err != nil {
				t.Fatalf("ParseForm: %v", err)
			}
			filter, ok := buildPointFilterFromForm(r)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v (filter %+v)", ok, tc.wantOK, filter)
			}
			// A provided-but-short text term must never survive as a
			// broadened filter: when ok is false nothing may be searched,
			// and when ok is true every provided text field must be present.
			if ok {
				if v := tc.vals.Get("system_name"); v != "" && (filter.SystemName == nil || *filter.SystemName != v) {
					t.Errorf("system_name dropped from filter: %+v", filter)
				}
				if v := tc.vals.Get("location"); v != "" && (filter.Location == nil || *filter.Location != v) {
					t.Errorf("location dropped from filter: %+v", filter)
				}
			}
		})
	}
}
