package emsi

import (
	"strings"
	"testing"
)

// ============================================================================
// Link Code Building Tests
// ============================================================================

func TestBuildLinkCodes_Default(t *testing.T) {
	cfg := DefaultConfig()
	codes := buildLinkCodes(cfg, 1)

	if codes != "PUA" {
		t.Errorf("Expected 'PUA', got %q", codes)
	}
}

func TestBuildLinkCodes_WithQualifiers(t *testing.T) {
	cfg := DefaultConfig()
	cfg.LinkCode = "PUP"
	cfg.LinkQualifiers = []string{"PMO", "NFE"}

	codes := buildLinkCodes(cfg, 1)

	if codes != "PUP,PMO,NFE" {
		t.Errorf("Expected 'PUP,PMO,NFE', got %q", codes)
	}
}

func TestBuildLinkCodes_WithFNC(t *testing.T) {
	cfg := DefaultConfig()
	cfg.EnableEMSI2 = true
	cfg.RequireFNC = true

	codes := buildLinkCodes(cfg, 1)

	if !strings.Contains(codes, "FNC") {
		t.Errorf("Expected FNC in codes when RequireFNC=true, got %q", codes)
	}
}

func TestBuildPerAKACodes(t *testing.T) {
	pickup := true
	hold := true
	holdRequests := true

	cfg := &PerAKAConfig{
		Pickup:       &pickup,
		Hold:         &hold,
		HoldRequests: &holdRequests,
	}

	codes := buildPerAKACodes(2, cfg)

	// Should contain PU2, HA2, HR2
	expected := map[string]bool{"PU2": true, "HA2": true, "HR2": true}
	for _, code := range codes {
		if !expected[code] {
			t.Errorf("Unexpected code in per-AKA codes: %s", code)
		}
		delete(expected, code)
	}
	if len(expected) > 0 {
		t.Errorf("Missing expected codes: %v", expected)
	}
}

func TestBuildPerAKACodes_Nil(t *testing.T) {
	codes := buildPerAKACodes(0, nil)
	if codes != nil {
		t.Errorf("Expected nil for nil config, got %v", codes)
	}
}

func TestBuildLinkCodes_PerAKA(t *testing.T) {
	cfg := DefaultConfig()
	cfg.EnableEMSI2 = true
	cfg.LinkCode = "PUA"

	pickup := true
	hold := true
	cfg.PerAKAFlags = map[int]*PerAKAConfig{
		0: {Pickup: &pickup},
		1: {Hold: &hold},
	}

	// With 2 addresses, per-AKA flags should be included
	codes := buildLinkCodes(cfg, 2)

	if !strings.Contains(codes, "PU0") {
		t.Errorf("Expected PU0 in codes for multi-address, got %q", codes)
	}
	if !strings.Contains(codes, "HA1") {
		t.Errorf("Expected HA1 in codes for multi-address, got %q", codes)
	}
}

func TestBuildLinkCodes_SingleAddressWithPerAKA(t *testing.T) {
	cfg := DefaultConfig()
	cfg.EnableEMSI2 = true

	pickup := true
	cfg.PerAKAFlags = map[int]*PerAKAConfig{
		0: {Pickup: &pickup},
	}

	// FSC-0088.001: Single address with per-AKA config should use PU0 format
	// instead of PUA (neither PUA nor PUP should be presented with per-AKA flags)
	codes := buildLinkCodes(cfg, 1)

	if !strings.Contains(codes, "PU0") {
		t.Errorf("Single address with per-AKA should use PU0 format, got %q", codes)
	}
	if strings.Contains(codes, "PUA") {
		t.Errorf("Single address with per-AKA should NOT include PUA, got %q", codes)
	}
}

// ============================================================================
// Compatibility Code Building Tests
// ============================================================================

func TestBuildCompatibilityCodes_EMSI1(t *testing.T) {
	cfg := DefaultConfig()
	cfg.EnableEMSI2 = false
	cfg.SuppressDeprecated = false

	codes := buildCompatibilityCodes(cfg, nil)

	// Should include protocols
	if !strings.Contains(codes, "ZMO") || !strings.Contains(codes, "ZAP") {
		t.Errorf("Expected protocols in codes, got %q", codes)
	}

	// Should include ARC, XMA for EMSI-I compatibility
	if !strings.Contains(codes, "ARC") || !strings.Contains(codes, "XMA") {
		t.Errorf("EMSI-I mode should include ARC, XMA, got %q", codes)
	}

	// Should NOT include EII
	if strings.Contains(codes, "EII") {
		t.Errorf("EMSI-I mode should not include EII, got %q", codes)
	}
}

