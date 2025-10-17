package daemon

import (
	"testing"

	"github.com/nodelistdb/internal/testing/models"
)

func createTestNodes() []*models.Node {
	nodes := []*models.Node{
		{Zone: 1, Net: 100, Node: 1, InternetHostnames: []string{"host1.example.com"}},
		{Zone: 1, Net: 100, Node: 2, InternetHostnames: []string{"host2.example.com"}},
		{Zone: 2, Net: 450, Node: 1024, InternetHostnames: []string{"host3.example.com", "host4.example.com"}},
		{Zone: 2, Net: 450, Node: 2000, InternetHostnames: []string{"host5.example.com"}},
		{Zone: 2, Net: 5001, Node: 100, InternetHostnames: []string{"host6.example.com"}},
	}

	// Set protocols for some nodes
	nodes[0].InternetProtocols = []string{"IBN"}       // BinkP
	nodes[1].InternetProtocols = []string{"IFC"}       // EMSI/Ifcico
	nodes[2].InternetProtocols = []string{"IBN", "IFC"} // Both
	nodes[3].InternetProtocols = []string{"ITN"}       // Telnet
	nodes[4].InternetProtocols = []string{"IBN"}       // BinkP

	// Set some flags
	nodes[0].Flags = []string{"CM", "XA"} // Continuous Mail
	nodes[1].Flags = []string{"MO"}
	nodes[2].Flags = []string{"CM", "LO"}

	return nodes
}

func TestFilterByTestLimit_All(t *testing.T) {
	filter := NewNodeFilter()
	nodes := createTestNodes()

	result := filter.FilterByTestLimit(nodes, "all")
	if len(result) != len(nodes) {
		t.Errorf("Expected %d nodes, got %d", len(nodes), len(result))
	}

	result = filter.FilterByTestLimit(nodes, "")
	if len(result) != len(nodes) {
		t.Errorf("Expected %d nodes for empty limit, got %d", len(nodes), len(result))
	}
}

func TestFilterByTestLimit_SpecificNode(t *testing.T) {
	filter := NewNodeFilter()
	nodes := createTestNodes()

	result := filter.FilterByTestLimit(nodes, "2:5001/100")
	if len(result) != 1 {
		t.Fatalf("Expected 1 node, got %d", len(result))
	}

	if result[0].Zone != 2 || result[0].Net != 5001 || result[0].Node != 100 {
		t.Errorf("Expected node 2:5001/100, got %d:%d/%d", result[0].Zone, result[0].Net, result[0].Node)
	}
}

func TestFilterByTestLimit_SpecificNode_NotFound(t *testing.T) {
	filter := NewNodeFilter()
	nodes := createTestNodes()

	result := filter.FilterByTestLimit(nodes, "99:99/99")
	if len(result) != 0 {
		t.Errorf("Expected 0 nodes for non-existent address, got %d", len(result))
	}
}

func TestFilterByTestLimit_Percentage(t *testing.T) {
	filter := NewNodeFilter()
	nodes := createTestNodes()

	result := filter.FilterByTestLimit(nodes, "50%")
	expectedCount := len(nodes) * 50 / 100
	if len(result) != expectedCount {
		t.Errorf("Expected %d nodes (50%%), got %d", expectedCount, len(result))
	}

	result = filter.FilterByTestLimit(nodes, "100%")
	if len(result) != len(nodes) {
		t.Errorf("Expected %d nodes (100%%), got %d", len(nodes), len(result))
	}

	// Edge case: small percentage should still return at least 1
	result = filter.FilterByTestLimit(nodes, "1%")
	if len(result) < 1 {
		t.Error("Expected at least 1 node for small percentage")
	}
}

