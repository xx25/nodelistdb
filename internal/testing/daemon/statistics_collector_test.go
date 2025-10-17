package daemon

import (
	"testing"

	"github.com/nodelistdb/internal/testing/models"
)

func TestCalculateStatistics_EmptyResults(t *testing.T) {
	sc := NewStatisticsCollector()
	stats := sc.CalculateStatistics([]*models.TestResult{})

	if stats.TotalNodesTested != 0 {
		t.Errorf("Expected 0 total nodes, got %d", stats.TotalNodesTested)
	}

	if stats.NodesOperational != 0 {
		t.Errorf("Expected 0 operational nodes, got %d", stats.NodesOperational)
	}
}

func TestCalculateStatistics_NilResults(t *testing.T) {
	sc := NewStatisticsCollector()

	results := []*models.TestResult{
		{Zone: 1, Net: 1, Node: 1, IsOperational: true},
		nil, // Should be skipped
		{Zone: 1, Net: 1, Node: 2, IsOperational: false},
	}

	stats := sc.CalculateStatistics(results)

	// Should count non-nil results
	if stats.TotalNodesTested != 3 {
		t.Errorf("Expected 3 total nodes (including nil), got %d", stats.TotalNodesTested)
	}

	if stats.NodesOperational != 1 {
		t.Errorf("Expected 1 operational node, got %d", stats.NodesOperational)
	}
}

func TestCalculateStatistics_OperationalCounts(t *testing.T) {
	sc := NewStatisticsCollector()

	results := []*models.TestResult{
		{Zone: 1, Net: 1, Node: 1, IsOperational: true},
		{Zone: 1, Net: 1, Node: 2, IsOperational: true},
		{Zone: 1, Net: 1, Node: 3, IsOperational: false},
		{Zone: 1, Net: 1, Node: 4, IsOperational: false},
		{Zone: 1, Net: 1, Node: 5, IsOperational: false},
	}

	stats := sc.CalculateStatistics(results)

	if stats.TotalNodesTested != 5 {
		t.Errorf("Expected 5 total nodes, got %d", stats.TotalNodesTested)
	}

	if stats.NodesOperational != 2 {
		t.Errorf("Expected 2 operational nodes, got %d", stats.NodesOperational)
	}

	if stats.NodesWithIssues != 3 {
		t.Errorf("Expected 3 nodes with issues, got %d", stats.NodesWithIssues)
	}
}

func TestCalculateStatistics_DNSFailures(t *testing.T) {
	sc := NewStatisticsCollector()

	results := []*models.TestResult{
		{Zone: 1, Net: 1, Node: 1, DNSError: ""},
		{Zone: 1, Net: 1, Node: 2, DNSError: "DNS resolution failed"},
		{Zone: 1, Net: 1, Node: 3, DNSError: "Timeout"},
	}

	stats := sc.CalculateStatistics(results)

	if stats.NodesDNSFailed != 2 {
		t.Errorf("Expected 2 DNS failures, got %d", stats.NodesDNSFailed)
	}

	if stats.ErrorTypes["DNS_FAIL"] != 2 {
		t.Errorf("Expected 2 DNS_FAIL errors, got %d", stats.ErrorTypes["DNS_FAIL"])
	}
}

func TestCalculateStatistics_GeographicData(t *testing.T) {
	sc := NewStatisticsCollector()

	results := []*models.TestResult{
		{Zone: 1, Net: 1, Node: 1, Country: "US", ISP: "ISP1"},
		{Zone: 1, Net: 1, Node: 2, Country: "US", ISP: "ISP1"},
		{Zone: 1, Net: 1, Node: 3, Country: "DE", ISP: "ISP2"},
		{Zone: 1, Net: 1, Node: 4, Country: "DE", ISP: "ISP2"},
		{Zone: 1, Net: 1, Node: 5, Country: "FR", ISP: "ISP3"},
	}

	stats := sc.CalculateStatistics(results)

	if stats.Countries["US"] != 2 {
		t.Errorf("Expected 2 US nodes, got %d", stats.Countries["US"])
	}

	if stats.Countries["DE"] != 2 {
		t.Errorf("Expected 2 DE nodes, got %d", stats.Countries["DE"])
	}

	if stats.Countries["FR"] != 1 {
		t.Errorf("Expected 1 FR node, got %d", stats.Countries["FR"])
	}

	if stats.ISPs["ISP1"] != 2 {
		t.Errorf("Expected 2 ISP1 nodes, got %d", stats.ISPs["ISP1"])
	}
}

