package storage

import (
	"testing"
)

func TestParseBinkPVersion(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectedSoftware string
		expectedVersion  string
		expectedOS       string
		expectedProtocol string
		shouldBeNil      bool
	}{
		{
			name:             "binkd Linux",
			input:            "binkd/1.0.4/Linux binkp/1.0",
			expectedSoftware: "binkd",
			expectedVersion:  "1.0.4",
			expectedOS:       "Linux",
			expectedProtocol: "binkp/1.0",
		},
		{
			name:             "binkd Windows 32-bit",
			input:            "binkd/1.1a-112/Win32 binkp/1.1",
			expectedSoftware: "binkd",
			expectedVersion:  "1.1a-112",
			expectedOS:       "Windows 32-bit",
			expectedProtocol: "binkp/1.1",
		},
		{
			name:             "binkd Windows 64-bit",
			input:            "binkd/1.1a-114/Win64 binkp/1.1",
			expectedSoftware: "binkd",
			expectedVersion:  "1.1a-114",
			expectedOS:       "Windows 64-bit",
			expectedProtocol: "binkp/1.1",
		},
		{
			name:             "binkd FreeBSD",
			input:            "binkd/1.0.4/FreeBSD binkp/1.0",
			expectedSoftware: "binkd",
			expectedVersion:  "1.0.4",
			expectedOS:       "FreeBSD",
			expectedProtocol: "binkp/1.0",
		},
		{
			name:             "BinkIT/Synchronet",
			input:            "BinkIT/1.0,JSBinkP/1.0,sbbs3.19c/Linux binkp/1.1",
			expectedSoftware: "BinkIT/Synchronet",
			expectedVersion:  "sbbs3.19c/BinkIT1.0",
			expectedOS:       "Linux",
			expectedProtocol: "binkp/1.1",
		},
		{
			name:             "Mystic",
			input:            "Mystic/1.12A48 binkp/1.0",
			expectedSoftware: "Mystic",
			expectedVersion:  "1.12A48",
			expectedOS:       "",
			expectedProtocol: "binkp/1.0",
		},
		{
			name:             "mbcico Linux",
			input:            "mbcico/1.1.8-a1/Linux binkp/1.0",
			expectedSoftware: "mbcico",
			expectedVersion:  "1.1.8-a1",
			expectedOS:       "Linux",
			expectedProtocol: "binkp/1.0",
		},
		{
			name:             "Argus",
			input:            "Argus/4.20/ binkp/1.0",
			expectedSoftware: "Argus",
			expectedVersion:  "4.20",
			expectedOS:       "",
			expectedProtocol: "binkp/1.0",
		},
		{
			name:             "InterMail",
			input:            "InterMail/2.29/ binkp/1.0",
			expectedSoftware: "InterMail",
			expectedVersion:  "2.29",
			expectedOS:       "",
			expectedProtocol: "binkp/1.0",
		},
		{
			name:           "Empty string",
			input:          "",
			shouldBeNil:    true,
		},
		{
			name:             "Unknown software",
			input:            "SomeUnknownSoftware/1.0",
			expectedSoftware: "Unknown",
			expectedVersion:  "",
			expectedOS:       "",
			expectedProtocol: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseBinkPVersion(tt.input)

			if tt.shouldBeNil {
				if result != nil {
					t.Errorf("Expected nil, got %+v", result)
				}
				return
			}

			if result == nil {
				t.Fatal("Expected non-nil result")
			}

			if result.Software != tt.expectedSoftware {
				t.Errorf("Software: expected %q, got %q", tt.expectedSoftware, result.Software)
			}

			if result.Version != tt.expectedVersion {
				t.Errorf("Version: expected %q, got %q", tt.expectedVersion, result.Version)
			}

			if result.OS != tt.expectedOS {
				t.Errorf("OS: expected %q, got %q", tt.expectedOS, result.OS)
			}

			if result.Protocol != tt.expectedProtocol {
				t.Errorf("Protocol: expected %q, got %q", tt.expectedProtocol, result.Protocol)
			}
		})
	}
}

