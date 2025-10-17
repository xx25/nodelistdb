package daemon

import (
	"testing"

	"github.com/nodelistdb/internal/testing/models"
)

func TestCreateAggregatedResult_EmptyResults(t *testing.T) {
	node := &models.Node{
		Zone: 2,
		Net:  450,
		Node: 1024,
	}

	aggregator := NewTestAggregator()
	result := aggregator.CreateAggregatedResult(node, []*models.TestResult{})

	if result != nil {
		t.Error("Expected nil result for empty results slice")
	}
}

func TestCreateAggregatedResult_SingleHostname(t *testing.T) {
	node := &models.Node{
		Zone:              2,
		Net:               450,
		Node:              1024,
		InternetHostnames: []string{"test.example.com"},
	}

	testResult := &models.TestResult{
		Zone:           2,
		Net:            450,
		Node:           1024,
		TestedHostname: "test.example.com",
		ResolvedIPv4:   []string{"192.168.1.1"},
		ResolvedIPv6:   []string{"2001:db8::1"},
		IsOperational:  true,
		Country:        "US",
		CountryCode:    "US",
		City:           "New York",
		BinkPResult: &models.ProtocolTestResult{
			Tested:  true,
			Success: true,
		},
	}

	aggregator := NewTestAggregator()
	result := aggregator.CreateAggregatedResult(node, []*models.TestResult{testResult})

	if result == nil {
		t.Fatal("Expected non-nil aggregated result")
	}

	if !result.IsAggregated {
		t.Error("Expected IsAggregated to be true")
	}

	if result.Zone != 2 || result.Net != 450 || result.Node != 1024 {
		t.Errorf("Expected address 2:450/1024, got %d:%d/%d", result.Zone, result.Net, result.Node)
	}

	if !result.IsOperational {
		t.Error("Expected IsOperational to be true")
	}

	if len(result.ResolvedIPv4) != 1 || result.ResolvedIPv4[0] != "192.168.1.1" {
		t.Errorf("Expected IPv4 [192.168.1.1], got %v", result.ResolvedIPv4)
	}

	if len(result.ResolvedIPv6) != 1 || result.ResolvedIPv6[0] != "2001:db8::1" {
		t.Errorf("Expected IPv6 [2001:db8::1], got %v", result.ResolvedIPv6)
	}

	if result.Country != "US" {
		t.Errorf("Expected Country US, got %s", result.Country)
	}

	if result.BinkPResult == nil || !result.BinkPResult.Success {
		t.Error("Expected successful BinkP result")
	}
}

func TestCreateAggregatedResult_MultipleHostnames_AllSuccess(t *testing.T) {
	node := &models.Node{
		Zone:              2,
		Net:               450,
		Node:              1024,
		InternetHostnames: []string{"host1.example.com", "host2.example.com"},
		InternetProtocols: []string{"IBN", "IFC"},
	}

	results := []*models.TestResult{
		{
			Zone:           2,
			Net:            450,
			Node:           1024,
			TestedHostname: "host1.example.com",
			HostnameIndex:  0,
			ResolvedIPv4:   []string{"192.168.1.1"},
			ResolvedIPv6:   []string{"2001:db8::1"},
			IsOperational:  true,
			Country:        "US",
			BinkPResult: &models.ProtocolTestResult{
				Tested:  true,
				Success: true,
			},
		},
		{
			Zone:           2,
			Net:            450,
			Node:           1024,
			TestedHostname: "host2.example.com",
			HostnameIndex:  1,
			ResolvedIPv4:   []string{"192.168.1.2"},
			ResolvedIPv6:   []string{"2001:db8::2"},
			IsOperational:  true,
			IfcicoResult: &models.ProtocolTestResult{
				Tested:  true,
				Success: true,
			},
		},
	}

	aggregator := NewTestAggregator()
	result := aggregator.CreateAggregatedResult(node, results)

	if result == nil {
		t.Fatal("Expected non-nil aggregated result")
	}

	if !result.IsAggregated {
		t.Error("Expected IsAggregated to be true")
	}

	if !result.IsOperational {
		t.Error("Expected IsOperational to be true")
	}

	// Should have all unique IPs from both hostnames
	if len(result.ResolvedIPv4) != 2 {
		t.Errorf("Expected 2 IPv4 addresses, got %d: %v", len(result.ResolvedIPv4), result.ResolvedIPv4)
	}

	if len(result.ResolvedIPv6) != 2 {
		t.Errorf("Expected 2 IPv6 addresses, got %d: %v", len(result.ResolvedIPv6), result.ResolvedIPv6)
	}

	// Both protocols should succeed
	if result.BinkPResult == nil || !result.BinkPResult.Success {
		t.Error("Expected successful BinkP result")
	}

	if result.IfcicoResult == nil || !result.IfcicoResult.Success {
		t.Error("Expected successful Ifcico result")
	}

	// Hostname counts
	if result.TotalHostnames != 2 {
		t.Errorf("Expected TotalHostnames 2, got %d", result.TotalHostnames)
	}

	if result.HostnamesTested != 2 {
		t.Errorf("Expected HostnamesTested 2, got %d", result.HostnamesTested)
	}

	if result.HostnamesOperational != 2 {
		t.Errorf("Expected HostnamesOperational 2, got %d", result.HostnamesOperational)
	}
}