func TestFilterByTestLimit_NumericCount(t *testing.T) {
	filter := NewNodeFilter()
	nodes := createTestNodes()

	result := filter.FilterByTestLimit(nodes, "3")
	if len(result) != 3 {
		t.Errorf("Expected 3 nodes, got %d", len(result))
	}

	result = filter.FilterByTestLimit(nodes, "10")
	if len(result) != len(nodes) {
		t.Errorf("Expected %d nodes (requested more than available), got %d", len(nodes), len(result))
	}

	result = filter.FilterByTestLimit(nodes, "0")
	if len(result) != 0 {
		t.Errorf("Expected 0 nodes, got %d", len(result))
	}

	result = filter.FilterByTestLimit(nodes, "-5")
	if len(result) != 0 {
		t.Errorf("Expected 0 nodes for negative count, got %d", len(result))
	}
}

func TestFilterByTestLimit_Zone(t *testing.T) {
	filter := NewNodeFilter()
	nodes := createTestNodes()

	result := filter.FilterByTestLimit(nodes, "zone:1")
	if len(result) != 2 {
		t.Errorf("Expected 2 nodes in zone 1, got %d", len(result))
	}

	for _, node := range result {
		if node.Zone != 1 {
			t.Errorf("Expected zone 1, got zone %d", node.Zone)
		}
	}

	result = filter.FilterByTestLimit(nodes, "zone:2")
	if len(result) != 3 {
		t.Errorf("Expected 3 nodes in zone 2, got %d", len(result))
	}
}

func TestFilterByTestLimit_Network(t *testing.T) {
	filter := NewNodeFilter()
	nodes := createTestNodes()

	result := filter.FilterByTestLimit(nodes, "net:2:450")
	if len(result) != 2 {
		t.Errorf("Expected 2 nodes in network 2:450, got %d", len(result))
	}

	for _, node := range result {
		if node.Zone != 2 || node.Net != 450 {
			t.Errorf("Expected network 2:450, got %d:%d", node.Zone, node.Net)
		}
	}

	result = filter.FilterByTestLimit(nodes, "net:1:100")
	if len(result) != 2 {
		t.Errorf("Expected 2 nodes in network 1:100, got %d", len(result))
	}
}

func TestFilterByTestLimit_BinkP(t *testing.T) {
	filter := NewNodeFilter()
	nodes := createTestNodes()

	result := filter.FilterByTestLimit(nodes, "binkp")
	// HasBinkP() returns true for IBN or IFC protocols
	// nodes[0]=IBN, nodes[1]=IFC, nodes[2]=IBN+IFC, nodes[4]=IBN = 4 nodes
	if len(result) != 4 {
		t.Errorf("Expected 4 nodes with BinkP, got %d", len(result))
	}

	for _, node := range result {
		if !node.HasBinkP() {
			t.Errorf("Node %d:%d/%d should have BinkP", node.Zone, node.Net, node.Node)
		}
	}
}

func TestFilterByTestLimit_EMSI(t *testing.T) {
	filter := NewNodeFilter()
	nodes := createTestNodes()

	result := filter.FilterByTestLimit(nodes, "emsi")
	// HasIFCICO() returns true for IFC or ITN protocols
	// nodes[1]=IFC, nodes[2]=IFC, nodes[3]=ITN = 3 nodes
	if len(result) != 3 {
		t.Errorf("Expected 3 nodes with EMSI, got %d", len(result))
	}

	for _, node := range result {
		if !node.HasIFCICO() {
			t.Errorf("Node %d:%d/%d should have IFCICO", node.Zone, node.Net, node.Node)
		}
	}
}

func TestFilterByTestLimit_Telnet(t *testing.T) {
	filter := NewNodeFilter()
	nodes := createTestNodes()

	result := filter.FilterByTestLimit(nodes, "telnet")
	if len(result) != 1 {
		t.Errorf("Expected 1 node with Telnet, got %d", len(result))
	}

	if len(result) > 0 && !result[0].HasTEL() {
		t.Error("Filtered node should have Telnet")
	}
}

func TestFilterByTestLimit_MultiHostname(t *testing.T) {
	filter := NewNodeFilter()
	nodes := createTestNodes()

	result := filter.FilterByTestLimit(nodes, "multi")
	if len(result) != 1 {
		t.Errorf("Expected 1 node with multiple hostnames, got %d", len(result))
	}

	if len(result) > 0 && len(result[0].InternetHostnames) <= 1 {
		t.Error("Filtered node should have multiple hostnames")
	}

	// Alternative form
	result = filter.FilterByTestLimit(nodes, "multihostname")
	if len(result) != 1 {
		t.Errorf("Expected 1 node with multiple hostnames, got %d", len(result))
	}
}

