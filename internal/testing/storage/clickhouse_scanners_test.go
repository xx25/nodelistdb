package storage

import (
	"testing"

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