func TestCreateAggregatedResult_MultipleHostnames_PartialSuccess(t *testing.T) {
	node := &models.Node{
		Zone:              2,
		Net:               450,
		Node:              1024,
		InternetHostnames: []string{"host1.example.com", "host2.example.com", "host3.example.com"},
		InternetProtocols: []string{"IBN"},
	}

	results := []*models.TestResult{
		{
			Zone:           2,
			Net:            450,
			Node:           1024,
			TestedHostname: "host1.example.com",
			HostnameIndex:  0,
			ResolvedIPv4:   []string{"192.168.1.1"},
			IsOperational:  true,
			BinkPResult: &models.ProtocolTestResult{
				Tested:  true,
				Success: true,
			},
		},
		{
			Zone:           2,
			Net:            450,
			Node:           1024,
			TestedHostname: "host2.example.com",
			HostnameIndex:  1,
			DNSError:       "DNS resolution failed",
			IsOperational:  false,
		},
		{
			Zone:           2,
			Net:            450,
			Node:           1024,
			TestedHostname: "host3.example.com",
			HostnameIndex:  2,
			ResolvedIPv4:   []string{"192.168.1.3"},
			IsOperational:  false,
			BinkPResult: &models.ProtocolTestResult{
				Tested:  true,
				Success: false,
				Error:   "Connection timeout",
			},
		},
	}

	aggregator := NewTestAggregator()
	result := aggregator.CreateAggregatedResult(node, results)

	if result == nil {
		t.Fatal("Expected non-nil aggregated result")
	}

	// Should be operational because at least one hostname succeeded
	if !result.IsOperational {
		t.Error("Expected IsOperational to be true (one hostname succeeded)")
	}

	// Should aggregate successful BinkP from host1
	if result.BinkPResult == nil || !result.BinkPResult.Success {
		t.Error("Expected successful BinkP result from host1")
	}

	// Should have IPs from host1 and host3 (host2 failed DNS)
	if len(result.ResolvedIPv4) != 2 {
		t.Errorf("Expected 2 IPv4 addresses, got %d: %v", len(result.ResolvedIPv4), result.ResolvedIPv4)
	}

	// Hostname counts
	if result.TotalHostnames != 3 {
		t.Errorf("Expected TotalHostnames 3, got %d", result.TotalHostnames)
	}

	if result.HostnamesTested != 3 {
		t.Errorf("Expected HostnamesTested 3, got %d", result.HostnamesTested)
	}

	if result.HostnamesOperational != 1 {
		t.Errorf("Expected HostnamesOperational 1, got %d", result.HostnamesOperational)
	}

	// DNS error should be cleared since at least one hostname succeeded
	if result.DNSError != "" {
		t.Errorf("Expected no DNS error, got %s", result.DNSError)
	}
}