func TestFilterByTestLimit_UnknownFilter(t *testing.T) {
	filter := NewNodeFilter()
	nodes := createTestNodes()

	// Unknown filter should return all nodes with a warning
	result := filter.FilterByTestLimit(nodes, "unknownfilter")
	if len(result) != len(nodes) {
		t.Errorf("Expected %d nodes for unknown filter, got %d", len(nodes), len(result))
	}
}

func TestFilterByZone(t *testing.T) {
	filter := NewNodeFilter()
	nodes := createTestNodes()

	result := filter.filterByZone(nodes, 1)
	if len(result) != 2 {
		t.Errorf("Expected 2 nodes in zone 1, got %d", len(result))
	}

	for _, node := range result {
		if node.Zone != 1 {
			t.Errorf("Expected zone 1, got %d", node.Zone)
		}
	}

	result = filter.filterByZone(nodes, 99)
	if len(result) != 0 {
		t.Errorf("Expected 0 nodes in non-existent zone, got %d", len(result))
	}
}

func TestFilterByNetwork(t *testing.T) {
	filter := NewNodeFilter()
	nodes := createTestNodes()

	result := filter.filterByNetwork(nodes, 2, 450)
	if len(result) != 2 {
		t.Errorf("Expected 2 nodes in network 2:450, got %d", len(result))
	}

	for _, node := range result {
		if node.Zone != 2 || node.Net != 450 {
			t.Errorf("Expected network 2:450, got %d:%d", node.Zone, node.Net)
		}
	}
}

func TestFilterHubs(t *testing.T) {
	filter := NewNodeFilter()
	nodes := []*models.Node{
		{Zone: 1, Net: 100, Node: 0},  // Hub (node 0)
		{Zone: 1, Net: 100, Node: 1},
		{Zone: 2, Net: 450, Node: 0},  // Hub (node 0)
		{Zone: 2, Net: 450, Node: 100},
	}

	result := filter.filterHubs(nodes)
	if len(result) != 2 {
		t.Errorf("Expected 2 hub nodes, got %d", len(result))
	}

	for _, node := range result {
		if node.Node != 0 {
			t.Errorf("Hub node should have node number 0, got %d", node.Node)
		}
	}
}

func TestFilterMultiHostname(t *testing.T) {
	filter := NewNodeFilter()
	nodes := []*models.Node{
		{Zone: 1, Net: 100, Node: 1, InternetHostnames: []string{"host1.example.com"}},
		{Zone: 1, Net: 100, Node: 2, InternetHostnames: []string{"host1.example.com", "host2.example.com"}},
		{Zone: 2, Net: 450, Node: 1, InternetHostnames: []string{"host1.example.com", "host2.example.com", "host3.example.com"}},
		{Zone: 2, Net: 450, Node: 2}, // No hostnames
	}

	result := filter.filterMultiHostname(nodes)
	if len(result) != 2 {
		t.Errorf("Expected 2 nodes with multiple hostnames, got %d", len(result))
	}

	for _, node := range result {
		if len(node.InternetHostnames) <= 1 {
			t.Errorf("Node should have multiple hostnames, got %d", len(node.InternetHostnames))
		}
	}
}

func TestSelectRandom(t *testing.T) {
	filter := NewNodeFilter()
	nodes := createTestNodes()

	// Request fewer than available
	result := filter.selectRandom(nodes, 3)
	if len(result) != 3 {
		t.Errorf("Expected 3 random nodes, got %d", len(result))
	}

	// Verify we got actual nodes from the original set
	for _, node := range result {
		found := false
		for _, original := range nodes {
			if node.Zone == original.Zone && node.Net == original.Net && node.Node == original.Node {
				found = true
				break
			}
		}
		if !found {
			t.Error("Selected node not found in original set")
		}
	}

	// Request more than available
	result = filter.selectRandom(nodes, 100)
	if len(result) != len(nodes) {
		t.Errorf("Expected %d nodes (all available), got %d", len(nodes), len(result))
	}

	// Request exactly the available count
	result = filter.selectRandom(nodes, len(nodes))
	if len(result) != len(nodes) {
		t.Errorf("Expected %d nodes, got %d", len(nodes), len(result))
	}
}

