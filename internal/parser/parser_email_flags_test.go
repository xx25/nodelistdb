package parser

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/nodelistdb/internal/database"
	"github.com/nodelistdb/internal/emailflags"
)

// extractEmail runs a flags string through the real parser and then through
// the capability resolver, exercising the whole pipeline: where the parser
// files each flag, and how the resolver reassembles it.
func extractEmail(t *testing.T, flagsStr string, opts emailflags.Options) []emailflags.Capability {
	t.Helper()

	p := New(false)
	flags, configJSON := p.parseFlagsWithConfig(flagsStr)

	var ic *database.InternetConfiguration
	if len(configJSON) > 0 {
		var parsed database.InternetConfiguration
		if err := json.Unmarshal(configJSON, &parsed); err != nil {
			t.Fatalf("internet_config from %q did not round-trip: %v (raw: %s)", flagsStr, err, configJSON)
		}
		ic = &parsed
	}

	return emailflags.Extract(flags, ic, opts)
}

// capSummary renders capabilities as "FLAG=addr1|addr2(source)" for compact
// table-test comparison.
func capSummary(caps []emailflags.Capability) string {
	parts := make([]string, 0, len(caps))
	for _, c := range caps {
		parts = append(parts, c.Flag+"="+strings.Join(c.Addresses, "|")+"("+c.Source.String()+")")
	}
	return strings.Join(parts, " ")
}

// TestEmailFlagResolution covers the FTS-5001 examples and the combinations a
// nodelist actually produces.
func TestEmailFlagResolution(t *testing.T) {
	tests := []struct {
		name  string
		flags string
		want  string
	}{
		{
			name:  "IEM alone supplies an address but names no method",
			flags: "CM,IEM:user@example.net",
			want:  "IEM=user@example.net(explicit)",
		},
		{
			name:  "IEM supplies the default address for a bare method flag",
			flags: "IEM:user@example.net,IMI",
			want:  "IEM=user@example.net(explicit) IMI=user@example.net(IEM default)",
		},
		{
			name:  "address attached directly to the method flag",
			flags: "IMI:user@example.net",
			want:  "IMI=user@example.net(explicit)",
		},
		{
			name:  "one IEM address shared by two encodings",
			flags: "IEM:user@example.net,IUC,IMI",
			want:  "IEM=user@example.net(explicit) IMI=user@example.net(IEM default) IUC=user@example.net(IEM default)",
		},
		{
			name:  "SEAT carries its own address, IUC inherits from it",
			flags: "ISE:user@example.net,IUC",
			want:  "ISE=user@example.net(explicit) IUC=user@example.net(other email flag)",
		},
		{
			name:  "full SEAT declaration with both encodings",
			flags: "IEM:user@example.net,ISE,IUC,IMI",
			want:  "IEM=user@example.net(explicit) IMI=user@example.net(IEM default) ISE=user@example.net(IEM default) IUC=user@example.net(IEM default)",
		},
		{
			name:  "multihomed: repeated IEM yields an endpoint pool",
			flags: "IEM:first@example.net,IEM:second@example.net,ISE,IUC",
			want:  "IEM=first@example.net|second@example.net(explicit) ISE=first@example.net|second@example.net(IEM default) IUC=first@example.net|second@example.net(IEM default)",
		},
		{
			name:  "distinct addresses per method are kept apart",
			flags: "IUC:uu@example.net,IMI:mime@example.net",
			want:  "IMI=mime@example.net(explicit) IUC=uu@example.net(explicit)",
		},
		{
			name:  "bare method flag with no address anywhere",
			flags: "IMI",
			want:  "IMI=(unresolved)",
		},
		{
			name:  "bare IEM with no address anywhere",
			flags: "IEM",
			want:  "IEM=(unresolved)",
		},
		{
			name:  "non-standard EMA with an address is still reported",
			flags: "EMA:user@example.net",
			want:  "EMA=user@example.net(explicit)",
		},
		{
			name:  "non-standard EVY with an address is still reported",
			flags: "EVY:user@example.net",
			want:  "EVY=user@example.net(explicit)",
		},
		{
			name:  "unrelated flags produce no capabilities",
			flags: "CM,XA,IBN:bbs.example.org,INA:bbs.example.org",
			want:  "",
		},
		{
			name:  "INA is never used as an email address",
			flags: "INA:bbs.example.org,IMI",
			want:  "IMI=(unresolved)",
		},
		{
			name:  "duplicate identical IEM collapses to one address",
			flags: "IEM:user@example.net,IEM:user@example.net",
			want:  "IEM=user@example.net(explicit)",
		},
		{
			name:  "flag order does not affect resolution",
			flags: "IMI,IEM:user@example.net",
			want:  "IEM=user@example.net(explicit) IMI=user@example.net(IEM default)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := capSummary(extractEmail(t, tt.flags, emailflags.Options{}))
			if got != tt.want {
				t.Errorf("flags %q\n got: %s\nwant: %s", tt.flags, got, tt.want)
			}
		})
	}
}