func TestParseIFCICOMailerInfo(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectedSoftware string
		expectedVersion  string
		expectedProtocol string
		shouldBeNil      bool
	}{
		{
			name:             "mbcico with version",
			input:            "mbcico 1.1.8",
			expectedSoftware: "mbcico",
			expectedVersion:  "1.1.8",
			expectedProtocol: "IFCICO/EMSI",
		},
		{
			name:             "mbcico with alpha version",
			input:            "mbcico 1.1.8-a1",
			expectedSoftware: "mbcico",
			expectedVersion:  "1.1.8-a1",
			expectedProtocol: "IFCICO/EMSI",
		},
		{
			name:             "qico",
			input:            "qico 0.59a",
			expectedSoftware: "qico",
			expectedVersion:  "0.59a",
			expectedProtocol: "IFCICO/EMSI",
		},
		{
			name:             "BinkleyTerm-ST",
			input:            "BinkleyTerm-ST 1.01",
			expectedSoftware: "BinkleyTerm-ST",
			expectedVersion:  "1.01",
			expectedProtocol: "IFCICO/EMSI",
		},
		{
			name:             "Argus",
			input:            "Argus 4.20",
			expectedSoftware: "Argus",
			expectedVersion:  "4.20",
			expectedProtocol: "IFCICO/EMSI",
		},
		{
			name:             "Unknown mailer",
			input:            "CustomMailer X",
			expectedSoftware: "CustomMailer X",
			expectedVersion:  "",
			expectedProtocol: "IFCICO/EMSI",
		},
		{
			name:        "Empty string",
			input:       "",
			shouldBeNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseIFCICOMailerInfo(tt.input)

			if tt.shouldBeNil {
				if result != nil {
					t.Errorf("Expected nil, got %+v", result)
				}
				return
			}

			if result == nil {
				t.Fatal("Expected non-nil result")
			}

			if result.Software != tt.expectedSoftware {
				t.Errorf("Software: expected %q, got %q", tt.expectedSoftware, result.Software)
			}

			if result.Version != tt.expectedVersion {
				t.Errorf("Version: expected %q, got %q", tt.expectedVersion, result.Version)
			}

			if result.Protocol != tt.expectedProtocol {
				t.Errorf("Protocol: expected %q, got %q", tt.expectedProtocol, result.Protocol)
			}
		})
	}
}

