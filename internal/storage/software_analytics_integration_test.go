package storage

import (
	"testing"
)
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
