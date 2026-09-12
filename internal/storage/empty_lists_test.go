package storage

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestAnalyticsListsAreNeverNil pins the shape the API promises: a report
// with nothing in it carries empty arrays, not nulls. The mappers used to
// start from `var stats []T`, so a window without a single handshake
// published "software_types": null against a schema that says array.
func TestAnalyticsListsAreNeverNil(t *testing.T) {
	empty := map[string]int{}
	if mapToSoftwareTypeStats(empty, 0) == nil {
		t.Error("mapToSoftwareTypeStats(empty) = nil, want []")
	}
	if mapToVersionStats(empty, 0) == nil {
		t.Error("mapToVersionStats(empty) = nil, want []")
	}
	if mapToOSStats(empty, 0) == nil {
		t.Error("mapToOSStats(empty) = nil, want []")
	}
	if mapToBinkdVersionStats(empty, 0) == nil {
		t.Error("mapToBinkdVersionStats(empty) = nil, want []")
	}

	body, err := json.Marshal(newPingTraceSummary("fidonet", 90))
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"nodes":[]`, `"tracers":[]`, `"robots":[]`} {
		if !strings.Contains(string(body), key) {
			t.Errorf("empty PING summary lacks %s: %s", key, body)
		}
	}
}