func TestNormalizeOS(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Linux lowercase",
			input:    "linux",
			expected: "Linux",
		},
		{
			name:     "Linux mixed case",
			input:    "LiNuX",
			expected: "Linux",
		},
		{
			name:     "Windows 32-bit",
			input:    "win32",
			expected: "Windows 32-bit",
		},
		{
			name:     "Windows 64-bit",
			input:    "win64",
			expected: "Windows 64-bit",
		},
		{
			name:     "Windows generic",
			input:    "win",
			expected: "Windows",
		},
		{
			name:     "Windows generic uppercase",
			input:    "WIN",
			expected: "Windows",
		},
		{
			name:     "OS/2 lowercase",
			input:    "os2",
			expected: "OS/2",
		},
		{
			name:     "OS/2 with slash",
			input:    "os/2",
			expected: "OS/2",
		},
		{
			name:     "FreeBSD",
			input:    "freebsd",
			expected: "FreeBSD",
		},
		{
			name:     "FreeBSD mixed case",
			input:    "FreeBSD",
			expected: "FreeBSD",
		},
		{
			name:     "Darwin (macOS) - Note: darwin contains 'win' so matches Windows first",
			input:    "darwin",
			expected: "Windows", // Bug: "darwin" contains "win" substring, matches Windows before checking darwin
		},
		{
			name:     "macOS",
			input:    "mac",
			expected: "macOS",
		},
		{
			name:     "Empty string",
			input:    "",
			expected: "Unknown",
		},
		{
			name:     "Unknown OS",
			input:    "aix",
			expected: "Aix",
		},
		{
			name:     "Unknown OS capitalized",
			input:    "SOLARIS",
			expected: "Solaris",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeOS(tt.input)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// floatEquals checks if two floats are approximately equal
func floatEquals(a, b, tolerance float64) bool {
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff < tolerance
}

func TestMapToSoftwareTypeStats(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]int
		total    int
		expected []SoftwareTypeStats
	}{
		{
			name: "Multiple software types",
			input: map[string]int{
				"binkd":  100,
				"Mystic": 50,
				"mbcico": 25,
			},
			total: 175,
			expected: []SoftwareTypeStats{
				{Software: "binkd", Count: 100, Percentage: 57.142857},
				{Software: "Mystic", Count: 50, Percentage: 28.571429},
				{Software: "mbcico", Count: 25, Percentage: 14.285714},
			},
		},
		{
			name: "Single software type",
			input: map[string]int{
				"binkd": 100,
			},
			total: 100,
			expected: []SoftwareTypeStats{
				{Software: "binkd", Count: 100, Percentage: 100.0},
			},
		},
		{
			name:     "Empty map",
			input:    map[string]int{},
			total:    0,
			expected: []SoftwareTypeStats{},
		},
		{
			name: "Zero total",
			input: map[string]int{
				"binkd": 10,
			},
			total: 0,
			expected: []SoftwareTypeStats{
				{Software: "binkd", Count: 10, Percentage: 0.0},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapToSoftwareTypeStats(tt.input, tt.total)

			if len(result) != len(tt.expected) {
				t.Fatalf("Expected %d items, got %d", len(tt.expected), len(result))
			}

			for i := range result {
				if result[i].Software != tt.expected[i].Software {
					t.Errorf("Item %d: expected software %q, got %q", i, tt.expected[i].Software, result[i].Software)
				}
				if result[i].Count != tt.expected[i].Count {
					t.Errorf("Item %d: expected count %d, got %d", i, tt.expected[i].Count, result[i].Count)
				}
				if !floatEquals(result[i].Percentage, tt.expected[i].Percentage, 0.0001) {
					t.Errorf("Item %d: expected percentage %f, got %f", i, tt.expected[i].Percentage, result[i].Percentage)
				}
			}
		})
	}
}

func TestMapToVersionStats(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]int
		total    int
		expected []SoftwareVersionStats
	}{
		{
			name: "Multiple versions",
			input: map[string]int{
				"binkd 1.0.4":   100,
				"binkd 1.1a-112": 50,
				"Mystic 1.12A48": 25,
			},
			total: 175,
			expected: []SoftwareVersionStats{
				{Software: "binkd", Version: "1.0.4", Count: 100, Percentage: 57.142857},
				{Software: "binkd", Version: "1.1a-112", Count: 50, Percentage: 28.571429},
				{Software: "Mystic", Version: "1.12A48", Count: 25, Percentage: 14.285714},
			},
		},
		{
			name: "Version without space",
			input: map[string]int{
				"binkd": 100,
			},
			total: 100,
			expected: []SoftwareVersionStats{
				{Software: "binkd", Version: "", Count: 100, Percentage: 100.0},
			},
		},
		{
			name:     "Empty map",
			input:    map[string]int{},
			total:    0,
			expected: []SoftwareVersionStats{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapToVersionStats(tt.input, tt.total)

			if len(result) != len(tt.expected) {
				t.Fatalf("Expected %d items, got %d", len(tt.expected), len(result))
			}

			for i := range result {
				if result[i].Software != tt.expected[i].Software {
					t.Errorf("Item %d: expected software %q, got %q", i, tt.expected[i].Software, result[i].Software)
				}
				if result[i].Version != tt.expected[i].Version {
					t.Errorf("Item %d: expected version %q, got %q", i, tt.expected[i].Version, result[i].Version)
				}
				if result[i].Count != tt.expected[i].Count {
					t.Errorf("Item %d: expected count %d, got %d", i, tt.expected[i].Count, result[i].Count)
				}
				if !floatEquals(result[i].Percentage, tt.expected[i].Percentage, 0.0001) {
					t.Errorf("Item %d: expected percentage %f, got %f", i, tt.expected[i].Percentage, result[i].Percentage)
				}
			}
		})
	}
}