// TestEmailFlagMalformedInput checks that bad nodelist data is preserved and
// reported rather than crashing or silently vanishing.
func TestEmailFlagMalformedInput(t *testing.T) {
	tests := []struct {
		name          string
		flags         string
		wantFlags     []string
		wantAddresses map[string][]string
		wantMalformed map[string][]string
	}{
		{
			name:          "IEM with empty value still advertises the capability",
			flags:         "IEM:",
			wantFlags:     []string{"IEM"},
			wantAddresses: map[string][]string{"IEM": nil},
		},
		{
			name:          "IMI with empty value still advertises the capability",
			flags:         "IMI:",
			wantFlags:     []string{"IMI"},
			wantAddresses: map[string][]string{"IMI": nil},
		},
		{
			name:          "value that is not an address is reported as malformed",
			flags:         "IEM:not-an-address",
			wantFlags:     []string{"IEM"},
			wantAddresses: map[string][]string{"IEM": nil},
			wantMalformed: map[string][]string{"IEM": {"not-an-address"}},
		},
		{
			name:  "extra colon is part of the address, not a port",
			flags: "IEM:user@example.net:extra",
			// FTS-5001: the email flags do not carry a port number, so the
			// whole remainder is the value. It is not a valid address, so it
			// surfaces as malformed rather than being silently truncated.
			wantFlags:     []string{"IEM"},
			wantMalformed: map[string][]string{"IEM": {"user@example.net:extra"}},
		},
		{
			name:      "different addresses on IEM and IMI both survive",
			flags:     "IEM:user@example.net,IMI:user2@example.net",
			wantFlags: []string{"IEM", "IMI"},
			wantAddresses: map[string][]string{
				"IEM": {"user@example.net"},
				"IMI": {"user2@example.net"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caps := extractEmail(t, tt.flags, emailflags.Options{})

			var gotFlags []string
			for _, c := range caps {
				gotFlags = append(gotFlags, c.Flag)
			}
			if strings.Join(gotFlags, ",") != strings.Join(tt.wantFlags, ",") {
				t.Fatalf("flags %q: got capabilities %v, want %v", tt.flags, gotFlags, tt.wantFlags)
			}

			for _, c := range caps {
				if want, ok := tt.wantAddresses[c.Flag]; ok {
					if strings.Join(c.Addresses, "|") != strings.Join(want, "|") {
						t.Errorf("flags %q: %s addresses = %v, want %v", tt.flags, c.Flag, c.Addresses, want)
					}
				}
				if want, ok := tt.wantMalformed[c.Flag]; ok {
					if strings.Join(c.Malformed, "|") != strings.Join(want, "|") {
						t.Errorf("flags %q: %s malformed = %v, want %v", tt.flags, c.Flag, c.Malformed, want)
					}
				}
			}
		})
	}
}

// TestEmailFlagsReachInternetConfig pins the Phase 1 parser fix: every
// recognised email flag must land in internet_config, in both its bare and
// addressed form. Before the fix a bare IEM and an addressed IUC/EMA/EVY were
// left verbatim in the flags array, where no internet_config reader saw them.
func TestEmailFlagsReachInternetConfig(t *testing.T) {
	for _, flag := range []string{"IEM", "IMI", "ITX", "ISE", "IUC", "EMA", "EVY"} {
		for _, form := range []struct {
			name  string
			flags string
		}{
			{"bare", flag},
			{"addressed", flag + ":user@example.net"},
		} {
			t.Run(flag+"/"+form.name, func(t *testing.T) {
				p := New(false)
				flags, configJSON := p.parseFlagsWithConfig(form.flags)

				for _, raw := range flags {
					if strings.HasPrefix(strings.ToUpper(raw), flag) {
						t.Errorf("%q left %q in the raw flags array; it should be in internet_config", form.flags, raw)
					}
				}
				if len(configJSON) == 0 {
					t.Fatalf("%q produced no internet_config", form.flags)
				}
				if !strings.Contains(string(configJSON), flag) {
					t.Errorf("%q produced internet_config without %s: %s", form.flags, flag, configJSON)
				}
			})
		}
	}
}

// TestEmailFlagLegacyStorageShapes verifies the resolver still sees flags
// stored the pre-fix way, since those rows are permanent.
func TestEmailFlagLegacyStorageShapes(t *testing.T) {
	tests := []struct {
		name       string
		flags      []string
		configJSON string
		want       string
	}{
		{
			name:       "bare IEM left in the flags array",
			flags:      []string{"CM", "IEM"},
			configJSON: "",
			want:       "IEM=(unresolved)",
		},
		{
			name:       "addressed EMA left in the flags array",
			flags:      []string{"EMA:user@example.net"},
			configJSON: "",
			want:       "EMA=user@example.net(explicit)",
		},
		{
			name:       "addressed IUC in flags, IMI in config, sharing nothing",
			flags:      []string{"IUC:uu@example.net"},
			configJSON: `{"email_protocols":{"IMI":[{"email":"mime@example.net"}]}}`,
			want:       "IMI=mime@example.net(explicit) IUC=uu@example.net(explicit)",
		},
		{
			name:       "legacy scalar defaults.IEM still supplies the default",
			flags:      nil,
			configJSON: `{"defaults":{"IEM":"user@example.net"},"email_protocols":{"IMI":[{}]}}`,
			want:       "IEM=user@example.net(explicit) IMI=user@example.net(IEM default)",
		},
		{
			name:       "legacy bare-object email_protocols still decodes",
			flags:      nil,
			configJSON: `{"defaults":{"IEM":["user@example.net"]},"email_protocols":{"IMI":{"email":"mime@example.net"}}}`,
			want:       "IEM=user@example.net(explicit) IMI=mime@example.net(explicit)",
		},
		{
			name:       "bare IEM in flags plus addressed IMI in config",
			flags:      []string{"IEM"},
			configJSON: `{"email_protocols":{"IMI":[{"email":"mime@example.net"}]}}`,
			want:       "IEM=mime@example.net(other email flag) IMI=mime@example.net(explicit)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ic *database.InternetConfiguration
			if tt.configJSON != "" {
				var parsed database.InternetConfiguration
				if err := json.Unmarshal([]byte(tt.configJSON), &parsed); err != nil {
					t.Fatalf("could not decode %s: %v", tt.configJSON, err)
				}
				ic = &parsed
			}

			got := capSummary(emailflags.Extract(tt.flags, ic, emailflags.Options{}))
			if got != tt.want {
				t.Errorf("\n got: %s\nwant: %s", got, tt.want)
			}
		})
	}
}

