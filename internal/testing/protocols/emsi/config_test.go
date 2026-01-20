package emsi

import (
	"sync"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	// Test FSC-0056.001 standard timeouts
	if cfg.MasterTimeout != 60*time.Second {
		t.Errorf("MasterTimeout: expected 60s, got %v", cfg.MasterTimeout)
	}
	if cfg.StepTimeout != 20*time.Second {
		t.Errorf("StepTimeout: expected 20s, got %v", cfg.StepTimeout)
	}
	if cfg.FirstStepTimeout != 10*time.Second {
		t.Errorf("FirstStepTimeout: expected 10s, got %v", cfg.FirstStepTimeout)
	}
	if cfg.CharacterTimeout != 5*time.Second {
		t.Errorf("CharacterTimeout: expected 5s, got %v", cfg.CharacterTimeout)
	}

	// Test FSC-0056.001 standard retries
	if cfg.MaxRetries != 6 {
		t.Errorf("MaxRetries: expected 6, got %d", cfg.MaxRetries)
	}
	if cfg.RetryDelay != 1*time.Second {
		t.Errorf("RetryDelay: expected 1s, got %v", cfg.RetryDelay)
	}

	// Test initial handshake defaults
	if cfg.InitialStrategy != "wait" {
		t.Errorf("InitialStrategy: expected 'wait', got %q", cfg.InitialStrategy)
	}
	if cfg.InitialCRInterval != 500*time.Millisecond {
		t.Errorf("InitialCRInterval: expected 500ms, got %v", cfg.InitialCRInterval)
	}
	if cfg.InitialCRTimeout != 5*time.Second {
		t.Errorf("InitialCRTimeout: expected 5s, got %v", cfg.InitialCRTimeout)
	}

	// Test EMSI_INQ behavior
	if cfg.PreventiveINQ != false {
		t.Errorf("PreventiveINQ: expected false, got %v", cfg.PreventiveINQ)
	}
	if cfg.SendINQTwice != true {
		t.Errorf("SendINQTwice: expected true, got %v", cfg.SendINQTwice)
	}
	if cfg.INQInterval != 200*time.Millisecond {
		t.Errorf("INQInterval: expected 200ms, got %v", cfg.INQInterval)
	}

	// Test packet behavior
	if cfg.SendREQTwice != true {
		t.Errorf("SendREQTwice: expected true, got %v", cfg.SendREQTwice)
	}
	if cfg.SendACKTwice != true {
		t.Errorf("SendACKTwice: expected true, got %v", cfg.SendACKTwice)
	}
	if cfg.SendNAKOnRetry != true {
		t.Errorf("SendNAKOnRetry: expected true, got %v", cfg.SendNAKOnRetry)
	}

	// Test compatibility
	if cfg.AcceptFDLenWithCR != true {
		t.Errorf("AcceptFDLenWithCR: expected true, got %v", cfg.AcceptFDLenWithCR)
	}

	// Test protocols
	if len(cfg.Protocols) != 2 || cfg.Protocols[0] != "ZMO" || cfg.Protocols[1] != "ZAP" {
		t.Errorf("Protocols: expected [ZMO, ZAP], got %v", cfg.Protocols)
	}
}

func TestConfigCopy(t *testing.T) {
	orig := DefaultConfig()
	orig.Protocols = []string{"ZMO", "ZAP", "HYD"}

	cp := orig.Copy()

	// Verify copy has same values
	if cp.MasterTimeout != orig.MasterTimeout {
		t.Errorf("Copy MasterTimeout mismatch")
	}
	if len(cp.Protocols) != len(orig.Protocols) {
		t.Errorf("Copy Protocols length mismatch")
	}

	// Modify copy and verify original unchanged
	cp.MasterTimeout = 30 * time.Second
	cp.Protocols[0] = "DZA"

	if orig.MasterTimeout != 60*time.Second {
		t.Errorf("Original MasterTimeout was modified")
	}
	if orig.Protocols[0] != "ZMO" {
		t.Errorf("Original Protocols was modified")
	}
}