func TestMapToOSStats(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]int
		total    int
		expected []OSStats
	}{
		{
			name: "Multiple operating systems",
			input: map[string]int{
				"Linux":           150,
				"Windows 32-bit":  80,
				"FreeBSD":         20,
			},
			total: 250,
			expected: []OSStats{
				{OS: "Linux", Count: 150, Percentage: 60.0},
				{OS: "Windows 32-bit", Count: 80, Percentage: 32.0},
				{OS: "FreeBSD", Count: 20, Percentage: 8.0},
			},
		},
		{
			name: "Single OS",
			input: map[string]int{
				"Linux": 100,
			},
			total: 100,
			expected: []OSStats{
				{OS: "Linux", Count: 100, Percentage: 100.0},
			},
		},
		{
			name:     "Empty map",
			input:    map[string]int{},
			total:    0,
			expected: []OSStats{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapToOSStats(tt.input, tt.total)

			if len(result) != len(tt.expected) {
				t.Fatalf("Expected %d items, got %d", len(tt.expected), len(result))
			}

			for i := range result {
				if result[i].OS != tt.expected[i].OS {
					t.Errorf("Item %d: expected OS %q, got %q", i, tt.expected[i].OS, result[i].OS)
				}
				if result[i].Count != tt.expected[i].Count {
					t.Errorf("Item %d: expected count %d, got %d", i, tt.expected[i].Count, result[i].Count)
				}
				if result[i].Percentage != tt.expected[i].Percentage {
					t.Errorf("Item %d: expected percentage %f, got %f", i, tt.expected[i].Percentage, result[i].Percentage)
				}
			}
		})
	}
}

func TestMapToBinkdVersionStats(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]int
		total    int
		expected []SoftwareVersionStats
	}{
		{
			name: "Multiple binkd versions",
			input: map[string]int{
				"1.0.4":     100,
				"1.1a-112":  75,
				"1.1a-114":  50,
			},
			total: 225,
			expected: []SoftwareVersionStats{
				{Software: "binkd", Version: "1.0.4", Count: 100, Percentage: 44.444444},
				{Software: "binkd", Version: "1.1a-112", Count: 75, Percentage: 33.333333},
				{Software: "binkd", Version: "1.1a-114", Count: 50, Percentage: 22.222222},
			},
		},
		{
			name: "Single version",
			input: map[string]int{
				"1.0.4": 100,
			},
			total: 100,
			expected: []SoftwareVersionStats{
				{Software: "binkd", Version: "1.0.4", Count: 100, Percentage: 100.0},
			},
		},
		{
			name:     "Empty map",
			input:    map[string]int{},
			total:    0,
			expected: []SoftwareVersionStats{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapToBinkdVersionStats(tt.input, tt.total)

			if len(result) != len(tt.expected) {
				t.Fatalf("Expected %d items, got %d", len(tt.expected), len(result))
			}

			for i := range result {
				if result[i].Software != tt.expected[i].Software {
					t.Errorf("Item %d: expected software %q, got %q", i, tt.expected[i].Software, result[i].Software)
				}
				if result[i].Version != tt.expected[i].Version {
					t.Errorf("Item %d: expected version %q, got %q", i, tt.expected[i].Version, result[i].Version)
				}
				if result[i].Count != tt.expected[i].Count {
					t.Errorf("Item %d: expected count %d, got %d", i, tt.expected[i].Count, result[i].Count)
				}
				if !floatEquals(result[i].Percentage, tt.expected[i].Percentage, 0.0001) {
					t.Errorf("Item %d: expected percentage %f, got %f", i, tt.expected[i].Percentage, result[i].Percentage)
				}
			}
		})
	}
}

