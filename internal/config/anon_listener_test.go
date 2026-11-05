package config

import (
	"strings"
	"testing"
)

func TestValidateAnonListeners(t *testing.T) {
	ok := AnonListener{Name: "tor", Address: "127.0.0.1:8082"}
	cases := []struct {
		name string
		in   []AnonListener
		want string // substring of the error, "" for none
	}{
		{"none", nil, ""},
		{"valid pair", []AnonListener{ok, {Name: "i2p", Address: "127.0.0.1:8083"}}, ""},
		{"ipv6 loopback", []AnonListener{{Name: "tor", Address: "[::1]:8082"}}, ""},
		{"public bind", []AnonListener{{Name: "tor", Address: "0.0.0.0:8082"}}, "loopback"},
		{"vcn address", []AnonListener{{Name: "tor", Address: "10.0.0.197:8082"}}, "loopback"},
		{"hostname", []AnonListener{{Name: "tor", Address: "localhost:8082"}}, "loopback"},
		{"no port", []AnonListener{{Name: "tor", Address: "127.0.0.1"}}, "address"},
		{"bad port", []AnonListener{{Name: "tor", Address: "127.0.0.1:0"}}, "port"},
		{"empty name", []AnonListener{{Address: "127.0.0.1:8082"}}, "name"},
		{"upper case", []AnonListener{{Name: "Tor", Address: "127.0.0.1:8082"}}, "name"},
		{"reserved", []AnonListener{{Name: "main", Address: "127.0.0.1:8082"}}, "reserved"},
		{"same name", []AnonListener{ok, {Name: "tor", Address: "127.0.0.1:8083"}}, "duplicate"},
		{"same address", []AnonListener{ok, {Name: "i2p", Address: "127.0.0.1:8082"}}, "duplicate"},
		{"negative rate", []AnonListener{{Name: "tor", Address: "127.0.0.1:8082", DownloadBytesPerSec: -1}}, "negative"},
		{"negative slots", []AnonListener{{Name: "tor", Address: "127.0.0.1:8082", MaxConcurrentDownloads: -1}}, "negative"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateAnonListeners(tc.in)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}

func TestAnonListenerDefaults(t *testing.T) {
	got := AnonListener{Name: "tor"}.WithDefaults()
	if got.DownloadBytesPerSec != DefaultAnonDownloadBytesPerSec || got.MaxConcurrentDownloads != DefaultAnonMaxDownloads ||
		got.ExpensiveRPS != DefaultAnonExpensiveRPS || got.ExpensiveBurst != DefaultAnonExpensiveBurst {
		t.Errorf("defaults = %+v", got)
	}
	set := AnonListener{DownloadBytesPerSec: 1, MaxConcurrentDownloads: 2, ExpensiveRPS: 3, ExpensiveBurst: 4}.WithDefaults()
	if set.DownloadBytesPerSec != 1 || set.MaxConcurrentDownloads != 2 || set.ExpensiveRPS != 3 || set.ExpensiveBurst != 4 {
		t.Errorf("set values overwritten: %+v", set)
	}
}
