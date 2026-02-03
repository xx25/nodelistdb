package flags

import (
	"testing"
)

func TestFileRequestCapabilities(t *testing.T) {
	tests := []struct {
		flag               string
		valid              bool
		barkFile           bool
		barkUpdate         bool
		wazooFile          bool
		wazooUpdate        bool
		hasAny             bool
		hasBark            bool
		hasWaZOO           bool
		hasFull            bool
	}{
		// XA: All capabilities
		{"XA", true, true, true, true, true, true, true, true, true},
		// XB: Bark all + WaZOO file only
		{"XB", true, true, true, true, false, true, true, true, false},
		// XC: Bark file + WaZOO all
		{"XC", true, true, false, true, true, true, true, true, false},
		// XP: Bark only
		{"XP", true, true, true, false, false, true, true, false, false},
		// XR: Bark file + WaZOO file
		{"XR", true, true, false, true, false, true, true, true, false},
		// XW: WaZOO file only
		{"XW", true, false, false, true, false, true, false, true, false},
		// XX: WaZOO only
		{"XX", true, false, false, true, true, true, false, true, false},
		// Invalid flag
		{"YY", false, false, false, false, false, false, false, false, false},
		// Empty flag
		{"", false, false, false, false, false, false, false, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.flag, func(t *testing.T) {
			caps, valid := GetFileRequestCapabilities(tt.flag)

			if valid != tt.valid {
				t.Errorf("GetFileRequestCapabilities(%q) valid = %v, want %v", tt.flag, valid, tt.valid)
			}

			if !valid {
				return
			}

			if caps.BarkFileRequest != tt.barkFile {
				t.Errorf("BarkFileRequest = %v, want %v", caps.BarkFileRequest, tt.barkFile)
			}
			if caps.BarkUpdateRequest != tt.barkUpdate {
				t.Errorf("BarkUpdateRequest = %v, want %v", caps.BarkUpdateRequest, tt.barkUpdate)
			}
			if caps.WaZOOFileRequest != tt.wazooFile {
				t.Errorf("WaZOOFileRequest = %v, want %v", caps.WaZOOFileRequest, tt.wazooFile)
			}
			if caps.WaZOOUpdateRequest != tt.wazooUpdate {
				t.Errorf("WaZOOUpdateRequest = %v, want %v", caps.WaZOOUpdateRequest, tt.wazooUpdate)
			}
			if caps.HasAnyCapability() != tt.hasAny {
				t.Errorf("HasAnyCapability() = %v, want %v", caps.HasAnyCapability(), tt.hasAny)
			}
			if caps.HasBarkSupport() != tt.hasBark {
				t.Errorf("HasBarkSupport() = %v, want %v", caps.HasBarkSupport(), tt.hasBark)
			}
			if caps.HasWaZOOSupport() != tt.hasWaZOO {
				t.Errorf("HasWaZOOSupport() = %v, want %v", caps.HasWaZOOSupport(), tt.hasWaZOO)
			}
			if caps.HasFullSupport() != tt.hasFull {
				t.Errorf("HasFullSupport() = %v, want %v", caps.HasFullSupport(), tt.hasFull)
			}
		})
	}
}

func TestGetFileRequestCapabilitiesFromFlags(t *testing.T) {
	tests := []struct {
		name     string
		flags    []string
		wantFlag string
		wantAny  bool
	}{
		{
			name:     "XA in middle",
			flags:    []string{"CM", "IBN", "XA", "V34"},
			wantFlag: "XA",
			wantAny:  true,
		},
		{
			name:     "XX at start",
			flags:    []string{"XX", "CM", "IBN"},
			wantFlag: "XX",
			wantAny:  true,
		},
		{
			name:     "XW at end",
			flags:    []string{"CM", "IBN", "V34", "XW"},
			wantFlag: "XW",
			wantAny:  true,
		},
		{
			name:     "no file request flag",
			flags:    []string{"CM", "IBN", "V34"},
			wantFlag: "",
			wantAny:  false,
		},
		{
			name:     "empty flags",
			flags:    []string{},
			wantFlag: "",
			wantAny:  false,
		},
		{
			name:     "first X-flag wins",
			flags:    []string{"XP", "XA", "XX"},
			wantFlag: "XP",
			wantAny:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caps := GetFileRequestCapabilitiesFromFlags(tt.flags)

			if caps.Flag != tt.wantFlag {
				t.Errorf("Flag = %q, want %q", caps.Flag, tt.wantFlag)
			}
			if caps.HasAnyCapability() != tt.wantAny {
				t.Errorf("HasAnyCapability() = %v, want %v", caps.HasAnyCapability(), tt.wantAny)
			}
		})
	}
}