func TestBuildCompatibilityCodes_EMSI2(t *testing.T) {
	cfg := DefaultConfig()
	cfg.EnableEMSI2 = true
	cfg.SuppressDeprecated = true
	cfg.FileRequestCapable = true

	codes := buildCompatibilityCodes(cfg, nil)

	// Should include EII
	if !strings.Contains(codes, "EII") {
		t.Errorf("EMSI-II mode should include EII, got %q", codes)
	}

	// Should include FRQ
	if !strings.Contains(codes, "FRQ") {
		t.Errorf("Should include FRQ when FileRequestCapable=true, got %q", codes)
	}

	// Should NOT include ARC, XMA when suppressed
	if strings.Contains(codes, "ARC") || strings.Contains(codes, "XMA") {
		t.Errorf("EMSI-II mode should suppress ARC, XMA when configured, got %q", codes)
	}
}

func TestBuildCompatibilityCodes_WithDFB(t *testing.T) {
	cfg := DefaultConfig()
	cfg.EnableDFB = true

	codes := buildCompatibilityCodes(cfg, nil)

	if !strings.Contains(codes, "DFB") {
		t.Errorf("Should include DFB when EnableDFB=true, got %q", codes)
	}
}

func TestBuildCompatibilityCodes_WithNRQ(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NoFileRequests = true

	codes := buildCompatibilityCodes(cfg, nil)

	if !strings.Contains(codes, "NRQ") {
		t.Errorf("Should include NRQ when NoFileRequests=true, got %q", codes)
	}
}

// ============================================================================
// Link Code Parsing Tests
// ============================================================================

func TestParseLinkCodes_Basic(t *testing.T) {
	baseCode, qualifiers, perAKA := parseLinkCodes("PUA")

	if baseCode != "PUA" {
		t.Errorf("Expected base code PUA, got %s", baseCode)
	}
	if len(qualifiers) != 0 {
		t.Errorf("Expected no qualifiers, got %v", qualifiers)
	}
	if len(perAKA) != 0 {
		t.Errorf("Expected no per-AKA flags, got %v", perAKA)
	}
}

func TestParseLinkCodes_WithQualifiers(t *testing.T) {
	baseCode, qualifiers, perAKA := parseLinkCodes("PUP,PMO,NFE,HXT")

	if baseCode != "PUP" {
		t.Errorf("Expected base code PUP, got %s", baseCode)
	}
	if len(qualifiers) != 3 {
		t.Errorf("Expected 3 qualifiers, got %d: %v", len(qualifiers), qualifiers)
	}
	if len(perAKA) != 0 {
		t.Errorf("Expected no per-AKA flags, got %v", perAKA)
	}
}

func TestParseLinkCodes_PerAKA(t *testing.T) {
	baseCode, _, perAKA := parseLinkCodes("PUA,PM0,NF1,HA2,HR2")

	if baseCode != "PUA" {
		t.Errorf("Expected base code PUA, got %s", baseCode)
	}

	// Check per-AKA flags
	if len(perAKA[0]) != 1 || perAKA[0][0] != "PM" {
		t.Errorf("Expected PM for AKA 0, got %v", perAKA[0])
	}
	if len(perAKA[1]) != 1 || perAKA[1][0] != "NF" {
		t.Errorf("Expected NF for AKA 1, got %v", perAKA[1])
	}
	if len(perAKA[2]) != 2 {
		t.Errorf("Expected 2 flags for AKA 2, got %d", len(perAKA[2]))
	}
}

func TestParseLinkCodes_AllBaseCodes(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"PUA", "PUA"},
		{"PUP", "PUP"},
		{"NPU", "NPU"},
		{"HAT", "HAT"},
	}

	for _, tt := range tests {
		baseCode, _, _ := parseLinkCodes(tt.input)
		if baseCode != tt.expected {
			t.Errorf("For input %q: expected %s, got %s", tt.input, tt.expected, baseCode)
		}
	}
}