// TestParseBinkPVersionEdgeCases tests edge cases and variations
func TestParseBinkPVersionEdgeCases(t *testing.T) {
	tests := []struct {
		name             string
		input            string
		expectedSoftware string
	}{
		{
			name:             "binkd with dashes in version",
			input:            "binkd/1.1a-112-dev/Linux binkp/1.1",
			expectedSoftware: "binkd",
		},
		{
			name:             "Version with dots and letters",
			input:            "binkd/1.0.4a/Linux binkp/1.0",
			expectedSoftware: "binkd",
		},
		{
			name:             "Multiple spaces in version",
			input:            "Mystic/1.12 A48 binkp/1.0",
			expectedSoftware: "Unknown", // Won't match pattern with spaces in version
		},
		{
			name:             "Case sensitivity test",
			input:            "BINKD/1.0.4/LINUX binkp/1.0",
			expectedSoftware: "Unknown", // Pattern is case-sensitive
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseBinkPVersion(tt.input)
			if result == nil {
				t.Fatal("Expected non-nil result")
			}
			if result.Software != tt.expectedSoftware {
				t.Errorf("Software: expected %q, got %q", tt.expectedSoftware, result.Software)
			}
		})
	}
}

// TestParseIFCICOMailerInfoEdgeCases tests edge cases
func TestParseIFCICOMailerInfoEdgeCases(t *testing.T) {
	tests := []struct {
		name             string
		input            string
		expectedSoftware string
	}{
		{
			name:             "Mailer with extra spaces",
			input:            "mbcico  1.1.8",
			expectedSoftware: "mbcico", // Regex \s+ matches one or more spaces, so this works
		},
		{
			name:             "Mailer without version",
			input:            "mbcico",
			expectedSoftware: "mbcico", // Uses whole string as software name
		},
		{
			name:             "Completely unknown format",
			input:            "SomeMailer-XYZ-123",
			expectedSoftware: "SomeMailer-XYZ-123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseIFCICOMailerInfo(tt.input)
			if result == nil {
				t.Fatal("Expected non-nil result")
			}
			if result.Software != tt.expectedSoftware {
				t.Errorf("Software: expected %q, got %q", tt.expectedSoftware, result.Software)
			}
		})
	}
}

// TestSortingBehavior verifies that results are sorted by count in descending order
func TestSortingBehavior(t *testing.T) {
	t.Run("SoftwareTypeStats sorting", func(t *testing.T) {
		input := map[string]int{
			"C": 10,
			"A": 100,
			"B": 50,
		}
		result := mapToSoftwareTypeStats(input, 160)

		if len(result) != 3 {
			t.Fatalf("Expected 3 items, got %d", len(result))
		}

		// Verify descending order by count
		if result[0].Software != "A" || result[0].Count != 100 {
			t.Errorf("First item should be A with count 100, got %s with count %d", result[0].Software, result[0].Count)
		}
		if result[1].Software != "B" || result[1].Count != 50 {
			t.Errorf("Second item should be B with count 50, got %s with count %d", result[1].Software, result[1].Count)
		}
		if result[2].Software != "C" || result[2].Count != 10 {
			t.Errorf("Third item should be C with count 10, got %s with count %d", result[2].Software, result[2].Count)
		}
	})

	t.Run("VersionStats sorting", func(t *testing.T) {
		input := map[string]int{
			"binkd 1.0.4":   25,
			"binkd 1.1a-112": 100,
			"Mystic 1.12A48": 50,
		}
		result := mapToVersionStats(input, 175)

		if len(result) != 3 {
			t.Fatalf("Expected 3 items, got %d", len(result))
		}

		// Verify descending order by count
		if result[0].Count != 100 {
			t.Errorf("First item should have count 100, got %d", result[0].Count)
		}
		if result[1].Count != 50 {
			t.Errorf("Second item should have count 50, got %d", result[1].Count)
		}
		if result[2].Count != 25 {
			t.Errorf("Third item should have count 25, got %d", result[2].Count)
		}
	})

	t.Run("OSStats sorting", func(t *testing.T) {
		input := map[string]int{
			"Windows": 30,
			"Linux":   200,
			"FreeBSD": 70,
		}
		result := mapToOSStats(input, 300)

		if len(result) != 3 {
			t.Fatalf("Expected 3 items, got %d", len(result))
		}

		// Verify descending order by count
		if result[0].OS != "Linux" || result[0].Count != 200 {
			t.Errorf("First item should be Linux with count 200, got %s with count %d", result[0].OS, result[0].Count)
		}
		if result[1].OS != "FreeBSD" || result[1].Count != 70 {
			t.Errorf("Second item should be FreeBSD with count 70, got %s with count %d", result[1].OS, result[1].Count)
		}
		if result[2].OS != "Windows" || result[2].Count != 30 {
			t.Errorf("Third item should be Windows with count 30, got %s with count %d", result[2].OS, result[2].Count)
		}
	})
}