func TestConfigApplyOverride(t *testing.T) {
	cfg := DefaultConfig()

	// Create override with only some fields set
	stepTimeout := 30 * time.Second
	preventiveINQ := true
	protocols := []string{"ZMO"}

	override := &NodeOverride{
		StepTimeout:   &stepTimeout,
		PreventiveINQ: &preventiveINQ,
		Protocols:     &protocols,
	}

	cfg.ApplyOverride(override)

	// Verify overridden fields
	if cfg.StepTimeout != 30*time.Second {
		t.Errorf("StepTimeout: expected 30s, got %v", cfg.StepTimeout)
	}
	if cfg.PreventiveINQ != true {
		t.Errorf("PreventiveINQ: expected true, got %v", cfg.PreventiveINQ)
	}
	if len(cfg.Protocols) != 1 || cfg.Protocols[0] != "ZMO" {
		t.Errorf("Protocols: expected [ZMO], got %v", cfg.Protocols)
	}

	// Verify non-overridden fields unchanged
	if cfg.MasterTimeout != 60*time.Second {
		t.Errorf("MasterTimeout should be unchanged: expected 60s, got %v", cfg.MasterTimeout)
	}
	if cfg.MaxRetries != 6 {
		t.Errorf("MaxRetries should be unchanged: expected 6, got %d", cfg.MaxRetries)
	}
}

func TestConfigApplyOverrideEmptyProtocols(t *testing.T) {
	cfg := DefaultConfig()

	// Override with empty protocols list (NCP)
	emptyProtocols := []string{}
	override := &NodeOverride{
		Protocols: &emptyProtocols,
	}

	cfg.ApplyOverride(override)

	// Verify protocols is now empty (for NCP)
	if len(cfg.Protocols) != 0 {
		t.Errorf("Protocols: expected empty slice for NCP, got %v", cfg.Protocols)
	}
}

func TestConfigApplyOverrideNil(t *testing.T) {
	cfg := DefaultConfig()
	origTimeout := cfg.MasterTimeout

	// Apply nil override should be no-op
	cfg.ApplyOverride(nil)

	if cfg.MasterTimeout != origTimeout {
		t.Errorf("Nil override should not modify config")
	}
}

func TestConfigMergeFrom(t *testing.T) {
	cfg := DefaultConfig()

	// Merge partial config
	partial := &Config{
		MasterTimeout:   90 * time.Second,
		InitialStrategy: "send_cr",
		Protocols:       []string{"HYD", "ZMO"},
	}

	cfg.MergeFrom(partial)

	// Verify merged fields
	if cfg.MasterTimeout != 90*time.Second {
		t.Errorf("MasterTimeout: expected 90s, got %v", cfg.MasterTimeout)
	}
	if cfg.InitialStrategy != "send_cr" {
		t.Errorf("InitialStrategy: expected 'send_cr', got %q", cfg.InitialStrategy)
	}
	if len(cfg.Protocols) != 2 || cfg.Protocols[0] != "HYD" {
		t.Errorf("Protocols: expected [HYD, ZMO], got %v", cfg.Protocols)
	}

	// Verify zero values don't override defaults
	if cfg.StepTimeout != 20*time.Second {
		t.Errorf("StepTimeout should be preserved: expected 20s, got %v", cfg.StepTimeout)
	}
}

func TestConfigMergeFromNil(t *testing.T) {
	cfg := DefaultConfig()
	origTimeout := cfg.MasterTimeout

	// Merge nil should be no-op
	cfg.MergeFrom(nil)

	if cfg.MasterTimeout != origTimeout {
		t.Errorf("Nil merge should not modify config")
	}
}

func TestNormalizeConfigAddress(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"2:5020/2021", "2:5020/2021"},
		{"2:5020/2021.0", "2:5020/2021"},
		{"  2:5020/2021  ", "2:5020/2021"},
		{"2:5020/2021.0  ", "2:5020/2021"},
		{"1:123/456.1", "1:123/456.1"}, // Keep non-.0 point
	}

	for _, tc := range tests {
		result := normalizeConfigAddress(tc.input)
		if result != tc.expected {
			t.Errorf("normalizeConfigAddress(%q): expected %q, got %q", tc.input, tc.expected, result)
		}
	}
}

func TestConfigManager(t *testing.T) {
	mgr := NewConfigManager()

	// Test getting default config
	cfg := mgr.GetConfigForNode("2:5020/100")
	if cfg.MasterTimeout != 60*time.Second {
		t.Errorf("Default config MasterTimeout: expected 60s, got %v", cfg.MasterTimeout)
	}

	// Set override for specific node
	stepTimeout := 30 * time.Second
	mgr.SetOverride("2:5020/2021", &NodeOverride{
		StepTimeout: &stepTimeout,
	})

	// Get config for overridden node
	cfg = mgr.GetConfigForNode("2:5020/2021")
	if cfg.StepTimeout != 30*time.Second {
		t.Errorf("Overridden StepTimeout: expected 30s, got %v", cfg.StepTimeout)
	}

	// Get config for non-overridden node (should be defaults)
	cfg = mgr.GetConfigForNode("2:5020/100")
	if cfg.StepTimeout != 20*time.Second {
		t.Errorf("Non-overridden StepTimeout: expected 20s, got %v", cfg.StepTimeout)
	}
}

