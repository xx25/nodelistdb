package pingtrace

import (
	"reflect"
	"testing"
	"time"
)

func TestParseViaLine(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want Hop
	}{
		{
			name: "canonical FTS-4009 with domain and UTC",
			in:   "2:5001/100@fidonet @20260903.120000.UTC FidoMail 0.1.3",
			want: Hop{Address: "2:5001/100", Time: time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC), TimeIsUTC: true, Software: "FidoMail 0.1.3"},
		},
		{
			name: "wire kludge prefix",
			in:   "\x01Via 2:5020/715 @20260903.121530 hpt/lnx 1.9.0-cur 2024-01-01",
			want: Hop{Address: "2:5020/715", Time: time.Date(2026, 9, 3, 12, 15, 30, 0, time.UTC), Software: "hpt/lnx 1.9.0-cur 2024-01-01"},
		},
		{
			name: "visible ^AVia spelling with four-digit clock",
			in:   "^AVia 1:1/19 @20260904.0301 FrontDoor 2.33",
			want: Hop{Address: "1:1/19", Time: time.Date(2026, 9, 4, 3, 1, 0, 0, time.UTC), Software: "FrontDoor 2.33"},
		},
		{
			name: "deprecated comma form",
			in:   "@Via FidoMail 1.0, 2:5020/100, 20260517 03:00:00",
			want: Hop{Address: "2:5020/100", Time: time.Date(2026, 5, 17, 3, 0, 0, 0, time.UTC), Software: "FidoMail 1.0"},
		},
		{
			name: "ISO-ish timestamp",
			in:   "Via 2:5020/100.5, binkd/1.1a, 2026.05.17 03:00:00",
			want: Hop{Address: "2:5020/100.5", Time: time.Date(2026, 5, 17, 3, 0, 0, 0, time.UTC), Software: "binkd/1.1a"},
		},
		{
			name: "no address at all is kept raw",
			in:   "ZoneGate V8.1 by Alexey Presniakov, id : 493C",
			want: Hop{Software: "ZoneGate V8.1 by Alexey Presniakov id : 493C"},
		},
		{
			name: "precise field is skipped",
			in:   "2:280/5555 @20260903.120000.W123.UTC FMail 2.3",
			want: Hop{Address: "2:280/5555", Time: time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC), TimeIsUTC: true, Software: "FMail 2.3"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ParseViaLine(tc.in)
			if !ok {
				t.Fatalf("ParseViaLine(%q) not ok", tc.in)
			}
			got.Raw = ""
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ParseViaLine(%q)\n got %+v\nwant %+v", tc.in, got, tc.want)
			}
		})
	}
	if _, ok := ParseViaLine("   "); ok {
		t.Error("blank line must not parse")
	}
}

func TestExtractPathOnlyTakesViaLines(t *testing.T) {
	body := "Your PING message arrived at its destination 2:280/5555 on 03 Sep 2026 14:00:00 UTC.\r\n" +
		"\r\n" +
		"Original message:\r\n" +
		"  From: NodelistDB, 2:5001/100\r\n" +
		"  MSGID: 2:5001/100 68b8a1c2\r\n" +
		"\r\n" +
		"It travelled via:\r\n" +
		" > @Via 2:5001/100@fidonet @20260903.120000.UTC FidoMail 0.1.3\r\n" +
		"@Via 2:5020/715 @20260903.121530 hpt/lnx 1.9.0-cur\r\n" +
		"@Via 2:5020/715 @20260903.121530 hpt/lnx 1.9.0-cur\r\n" +
		"@Via 2:280/5555 @20260903.135900.UTC FMail 2.3\r\n" +
		"(this system appends its own Via line on the forwarded copy)\r\n" +
		"\r\n" +
		"Quoted message text:\r\n" +
		"> Netmail PING from 2:5001/100 -- see 2:5020/0 for details.\r\n"

	got := Addresses(ExtractPath(body))
	want := []string{"2:5001/100", "2:5020/715", "2:280/5555"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ExtractPath addresses = %v, want %v", got, want)
	}
}

func TestParseViasKeepsEveryElement(t *testing.T) {
	hops := ParseVias([]string{
		"2:280/5555 @20260903.140000.UTC FMail 2.3",
		"ZoneGate V8.1 by Alexey Presniakov, id : 493C",
		"",
		"2:5020/715 @20260903.150000 hpt/lnx 1.9.0",
	})
	if len(hops) != 3 {
		t.Fatalf("got %d hops, want 3", len(hops))
	}
	if hops[1].Address != "" || hops[1].Raw == "" {
		t.Errorf("address-less line must keep its raw text: %+v", hops[1])
	}
	if hops[2].Address != "2:5020/715" {
		t.Errorf("third hop = %+v", hops[2])
	}
}

func TestNode3D(t *testing.T) {
	if Node3D("2:5020/100.5") != "2:5020/100" || Node3D("2:5020/100") != "2:5020/100" {
		t.Error("Node3D must drop the point only")
	}
}

func TestExtractPathAcceptsBareCanonicalLines(t *testing.T) {
	body := "Via lines:\n" +
		"2:5001/100@fidonet @20260903.120000.UTC FidoMail 0.1.3\n" +
		"2:5020/715 @20260903.123000 hpt/lnx 1.9.0\n" +
		"Please contact 2:5020/0 for details.\n"
	got := Addresses(ExtractPath(body))
	want := []string{"2:5001/100", "2:5020/715"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("bare canonical lines = %v, want %v", got, want)
	}
}

// TestTimeIsUTCInVia covers the flag's round trip: it has no column of its
// own in the ping tables, so a hop read back from storage re-derives it
// from the raw line.
func TestTimeIsUTCInVia(t *testing.T) {
	cases := map[string]bool{
		"2:5020/715 @20260903.234155.UTC RNtrack 2.3.0/Lnx/Perl": true,
		"2:292/854 @20260904.014604 D'Bridge 4":                  false,
		"ZoneGate V8.1 by Alexey Presniakov, id : 493C":          false,
		"": false,
	}
	for raw, want := range cases {
		if got := TimeIsUTCInVia(raw); got != want {
			t.Errorf("TimeIsUTCInVia(%q) = %v, want %v", raw, got, want)
		}
	}
}
