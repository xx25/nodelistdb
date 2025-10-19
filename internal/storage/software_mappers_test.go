package storage

import (
	"testing"
)
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
