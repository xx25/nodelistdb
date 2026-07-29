package web

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// A protocol analytics page makes one of two claims, and the two need different
// queries behind them:
//
//	"Nodes that have been successfully tested with X protocol"
//	    → the probe is the whole claim. Gating on the nodelist flag would drop
//	      AKA-derived results, whose probe succeeded against the same host under
//	      another network's entry, and the page would contradict itself.
//	"...on their announced IVM port"
//	    → the nodelist flag is part of the claim, and a probe alone cannot
//	      establish it. 2:221/1, IVM-free since 2014, was listed for want of
//	      this gate.
//
// storage.announcementGatedProtocols decides which query each page gets, but the
// claim lives here in the page copy, one package away. This test fails when the
// copy crosses the line without the map following — the sort of drift nothing
// else would surface, because both pages keep rendering perfectly either way.
func TestProtocolPageCopyMatchesItsGate(t *testing.T) {
	body, err := os.ReadFile("handlers_analytics.go")
	if err != nil {
		t.Fatal(err)
	}

	// Each protocol page handler ends by delegating to renderAnalyticsNodes
	// with the storage method naming its protocol.
	handler := regexp.MustCompile(`(?s)config := ProtocolPageConfig\{(.*?)renderAnalyticsNodes\(w, r, config, "unified_analytics", s\.storage\.Get(\w+?)EnabledNodes\)`)

	// Protocols whose page copy asserts an announcement, per
	// storage.announcementGatedProtocols. Keep the two in step.
	wantAnnouncementClaim := map[string]bool{"vmodem": true}

	seen := map[string]bool{}
	for _, m := range handler.FindAllStringSubmatch(string(body), -1) {
		config, protocol := m[1], strings.ToLower(m[2])
		seen[protocol] = true

		claimsAnnouncement := strings.Contains(config, "announced")
		if claimsAnnouncement != wantAnnouncementClaim[protocol] {
			if claimsAnnouncement {
				t.Errorf("/analytics/%s now claims an announcement in its page copy, but its query gates on "+
					"nodelist membership only; add %q to storage.announcementGatedProtocols", protocol, protocol)
			} else {
				t.Errorf("/analytics/%s no longer claims an announcement, but its query still gates on the "+
					"protocol flag, which drops AKA-derived results; remove %q from storage.announcementGatedProtocols",
					protocol, protocol)
			}
		}
	}

	want := []string{"binkp", "ftp", "ifcico", "telnet", "vmodem"}
	var got []string
	for p := range seen {
		got = append(got, p)
	}
	sort.Strings(got)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("found protocol pages %v, want %v — a page was added, removed or renamed, "+
			"so this test is no longer checking what it thinks it is", got, want)
	}
}
