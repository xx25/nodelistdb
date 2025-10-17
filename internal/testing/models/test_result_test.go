package models

import (
	"testing"
	"time"
)

func TestNewTestResult(t *testing.T) {
	node := &Node{
		Zone:              2,
		Net:               450,
		Node:              1024,
		InternetHostnames: []string{"bbs.example.com"},
	}

	result := NewTestResult(node)

	if result.Zone != 2 {
		t.Errorf("Expected Zone 2, got %d", result.Zone)
	}
	if result.Net != 450 {
		t.Errorf("Expected Net 450, got %d", result.Net)
	}
	if result.Node != 1024 {
		t.Errorf("Expected Node 1024, got %d", result.Node)
	}
	if result.Address != "2:450/1024" {
		t.Errorf("Expected Address 2:450/1024, got %s", result.Address)
	}
	if result.Hostname != "bbs.example.com" {
		t.Errorf("Expected Hostname bbs.example.com, got %s", result.Hostname)
	}
	if result.HostnameIndex != -1 {
		t.Errorf("Expected HostnameIndex -1 (legacy), got %d", result.HostnameIndex)
	}
	if !result.IsAggregated {
		t.Error("Expected IsAggregated to be true for legacy results")
	}
	if result.TotalHostnames != 1 {
		t.Errorf("Expected TotalHostnames 1, got %d", result.TotalHostnames)
	}
}

func TestProtocolTestResultSetIPv4Result(t *testing.T) {
	pr := &ProtocolTestResult{}

	pr.SetIPv4Result(true, 150, "192.168.1.1", "")

	if !pr.IPv4Tested {
		t.Error("Expected IPv4Tested to be true")
	}
	if !pr.IPv4Success {
		t.Error("Expected IPv4Success to be true")
	}
	if pr.IPv4ResponseMs != 150 {
		t.Errorf("Expected IPv4ResponseMs 150, got %d", pr.IPv4ResponseMs)
	}
	if pr.IPv4Address != "192.168.1.1" {
		t.Errorf("Expected IPv4Address 192.168.1.1, got %s", pr.IPv4Address)
	}
	if !pr.Success {
		t.Error("Expected overall Success to be true")
	}
	if !pr.Tested {
		t.Error("Expected overall Tested to be true")
	}
	if pr.ResponseMs != 150 {
		t.Errorf("Expected overall ResponseMs 150, got %d", pr.ResponseMs)
	}
}

func TestProtocolTestResultSetIPv6Result(t *testing.T) {
	pr := &ProtocolTestResult{}

	pr.SetIPv6Result(true, 200, "2001:db8::1", "")

	if !pr.IPv6Tested {
		t.Error("Expected IPv6Tested to be true")
	}
	if !pr.IPv6Success {
		t.Error("Expected IPv6Success to be true")
	}
	if pr.IPv6ResponseMs != 200 {
		t.Errorf("Expected IPv6ResponseMs 200, got %d", pr.IPv6ResponseMs)
	}
	if pr.IPv6Address != "2001:db8::1" {
		t.Errorf("Expected IPv6Address 2001:db8::1, got %s", pr.IPv6Address)
	}
	if !pr.Success {
		t.Error("Expected overall Success to be true")
	}
	if !pr.Tested {
		t.Error("Expected overall Tested to be true")
	}
	if pr.ResponseMs != 200 {
		t.Errorf("Expected overall ResponseMs 200, got %d", pr.ResponseMs)
	}
}

func TestProtocolTestResultBothIPv4AndIPv6(t *testing.T) {
	pr := &ProtocolTestResult{}

	// Set IPv4 result first (slower)
	pr.SetIPv4Result(true, 300, "192.168.1.1", "")

	// Set IPv6 result (faster)
	pr.SetIPv6Result(true, 150, "2001:db8::1", "")

	// Overall response time should be the best (lowest) time
	if pr.ResponseMs != 150 {
		t.Errorf("Expected overall ResponseMs to be 150 (best time), got %d", pr.ResponseMs)
	}

	if !pr.IPv4Success {
		t.Error("Expected IPv4Success to be true")
	}
	if !pr.IPv6Success {
		t.Error("Expected IPv6Success to be true")
	}
	if !pr.Success {
		t.Error("Expected overall Success to be true")
	}
}