func TestCalculateStatistics_BinkPProtocol(t *testing.T) {
	sc := NewStatisticsCollector()

	results := []*models.TestResult{
		{
			Zone: 1, Net: 1, Node: 1,
			BinkPResult: &models.ProtocolTestResult{
				Tested:     true,
				Success:    true,
				ResponseMs: 100,
			},
		},
		{
			Zone: 1, Net: 1, Node: 2,
			BinkPResult: &models.ProtocolTestResult{
				Tested:     true,
				Success:    true,
				ResponseMs: 200,
			},
		},
		{
			Zone: 1, Net: 1, Node: 3,
			BinkPResult: &models.ProtocolTestResult{
				Tested:  true,
				Success: false,
				Error:   "Connection timeout",
			},
		},
	}

	stats := sc.CalculateStatistics(results)

	if stats.NodesWithBinkP != 3 {
		t.Errorf("Expected 3 nodes with BinkP, got %d", stats.NodesWithBinkP)
	}

	if stats.ProtocolStats["BinkP_Tested"] != 3 {
		t.Errorf("Expected 3 BinkP tested, got %d", stats.ProtocolStats["BinkP_Tested"])
	}

	if stats.ProtocolStats["BinkP_Success"] != 2 {
		t.Errorf("Expected 2 BinkP successes, got %d", stats.ProtocolStats["BinkP_Success"])
	}

	if stats.ProtocolStats["BinkP_Failed"] != 1 {
		t.Errorf("Expected 1 BinkP failure, got %d", stats.ProtocolStats["BinkP_Failed"])
	}

	// Average response time: (100 + 200) / 2 = 150
	if stats.AvgBinkPResponseMs != 150.0 {
		t.Errorf("Expected avg BinkP response 150ms, got %f", stats.AvgBinkPResponseMs)
	}
}

func TestCalculateStatistics_IfcicoProtocol(t *testing.T) {
	sc := NewStatisticsCollector()

	results := []*models.TestResult{
		{
			Zone: 1, Net: 1, Node: 1,
			IfcicoResult: &models.ProtocolTestResult{
				Tested:     true,
				Success:    true,
				ResponseMs: 50,
			},
		},
		{
			Zone: 1, Net: 1, Node: 2,
			IfcicoResult: &models.ProtocolTestResult{
				Tested:     true,
				Success:    true,
				ResponseMs: 150,
			},
		},
	}

	stats := sc.CalculateStatistics(results)

	if stats.NodesWithIfcico != 2 {
		t.Errorf("Expected 2 nodes with Ifcico, got %d", stats.NodesWithIfcico)
	}

	if stats.ProtocolStats["Ifcico_Success"] != 2 {
		t.Errorf("Expected 2 Ifcico successes, got %d", stats.ProtocolStats["Ifcico_Success"])
	}

	// Average: (50 + 150) / 2 = 100
	if stats.AvgIfcicoResponseMs != 100.0 {
		t.Errorf("Expected avg Ifcico response 100ms, got %f", stats.AvgIfcicoResponseMs)
	}
}

func TestCalculateStatistics_TelnetProtocol(t *testing.T) {
	sc := NewStatisticsCollector()

	results := []*models.TestResult{
		{
			Zone: 1, Net: 1, Node: 1,
			TelnetResult: &models.ProtocolTestResult{
				Tested:  true,
				Success: true,
			},
		},
		{
			Zone: 1, Net: 1, Node: 2,
			TelnetResult: &models.ProtocolTestResult{
				Tested:  true,
				Success: false,
			},
		},
	}

	stats := sc.CalculateStatistics(results)

	if stats.ProtocolStats["Telnet_Tested"] != 2 {
		t.Errorf("Expected 2 Telnet tested, got %d", stats.ProtocolStats["Telnet_Tested"])
	}

	if stats.ProtocolStats["Telnet_Success"] != 1 {
		t.Errorf("Expected 1 Telnet success, got %d", stats.ProtocolStats["Telnet_Success"])
	}

	if stats.ProtocolStats["Telnet_Failed"] != 1 {
		t.Errorf("Expected 1 Telnet failure, got %d", stats.ProtocolStats["Telnet_Failed"])
	}
}

func TestCalculateStatistics_FTPProtocol(t *testing.T) {
	sc := NewStatisticsCollector()

	results := []*models.TestResult{
		{
			Zone: 1, Net: 1, Node: 1,
			FTPResult: &models.ProtocolTestResult{
				Tested:  true,
				Success: true,
			},
		},
	}

	stats := sc.CalculateStatistics(results)

	if stats.ProtocolStats["FTP_Tested"] != 1 {
		t.Errorf("Expected 1 FTP tested, got %d", stats.ProtocolStats["FTP_Tested"])
	}

	if stats.ProtocolStats["FTP_Success"] != 1 {
		t.Errorf("Expected 1 FTP success, got %d", stats.ProtocolStats["FTP_Success"])
	}
}

func TestCalculateStatistics_VModemProtocol(t *testing.T) {
	sc := NewStatisticsCollector()

	results := []*models.TestResult{
		{
			Zone: 1, Net: 1, Node: 1,
			VModemResult: &models.ProtocolTestResult{
				Tested:  true,
				Success: false,
			},
		},
	}

	stats := sc.CalculateStatistics(results)

	if stats.ProtocolStats["VModem_Tested"] != 1 {
		t.Errorf("Expected 1 VModem tested, got %d", stats.ProtocolStats["VModem_Tested"])
	}

	if stats.ProtocolStats["VModem_Failed"] != 1 {
		t.Errorf("Expected 1 VModem failure, got %d", stats.ProtocolStats["VModem_Failed"])
	}
}