func TestConfigManagerWithConfig(t *testing.T) {
	customGlobal := &Config{
		MasterTimeout:   90 * time.Second,
		InitialStrategy: "send_inq",
	}

	mgr := NewConfigManagerWithConfig(customGlobal)

	cfg := mgr.GetConfigForNode("any:node/addr")

	// Custom global values should be applied
	if cfg.MasterTimeout != 90*time.Second {
		t.Errorf("Custom MasterTimeout: expected 90s, got %v", cfg.MasterTimeout)
	}
	if cfg.InitialStrategy != "send_inq" {
		t.Errorf("Custom InitialStrategy: expected 'send_inq', got %q", cfg.InitialStrategy)
	}

	// Default values for unspecified fields should be preserved
	if cfg.StepTimeout != 20*time.Second {
		t.Errorf("Default StepTimeout should be preserved: expected 20s, got %v", cfg.StepTimeout)
	}
}

func TestConfigManagerWithNilConfig(t *testing.T) {
	mgr := NewConfigManagerWithConfig(nil)

	cfg := mgr.GetConfigForNode("any:node/addr")

	// Should have all defaults
	if cfg.MasterTimeout != 60*time.Second {
		t.Errorf("MasterTimeout: expected default 60s, got %v", cfg.MasterTimeout)
	}
}

func TestConfigManagerLoadOverrides(t *testing.T) {
	mgr := NewConfigManager()

	stepTimeout30 := 30 * time.Second
	stepTimeout40 := 40 * time.Second

	overrides := map[string]*NodeOverride{
		"2:5020/2021": {StepTimeout: &stepTimeout30},
		"1:123/456":   {StepTimeout: &stepTimeout40},
	}

	mgr.LoadOverrides(overrides)

	// Verify both overrides
	cfg := mgr.GetConfigForNode("2:5020/2021")
	if cfg.StepTimeout != 30*time.Second {
		t.Errorf("Node 2:5020/2021 StepTimeout: expected 30s, got %v", cfg.StepTimeout)
	}

	cfg = mgr.GetConfigForNode("1:123/456")
	if cfg.StepTimeout != 40*time.Second {
		t.Errorf("Node 1:123/456 StepTimeout: expected 40s, got %v", cfg.StepTimeout)
	}
}

func TestConfigManagerAddressNormalization(t *testing.T) {
	mgr := NewConfigManager()

	stepTimeout := 30 * time.Second
	mgr.SetOverride("2:5020/2021.0", &NodeOverride{
		StepTimeout: &stepTimeout,
	})

	// Should find override with normalized address
	cfg := mgr.GetConfigForNode("2:5020/2021")
	if cfg.StepTimeout != 30*time.Second {
		t.Errorf("Override should be found with normalized address")
	}

	// Should also find with .0 suffix
	cfg = mgr.GetConfigForNode("2:5020/2021.0")
	if cfg.StepTimeout != 30*time.Second {
		t.Errorf("Override should be found with .0 suffix")
	}
}

func TestConfigManagerConcurrentAccess(t *testing.T) {
	mgr := NewConfigManager()

	stepTimeout := 30 * time.Second
	mgr.SetOverride("2:5020/2021", &NodeOverride{
		StepTimeout: &stepTimeout,
	})

	var wg sync.WaitGroup
	errors := make(chan error, 100)

	// Concurrent readers
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cfg := mgr.GetConfigForNode("2:5020/2021")
			if cfg.StepTimeout != 30*time.Second {
				errors <- nil // Just count errors, don't fail
			}
		}()
	}

	// Concurrent writers
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			timeout := time.Duration(n) * time.Second
			mgr.SetOverride("2:5020/9999", &NodeOverride{
				StepTimeout: &timeout,
			})
		}(i)
	}

	wg.Wait()
	close(errors)

	// If we get here without panic, concurrent access is safe
}

func TestConfigReturnsCopy(t *testing.T) {
	mgr := NewConfigManager()

	cfg1 := mgr.GetConfigForNode("2:5020/100")
	cfg2 := mgr.GetConfigForNode("2:5020/100")

	// Modify cfg1
	cfg1.MasterTimeout = 999 * time.Second

	// cfg2 should be unaffected
	if cfg2.MasterTimeout != 60*time.Second {
		t.Errorf("GetConfigForNode should return copies, not references")
	}
}

// Helper for creating pointer to duration
func ptrDuration(d time.Duration) *time.Duration {
	return &d
}

// Helper for creating pointer to int
func ptrInt(i int) *int {
	return &i
}