func TestParseLinkCodes_EmptyDefaultsToPUA(t *testing.T) {
	baseCode, _, _ := parseLinkCodes("")

	if baseCode != "PUA" {
		t.Errorf("Empty input should default to PUA, got %s", baseCode)
	}
}

// ============================================================================
// Compatibility Code Parsing Tests
// ============================================================================

func TestParseCompatibilityCodes_Protocols(t *testing.T) {
	data := &EMSIData{}
	parseCompatibilityCodes("ZMO,ZAP,HYD", data)

	if len(data.Protocols) != 3 {
		t.Errorf("Expected 3 protocols, got %d: %v", len(data.Protocols), data.Protocols)
	}
}

func TestParseCompatibilityCodes_EMSI2Flags(t *testing.T) {
	data := &EMSIData{}
	parseCompatibilityCodes("ZMO,EII,DFB,FRQ,NRQ", data)

	if !data.HasEII {
		t.Error("Expected HasEII=true")
	}
	if !data.HasDFB {
		t.Error("Expected HasDFB=true")
	}
	if !data.HasFRQ {
		t.Error("Expected HasFRQ=true")
	}
	if !data.HasNRQ {
		t.Error("Expected HasNRQ=true")
	}
}

func TestParseCompatibilityCodes_HydraFlags(t *testing.T) {
	// FSC-0088.001: RMA/RH1 are link-session options, not compatibility codes
	// They should be parsed from link codes field
	data := &EMSIData{}
	parseCompatibilityCodes("HYD,RMA,RH1", data)

	// RMA and RH1 should be ignored in compat codes
	// HYD should be in protocols
	if len(data.Protocols) != 1 || data.Protocols[0] != "HYD" {
		t.Errorf("Expected only HYD in protocols, got %v", data.Protocols)
	}
}

func TestParseLinkCodes_SessionOptions(t *testing.T) {
	// FSC-0088.001: FNC/RMA/RH1 are link-session options parsed from link codes
	result := parseLinkCodesEx("PUA,FNC,RMA,RH1")

	if !result.HasFNC {
		t.Error("Expected HasFNC=true from link codes")
	}
	if !result.HasRMA {
		t.Error("Expected HasRMA=true from link codes")
	}
	if !result.HasRH1 {
		t.Error("Expected HasRH1=true from link codes")
	}
}

func TestParseCompatibilityCodes_IgnoresDeprecated(t *testing.T) {
	data := &EMSIData{}
	parseCompatibilityCodes("ZMO,ARC,XMA", data)

	// ARC and XMA should be ignored, only ZMO should be in protocols
	if len(data.Protocols) != 1 || data.Protocols[0] != "ZMO" {
		t.Errorf("Expected only ZMO in protocols, got %v", data.Protocols)
	}
}

func TestParseCompatibilityCodes_NCP(t *testing.T) {
	data := &EMSIData{}
	parseCompatibilityCodes("NCP", data)

	// NCP should result in empty protocols
	if len(data.Protocols) != 0 {
		t.Errorf("NCP should result in empty protocols, got %v", data.Protocols)
	}
}

// ============================================================================
// CreateEMSI_DAT Tests
// ============================================================================

func TestCreateEMSI_DAT_LegacyCompatibility(t *testing.T) {
	data := &EMSIData{
		SystemName:    "Test System",
		Location:      "Test Location",
		Sysop:         "Test Sysop",
		MailerName:    "TestMailer",
		MailerVersion: "1.0",
		MailerSerial:  "LNX",
		Addresses:     []string{"2:5020/100"},
		Protocols:     []string{"ZMO", "ZAP"},
	}

	// Legacy function should still work
	packet := CreateEMSI_DAT(data)

	if !strings.HasPrefix(packet, "**EMSI_DAT") {
		t.Errorf("Packet should start with **EMSI_DAT, got %s", packet[:20])
	}
}

