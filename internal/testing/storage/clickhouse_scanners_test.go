package storage

import (
	"testing"
	"time"

	"github.com/nodelistdb/internal/testing/models"
)

func TestApplyInternetConfigHostnames(t *testing.T) {
	tests := []struct {
		name       string
		configJSON string
		want       []string
	}{
		{
			name:       "empty config",
			configJSON: "{}",
			want:       []string{},
		},
		{
			name:       "legacy scalar INA",
			configJSON: `{"defaults":{"INA":"bbs.example.org"},"protocols":{"IBN":[{"port":24554}]}}`,
			want:       []string{"bbs.example.org"},
		},
		{
			name:       "repeated INA keeps every hostname",
			configJSON: `{"defaults":{"INA":["first.example.org","second.example.org"]},"protocols":{"IBN":[{"port":24554}]}}`,
			want:       []string{"first.example.org", "second.example.org"},
		},
		{
			name:       "protocol addresses come before INA defaults",
			configJSON: `{"defaults":{"INA":["ina.example.org"]},"protocols":{"IBN":[{"address":"ibn.example.org","port":24554}]}}`,
			want:       []string{"ibn.example.org", "ina.example.org"},
		},
		{
			name:       "INA duplicating a protocol address is not repeated",
			configJSON: `{"defaults":{"INA":["bbs.example.org"]},"protocols":{"IBN":[{"address":"bbs.example.org","port":24554}]}}`,
			want:       []string{"bbs.example.org"},
		},
		{
			name:       "multiple protocol addresses in sorted protocol order",
			configJSON: `{"protocols":{"ITN":[{"address":"itn.example.org","port":23}],"IBN":[{"address":"ibn.example.org","port":24554}]}}`,
			want:       []string{"ibn.example.org", "itn.example.org"},
		},
		{
			name:       "old single-object protocol format",
			configJSON: `{"protocols":{"IBN":{"address":"ibn.example.org","port":24554}}}`,
			want:       []string{"ibn.example.org"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := &models.Node{}
			applyInternetConfig(node, tt.configJSON)

			if len(node.InternetHostnames) != len(tt.want) {
				t.Fatalf("hostnames = %q, want %q", node.InternetHostnames, tt.want)
			}
			for i := range tt.want {
				if node.InternetHostnames[i] != tt.want[i] {
					t.Errorf("hostname %d = %q, want %q", i, node.InternetHostnames[i], tt.want[i])
				}
			}
		})
	}
}

// The hostname list drives hostname_index in stored test results, so a node's
// hostnames must not reshuffle between cycles.
func TestApplyInternetConfigHostnameOrderIsStable(t *testing.T) {
	configJSON := `{"defaults":{"INA":["ina1.example.org","ina2.example.org"]},` +
		`"protocols":{"IBN":[{"address":"ibn.example.org","port":24554}],` +
		`"IFC":[{"address":"ifc.example.org","port":60179}],` +
		`"ITN":[{"address":"itn.example.org","port":23}]}}`

	first := &models.Node{}
	applyInternetConfig(first, configJSON)

	for i := 0; i < 50; i++ {
		node := &models.Node{}
		applyInternetConfig(node, configJSON)

		if len(node.InternetHostnames) != len(first.InternetHostnames) {
			t.Fatalf("run %d: hostnames = %q, want %q", i, node.InternetHostnames, first.InternetHostnames)
		}
		for j := range first.InternetHostnames {
			if node.InternetHostnames[j] != first.InternetHostnames[j] {
				t.Fatalf("run %d: hostnames = %q, want %q", i, node.InternetHostnames, first.InternetHostnames)
			}
		}
	}
}

func TestApplyInternetConfigProtocolPorts(t *testing.T) {
	node := &models.Node{}
	applyInternetConfig(node, `{"protocols":{"IBN":[{"address":"bbs.example.org","port":2424}],"IVM":[{"port":3141}]}}`)

	if got := node.GetProtocolPort("IBN"); got != 2424 {
		t.Errorf("IBN port = %d, want 2424", got)
	}
	if got := node.GetProtocolPort("IVM"); got != 3141 {
		t.Errorf("IVM port = %d, want 3141", got)
	}
	if got := node.GetProtocolPort("ITN"); got != 0 {
		t.Errorf("ITN port = %d, want 0 (default)", got)
	}
}

// TestJoinHopsRestoresTheUTCMarker: ping_tests keeps each hop's raw Via
// line but has no column for the FTS-4009 "UTC" marker, so the flag is
// re-derived from that line when a ping is read back.
func TestJoinHopsRestoresTheUTCMarker(t *testing.T) {
	addrs := []string{"2:5020/715", "2:292/854"}
	times := []time.Time{
		time.Date(2026, 9, 3, 23, 41, 55, 0, time.UTC),
		time.Date(2026, 9, 4, 1, 46, 4, 0, time.UTC),
	}
	soft := []string{"RNtrack 2.3.0/Lnx/Perl", "D'Bridge 4"}
	raw := []string{
		"2:5020/715 @20260903.234155.UTC RNtrack 2.3.0/Lnx/Perl",
		"2:292/854 @20260904.014604 D'Bridge 4",
	}
	hops := joinHops(addrs, times, soft, raw)
	if len(hops) != 2 || !hops[0].TimeIsUTC || hops[1].TimeIsUTC {
		t.Errorf("UTC marker lost on read: %+v", hops)
	}
}
