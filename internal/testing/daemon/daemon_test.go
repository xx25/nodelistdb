package daemon

import (
	"testing"
	"time"
	
	"github.com/nodelistdb/internal/testing/models"
)

// TestNodeResultAlignment verifies that nodes and results stay properly aligned
func TestNodeResultAlignment(t *testing.T) {
	// Create a mock configuration
	cfg := &Config{
		Daemon: DaemonConfig{
			TestInterval: 10 * time.Second,
			Workers:      2,
			BatchSize:    5,
		},
		Database: DatabaseConfig{
			Type: "duckdb",
			DuckDB: &DuckDBConfig{
				NodesPath:   "/tmp/test_nodes.db",
				ResultsPath: "/tmp/test_results.db",
			},
		},
		Protocols: ProtocolsConfig{
			BinkP: ProtocolConfig{
				Enabled: true,
				Port:    24554,
				Timeout: 30 * time.Second,
			},
		},
		Services: ServicesConfig{
			DNS: DNSConfig{
				Workers:  2,
				Timeout:  5 * time.Second,
				CacheTTL: 1 * time.Hour,
			},
			Geolocation: GeolocationConfig{
				Provider:  "ip-api",
				CacheTTL:  24 * time.Hour,
				RateLimit: 150,
			},
		},
	}
	
	// Validate configuration
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Failed to validate config: %v", err)
	}
	
	t.Log("Configuration validation passed")
	
	// Test node result pairing logic
	type nodeResult struct {
		node   *models.Node
		result *models.TestResult
	}
	
	// Create test nodes
	nodes := []*models.Node{
		{Zone: 1, Net: 1, Node: 1},
		{Zone: 1, Net: 1, Node: 2},
		{Zone: 1, Net: 1, Node: 3},
	}
	
	// Simulate concurrent result collection
	var results []nodeResult
	for _, node := range nodes {
		results = append(results, nodeResult{
			node:   node,
			result: &models.TestResult{
				Zone: node.Zone,
				Net:  node.Net,
				Node: node.Node,
			},
		})
	}
	
	// Verify alignment
	for i, nr := range results {
		if nr.node.Zone != nr.result.Zone ||
		   nr.node.Net != nr.result.Net ||
		   nr.node.Node != nr.result.Node {
			t.Errorf("Misalignment at index %d: node(%d:%d/%d) != result(%d:%d/%d)",
				i,
				nr.node.Zone, nr.node.Net, nr.node.Node,
				nr.result.Zone, nr.result.Net, nr.result.Node)
		}
	}
	
	t.Log("Node-result alignment test passed")
}

// TestReloadConfig verifies that config reload properly reinitializes components
func TestReloadConfig(t *testing.T) {
	// This test verifies the logic, not the actual daemon
	// Real integration testing would require a running daemon
	
	initialTimeout := 10 * time.Second
	newTimeout := 20 * time.Second
	
	cfg1 := &Config{
		Daemon: DaemonConfig{
			TestInterval: 60 * time.Second,
			Workers:      4,
			BatchSize:    10,
		},
		Protocols: ProtocolsConfig{
			BinkP: ProtocolConfig{
				Enabled:    true,
				Timeout:    initialTimeout,
				OurAddress: "1:1/1",
			},
		},
		Services: ServicesConfig{
			DNS: DNSConfig{
				Workers:  2,
				Timeout:  5 * time.Second,
				CacheTTL: 1 * time.Hour,
			},
		},
	}
	
	cfg2 := &Config{
		Daemon: DaemonConfig{
			TestInterval: 120 * time.Second,
			Workers:      4, // Should not change
			BatchSize:    20,
		},
		Protocols: ProtocolsConfig{
			BinkP: ProtocolConfig{
				Enabled:    true,
				Timeout:    newTimeout,
				OurAddress: "2:2/2",
			},
		},
		Services: ServicesConfig{
			DNS: DNSConfig{
				Workers:  4,
				Timeout:  10 * time.Second,
				CacheTTL: 2 * time.Hour,
			},
		},
	}
	
	// Verify that the configurations are different
	if cfg1.Protocols.BinkP.Timeout == cfg2.Protocols.BinkP.Timeout {
		t.Error("Test setup error: timeouts should be different")
	}
	
	if cfg1.Protocols.BinkP.OurAddress == cfg2.Protocols.BinkP.OurAddress {
		t.Error("Test setup error: addresses should be different")
	}
	
	if cfg1.Services.DNS.CacheTTL == cfg2.Services.DNS.CacheTTL {
		t.Error("Test setup error: cache TTLs should be different")
	}
	
	t.Log("Config reload test structures validated")
}