func TestCreateEMSI_DATWithConfig_EMSI1(t *testing.T) {
	cfg := DefaultConfig()
	cfg.EnableEMSI2 = false

	data := &EMSIData{
		SystemName:    "Test System",
		Location:      "Test Location",
		Sysop:         "Test Sysop",
		MailerName:    "TestMailer",
		MailerVersion: "1.0",
		MailerSerial:  "LNX",
		Addresses:     []string{"2:5020/100"},
	}

	packet := CreateEMSI_DATWithConfig(data, cfg)

	// Should contain PUA (default link code)
	if !strings.Contains(packet, "{PUA}") {
		t.Errorf("EMSI-I packet should contain {PUA}, got %s", packet)
	}

	// Should contain ARC,XMA (deprecated but included in EMSI-I)
	if !strings.Contains(packet, "ARC") || !strings.Contains(packet, "XMA") {
		t.Errorf("EMSI-I packet should contain ARC,XMA, got %s", packet)
	}
}

func TestCreateEMSI_DATWithConfig_EMSI2(t *testing.T) {
	cfg := DefaultConfig()
	cfg.EnableEMSI2 = true
	cfg.SuppressDeprecated = true
	cfg.LinkCode = "PUP"
	cfg.LinkQualifiers = []string{"PMO"}

	data := &EMSIData{
		SystemName:    "Test System",
		Location:      "Test Location",
		Sysop:         "Test Sysop",
		MailerName:    "TestMailer",
		MailerVersion: "1.0",
		MailerSerial:  "LNX",
		Addresses:     []string{"2:5020/100"},
	}

	packet := CreateEMSI_DATWithConfig(data, cfg)

	// Should contain PUP,PMO
	if !strings.Contains(packet, "PUP") || !strings.Contains(packet, "PMO") {
		t.Errorf("EMSI-II packet should contain PUP,PMO link codes, got %s", packet)
	}

	// Should contain EII
	if !strings.Contains(packet, "EII") {
		t.Errorf("EMSI-II packet should contain EII, got %s", packet)
	}

	// Should NOT contain ARC,XMA when suppressed
	if strings.Contains(packet, "ARC") || strings.Contains(packet, "XMA") {
		t.Errorf("EMSI-II packet should not contain ARC,XMA when suppressed, got %s", packet)
	}
}

// ============================================================================
// ParseEMSI_DAT Tests
// ============================================================================

func TestParseEMSI_DAT_LinkCodes(t *testing.T) {
	// Create a packet with specific link codes
	cfg := DefaultConfig()
	cfg.LinkCode = "PUP"
	cfg.LinkQualifiers = []string{"PMO", "NFE"}

	data := &EMSIData{
		SystemName:    "Test System",
		Location:      "Test Location",
		Sysop:         "Test Sysop",
		MailerName:    "TestMailer",
		MailerVersion: "1.0",
		MailerSerial:  "LNX",
		Addresses:     []string{"2:5020/100"},
	}

	packet := CreateEMSI_DATWithConfig(data, cfg)
	parsed, err := ParseEMSI_DAT(packet)
	if err != nil {
		t.Fatalf("ParseEMSI_DAT failed: %v", err)
	}

	if parsed.LinkCode != "PUP" {
		t.Errorf("Expected LinkCode=PUP, got %s", parsed.LinkCode)
	}
	if len(parsed.LinkQualifiers) != 2 {
		t.Errorf("Expected 2 link qualifiers, got %d", len(parsed.LinkQualifiers))
	}
}

func TestParseEMSI_DAT_EMSI2Flags(t *testing.T) {
	// Create an EMSI-II packet
	cfg := DefaultConfig()
	cfg.EnableEMSI2 = true
	cfg.EnableDFB = true
	cfg.FileRequestCapable = true

	data := &EMSIData{
		SystemName:    "Test System",
		Location:      "Test Location",
		Sysop:         "Test Sysop",
		MailerName:    "TestMailer",
		MailerVersion: "1.0",
		MailerSerial:  "LNX",
		Addresses:     []string{"2:5020/100"},
	}

	packet := CreateEMSI_DATWithConfig(data, cfg)
	parsed, err := ParseEMSI_DAT(packet)
	if err != nil {
		t.Fatalf("ParseEMSI_DAT failed: %v", err)
	}

	if !parsed.HasEII {
		t.Error("Expected HasEII=true after parsing EMSI-II packet")
	}
	if !parsed.HasDFB {
		t.Error("Expected HasDFB=true after parsing packet with DFB")
	}
	if !parsed.HasFRQ {
		t.Error("Expected HasFRQ=true after parsing packet with FRQ")
	}
}

// ============================================================================
// EII Ordering Tests (FSC-0088.001)
// ============================================================================

