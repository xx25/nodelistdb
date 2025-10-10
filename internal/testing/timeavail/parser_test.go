package timeavail

import (
	"fmt"
	"testing"
	"time"
)

func TestParseTIMSLetter(t *testing.T) {
	tests := []struct {
		name      string
		letter    rune
		wantErr   bool
		checkDays func([]time.Weekday) bool
		checkTime func(TimeWindow) bool
	}{
		{
			name:    "Letter A - Night (00:00-06:00)",
			letter:  'A',
			wantErr: false,
			checkDays: func(days []time.Weekday) bool {
				return len(days) == 7 // All days
			},
			checkTime: func(w TimeWindow) bool {
				return w.StartUTC.Hour() == 0 && w.EndUTC.Hour() == 6
			},
		},
		{
			name:    "Letter T - Evening/Night (18:00-08:00)",
			letter:  'T',
			wantErr: false,
			checkDays: func(days []time.Weekday) bool {
				return len(days) == 7 // All days
			},
			checkTime: func(w TimeWindow) bool {
				return w.StartUTC.Hour() == 18 && w.EndUTC.Hour() == 8
			},
		},
		{
			name:    "Letter W - Weekdays only",
			letter:  'W',
			wantErr: false,
			checkDays: func(days []time.Weekday) bool {
				return len(days) == 5 && !containsWeekend(days)
			},
			checkTime: func(w TimeWindow) bool {
				return w.StartUTC.Hour() == 0 && w.EndUTC.Hour() == 23
			},
		},
		{
			name:    "Letter H - Weekends only",
			letter:  'H',
			wantErr: false,
			checkDays: func(days []time.Weekday) bool {
				return len(days) == 2 && containsWeekend(days)
			},
		},
		{
			name:    "Invalid letter",
			letter:  '!',
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			window, err := ParseTIMSLetter(tt.letter)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseTIMSLetter() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if window == nil {
					t.Error("ParseTIMSLetter() returned nil window")
					return
				}
				if tt.checkDays != nil && !tt.checkDays(window.Days) {
					t.Errorf("ParseTIMSLetter() days check failed for %c: got %v", tt.letter, window.Days)
				}
				if tt.checkTime != nil && !tt.checkTime(*window) {
					t.Errorf("ParseTIMSLetter() time check failed for %c: start=%v, end=%v",
						tt.letter, window.StartUTC, window.EndUTC)
				}
			}
		})
	}
}

func TestParseAvailability(t *testing.T) {
	tests := []struct {
		name        string
		flags       []string
		zone        int
		phoneNumber string
		expectCM    bool
		expectICM   bool
		expectZMH   bool
		windowCount int
	}{
		{
			name:        "CM flag - always available",
			flags:       []string{"CM"},
			zone:        2,
			phoneNumber: "+1234567890",
			expectCM:    true,
			expectICM:   false,
			expectZMH:   false,
			windowCount: 0,
		},
		{
			name:        "ICM flag - IP only 24/7",
			flags:       []string{"ICM"},
			zone:        2,
			phoneNumber: "",
			expectCM:    false,
			expectICM:   true,
			expectZMH:   false,
			windowCount: 0,
		},
		{
			name:        "ZMH flag - zone mail hour",
			flags:       []string{"ZMH"},
			zone:        2,
			phoneNumber: "",
			expectCM:    false,
			expectICM:   false,
			expectZMH:   true,
			windowCount: 1,
		},
		{
			name:        "T-flag with single letter",
			flags:       []string{"TA"},
			zone:        2,
			phoneNumber: "",
			expectCM:    false,
			expectICM:   false,
			expectZMH:   false,
			windowCount: 1,
		},
		{
			name:        "Multiple T-flags",
			flags:       []string{"TA", "TB"},
			zone:        2,
			phoneNumber: "",
			windowCount: 1, // These get merged since they're consecutive
		},
		{
			name:        "Number flag #09",
			flags:       []string{"#09"},
			zone:        2,
			phoneNumber: "",
			windowCount: 1,
		},
		{
			name:        "Combined flags",
			flags:       []string{"ZMH", "TA", "#09"},
			zone:        2,
			phoneNumber: "",
			windowCount: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			availability, err := ParseAvailability(tt.flags, tt.zone, tt.phoneNumber)
			if err != nil {
				t.Errorf("ParseAvailability() error = %v", err)
				return
			}

			if availability.IsCM != tt.expectCM {
				t.Errorf("IsCM = %v, want %v", availability.IsCM, tt.expectCM)
			}
			if availability.IsICM != tt.expectICM {
				t.Errorf("IsICM = %v, want %v", availability.IsICM, tt.expectICM)
			}
			if len(availability.Windows) != tt.windowCount {
				t.Errorf("Windows count = %v, want %v", len(availability.Windows), tt.windowCount)
			}
		})
	}
}