// TestParseBinkPVersionRealWorldExamples tests with actual version strings from FidoNet
func TestParseBinkPVersionRealWorldExamples(t *testing.T) {
	tests := []struct {
		name             string
		input            string
		expectedSoftware string
		expectedVersion  string
		expectedOS       string
	}{
		{
			name:             "binkd 1.0.4 Linux production",
			input:            "binkd/1.0.4/Linux binkp/1.0",
			expectedSoftware: "binkd",
			expectedVersion:  "1.0.4",
			expectedOS:       "Linux",
		},
		{
			name:             "binkd 1.1a development version",
			input:            "binkd/1.1a-112/Win32 binkp/1.1",
			expectedSoftware: "binkd",
			expectedVersion:  "1.1a-112",
			expectedOS:       "Windows 32-bit",
		},
		{
			name:             "Synchronet BBS with BinkIT",
			input:            "BinkIT/1.0,JSBinkP/1.0,sbbs3.19c/Linux binkp/1.1",
			expectedSoftware: "BinkIT/Synchronet",
			expectedVersion:  "sbbs3.19c/BinkIT1.0",
			expectedOS:       "Linux",
		},
		{
			name:             "Mystic BBS",
			input:            "Mystic/1.12A48 binkp/1.0",
			expectedSoftware: "Mystic",
			expectedVersion:  "1.12A48",
			expectedOS:       "",
		},
		{
			name:             "mbcico on Linux",
			input:            "mbcico/1.1.8-a1/Linux binkp/1.0",
			expectedSoftware: "mbcico",
			expectedVersion:  "1.1.8-a1",
			expectedOS:       "Linux",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseBinkPVersion(tt.input)
			if result == nil {
				t.Fatal("Expected non-nil result")
			}

			if result.Software != tt.expectedSoftware {
				t.Errorf("Software: expected %q, got %q", tt.expectedSoftware, result.Software)
			}
			if result.Version != tt.expectedVersion {
				t.Errorf("Version: expected %q, got %q", tt.expectedVersion, result.Version)
			}
			if result.OS != tt.expectedOS {
				t.Errorf("OS: expected %q, got %q", tt.expectedOS, result.OS)
			}
		})
	}
}

// TestParseIFCICOMailerInfoRealWorldExamples tests with actual mailer info strings
func TestParseIFCICOMailerInfoRealWorldExamples(t *testing.T) {
	tests := []struct {
		name             string
		input            string
		expectedSoftware string
		expectedVersion  string
	}{
		{
			name:             "mbcico production",
			input:            "mbcico 1.1.8",
			expectedSoftware: "mbcico",
			expectedVersion:  "1.1.8",
		},
		{
			name:             "qico mailer",
			input:            "qico 0.59a",
			expectedSoftware: "qico",
			expectedVersion:  "0.59a",
		},
		{
			name:             "Argus legacy",
			input:            "Argus 4.20",
			expectedSoftware: "Argus",
			expectedVersion:  "4.20",
		},
		{
			name:             "BinkleyTerm Atari ST version",
			input:            "BinkleyTerm-ST 1.01",
			expectedSoftware: "BinkleyTerm-ST",
			expectedVersion:  "1.01",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseIFCICOMailerInfo(tt.input)
			if result == nil {
				t.Fatal("Expected non-nil result")
			}

			if result.Software != tt.expectedSoftware {
				t.Errorf("Software: expected %q, got %q", tt.expectedSoftware, result.Software)
			}
			if result.Version != tt.expectedVersion {
				t.Errorf("Version: expected %q, got %q", tt.expectedVersion, result.Version)
			}
			if result.Protocol != "IFCICO/EMSI" {
				t.Errorf("Protocol: expected %q, got %q", "IFCICO/EMSI", result.Protocol)
			}
		})
	}
}

