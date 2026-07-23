package storage

import (
	"encoding/json"
	"testing"
)

func TestDetectInternetConfigChangesDefaults(t *testing.T) {
	tests := []struct {
		name string
		prev string
		curr string
		want map[string]string
	}{
		{
			// Rows written before INA became a list stored a bare string.
			// Re-shaping the same value must not show up as a node change.
			name: "legacy scalar to single-element list is not a change",
			prev: `{"defaults":{"INA":"bbs.example.org"},"protocols":{"IBN":[{"port":24554}]}}`,
			curr: `{"defaults":{"INA":["bbs.example.org"]},"protocols":{"IBN":[{"port":24554}]}}`,
			want: map[string]string{},
		},
		{
			name: "second INA added",
			prev: `{"defaults":{"INA":["first.example.org"]},"protocols":{"IBN":[{"port":24554}]}}`,
			curr: `{"defaults":{"INA":["first.example.org","second.example.org"]},"protocols":{"IBN":[{"port":24554}]}}`,
			want: map[string]string{"inet_INA": "first.example.org → first.example.org, second.example.org"},
		},
		{
			name: "INA replaced",
			prev: `{"defaults":{"INA":"old.example.org"},"protocols":{"IBN":[{"port":24554}]}}`,
			curr: `{"defaults":{"INA":["new.example.org"]},"protocols":{"IBN":[{"port":24554}]}}`,
			want: map[string]string{"inet_INA": "old.example.org → new.example.org"},
		},
		{
			name: "INA removed",
			prev: `{"defaults":{"INA":["a.example.org","b.example.org"]},"protocols":{"IBN":[{"port":24554}]}}`,
			curr: `{"protocols":{"IBN":[{"port":24554}]}}`,
			want: map[string]string{"inet_INA": "Removed a.example.org, b.example.org"},
		},
		{
			name: "config added from empty",
			prev: `{}`,
			curr: `{"defaults":{"INA":["a.example.org","b.example.org"]}}`,
			want: map[string]string{"inet_INA": "Added a.example.org, b.example.org"},
		},
	}

	so := &SearchOperations{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := so.detectInternetConfigChanges(json.RawMessage(tt.prev), json.RawMessage(tt.curr))

			if len(got) != len(tt.want) {
				t.Fatalf("changes = %v, want %v", got, tt.want)
			}
			for key, wantVal := range tt.want {
				if got[key] != wantVal {
					t.Errorf("changes[%q] = %q, want %q", key, got[key], wantVal)
				}
			}
		})
	}
}