func TestSelectRandom_NoModifyOriginal(t *testing.T) {
	filter := NewNodeFilter()
	nodes := createTestNodes()
	originalLen := len(nodes)
	originalFirst := nodes[0]

	// Select random subset
	filter.selectRandom(nodes, 2)

	// Original slice should not be modified
	if len(nodes) != originalLen {
		t.Errorf("Original slice length changed from %d to %d", originalLen, len(nodes))
	}

	if nodes[0] != originalFirst {
		t.Error("Original slice was modified")
	}
}

func TestFilterByCapabilities_24h(t *testing.T) {
	filter := NewNodeFilter()
	nodes := createTestNodes()

	result := filter.FilterByCapabilities(nodes, []string{"24h"})

	// Should only return nodes with CM flag
	for _, node := range result {
		hasCM := false
		for _, flag := range node.Flags {
			if flag == "CM" {
				hasCM = true
				break
			}
		}
		if !hasCM {
			t.Errorf("Node %d:%d/%d should have CM flag", node.Zone, node.Net, node.Node)
		}
	}
}

func TestFilterByCapabilities_Secure(t *testing.T) {
	filter := NewNodeFilter()
	nodes := createTestNodes()

	result := filter.FilterByCapabilities(nodes, []string{"secure"})

	// Should return nodes with IBN or IFC protocols
	for _, node := range result {
		if !node.HasBinkP() && !node.HasIFCICO() {
			t.Errorf("Node %d:%d/%d should have BinkP or IFCICO", node.Zone, node.Net, node.Node)
		}
	}
}

func TestFilterByCapabilities_Empty(t *testing.T) {
	filter := NewNodeFilter()
	nodes := createTestNodes()

	result := filter.FilterByCapabilities(nodes, []string{})
	if len(result) != len(nodes) {
		t.Errorf("Expected %d nodes for empty capabilities, got %d", len(nodes), len(result))
	}
}

func TestFilterByCapabilities_Multiple(t *testing.T) {
	filter := NewNodeFilter()
	nodes := createTestNodes()

	result := filter.FilterByCapabilities(nodes, []string{"24h", "secure"})

	// Should return nodes with both CM flag AND secure protocols
	for _, node := range result {
		hasCM := false
		for _, flag := range node.Flags {
			if flag == "CM" {
				hasCM = true
				break
			}
		}
		hasSecure := node.HasBinkP() || node.HasIFCICO()

		if !hasCM || !hasSecure {
			t.Errorf("Node %d:%d/%d should have both CM flag and secure protocols", node.Zone, node.Net, node.Node)
		}
	}
}

func TestFilterByProtocol(t *testing.T) {
	filter := NewNodeFilter()
	nodes := createTestNodes()

	hasIBN := func(n *models.Node) bool { return n.HasBinkP() }
	result := filter.filterByProtocol(nodes, hasIBN)

	// HasBinkP() returns true for IBN or IFC protocols
	// nodes[0]=IBN, nodes[1]=IFC, nodes[2]=IBN+IFC, nodes[4]=IBN = 4 nodes
	if len(result) != 4 {
		t.Errorf("Expected 4 nodes with BinkP, got %d", len(result))
	}

	for _, node := range result {
		if !node.HasBinkP() {
			t.Errorf("Node %d:%d/%d should have BinkP", node.Zone, node.Net, node.Node)
		}
	}
}

func TestNewNodeFilter(t *testing.T) {
	filter := NewNodeFilter()
	if filter == nil {
		t.Fatal("Expected non-nil filter")
	}

	if filter.random == nil {
		t.Error("Expected random number generator to be initialized")
	}
}