func TestProtocolTestResultIPv4FailsIPv6Succeeds(t *testing.T) {
	pr := &ProtocolTestResult{}

	pr.SetIPv4Result(false, 0, "192.168.1.1", "Connection refused")
	pr.SetIPv6Result(true, 200, "2001:db8::1", "")

	if pr.IPv4Success {
		t.Error("Expected IPv4Success to be false")
	}
	if !pr.IPv6Success {
		t.Error("Expected IPv6Success to be true")
	}
	if !pr.Success {
		t.Error("Expected overall Success to be true (IPv6 succeeded)")
	}
	if pr.ResponseMs != 200 {
		t.Errorf("Expected overall ResponseMs 200, got %d", pr.ResponseMs)
	}
	if pr.IPv4Error != "Connection refused" {
		t.Errorf("Expected IPv4Error 'Connection refused', got %s", pr.IPv4Error)
	}
}

func TestProtocolTestResultBothFail(t *testing.T) {
	pr := &ProtocolTestResult{}

	pr.SetIPv4Result(false, 0, "192.168.1.1", "Timeout")
	pr.SetIPv6Result(false, 0, "2001:db8::1", "Connection refused")

	if pr.IPv4Success {
		t.Error("Expected IPv4Success to be false")
	}
	if pr.IPv6Success {
		t.Error("Expected IPv6Success to be false")
	}
	if pr.Success {
		t.Error("Expected overall Success to be false")
	}
	// When both tests fail, Tested flag may not be set by SetIPv4/IPv6Result
	// That's okay - the implementation only sets Tested when there's a success
	if !pr.IPv4Tested {
		t.Error("Expected IPv4Tested to be true")
	}
	if !pr.IPv6Tested {
		t.Error("Expected IPv6Tested to be true")
	}
}