// Helper for creating pointer to bool
func ptrBool(b bool) *bool {
	return &b
}

// Helper for creating pointer to string
func ptrString(s string) *string {
	return &s
}

func TestNodeOverrideAllFields(t *testing.T) {
	cfg := DefaultConfig()

	// Create override with all fields set
	protocols := []string{"HYD"}
	override := &NodeOverride{
		MasterTimeout:        ptrDuration(90 * time.Second),
		StepTimeout:          ptrDuration(30 * time.Second),
		FirstStepTimeout:     ptrDuration(15 * time.Second),
		CharacterTimeout:     ptrDuration(10 * time.Second),
		MaxRetries:           ptrInt(10),
		RetryDelay:           ptrDuration(2 * time.Second),
		InitialStrategy:      ptrString("send_cr"),
		InitialCRInterval:    ptrDuration(1 * time.Second),
		InitialCRTimeout:     ptrDuration(10 * time.Second),
		PreventiveINQ:        ptrBool(true),
		PreventiveINQTimeout: ptrDuration(10 * time.Second),
		SendINQTwice:         ptrBool(false),
		INQInterval:          ptrDuration(500 * time.Millisecond),
		SendREQTwice:         ptrBool(false),
		SendACKTwice:         ptrBool(false),
		SendNAKOnRetry:       ptrBool(false),
		AcceptFDLenWithCR:    ptrBool(false),
		Protocols:            &protocols,
	}

	cfg.ApplyOverride(override)

	// Verify all fields
	if cfg.MasterTimeout != 90*time.Second {
		t.Errorf("MasterTimeout mismatch")
	}
	if cfg.StepTimeout != 30*time.Second {
		t.Errorf("StepTimeout mismatch")
	}
	if cfg.FirstStepTimeout != 15*time.Second {
		t.Errorf("FirstStepTimeout mismatch")
	}
	if cfg.CharacterTimeout != 10*time.Second {
		t.Errorf("CharacterTimeout mismatch")
	}
	if cfg.MaxRetries != 10 {
		t.Errorf("MaxRetries mismatch")
	}
	if cfg.RetryDelay != 2*time.Second {
		t.Errorf("RetryDelay mismatch")
	}
	if cfg.InitialStrategy != "send_cr" {
		t.Errorf("InitialStrategy mismatch")
	}
	if cfg.InitialCRInterval != 1*time.Second {
		t.Errorf("InitialCRInterval mismatch")
	}
	if cfg.InitialCRTimeout != 10*time.Second {
		t.Errorf("InitialCRTimeout mismatch")
	}
	if cfg.PreventiveINQ != true {
		t.Errorf("PreventiveINQ mismatch")
	}
	if cfg.PreventiveINQTimeout != 10*time.Second {
		t.Errorf("PreventiveINQTimeout mismatch")
	}
	if cfg.SendINQTwice != false {
		t.Errorf("SendINQTwice mismatch")
	}
	if cfg.INQInterval != 500*time.Millisecond {
		t.Errorf("INQInterval mismatch")
	}
	if cfg.SendREQTwice != false {
		t.Errorf("SendREQTwice mismatch")
	}
	if cfg.SendACKTwice != false {
		t.Errorf("SendACKTwice mismatch")
	}
	if cfg.SendNAKOnRetry != false {
		t.Errorf("SendNAKOnRetry mismatch")
	}
	if cfg.AcceptFDLenWithCR != false {
		t.Errorf("AcceptFDLenWithCR mismatch")
	}
	if len(cfg.Protocols) != 1 || cfg.Protocols[0] != "HYD" {
		t.Errorf("Protocols mismatch")
	}
}

// Test that MergeFrom preserves bool defaults (doesn't overwrite with zero values)
func TestMergeFromPreservesBoolDefaults(t *testing.T) {
	cfg := DefaultConfig()

	// Verify default bool values
	if !cfg.SendINQTwice {
		t.Fatal("SendINQTwice should default to true")
	}
	if !cfg.SendACKTwice {
		t.Fatal("SendACKTwice should default to true")
	}
	if !cfg.AcceptFDLenWithCR {
		t.Fatal("AcceptFDLenWithCR should default to true")
	}

	// Merge a partial config (simulating YAML with only timeout specified)
	partial := &Config{
		MasterTimeout: 90 * time.Second,
		// All bool fields are zero (false) - simulating unmarshaled YAML
	}

	cfg.MergeFrom(partial)

	// Bool defaults should be preserved (not overwritten with false)
	if !cfg.SendINQTwice {
		t.Errorf("SendINQTwice should remain true after MergeFrom")
	}
	if !cfg.SendACKTwice {
		t.Errorf("SendACKTwice should remain true after MergeFrom")
	}
	if !cfg.AcceptFDLenWithCR {
		t.Errorf("AcceptFDLenWithCR should remain true after MergeFrom")
	}

	// Non-bool values should be merged
	if cfg.MasterTimeout != 90*time.Second {
		t.Errorf("MasterTimeout should be updated to 90s")
	}
}