func TestCreateAggregatedResult_AllHostnamesFail(t *testing.T) {
	node := &models.Node{
		Zone:              2,
		Net:               450,
		Node:              1024,
		InternetHostnames: []string{"host1.example.com", "host2.example.com"},
		InternetProtocols: []string{"IBN"},
	}

	results := []*models.TestResult{
		{
			Zone:           2,
			Net:            450,
			Node:           1024,
			TestedHostname: "host1.example.com",
			HostnameIndex:  0,
			DNSError:       "DNS resolution failed",
			IsOperational:  false,
		},
		{
			Zone:           2,
			Net:            450,
			Node:           1024,
			TestedHostname: "host2.example.com",
			HostnameIndex:  1,
			DNSError:       "DNS resolution failed",
			IsOperational:  false,
		},
	}

	aggregator := NewTestAggregator()
	result := aggregator.CreateAggregatedResult(node, results)

	if result == nil {
		t.Fatal("Expected non-nil aggregated result")
	}

	if result.IsOperational {
		t.Error("Expected IsOperational to be false (all hostnames failed)")
	}

	if result.DNSError == "" {
		t.Error("Expected DNS error to be set")
	}

	if result.BinkPResult == nil {
		t.Error("Expected BinkP result to be set (as failed)")
	} else if result.BinkPResult.Success {
		t.Error("Expected BinkP result to be failed")
	}

	if result.HostnamesOperational != 0 {
		t.Errorf("Expected HostnamesOperational 0, got %d", result.HostnamesOperational)
	}
}

func TestCreateAggregatedResult_DuplicateIPs(t *testing.T) {
	node := &models.Node{
		Zone:              2,
		Net:               450,
		Node:              1024,
		InternetHostnames: []string{"host1.example.com", "host2.example.com"},
	}

	// Both hostnames resolve to the same IPs
	results := []*models.TestResult{
		{
			Zone:           2,
			Net:            450,
			Node:           1024,
			TestedHostname: "host1.example.com",
			ResolvedIPv4:   []string{"192.168.1.1", "192.168.1.2"},
			ResolvedIPv6:   []string{"2001:db8::1"},
			IsOperational:  true,
		},
		{
			Zone:           2,
			Net:            450,
			Node:           1024,
			TestedHostname: "host2.example.com",
			ResolvedIPv4:   []string{"192.168.1.1", "192.168.1.3"}, // 192.168.1.1 is duplicate
			ResolvedIPv6:   []string{"2001:db8::1"},                 // Duplicate IPv6
			IsOperational:  true,
		},
	}

	aggregator := NewTestAggregator()
	result := aggregator.CreateAggregatedResult(node, results)

	if result == nil {
		t.Fatal("Expected non-nil aggregated result")
	}

	// Should deduplicate IPs
	expectedIPv4Count := 3 // 192.168.1.1, 192.168.1.2, 192.168.1.3
	if len(result.ResolvedIPv4) != expectedIPv4Count {
		t.Errorf("Expected %d unique IPv4 addresses, got %d: %v", expectedIPv4Count, len(result.ResolvedIPv4), result.ResolvedIPv4)
	}

	expectedIPv6Count := 1 // 2001:db8::1 (deduplicated)
	if len(result.ResolvedIPv6) != expectedIPv6Count {
		t.Errorf("Expected %d unique IPv6 address, got %d: %v", expectedIPv6Count, len(result.ResolvedIPv6), result.ResolvedIPv6)
	}
}

func TestCreateAggregatedResult_GeolocationPriority(t *testing.T) {
	node := &models.Node{
		Zone:              2,
		Net:               450,
		Node:              1024,
		InternetHostnames: []string{"host1.example.com", "host2.example.com"},
	}

	results := []*models.TestResult{
		{
			Zone:           2,
			Net:            450,
			Node:           1024,
			TestedHostname: "host1.example.com",
			ResolvedIPv4:   []string{"192.168.1.1"},
			IsOperational:  false,
			// No geolocation for host1
		},
		{
			Zone:           2,
			Net:            450,
			Node:           1024,
			TestedHostname: "host2.example.com",
			ResolvedIPv4:   []string{"192.168.1.2"},
			IsOperational:  true,
			Country:        "DE",
			CountryCode:    "DE",
			City:           "Berlin",
			Region:         "Berlin",
			Latitude:       52.52,
			Longitude:      13.405,
		},
	}

	aggregator := NewTestAggregator()
	result := aggregator.CreateAggregatedResult(node, results)

	if result == nil {
		t.Fatal("Expected non-nil aggregated result")
	}

	// Should use geolocation from first result that has it
	if result.Country != "DE" {
		t.Errorf("Expected Country DE, got %s", result.Country)
	}

	if result.City != "Berlin" {
		t.Errorf("Expected City Berlin, got %s", result.City)
	}

	if result.Latitude != 52.52 {
		t.Errorf("Expected Latitude 52.52, got %f", result.Latitude)
	}
}