func TestBuildCompatibilityCodes_EIIFirst(t *testing.T) {
	cfg := DefaultConfig()
	cfg.EnableEMSI2 = true
	cfg.Protocols = []string{"ZMO", "ZAP", "HYD"}

	codes := buildCompatibilityCodes(cfg, nil)

	// FSC-0088.001: EII must be FIRST in compatibility codes
	if !strings.HasPrefix(codes, "EII,") {
		t.Errorf("EII must be first in compatibility codes per FSC-0088.001, got %q", codes)
	}
}

func TestCreateEMSI_DAT_EIIFirst(t *testing.T) {
	cfg := DefaultConfig()
	cfg.EnableEMSI2 = true
	cfg.Protocols = []string{"ZMO", "ZAP"}

	data := &EMSIData{
		SystemName:    "Test System",
		Location:      "Test Location",
		Sysop:         "Test Sysop",
		MailerName:    "TestMailer",
		MailerVersion: "1.0",
		MailerSerial:  "LNX",
		Addresses:     []string{"2:5020/100"},
	}

	packet := CreateEMSI_DATWithConfig(data, cfg)

	// Find the compatibility codes field and verify EII is first
	// Format: ...}{EII,ZMO,ZAP,...}{...
	if !strings.Contains(packet, "{EII,") {
		t.Errorf("EMSI-II packet should have EII first in compat codes, got %s", packet)
	}
}

// ============================================================================
// Backward Compatibility Tests
// ============================================================================

func TestCreateEMSI_DAT_UsesDataProtocols(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Protocols = []string{"ZMO"} // Config has only ZMO

	data := &EMSIData{
		SystemName:    "Test System",
		Location:      "Test Location",
		Sysop:         "Test Sysop",
		MailerName:    "TestMailer",
		MailerVersion: "1.0",
		MailerSerial:  "LNX",
		Addresses:     []string{"2:5020/100"},
		Protocols:     []string{"ZAP", "HYD"}, // Data has ZAP and HYD
	}

	packet := CreateEMSI_DATWithConfig(data, cfg)

	// Should use data.Protocols (ZAP, HYD) over config.Protocols (ZMO)
	if !strings.Contains(packet, "ZAP") || !strings.Contains(packet, "HYD") {
		t.Errorf("Should use EMSIData.Protocols for backward compatibility, got %s", packet)
	}
}

func TestCreateEMSI_DAT_FallsBackToConfigProtocols(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Protocols = []string{"ZMO", "ZAP"}

	data := &EMSIData{
		SystemName:    "Test System",
		Location:      "Test Location",
		Sysop:         "Test Sysop",
		MailerName:    "TestMailer",
		MailerVersion: "1.0",
		MailerSerial:  "LNX",
		Addresses:     []string{"2:5020/100"},
		Protocols:     nil, // No protocols in data
	}

	packet := CreateEMSI_DATWithConfig(data, cfg)

	// Should fall back to config.Protocols
	if !strings.Contains(packet, "ZMO") || !strings.Contains(packet, "ZAP") {
		t.Errorf("Should fall back to config.Protocols when data.Protocols is empty, got %s", packet)
	}
}

// ============================================================================
// Per-AKA Flag Ordering Tests
// ============================================================================

func TestBuildLinkCodes_PerAKAOrdering(t *testing.T) {
	cfg := DefaultConfig()
	cfg.EnableEMSI2 = true

	pickup := true
	hold := true
	cfg.PerAKAFlags = map[int]*PerAKAConfig{
		2: {Hold: &hold},
		0: {Pickup: &pickup},
		1: {Pickup: &pickup},
	}

	codes := buildLinkCodes(cfg, 3)

	// Per-AKA flags should be sorted by index (0, 1, 2)
	pu0Idx := strings.Index(codes, "PU0")
	pu1Idx := strings.Index(codes, "PU1")
	ha2Idx := strings.Index(codes, "HA2")

	if pu0Idx == -1 || pu1Idx == -1 || ha2Idx == -1 {
		t.Errorf("Expected all per-AKA flags present, got %q", codes)
		return
	}

	if !(pu0Idx < pu1Idx && pu1Idx < ha2Idx) {
		t.Errorf("Per-AKA flags should be sorted by index, got %q", codes)
	}
}
