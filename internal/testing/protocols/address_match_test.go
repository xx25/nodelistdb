package protocols

import "testing"

func TestAnnouncedAddressMatches(t *testing.T) {
	tests := []struct {
		name      string
		announced []string
		expected  string
		want      bool
	}{
		{"exact", []string{"2:5030/0"}, "2:5030/0", true},
		{"among several AKAs", []string{"1:123/755", "2:5030/0", "21:1/100"}, "2:5030/0", true},
		// A node announcing its own point 0 is announcing itself.
		{"explicit point zero", []string{"2:5030/0.0"}, "2:5030/0", true},
		{"domain suffix", []string{"2:5030/0@fidonet"}, "2:5030/0", true},
		{"case and space", []string{"  2:5030/0@FidoNet  "}, "2:5030/0", true},
		{"expected side is normalized too", []string{"2:5030/0"}, "2:5030/0.0@FIDONET", true},

		{"different node", []string{"2:5030/1"}, "2:5030/0", false},
		// The whole point of the check: whoever now holds a node's old IP is
		// not that node, however healthy their mailer looks.
		{"stranger's mailer", []string{"1:1/0"}, "2:5030/0", false},
		{"real point is not the node", []string{"2:5030/0.7"}, "2:5030/0", false},
		{"nothing announced", nil, "2:5030/0", false},
		{"empty announcement", []string{}, "2:5030/0", false},
		{"nothing expected", []string{"2:5030/0"}, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := announcedAddressMatches(tt.announced, tt.expected); got != tt.want {
				t.Errorf("announcedAddressMatches(%v, %q) = %v, want %v", tt.announced, tt.expected, got, tt.want)
			}
		})
	}
}