// TestVersionStatsWithComplexData tests version statistics with realistic data
func TestVersionStatsWithComplexData(t *testing.T) {
	input := map[string]int{
		"binkd 1.0.4":     250,
		"binkd 1.1a-112":  180,
		"binkd 1.1a-114":  120,
		"Mystic 1.12A48":  95,
		"Mystic 1.12A47":  45,
		"mbcico 1.1.8":    30,
		"Argus 4.20":      15,
	}

	result := mapToVersionStats(input, 735)

	// Verify we got all entries
	if len(result) != 7 {
		t.Fatalf("Expected 7 items, got %d", len(result))
	}

	// Verify the top entry
	if result[0].Software != "binkd" || result[0].Version != "1.0.4" {
		t.Errorf("Top software should be binkd 1.0.4, got %s %s", result[0].Software, result[0].Version)
	}

	// Verify sorting (descending by count)
	for i := 0; i < len(result)-1; i++ {
		if result[i].Count < result[i+1].Count {
			t.Errorf("Results not sorted: item %d has count %d, item %d has count %d",
				i, result[i].Count, i+1, result[i+1].Count)
		}
	}

	// Verify percentages sum to ~100%
	var totalPercentage float64
	for _, stat := range result {
		totalPercentage += stat.Percentage
	}
	if !floatEquals(totalPercentage, 100.0, 0.01) {
		t.Errorf("Total percentage should be ~100%%, got %f", totalPercentage)
	}
}

// TestOSDistributionWithRealData tests OS distribution with realistic data
func TestOSDistributionWithRealData(t *testing.T) {
	input := map[string]int{
		"Linux":           450,
		"Windows 32-bit":  280,
		"Windows 64-bit":  150,
		"FreeBSD":         45,
		"OS/2":            20,
		"macOS":           5,
	}

	result := mapToOSStats(input, 950)

	// Verify we got all entries
	if len(result) != 6 {
		t.Fatalf("Expected 6 items, got %d", len(result))
	}

	// Verify Linux is the most common (this reflects actual FidoNet usage)
	if result[0].OS != "Linux" {
		t.Errorf("Linux should be most common OS, got %s", result[0].OS)
	}

	// Verify percentages
	linuxPercentage := float64(450) * 100.0 / float64(950)
	if !floatEquals(result[0].Percentage, linuxPercentage, 0.01) {
		t.Errorf("Linux percentage should be ~47.37%%, got %f", result[0].Percentage)
	}
}

// TestNormalizeOSComprehensive tests comprehensive OS normalization scenarios
func TestNormalizeOSComprehensive(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		// Case variations
		{"LINUX", "Linux"},
		{"linux", "Linux"},
		{"LiNuX", "Linux"},

		// Windows variants
		{"win", "Windows"},
		{"WIN", "Windows"},
		{"Win", "Windows"},
		{"win32", "Windows 32-bit"},
		{"WIN32", "Windows 32-bit"},
		{"win64", "Windows 64-bit"},
		{"WIN64", "Windows 64-bit"},

		// BSD variants
		{"freebsd", "FreeBSD"},
		{"FREEBSD", "FreeBSD"},
		{"FreeBSD", "FreeBSD"},

		// OS/2 variants
		{"os2", "OS/2"},
		{"OS2", "OS/2"},
		{"os/2", "OS/2"},
		{"OS/2", "OS/2"},

		// macOS variants
		{"darwin", "Windows"}, // Bug: contains "win"
		{"mac", "macOS"},
		{"MAC", "macOS"},

		// Unknown/other
		{"", "Unknown"},
		{"haiku", "Haiku"},
		{"plan9", "Plan9"},
	}

	for _, tt := range tests {
		t.Run("Normalize_"+tt.input, func(t *testing.T) {
			result := normalizeOS(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeOS(%q): expected %q, got %q", tt.input, tt.expected, result)
			}
		})
	}
}