func TestCalculateStatistics_ProtocolErrors(t *testing.T) {
	sc := NewStatisticsCollector()

	results := []*models.TestResult{
		{
			Zone: 1, Net: 1, Node: 1,
			BinkPResult: &models.ProtocolTestResult{
				Tested:  true,
				Success: false,
				Error:   "Connection timeout",
			},
		},
		{
			Zone: 1, Net: 1, Node: 2,
			IfcicoResult: &models.ProtocolTestResult{
				Tested:  true,
				Success: false,
				Error:   "Connection refused",
			},
		},
	}

	stats := sc.CalculateStatistics(results)

	if stats.ErrorTypes["BinkP_Connection timeout"] != 1 {
		t.Errorf("Expected 1 BinkP timeout error, got %d", stats.ErrorTypes["BinkP_Connection timeout"])
	}

	if stats.ErrorTypes["Ifcico_Connection refused"] != 1 {
		t.Errorf("Expected 1 Ifcico refused error, got %d", stats.ErrorTypes["Ifcico_Connection refused"])
	}
}

func TestCalculateStatistics_MixedProtocols(t *testing.T) {
	sc := NewStatisticsCollector()

	results := []*models.TestResult{
		{
			Zone: 1, Net: 1, Node: 1,
			IsOperational: true,
			Country:       "US",
			ISP:           "ISP1",
			BinkPResult: &models.ProtocolTestResult{
				Tested:     true,
				Success:    true,
				ResponseMs: 100,
			},
			IfcicoResult: &models.ProtocolTestResult{
				Tested:     true,
				Success:    false,
				Error:      "Timeout",
				ResponseMs: 0,
			},
		},
	}

	stats := sc.CalculateStatistics(results)

	if stats.NodesOperational != 1 {
		t.Errorf("Expected 1 operational node, got %d", stats.NodesOperational)
	}

	if stats.Countries["US"] != 1 {
		t.Errorf("Expected 1 US node, got %d", stats.Countries["US"])
	}

	if stats.NodesWithBinkP != 1 {
		t.Errorf("Expected 1 BinkP node, got %d", stats.NodesWithBinkP)
	}

	if stats.NodesWithIfcico != 1 {
		t.Errorf("Expected 1 Ifcico node, got %d", stats.NodesWithIfcico)
	}
}

func TestCalculateStatistics_ResponseTimeNoData(t *testing.T) {
	sc := NewStatisticsCollector()

	results := []*models.TestResult{
		{
			Zone: 1, Net: 1, Node: 1,
			BinkPResult: &models.ProtocolTestResult{
				Tested:     true,
				Success:    false,
				ResponseMs: 0, // No response time for failures
			},
		},
	}

	stats := sc.CalculateStatistics(results)

	// Should not calculate average without valid response times
	if stats.AvgBinkPResponseMs != 0 {
		t.Errorf("Expected 0 avg response time with no valid responses, got %f", stats.AvgBinkPResponseMs)
	}
}

func TestCalculateStatistics_UntestedProtocols(t *testing.T) {
	sc := NewStatisticsCollector()

	results := []*models.TestResult{
		{
			Zone: 1, Net: 1, Node: 1,
			BinkPResult: &models.ProtocolTestResult{
				Tested:  false, // Not tested
				Success: false,
			},
		},
	}

	stats := sc.CalculateStatistics(results)

	// Untested protocols should not be counted
	if stats.NodesWithBinkP != 0 {
		t.Errorf("Expected 0 BinkP nodes for untested protocol, got %d", stats.NodesWithBinkP)
	}

	if stats.ProtocolStats["BinkP_Tested"] != 0 {
		t.Errorf("Expected 0 BinkP tested for untested protocol, got %d", stats.ProtocolStats["BinkP_Tested"])
	}
}

func TestCalculateStatistics_EmptyCountryAndISP(t *testing.T) {
	sc := NewStatisticsCollector()

	results := []*models.TestResult{
		{Zone: 1, Net: 1, Node: 1, Country: "", ISP: ""},
		{Zone: 1, Net: 1, Node: 2, Country: "", ISP: ""},
	}

	stats := sc.CalculateStatistics(results)

	// Empty country and ISP should not be counted
	if len(stats.Countries) != 0 {
		t.Errorf("Expected 0 countries, got %d", len(stats.Countries))
	}

	if len(stats.ISPs) != 0 {
		t.Errorf("Expected 0 ISPs, got %d", len(stats.ISPs))
	}
}

func TestNewStatisticsCollector(t *testing.T) {
	sc := NewStatisticsCollector()
	if sc == nil {
		t.Fatal("Expected non-nil StatisticsCollector")
	}
}