func TestIsCallableNow(t *testing.T) {
	tests := []struct {
		name       string
		avail      *NodeAvailability
		testTime   time.Time
		expectCall bool
	}{
		{
			name: "CM node - always callable",
			avail: &NodeAvailability{
				IsCM: true,
			},
			testTime:   time.Now(),
			expectCall: true,
		},
		{
			name: "No restrictions",
			avail: &NodeAvailability{
				Windows: []TimeWindow{},
			},
			testTime:   time.Now(),
			expectCall: true,
		},
		{
			name: "Within window",
			avail: &NodeAvailability{
				Windows: []TimeWindow{
					{
						StartUTC: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
						EndUTC:   time.Date(2024, 1, 1, 18, 0, 0, 0, time.UTC),
						Days:     allDays(),
					},
				},
			},
			testTime:   time.Date(2024, 1, 1, 14, 0, 0, 0, time.UTC),
			expectCall: true,
		},
		{
			name: "Outside window",
			avail: &NodeAvailability{
				Windows: []TimeWindow{
					{
						StartUTC: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
						EndUTC:   time.Date(2024, 1, 1, 18, 0, 0, 0, time.UTC),
						Days:     allDays(),
					},
				},
			},
			testTime:   time.Date(2024, 1, 1, 20, 0, 0, 0, time.UTC),
			expectCall: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.avail.IsCallableNow(tt.testTime); got != tt.expectCall {
				t.Errorf("IsCallableNow() = %v, want %v", got, tt.expectCall)
			}
		})
	}
}

func TestWindowMerger(t *testing.T) {
	merger := NewWindowMerger()

	// Add overlapping windows
	merger.AddWindow(TimeWindow{
		StartUTC: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
		EndUTC:   time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		Source:   SourceTFlag,
		Days:     allDays(),
	})
	merger.AddWindow(TimeWindow{
		StartUTC: time.Date(2024, 1, 1, 11, 30, 0, 0, time.UTC),
		EndUTC:   time.Date(2024, 1, 1, 14, 0, 0, 0, time.UTC),
		Source:   SourceTFlag,
		Days:     allDays(),
	})

	merged := merger.Merge()

	if len(merged) != 1 {
		t.Errorf("Expected 1 merged window, got %d", len(merged))
	}

	if merged[0].StartUTC.Hour() != 10 || merged[0].EndUTC.Hour() != 14 {
		t.Errorf("Merged window has wrong times: %v to %v", merged[0].StartUTC, merged[0].EndUTC)
	}
}

func TestZMHDefaults(t *testing.T) {
	tests := []struct {
		zone      int
		wantStart int
		wantEnd   int
	}{
		{zone: 1, wantStart: 9, wantEnd: 10},
		{zone: 2, wantStart: 2, wantEnd: 3},
		{zone: 3, wantStart: 18, wantEnd: 19},
		{zone: 4, wantStart: 8, wantEnd: 9},
		{zone: 5, wantStart: 2, wantEnd: 3},
		{zone: 6, wantStart: 22, wantEnd: 23},
		{zone: 999, wantStart: 2, wantEnd: 3}, // Default for unknown zone
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("Zone %d", tt.zone), func(t *testing.T) {
			window := GetZMHWindow(tt.zone)
			if window == nil {
				t.Fatal("GetZMHWindow returned nil")
			}
			if window.StartUTC.Hour() != tt.wantStart {
				t.Errorf("Start hour = %d, want %d", window.StartUTC.Hour(), tt.wantStart)
			}
			if window.EndUTC.Hour() != tt.wantEnd {
				t.Errorf("End hour = %d, want %d", window.EndUTC.Hour(), tt.wantEnd)
			}
			if window.Source != SourceZMH {
				t.Errorf("Source = %s, want %s", window.Source, SourceZMH)
			}
		})
	}
}

func TestParseNumberFlag(t *testing.T) {
	parser := NewParser(2)

	tests := []struct {
		flag      string
		wantHour  int
		wantNil   bool
	}{
		{"#09", 9, false},
		{"#00", 0, false},
		{"#23", 23, false},
		{"#24", 0, true},  // Invalid hour
		{"#99", 0, true},  // Invalid hour
		{"#9", 0, true},   // Wrong format
		{"09", 0, true},   // Missing #
		{"#AB", 0, true},  // Not a number
	}

	for _, tt := range tests {
		t.Run(tt.flag, func(t *testing.T) {
			window := parser.parseNumberFlag(tt.flag)
			if tt.wantNil {
				if window != nil {
					t.Errorf("Expected nil window for %s, got %v", tt.flag, window)
				}
			} else {
				if window == nil {
					t.Errorf("Expected window for %s, got nil", tt.flag)
				} else if window.StartUTC.Hour() != tt.wantHour {
					t.Errorf("Start hour = %d, want %d", window.StartUTC.Hour(), tt.wantHour)
				}
			}
		})
	}
}

// Helper functions
func containsWeekend(days []time.Weekday) bool {
	for _, day := range days {
		if day == time.Saturday || day == time.Sunday {
			return true
		}
	}
	return false
}