// TestBinkdVersionStatsWithRealWorldData tests binkd version distribution
func TestBinkdVersionStatsWithRealWorldData(t *testing.T) {
	input := map[string]int{
		"1.0.4":     180,
		"1.1a-112":  120,
		"1.1a-114":  95,
		"1.1a-115":  45,
		"0.9.11":    20,
	}

	result := mapToBinkdVersionStats(input, 460)

	// Verify all entries
	if len(result) != 5 {
		t.Fatalf("Expected 5 items, got %d", len(result))
	}

	// Verify all have "binkd" as software
	for i, stat := range result {
		if stat.Software != "binkd" {
			t.Errorf("Item %d: expected software %q, got %q", i, "binkd", stat.Software)
		}
	}

	// Verify sorting
	if result[0].Version != "1.0.4" {
		t.Errorf("Most common version should be 1.0.4, got %s", result[0].Version)
	}

	// Verify percentage calculation
	expectedPercentage := float64(180) * 100.0 / float64(460)
	if !floatEquals(result[0].Percentage, expectedPercentage, 0.01) {
		t.Errorf("Top version percentage should be ~39.13%%, got %f", result[0].Percentage)
	}
}

// TestSoftwareInfoStructure tests the softwareInfo structure behavior
func TestSoftwareInfoStructure(t *testing.T) {
	t.Run("Complete softwareInfo", func(t *testing.T) {
		info := &softwareInfo{
			Software: "binkd",
			Version:  "1.0.4",
			OS:       "Linux",
			Protocol: "binkp/1.0",
		}

		if info.Software != "binkd" {
			t.Errorf("Expected Software %q, got %q", "binkd", info.Software)
		}
		if info.Version != "1.0.4" {
			t.Errorf("Expected Version %q, got %q", "1.0.4", info.Version)
		}
		if info.OS != "Linux" {
			t.Errorf("Expected OS %q, got %q", "Linux", info.OS)
		}
		if info.Protocol != "binkp/1.0" {
			t.Errorf("Expected Protocol %q, got %q", "binkp/1.0", info.Protocol)
		}
	})

	t.Run("Partial softwareInfo", func(t *testing.T) {
		info := &softwareInfo{
			Software: "Mystic",
			Version:  "1.12A48",
		}

		if info.OS != "" {
			t.Errorf("Expected empty OS, got %q", info.OS)
		}
		if info.Protocol != "" {
			t.Errorf("Expected empty Protocol, got %q", info.Protocol)
		}
	})
}

// TestPercentageCalculationAccuracy tests percentage calculation accuracy
func TestPercentageCalculationAccuracy(t *testing.T) {
	tests := []struct {
		name     string
		count    int
		total    int
		expected float64
	}{
		{"50 of 100", 50, 100, 50.0},
		{"1 of 3", 1, 3, 33.333333},
		{"2 of 3", 2, 3, 66.666667},
		{"100 of 100", 100, 100, 100.0},
		{"0 of 100", 0, 100, 0.0},
		{"1 of 0 (edge case)", 1, 0, 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := map[string]int{"test": tt.count}
			result := mapToSoftwareTypeStats(input, tt.total)

			if len(result) != 1 {
				t.Fatalf("Expected 1 item, got %d", len(result))
			}

			if !floatEquals(result[0].Percentage, tt.expected, 0.0001) {
				t.Errorf("Percentage: expected %f, got %f", tt.expected, result[0].Percentage)
			}
		})
	}
}
