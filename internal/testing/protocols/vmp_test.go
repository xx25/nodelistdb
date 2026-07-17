package protocols

import "testing"

func TestLooksLikeVMP(t *testing.T) {
	tests := []struct {
		name    string
		in      []byte
		wantOK  bool
		wantCmd int
	}{
		{
			// The exact disconnect frame captured from live OS/2 VMODEM nodes.
			name:    "live disconnect frame",
			in:      []byte{0x10, 0x02, 0x00, 0x04, 0x00, 0x01, 0x00, 0x08},
			wantOK:  true,
			wantCmd: 1,
		},
		{
			// Payload byte 0x10 arrives DLE-stuffed as 0x10 0x10 on the wire.
			name:    "stuffed payload",
			in:      []byte{0x10, 0x02, 0x00, 0x04, 0x00, 0x03, 0x00, 0x10, 0x10},
			wantOK:  true,
			wantCmd: 3,
		},
		{
			// Truncated read: marker + length + command word present, rest not.
			name:    "truncated but decodable",
			in:      []byte{0x10, 0x02, 0x00, 0x04, 0x00, 0x01},
			wantOK:  true,
			wantCmd: 1,
		},
		{name: "wrong marker", in: []byte{0x10, 0x03, 0x00, 0x04, 0x00, 0x01}, wantOK: false},
		{name: "emsi not vmp", in: []byte("**EMSI_REQA77E\r"), wantOK: false},
		{name: "command out of range", in: []byte{0x10, 0x02, 0x00, 0x04, 0x00, 0x99, 0x00, 0x00}, wantOK: false},
		{name: "too short", in: []byte{0x10, 0x02}, wantOK: false},
		{name: "implausible length", in: []byte{0x10, 0x02, 0xff, 0xff, 0x00, 0x01}, wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, cmd := looksLikeVMP(tt.in)
			if ok != tt.wantOK {
				t.Fatalf("looksLikeVMP ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && cmd != tt.wantCmd {
				t.Fatalf("looksLikeVMP command = %d, want %d", cmd, tt.wantCmd)
			}
		})
	}
}

func TestUnstuffDLE(t *testing.T) {
	got := unstuffDLE([]byte{0x00, 0x10, 0x10, 0x03})
	want := []byte{0x00, 0x10, 0x03}
	if string(got) != string(want) {
		t.Fatalf("unstuffDLE = % x, want % x", got, want)
	}
}