func TestIsFileRequestFlag(t *testing.T) {
	validFlags := []string{"XA", "XB", "XC", "XP", "XR", "XW", "XX"}
	invalidFlags := []string{"CM", "MO", "IBN", "V34", "XY", "AX", "", "xa", "Xa"}

	for _, flag := range validFlags {
		if !IsFileRequestFlag(flag) {
			t.Errorf("IsFileRequestFlag(%q) = false, want true", flag)
		}
	}

	for _, flag := range invalidFlags {
		if IsFileRequestFlag(flag) {
			t.Errorf("IsFileRequestFlag(%q) = true, want false", flag)
		}
	}
}

func TestGetFileRequestDescription(t *testing.T) {
	// Test that descriptions match GetFlagDescriptions (no drift)
	flagDescriptions := GetFlagDescriptions()

	for _, flag := range FileRequestFlagList {
		t.Run(flag, func(t *testing.T) {
			desc := GetFileRequestDescription(flag)
			if desc == "" {
				t.Errorf("GetFileRequestDescription(%q) returned empty", flag)
			}

			// Verify it matches the canonical description
			if info, ok := flagDescriptions[flag]; ok {
				if desc != info.Description {
					t.Errorf("GetFileRequestDescription(%q) = %q, want %q (from GetFlagDescriptions)", flag, desc, info.Description)
				}
			} else {
				t.Errorf("Flag %q not found in GetFlagDescriptions", flag)
			}
		})
	}

	// Invalid flag returns empty
	if desc := GetFileRequestDescription("YY"); desc != "" {
		t.Errorf("GetFileRequestDescription(YY) = %q, want empty", desc)
	}
}

func TestGetSoftwareForFlag(t *testing.T) {
	// XA should have several software packages
	xaSoftware := GetSoftwareForFlag("XA")
	if len(xaSoftware) < 5 {
		t.Errorf("GetSoftwareForFlag(XA) returned %d items, expected at least 5", len(xaSoftware))
	}

	// XX should have several software packages
	xxSoftware := GetSoftwareForFlag("XX")
	if len(xxSoftware) < 5 {
		t.Errorf("GetSoftwareForFlag(XX) returned %d items, expected at least 5", len(xxSoftware))
	}

	// Invalid flag returns empty slice (not nil) for JSON safety
	invalidSoftware := GetSoftwareForFlag("YY")
	if invalidSoftware == nil {
		t.Errorf("GetSoftwareForFlag(YY) = nil, want empty slice")
	}
	if len(invalidSoftware) != 0 {
		t.Errorf("GetSoftwareForFlag(YY) = %v, want empty slice", invalidSoftware)
	}
}

func TestFileRequestFlagList(t *testing.T) {
	expectedFlags := map[string]bool{
		"XA": true, "XB": true, "XC": true, "XP": true,
		"XR": true, "XW": true, "XX": true,
	}

	// Verify we have exactly 7 flags
	if len(FileRequestFlagList) != 7 {
		t.Errorf("FileRequestFlagList has %d items, want 7", len(FileRequestFlagList))
	}

	// Verify all flags in the list are valid and expected
	seen := make(map[string]bool)
	for _, flag := range FileRequestFlagList {
		if !IsFileRequestFlag(flag) {
			t.Errorf("FileRequestFlagList contains invalid flag %q", flag)
		}
		if !expectedFlags[flag] {
			t.Errorf("FileRequestFlagList contains unexpected flag %q", flag)
		}
		if seen[flag] {
			t.Errorf("FileRequestFlagList contains duplicate flag %q", flag)
		}
		seen[flag] = true
	}

	// Verify all expected flags are present
	for flag := range expectedFlags {
		if !seen[flag] {
			t.Errorf("FileRequestFlagList missing expected flag %q", flag)
		}
	}
}

func TestFileRequestFlagConsistency(t *testing.T) {
	// Verify that flags with category "filerequest" in GetFlagDescriptions
	// match exactly with IsFileRequestFlag
	descriptions := GetFlagDescriptions()

	for flag, info := range descriptions {
		isFileReq := IsFileRequestFlag(flag)
		hasFileReqCategory := info.Category == "filerequest"

		if isFileReq != hasFileReqCategory {
			t.Errorf("Flag %q: IsFileRequestFlag=%v but category=%q (inconsistent)",
				flag, isFileReq, info.Category)
		}
	}

	// Also verify all flags in fileRequestFlags map have "filerequest" category
	for _, flag := range FileRequestFlagList {
		if info, ok := descriptions[flag]; ok {
			if info.Category != "filerequest" {
				t.Errorf("Flag %q is in FileRequestFlagList but has category %q", flag, info.Category)
			}
		} else {
			t.Errorf("Flag %q is in FileRequestFlagList but not in GetFlagDescriptions", flag)
		}
	}
}