func TestCreateAggregatedResult_ConnectivityIssues(t *testing.T) {
	node := &models.Node{
		Zone:              2,
		Net:               450,
		Node:              1024,
		InternetHostnames: []string{"host1.example.com"},
		InternetProtocols: []string{"IBN"},
	}

	// DNS succeeds but protocol fails
	results := []*models.TestResult{
		{
			Zone:           2,
			Net:            450,
			Node:           1024,
			TestedHostname: "host1.example.com",
			ResolvedIPv4:   []string{"192.168.1.1"},
			IsOperational:  false,
			BinkPResult: &models.ProtocolTestResult{
				Tested:  true,
				Success: false,
				Error:   "Connection refused",
			},
		},
	}

	aggregator := NewTestAggregator()
	result := aggregator.CreateAggregatedResult(node, results)

	if result == nil {
		t.Fatal("Expected non-nil aggregated result")
	}

	if result.IsOperational {
		t.Error("Expected IsOperational to be false")
	}

	if !result.HasConnectivityIssues {
		t.Error("Expected HasConnectivityIssues to be true (DNS ok but protocol failed)")
	}
}

func TestCreateAggregatedResult_NilResults(t *testing.T) {
	node := &models.Node{
		Zone:              2,
		Net:               450,
		Node:              1024,
		InternetHostnames: []string{"host1.example.com", "host2.example.com"},
	}

	// Include nil results to test robustness
	results := []*models.TestResult{
		{
			Zone:           2,
			Net:            450,
			Node:           1024,
			TestedHostname: "host1.example.com",
			ResolvedIPv4:   []string{"192.168.1.1"},
			IsOperational:  true,
		},
		nil, // Nil result should be skipped
		{
			Zone:           2,
			Net:            450,
			Node:           1024,
			TestedHostname: "host2.example.com",
			ResolvedIPv4:   []string{"192.168.1.2"},
			IsOperational:  true,
		},
	}

	aggregator := NewTestAggregator()
	result := aggregator.CreateAggregatedResult(node, results)

	if result == nil {
		t.Fatal("Expected non-nil aggregated result")
	}

	// Should aggregate successfully despite nil result
	if len(result.ResolvedIPv4) != 2 {
		t.Errorf("Expected 2 IPv4 addresses, got %d", len(result.ResolvedIPv4))
	}

	if !result.IsOperational {
		t.Error("Expected IsOperational to be true")
	}
}

func TestCreateAggregatedResult_MultipleProtocols(t *testing.T) {
	node := &models.Node{
		Zone:              2,
		Net:               450,
		Node:              1024,
		InternetHostnames: []string{"host1.example.com", "host2.example.com"},
		InternetProtocols: []string{"IBN", "IFC", "ITN"},
	}

	results := []*models.TestResult{
		{
			Zone:           2,
			Net:            450,
			Node:           1024,
			TestedHostname: "host1.example.com",
			ResolvedIPv4:   []string{"192.168.1.1"},
			IsOperational:  true,
			BinkPResult: &models.ProtocolTestResult{
				Tested:  true,
				Success: true,
			},
			IfcicoResult: &models.ProtocolTestResult{
				Tested:  true,
				Success: false,
				Error:   "Timeout",
			},
		},
		{
			Zone:           2,
			Net:            450,
			Node:           1024,
			TestedHostname: "host2.example.com",
			ResolvedIPv4:   []string{"192.168.1.2"},
			IsOperational:  true,
			IfcicoResult: &models.ProtocolTestResult{
				Tested:  true,
				Success: true,
			},
			TelnetResult: &models.ProtocolTestResult{
				Tested:  true,
				Success: true,
			},
		},
	}

	aggregator := NewTestAggregator()
	result := aggregator.CreateAggregatedResult(node, results)

	if result == nil {
		t.Fatal("Expected non-nil aggregated result")
	}

	// BinkP succeeded on host1
	if result.BinkPResult == nil || !result.BinkPResult.Success {
		t.Error("Expected successful BinkP result")
	}

	// Ifcico succeeded on host2 (should override failure from host1)
	if result.IfcicoResult == nil || !result.IfcicoResult.Success {
		t.Error("Expected successful Ifcico result")
	}

	// Telnet succeeded on host2
	if result.TelnetResult == nil || !result.TelnetResult.Success {
		t.Error("Expected successful Telnet result")
	}
}
