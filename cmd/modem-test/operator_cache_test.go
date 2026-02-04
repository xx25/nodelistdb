// Package main provides tests for operator caching and failover logic.
package main

import (
	"testing"
	"time"
)

// Test isUserBusy function
func TestIsUserBusy(t *testing.T) {
	tests := []struct {
		name     string
		cdr      *AsteriskCDRData
		expected bool
	}{
		{
			name:     "nil CDR returns false",
			cdr:      nil,
			expected: false,
		},
		{
			name: "HangupCause 17 returns true",
			cdr: &AsteriskCDRData{
				HangupCause: 17,
			},
			expected: true,
		},
		{
			name: "HangupCause 16 (normal clearing) returns false",
			cdr: &AsteriskCDRData{
				HangupCause: 16,
			},
			expected: false,
		},
		{
			name: "BUSY disposition without cause 17 returns false",
			cdr: &AsteriskCDRData{
				Disposition: "BUSY",
				BillSec:     0,
				HangupCause: 21, // Call rejected
			},
			expected: false,
		},
		{
			name: "HangupCause 17 with ANSWERED disposition returns true",
			cdr: &AsteriskCDRData{
				HangupCause: 17,
				Disposition: "ANSWERED",
			},
			expected: true,
		},
		{
			name: "HangupCause 19 (no answer) returns false",
			cdr: &AsteriskCDRData{
				HangupCause: 19,
				Disposition: "NO ANSWER",
			},
			expected: false,
		},
		{
			name: "HangupCause 0 returns false",
			cdr: &AsteriskCDRData{
				HangupCause: 0,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isUserBusy(tt.cdr)
			if result != tt.expected {
				t.Errorf("isUserBusy() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// Test FindOperatorByName function
func TestFindOperatorByName(t *testing.T) {
	operators := []OperatorConfig{
		{Name: "Primary", Prefix: "01"},
		{Name: "Secondary", Prefix: "02"},
		{Name: "Tertiary", Prefix: "03"},
	}

	tests := []struct {
		name          string
		operators     []OperatorConfig
		searchName    string
		wantOp        OperatorConfig
		wantIndex     int
		wantFound     bool
	}{
		{
			name:       "finds operator by exact name",
			operators:  operators,
			searchName: "Secondary",
			wantOp:     OperatorConfig{Name: "Secondary", Prefix: "02"},
			wantIndex:  1,
			wantFound:  true,
		},
		{
			name:       "finds first operator",
			operators:  operators,
			searchName: "Primary",
			wantOp:     OperatorConfig{Name: "Primary", Prefix: "01"},
			wantIndex:  0,
			wantFound:  true,
		},
		{
			name:       "finds last operator",
			operators:  operators,
			searchName: "Tertiary",
			wantOp:     OperatorConfig{Name: "Tertiary", Prefix: "03"},
			wantIndex:  2,
			wantFound:  true,
		},
		{
			name:       "returns not found for non-existent name",
			operators:  operators,
			searchName: "NonExistent",
			wantOp:     OperatorConfig{},
			wantIndex:  -1,
			wantFound:  false,
		},
		{
			name:       "returns not found for empty list",
			operators:  []OperatorConfig{},
			searchName: "Primary",
			wantOp:     OperatorConfig{},
			wantIndex:  -1,
			wantFound:  false,
		},
		{
			name:       "returns not found for nil list",
			operators:  nil,
			searchName: "Primary",
			wantOp:     OperatorConfig{},
			wantIndex:  -1,
			wantFound:  false,
		},
		{
			name:       "case sensitive search",
			operators:  operators,
			searchName: "primary",
			wantOp:     OperatorConfig{},
			wantIndex:  -1,
			wantFound:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOp, gotIndex, gotFound := FindOperatorByName(tt.operators, tt.searchName)
			if gotFound != tt.wantFound {
				t.Errorf("FindOperatorByName() found = %v, want %v", gotFound, tt.wantFound)
			}
			if gotIndex != tt.wantIndex {
				t.Errorf("FindOperatorByName() index = %v, want %v", gotIndex, tt.wantIndex)
			}
			if gotOp != tt.wantOp {
				t.Errorf("FindOperatorByName() op = %v, want %v", gotOp, tt.wantOp)
			}
		})
	}
}

// Test ReorderOperatorsWithCached function
func TestReorderOperatorsWithCached(t *testing.T) {
	operators := []OperatorConfig{
		{Name: "Primary", Prefix: "01"},
		{Name: "Secondary", Prefix: "02"},
		{Name: "Tertiary", Prefix: "03"},
	}

	tests := []struct {
		name      string
		operators []OperatorConfig
		cached    *CachedOperator
		want      []OperatorConfig
	}{
		{
			name:      "nil cached returns original order",
			operators: operators,
			cached:    nil,
			want:      operators,
		},
		{
			name:      "empty operators returns empty",
			operators: []OperatorConfig{},
			cached:    &CachedOperator{OperatorName: "Primary"},
			want:      []OperatorConfig{},
		},
		{
			name:      "cached operator already first returns original order",
			operators: operators,
			cached:    &CachedOperator{OperatorName: "Primary"},
			want:      operators,
		},
		{
			name:      "cached operator moved to first - from second position",
			operators: operators,
			cached:    &CachedOperator{OperatorName: "Secondary"},
			want: []OperatorConfig{
				{Name: "Secondary", Prefix: "02"},
				{Name: "Primary", Prefix: "01"},
				{Name: "Tertiary", Prefix: "03"},
			},
		},
		{
			name:      "cached operator moved to first - from last position",
			operators: operators,
			cached:    &CachedOperator{OperatorName: "Tertiary"},
			want: []OperatorConfig{
				{Name: "Tertiary", Prefix: "03"},
				{Name: "Primary", Prefix: "01"},
				{Name: "Secondary", Prefix: "02"},
			},
		},
		{
			name:      "non-existent cached operator returns original order",
			operators: operators,
			cached:    &CachedOperator{OperatorName: "NonExistent"},
			want:      operators,
		},
		{
			name:      "single operator with matching cache returns as-is",
			operators: []OperatorConfig{{Name: "Only", Prefix: "00"}},
			cached:    &CachedOperator{OperatorName: "Only"},
			want:      []OperatorConfig{{Name: "Only", Prefix: "00"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ReorderOperatorsWithCached(tt.operators, tt.cached)

			if len(got) != len(tt.want) {
				t.Errorf("ReorderOperatorsWithCached() len = %d, want %d", len(got), len(tt.want))
				return
			}

			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("ReorderOperatorsWithCached()[%d] = %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// Test ReorderOperatorsWithCached preserves original slice
func TestReorderOperatorsWithCached_PreservesOriginal(t *testing.T) {
	original := []OperatorConfig{
		{Name: "Primary", Prefix: "01"},
		{Name: "Secondary", Prefix: "02"},
		{Name: "Tertiary", Prefix: "03"},
	}

	// Make a copy to compare later
	originalCopy := make([]OperatorConfig, len(original))
	copy(originalCopy, original)

	cached := &CachedOperator{OperatorName: "Tertiary"}
	_ = ReorderOperatorsWithCached(original, cached)

	// Original should be unchanged
	for i := range original {
		if original[i] != originalCopy[i] {
			t.Errorf("Original slice was modified at index %d: got %v, want %v",
				i, original[i], originalCopy[i])
		}
	}
}

// Test OperatorCache with real BadgerDB (integration-style test)
func TestOperatorCache_GetSet(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping BadgerDB test in short mode")
	}

	// Create cache in temp directory
	tmpDir := t.TempDir()
	cfg := OperatorCacheConfig{
		Enabled: true,
		Path:    tmpDir,
		TTL:     Duration(time.Hour),
	}

	cache, err := NewOperatorCache(cfg, nil)
	if err != nil {
		t.Fatalf("NewOperatorCache() error = %v", err)
	}
	defer cache.Close()

	testPhone := "79001234567"
	testOp := OperatorConfig{Name: "TestOp", Prefix: "1#"}

	// Get non-existent key
	got, found := cache.Get(testPhone)
	if found {
		t.Error("Get() found = true for non-existent key")
	}
	if got != nil {
		t.Error("Get() returned non-nil for non-existent key")
	}

	// Set and retrieve
	if err := cache.Set(testPhone, testOp); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	got, found = cache.Get(testPhone)
	if !found {
		t.Error("Get() found = false after Set()")
	}
	if got == nil {
		t.Fatal("Get() returned nil after Set()")
	}
	if got.OperatorName != testOp.Name {
		t.Errorf("Get().OperatorName = %q, want %q", got.OperatorName, testOp.Name)
	}
	if got.OperatorPrefix != testOp.Prefix {
		t.Errorf("Get().OperatorPrefix = %q, want %q", got.OperatorPrefix, testOp.Prefix)
	}

	// Set overwrites existing entry
	newOp := OperatorConfig{Name: "NewOp", Prefix: "2#"}
	if err := cache.Set(testPhone, newOp); err != nil {
		t.Fatalf("Set() overwrite error = %v", err)
	}

	got, found = cache.Get(testPhone)
	if !found {
		t.Error("Get() found = false after overwrite")
	}
	if got.OperatorName != newOp.Name {
		t.Errorf("Get().OperatorName after overwrite = %q, want %q", got.OperatorName, newOp.Name)
	}

	// Delete removes entry
	if err := cache.Delete(testPhone); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	got, found = cache.Get(testPhone)
	if found {
		t.Error("Get() found = true after Delete()")
	}
	if got != nil {
		t.Error("Get() returned non-nil after Delete()")
	}
}

// Test OperatorCache disabled mode
func TestOperatorCache_Disabled(t *testing.T) {
	cfg := OperatorCacheConfig{
		Enabled: false,
	}

	cache, err := NewOperatorCache(cfg, nil)
	if err != nil {
		t.Fatalf("NewOperatorCache() error = %v", err)
	}

	if cache != nil {
		t.Error("NewOperatorCache() returned non-nil for disabled config")
	}

	// Operations on nil cache should be safe
	var nilCache *OperatorCache

	got, found := nilCache.Get("anyphone")
	if found {
		t.Error("nil cache Get() found = true")
	}
	if got != nil {
		t.Error("nil cache Get() returned non-nil")
	}

	// Set on nil should not panic and return nil error
	if err := nilCache.Set("anyphone", OperatorConfig{Name: "Test"}); err != nil {
		t.Errorf("nil cache Set() error = %v, want nil", err)
	}

	// Delete on nil should not panic and return nil error
	if err := nilCache.Delete("anyphone"); err != nil {
		t.Errorf("nil cache Delete() error = %v, want nil", err)
	}

	// Close on nil should not panic and return nil error
	if err := nilCache.Close(); err != nil {
		t.Errorf("nil cache Close() error = %v, want nil", err)
	}
}

// Test OperatorCache path expansion
func TestOperatorCache_PathExpansion(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping path expansion test in short mode")
	}

	// Test with explicit path (no expansion needed)
	tmpDir := t.TempDir()
	cfg := OperatorCacheConfig{
		Enabled: true,
		Path:    tmpDir,
		TTL:     Duration(time.Hour),
	}

	cache, err := NewOperatorCache(cfg, nil)
	if err != nil {
		t.Fatalf("NewOperatorCache() with explicit path error = %v", err)
	}
	cache.Close()
}

// Test FailoverResult structure
func TestFailoverResult_Fields(t *testing.T) {
	// Success case
	successOp := &OperatorConfig{Name: "Primary", Prefix: "01"}
	successResult := FailoverResult{
		Success:         true,
		SuccessOperator: successOp,
		LastOperator:    successOp,
		TriedOperators:  1,
	}
	if !successResult.Success {
		t.Error("Success case should have Success=true")
	}
	if successResult.SuccessOperator == nil {
		t.Error("Success case should have SuccessOperator set")
	}
	if successResult.UserBusy {
		t.Error("Success case should not have UserBusy=true")
	}

	// User busy case
	lastOp := &OperatorConfig{Name: "VoIP", Prefix: "2#"}
	userBusyResult := FailoverResult{
		Success:        false,
		LastOperator:   lastOp,
		TriedOperators: 1,
		UserBusy:       true,
	}
	if userBusyResult.Success {
		t.Error("UserBusy case should have Success=false")
	}
	if !userBusyResult.UserBusy {
		t.Error("UserBusy case should have UserBusy=true")
	}
	if userBusyResult.AllOperatorsFail {
		t.Error("UserBusy case should not have AllOperatorsFail=true")
	}

	// All operators fail case
	allFailResult := FailoverResult{
		Success:          false,
		LastOperator:     lastOp,
		TriedOperators:   3,
		AllOperatorsFail: true,
	}
	if allFailResult.Success {
		t.Error("AllFail case should have Success=false")
	}
	if !allFailResult.AllOperatorsFail {
		t.Error("AllFail case should have AllOperatorsFail=true")
	}
	if allFailResult.UserBusy {
		t.Error("AllFail case should not have UserBusy=true")
	}
}