// TestEmailFlagFieldFallback covers the FSP-1012 section 2.3.4 recovery of an
// address from Location or System Name, which is off unless requested.
func TestEmailFlagFieldFallback(t *testing.T) {
	const flags = "IMI"

	t.Run("disabled by default", func(t *testing.T) {
		caps := extractEmail(t, flags, emailflags.Options{Location: "user@example.net"})
		if got := capSummary(caps); got != "IMI=(unresolved)" {
			t.Errorf("field fallback fired without being enabled: %s", got)
		}
	})

	t.Run("location field when enabled", func(t *testing.T) {
		caps := extractEmail(t, flags, emailflags.Options{
			Location:         "user@example.net",
			UseFieldFallback: true,
		})
		if got := capSummary(caps); got != "IMI=user@example.net(location field)" {
			t.Errorf("got %s", got)
		}
	})

	t.Run("address embedded in an underscored location", func(t *testing.T) {
		caps := extractEmail(t, flags, emailflags.Options{
			Location:         "Moscow_Russia_user@example.net",
			UseFieldFallback: true,
		})
		if got := capSummary(caps); got != "IMI=user@example.net(location field)" {
			t.Errorf("got %s", got)
		}
	})

	t.Run("system name used only when location has nothing", func(t *testing.T) {
		caps := extractEmail(t, flags, emailflags.Options{
			Location:         "Moscow_Russia",
			SystemName:       "sysop@example.net",
			UseFieldFallback: true,
		})
		if got := capSummary(caps); got != "IMI=sysop@example.net(system name)" {
			t.Errorf("got %s", got)
		}
	})

	t.Run("ordinary location text is not mistaken for an address", func(t *testing.T) {
		caps := extractEmail(t, flags, emailflags.Options{
			Location:         "Saint_Petersburg_Russia",
			SystemName:       "The_Lighthouse_BBS",
			UseFieldFallback: true,
		})
		if got := capSummary(caps); got != "IMI=(unresolved)" {
			t.Errorf("field fallback matched prose: %s", got)
		}
	})

	t.Run("an explicit address always wins over the fields", func(t *testing.T) {
		caps := extractEmail(t, "IMI:real@example.net", emailflags.Options{
			Location:         "decoy@example.net",
			UseFieldFallback: true,
		})
		if got := capSummary(caps); got != "IMI=real@example.net(explicit)" {
			t.Errorf("got %s", got)
		}
	})
}

// TestEmailFlagSpecProperties pins the per-flag facts FTS-5001 states.
func TestEmailFlagSpecProperties(t *testing.T) {
	tests := []struct {
		flag                  string
		standard              bool
		receiptRequired       bool
		wireProtocolSpecified bool
	}{
		{"IEM", true, false, false},
		{"IUC", true, false, false},
		{"IMI", true, false, false},
		{"ITX", true, true, false},
		{"ISE", true, true, true},
		{"EMA", false, false, false},
		{"EVY", false, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.flag, func(t *testing.T) {
			caps := extractEmail(t, tt.flag+":user@example.net", emailflags.Options{})
			if len(caps) != 1 {
				t.Fatalf("expected exactly one capability, got %d", len(caps))
			}
			c := caps[0]
			if c.Standard != tt.standard {
				t.Errorf("Standard = %v, want %v", c.Standard, tt.standard)
			}
			if c.ReceiptRequired != tt.receiptRequired {
				t.Errorf("ReceiptRequired = %v, want %v", c.ReceiptRequired, tt.receiptRequired)
			}
			if c.WireProtocolSpecified != tt.wireProtocolSpecified {
				t.Errorf("WireProtocolSpecified = %v, want %v", c.WireProtocolSpecified, tt.wireProtocolSpecified)
			}
		})
	}
}