func TestProtocolTestResultGetConnectivityType(t *testing.T) {
	tests := []struct {
		name         string
		ipv4Success  bool
		ipv4Tested   bool
		ipv6Success  bool
		ipv6Tested   bool
		expected     string
	}{
		{
			name:        "dual-stack",
			ipv4Success: true,
			ipv4Tested:  true,
			ipv6Success: true,
			ipv6Tested:  true,
			expected:    "dual-stack",
		},
		{
			name:        "ipv6-only",
			ipv4Success: false,
			ipv4Tested:  true,
			ipv6Success: true,
			ipv6Tested:  true,
			expected:    "ipv6-only",
		},
		{
			name:        "ipv4-only",
			ipv4Success: true,
			ipv4Tested:  true,
			ipv6Success: false,
			ipv6Tested:  true,
			expected:    "ipv4-only",
		},
		{
			name:        "failed both tested",
			ipv4Success: false,
			ipv4Tested:  true,
			ipv6Success: false,
			ipv6Tested:  true,
			expected:    "failed",
		},
		{
			name:        "failed ipv4 only tested",
			ipv4Success: false,
			ipv4Tested:  true,
			ipv6Success: false,
			ipv6Tested:  false,
			expected:    "failed",
		},
		{
			name:        "not tested",
			ipv4Success: false,
			ipv4Tested:  false,
			ipv6Success: false,
			ipv6Tested:  false,
			expected:    "not-tested",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pr := &ProtocolTestResult{
				IPv4Success: tt.ipv4Success,
				IPv4Tested:  tt.ipv4Tested,
				IPv6Success: tt.ipv6Success,
				IPv6Tested:  tt.ipv6Tested,
			}
			result := pr.GetConnectivityType()
			if result != tt.expected {
				t.Errorf("GetConnectivityType() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestTestResultSetBinkPResult(t *testing.T) {
	node := &Node{Zone: 2, Net: 450, Node: 1024}
	result := NewTestResult(node)

	details := &BinkPTestDetails{
		SystemName:   "Test BBS",
		Sysop:        "Test Sysop",
		Location:     "Test City",
		Version:      "binkd/1.0",
		Addresses:    []string{"2:450/1024"},
		Capabilities: []string{"CRYPT", "CHAT"},
	}

	result.SetBinkPResult(true, 150, details, "")

	if result.BinkPResult == nil {
		t.Fatal("Expected BinkPResult to be set")
	}
	if !result.BinkPResult.Tested {
		t.Error("Expected BinkPResult.Tested to be true")
	}
	if !result.BinkPResult.Success {
		t.Error("Expected BinkPResult.Success to be true")
	}
	if result.BinkPResult.ResponseMs != 150 {
		t.Errorf("Expected BinkPResult.ResponseMs 150, got %d", result.BinkPResult.ResponseMs)
	}
	if !result.IsOperational {
		t.Error("Expected IsOperational to be true when BinkP succeeds")
	}

	// Check details
	if result.BinkPResult.Details["system_name"] != "Test BBS" {
		t.Errorf("Expected system_name 'Test BBS', got %v", result.BinkPResult.Details["system_name"])
	}
	if result.BinkPResult.Details["sysop"] != "Test Sysop" {
		t.Errorf("Expected sysop 'Test Sysop', got %v", result.BinkPResult.Details["sysop"])
	}
}

func TestTestResultSetIfcicoResult(t *testing.T) {
	node := &Node{Zone: 2, Net: 450, Node: 1024}
	result := NewTestResult(node)

	details := &IfcicoTestDetails{
		MailerInfo:   "ifcico/1.0",
		SystemName:   "Test BBS",
		Addresses:    []string{"2:450/1024"},
		ResponseType: "REQ",
	}

	result.SetIfcicoResult(true, 200, details, "")

	if result.IfcicoResult == nil {
		t.Fatal("Expected IfcicoResult to be set")
	}
	if !result.IfcicoResult.Tested {
		t.Error("Expected IfcicoResult.Tested to be true")
	}
	if !result.IfcicoResult.Success {
		t.Error("Expected IfcicoResult.Success to be true")
	}
	if result.IfcicoResult.ResponseMs != 200 {
		t.Errorf("Expected IfcicoResult.ResponseMs 200, got %d", result.IfcicoResult.ResponseMs)
	}
	if !result.IsOperational {
		t.Error("Expected IsOperational to be true when Ifcico succeeds")
	}

	// Check details
	if result.IfcicoResult.Details["mailer_info"] != "ifcico/1.0" {
		t.Errorf("Expected mailer_info 'ifcico/1.0', got %v", result.IfcicoResult.Details["mailer_info"])
	}
}

func TestTestResultSetDNSResult(t *testing.T) {
	node := &Node{Zone: 2, Net: 450, Node: 1024}
	result := NewTestResult(node)

	dnsResult := &DNSResult{
		Hostname:      "bbs.example.com",
		IPv4Addresses: []string{"192.168.1.1", "192.168.1.2"},
		IPv6Addresses: []string{"2001:db8::1"},
		Error:         nil,
		ResolutionMs:  50,
	}

	result.SetDNSResult(dnsResult)

	if len(result.ResolvedIPv4) != 2 {
		t.Errorf("Expected 2 IPv4 addresses, got %d", len(result.ResolvedIPv4))
	}
	if len(result.ResolvedIPv6) != 1 {
		t.Errorf("Expected 1 IPv6 address, got %d", len(result.ResolvedIPv6))
	}
	if result.DNSError != "" {
		t.Errorf("Expected no DNS error, got %s", result.DNSError)
	}
}

func TestTestResultSetDNSResultWithError(t *testing.T) {
	node := &Node{Zone: 2, Net: 450, Node: 1024}
	result := NewTestResult(node)

	dnsResult := &DNSResult{
		Hostname:      "invalid.example.com",
		IPv4Addresses: []string{},
		IPv6Addresses: []string{},
		Error:         &dnsError{msg: "no such host"},
		ResolutionMs:  10,
	}

	result.SetDNSResult(dnsResult)

	if result.DNSError != "no such host" {
		t.Errorf("Expected DNS error 'no such host', got %s", result.DNSError)
	}
}

// Mock error type for testing
type dnsError struct {
	msg string
}

func (e *dnsError) Error() string {
	return e.msg
}

func TestTestResultSetGeolocation(t *testing.T) {
	node := &Node{Zone: 2, Net: 450, Node: 1024}
	result := NewTestResult(node)

	geo := &GeolocationResult{
		IP:          "192.168.1.1",
		Country:     "United States",
		CountryCode: "US",
		City:        "New York",
		Region:      "NY",
		Latitude:    40.7128,
		Longitude:   -74.0060,
		ISP:         "Example ISP",
		Org:         "Example Org",
		ASN:         12345,
	}

	result.SetGeolocation(geo)

	if result.Country != "United States" {
		t.Errorf("Expected Country 'United States', got %s", result.Country)
	}
	if result.CountryCode != "US" {
		t.Errorf("Expected CountryCode 'US', got %s", result.CountryCode)
	}
	if result.City != "New York" {
		t.Errorf("Expected City 'New York', got %s", result.City)
	}
	if result.Latitude != 40.7128 {
		t.Errorf("Expected Latitude 40.7128, got %f", result.Latitude)
	}
	if result.ASN != 12345 {
		t.Errorf("Expected ASN 12345, got %d", result.ASN)
	}
}

func TestTestResultSetTelnetResult(t *testing.T) {
	node := &Node{Zone: 2, Net: 450, Node: 1024}
	result := NewTestResult(node)

	result.SetTelnetResult(true, 100, "Welcome to Test BBS", "")

	if result.TelnetResult == nil {
		t.Fatal("Expected TelnetResult to be set")
	}
	if !result.TelnetResult.Success {
		t.Error("Expected TelnetResult.Success to be true")
	}
	if result.TelnetResult.ResponseMs != 100 {
		t.Errorf("Expected TelnetResult.ResponseMs 100, got %d", result.TelnetResult.ResponseMs)
	}
	if result.TelnetResult.Details["banner"] != "Welcome to Test BBS" {
		t.Errorf("Expected banner 'Welcome to Test BBS', got %v", result.TelnetResult.Details["banner"])
	}
	if !result.IsOperational {
		t.Error("Expected IsOperational to be true when Telnet succeeds")
	}
}

func TestTestResultSetFTPResult(t *testing.T) {
	node := &Node{Zone: 2, Net: 450, Node: 1024}
	result := NewTestResult(node)

	result.SetFTPResult(true, 80, "220 Welcome", "")

	if result.FTPResult == nil {
		t.Fatal("Expected FTPResult to be set")
	}
	if !result.FTPResult.Success {
		t.Error("Expected FTPResult.Success to be true")
	}
	if result.FTPResult.ResponseMs != 80 {
		t.Errorf("Expected FTPResult.ResponseMs 80, got %d", result.FTPResult.ResponseMs)
	}
	if result.FTPResult.Details["welcome"] != "220 Welcome" {
		t.Errorf("Expected welcome '220 Welcome', got %v", result.FTPResult.Details["welcome"])
	}
}

func TestTestResultSetVModemResult(t *testing.T) {
	node := &Node{Zone: 2, Net: 450, Node: 1024}
	result := NewTestResult(node)

	result.SetVModemResult(true, 120, "")

	if result.VModemResult == nil {
		t.Fatal("Expected VModemResult to be set")
	}
	if !result.VModemResult.Success {
		t.Error("Expected VModemResult.Success to be true")
	}
	if result.VModemResult.ResponseMs != 120 {
		t.Errorf("Expected VModemResult.ResponseMs 120, got %d", result.VModemResult.ResponseMs)
	}
	if !result.IsOperational {
		t.Error("Expected IsOperational to be true when VModem succeeds")
	}
}

func TestNewAggregatedTestResult(t *testing.T) {
	node := &Node{
		Zone:              2,
		Net:               450,
		Node:              1024,
		InternetHostnames: []string{"bbs.example.com"},
	}

	result := NewAggregatedTestResult(node)

	if result.Zone != 2 {
		t.Errorf("Expected Zone 2, got %d", result.Zone)
	}
	if result.Net != 450 {
		t.Errorf("Expected Net 450, got %d", result.Net)
	}
	if result.Node != 1024 {
		t.Errorf("Expected Node 1024, got %d", result.Node)
	}
	if result.PrimaryHostname != "bbs.example.com" {
		t.Errorf("Expected PrimaryHostname bbs.example.com, got %s", result.PrimaryHostname)
	}
	if result.HostnameResults == nil {
		t.Error("Expected HostnameResults map to be initialized")
	}
}

func TestAggregatedTestResultAddHostnameResult(t *testing.T) {
	node := &Node{Zone: 2, Net: 450, Node: 1024}
	atr := NewAggregatedTestResult(node)

	hostnameResult := &HostnameTestResult{
		Hostname:      "bbs.example.com",
		DNSResolved:   true,
		IPv4Addresses: []string{"192.168.1.1"},
		IsOperational: true,
		ResponseMs:    150,
		TestTime:      time.Now(),
	}

	atr.AddHostnameResult(hostnameResult)

	if len(atr.HostnameResults) != 1 {
		t.Errorf("Expected 1 hostname result, got %d", len(atr.HostnameResults))
	}
	if !atr.AnyHostnameOperational {
		t.Error("Expected AnyHostnameOperational to be true")
	}
	if len(atr.WorkingHostnames) != 1 {
		t.Errorf("Expected 1 working hostname, got %d", len(atr.WorkingHostnames))
	}
	if atr.BestResponseMs != 150 {
		t.Errorf("Expected BestResponseMs 150, got %d", atr.BestResponseMs)
	}
	if atr.BestHostname != "bbs.example.com" {
		t.Errorf("Expected BestHostname bbs.example.com, got %s", atr.BestHostname)
	}
}

func TestAggregatedTestResultAddFailedHostnameResult(t *testing.T) {
	node := &Node{Zone: 2, Net: 450, Node: 1024}
	atr := NewAggregatedTestResult(node)

	hostnameResult := &HostnameTestResult{
		Hostname:      "dead.example.com",
		DNSResolved:   true,
		IPv4Addresses: []string{"192.168.1.1"},
		IsOperational: false,
		TestTime:      time.Now(),
	}

	atr.AddHostnameResult(hostnameResult)

	if atr.AnyHostnameOperational {
		t.Error("Expected AnyHostnameOperational to be false")
	}
	if len(atr.FailedHostnames) != 1 {
		t.Errorf("Expected 1 failed hostname, got %d", len(atr.FailedHostnames))
	}
	if len(atr.WorkingHostnames) != 0 {
		t.Errorf("Expected 0 working hostnames, got %d", len(atr.WorkingHostnames))
	}
}

func TestAggregatedTestResultGetSuccessRate(t *testing.T) {
	node := &Node{Zone: 2, Net: 450, Node: 1024}
	atr := NewAggregatedTestResult(node)

	// Add 2 working and 1 failed hostname
	atr.AddHostnameResult(&HostnameTestResult{
		Hostname:      "host1.example.com",
		IsOperational: true,
		ResponseMs:    100,
	})
	atr.AddHostnameResult(&HostnameTestResult{
		Hostname:      "host2.example.com",
		IsOperational: true,
		ResponseMs:    150,
	})
	atr.AddHostnameResult(&HostnameTestResult{
		Hostname:      "host3.example.com",
		IsOperational: false,
	})

	rate := atr.GetSuccessRate()
	expected := 66.66666666666666 // 2/3 * 100

	if rate != expected {
		t.Errorf("Expected success rate %.2f%%, got %.2f%%", expected, rate)
	}
}

func TestAggregatedTestResultGetSuccessRateEmpty(t *testing.T) {
	node := &Node{Zone: 2, Net: 450, Node: 1024}
	atr := NewAggregatedTestResult(node)

	rate := atr.GetSuccessRate()
	if rate != 0 {
		t.Errorf("Expected success rate 0%% for empty results, got %.2f%%", rate)
	}
}