// Test that MergeFrom allows empty protocols for global NCP
func TestMergeFromAllowsEmptyProtocols(t *testing.T) {
	cfg := DefaultConfig()

	// Verify default protocols
	if len(cfg.Protocols) != 2 {
		t.Fatalf("Default should have 2 protocols, got %d", len(cfg.Protocols))
	}

	// Merge with explicitly empty protocols (for NCP)
	partial := &Config{
		Protocols: []string{}, // Explicitly empty, not nil
	}

	cfg.MergeFrom(partial)

	// Protocols should now be empty (for NCP)
	if len(cfg.Protocols) != 0 {
		t.Errorf("Protocols should be empty after merging empty slice, got %v", cfg.Protocols)
	}
}

// Test that nil protocols in MergeFrom doesn't affect existing protocols
func TestMergeFromNilProtocolsPreservesExisting(t *testing.T) {
	cfg := DefaultConfig()
	origProtocols := cfg.Protocols

	// Merge with nil protocols (not set in YAML)
	partial := &Config{
		MasterTimeout: 90 * time.Second,
		Protocols:     nil, // Not set
	}

	cfg.MergeFrom(partial)

	// Protocols should be unchanged
	if len(cfg.Protocols) != len(origProtocols) {
		t.Errorf("Protocols should be preserved when merging nil, got %v", cfg.Protocols)
	}
}

// Test NodeOverride.Copy()
func TestNodeOverrideCopy(t *testing.T) {
	protocols := []string{"ZMO", "ZAP"}
	timeout := 30 * time.Second
	preventive := true

	orig := &NodeOverride{
		StepTimeout:   &timeout,
		PreventiveINQ: &preventive,
		Protocols:     &protocols,
	}

	cp := orig.Copy()

	// Verify values are copied
	if cp.StepTimeout == nil || *cp.StepTimeout != 30*time.Second {
		t.Errorf("StepTimeout not copied correctly")
	}
	if cp.PreventiveINQ == nil || *cp.PreventiveINQ != true {
		t.Errorf("PreventiveINQ not copied correctly")
	}
	if cp.Protocols == nil || len(*cp.Protocols) != 2 {
		t.Errorf("Protocols not copied correctly")
	}

	// Modify original protocols
	(*orig.Protocols)[0] = "HYD"

	// Copy should be unaffected
	if (*cp.Protocols)[0] != "ZMO" {
		t.Errorf("Copy's Protocols should be independent, got %v", *cp.Protocols)
	}
}

// Test NodeOverride.Copy() with nil
func TestNodeOverrideCopyNil(t *testing.T) {
	var orig *NodeOverride = nil
	cp := orig.Copy()

	if cp != nil {
		t.Errorf("Copy of nil should be nil")
	}
}

// Test defensive copying in SetOverride
func TestSetOverrideDefensiveCopy(t *testing.T) {
	mgr := NewConfigManager()

	timeout := 30 * time.Second
	override := &NodeOverride{
		StepTimeout: &timeout,
	}

	mgr.SetOverride("2:5020/100", override)

	// Modify the original override
	newTimeout := 90 * time.Second
	override.StepTimeout = &newTimeout

	// Config from manager should have the original value
	cfg := mgr.GetConfigForNode("2:5020/100")
	if cfg.StepTimeout != 30*time.Second {
		t.Errorf("SetOverride should defensively copy, got StepTimeout=%v", cfg.StepTimeout)
	}
}

// Test defensive copying in LoadOverrides
func TestLoadOverridesDefensiveCopy(t *testing.T) {
	mgr := NewConfigManager()

	timeout := 30 * time.Second
	overrides := map[string]*NodeOverride{
		"2:5020/100": {StepTimeout: &timeout},
	}

	mgr.LoadOverrides(overrides)

	// Modify the original override
	newTimeout := 90 * time.Second
	overrides["2:5020/100"].StepTimeout = &newTimeout

	// Config from manager should have the original value
	cfg := mgr.GetConfigForNode("2:5020/100")
	if cfg.StepTimeout != 30*time.Second {
		t.Errorf("LoadOverrides should defensively copy, got StepTimeout=%v", cfg.StepTimeout)
	}
}
