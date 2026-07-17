package protocols

import (
	"strings"
	"testing"
)

// binkpNul builds a binkp M_NUL command frame carrying s.
func binkpNul(s string) []byte {
	body := append([]byte{0}, []byte(s)...) // 0 == M_NUL
	hdr := 0x8000 | len(body)
	return append([]byte{byte(hdr >> 8), byte(hdr)}, body...)
}

func TestParseBinkpGreeting(t *testing.T) {
	frame := append(binkpNul("SYS Test BBS"), binkpNul("VER binkd/1.1a-115/OS2 binkp/1.1")...)
	sw, sys, ok := parseBinkpGreeting(frame)
	if !ok {
		t.Fatal("expected binkp greeting to be recognized")
	}
	if sw != "binkd/1.1a-115/OS2 binkp/1.1" {
		t.Errorf("software = %q", sw)
	}
	if sys != "Test BBS" {
		t.Errorf("systemName = %q", sys)
	}

	if _, _, ok := parseBinkpGreeting([]byte("**EMSI_REQ")); ok {
		t.Error("EMSI bytes should not parse as binkp")
	}
}

func TestHasEMSIMarker(t *testing.T) {
	cases := []struct {
		text   string
		nudged bool
		want   bool
	}{
		{"**EMSI_REQA77E\r", true, true},
		{"**EMSI_DAT...", false, true},
		{"**EMSI_INQ\r", true, false}, // that's what we send
		{"**EMSI_INQ\r", false, true}, // peer-originated
		{"login: ", false, false},
	}
	for _, c := range cases {
		if got := hasEMSIMarker(c.text, c.nudged); got != c.want {
			t.Errorf("hasEMSIMarker(%q, nudged=%v) = %v, want %v", c.text, c.nudged, got, c.want)
		}
	}
}

func TestSniffSoftware(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Platinum Xpress/Win/WINServer v10.0gr12/PX96-0649M\r\n**EMSI_REQ", "Platinum Xpress"},
		{"\r\rFrontDoor/2 2.32.mL/CS000362; MultiLine", "FrontDoor"},
		{"\x00\x00**EMSI_REQ from ifcico 2.14-tx8.10", "ifcico"},
		{"Welcome to the BBS, please login:", ""},
	}
	for _, c := range cases {
		got := sniffSoftware(c.in)
		if c.want == "" {
			if got != "" {
				t.Errorf("sniffSoftware(%q) = %q, want empty", c.in, got)
			}
			continue
		}
		if !strings.Contains(got, c.want) {
			t.Errorf("sniffSoftware(%q) = %q, want to contain %q", c.in, got, c.want)
		}
	}
}

func TestIdentifyBanner(t *testing.T) {
	cases := []struct{ in, want string }{
		{"SSH-2.0-OpenSSH_8.4", "ssh"},
		{"220 mail.example.com ESMTP Postfix", "smtp"},
		{"220 FTP server ready", "ftp"},
		{"HTTP/1.1 200 OK", "http"},
		{"random noise", ""},
	}
	for _, c := range cases {
		if got, _ := identifyBanner(c.in); got != c.want {
			t.Errorf("identifyBanner(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
