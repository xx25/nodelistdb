package daemon

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/nodelistdb/internal/testing/logging"
	"github.com/nodelistdb/internal/testing/models"
)

// NodeFilter handles filtering of nodes based on various criteria
type NodeFilter struct {
	random *rand.Rand
}

// NewNodeFilter creates a new node filter
func NewNodeFilter() *NodeFilter {
	return &NodeFilter{
		random: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// FilterByTestLimit filters nodes based on a test limit specification
func (nf *NodeFilter) FilterByTestLimit(nodes []*models.Node, testLimit string) []*models.Node {
	if testLimit == "" || testLimit == "all" {
		return nodes
	}

	// Special case for specific node address format (e.g., "2:5001/100")
	var zone, net, node int
	if _, err := fmt.Sscanf(testLimit, "%d:%d/%d", &zone, &net, &node); err == nil {
		// Filter nodes to only include the specified one
		var filtered []*models.Node
		for _, n := range nodes {
			if n.Zone == zone && n.Net == net && n.Node == node {
				filtered = append(filtered, n)
				break // Only need one match
			}
		}
		return filtered
	}

	// Handle percentage limit (e.g., "10%")
	if strings.HasSuffix(testLimit, "%") {
		percentStr := strings.TrimSuffix(testLimit, "%")
		var percent float64
		_, err := fmt.Sscanf(percentStr, "%f", &percent)
		if err == nil {
			count := int(float64(len(nodes)) * percent / 100.0)
			if count < 1 {
				count = 1
			}
			return nf.selectRandom(nodes, count)
		}
	}

	// Handle numeric limit (e.g., "100")
	var count int
	if _, err := fmt.Sscanf(testLimit, "%d", &count); err == nil {
		if count <= 0 {
			return []*models.Node{}
		}
		if count >= len(nodes) {
			return nodes
		}
		return nf.selectRandom(nodes, count)
	}

	// Handle zone limit (e.g., "zone:2")
	if strings.HasPrefix(testLimit, "zone:") {
		zoneStr := strings.TrimPrefix(testLimit, "zone:")
		var zone uint16
		if _, err := fmt.Sscanf(zoneStr, "%d", &zone); err == nil {
			return nf.filterByZone(nodes, zone)
		}
	}

	// Handle network limit (e.g., "net:2:450")
	if strings.HasPrefix(testLimit, "net:") {
		netStr := strings.TrimPrefix(testLimit, "net:")
		parts := strings.Split(netStr, ":")
		if len(parts) == 2 {
			var zone, net uint16
			if _, err := fmt.Sscanf(parts[0], "%d", &zone); err == nil {
				if _, err := fmt.Sscanf(parts[1], "%d", &net); err == nil {
					return nf.filterByNetwork(nodes, zone, net)
				}
			}
		}
	}

	// Handle protocol-specific limits
	protocolFilters := map[string]func(*models.Node) bool{
		"binkp":  func(n *models.Node) bool { return n.HasBinkP() },
		"emsi":   func(n *models.Node) bool { return n.HasIFCICO() },
		"telnet": func(n *models.Node) bool { return n.HasTEL() },
		"ftp":    func(n *models.Node) bool { return n.FTPAddress() != "" },
		"vmodem": func(n *models.Node) bool { return false }, // Not implemented
	}

	if filterFunc, ok := protocolFilters[strings.ToLower(testLimit)]; ok {
		return nf.filterByProtocol(nodes, filterFunc)
	}

	// Handle online-only filter
	if testLimit == "online" {
		return nf.filterOnlineOnly(nodes)
	}

	// Handle hub/host filters
	if testLimit == "hubs" {
		return nf.filterHubs(nodes)
	}
	if testLimit == "hosts" {
		return nf.filterHosts(nodes)
	}

	// Handle multi-hostname filter
	if testLimit == "multi" || testLimit == "multihostname" {
		return nf.filterMultiHostname(nodes)
	}

	// Unknown filter, log warning and return all nodes
	logging.Warnf("Unknown test limit filter: %s", testLimit)
	return nodes
}

// selectRandom selects a random subset of nodes
func (nf *NodeFilter) selectRandom(nodes []*models.Node, count int) []*models.Node {
	if count >= len(nodes) {
		return nodes
	}

	// Create a copy of the nodes slice to avoid modifying the original
	nodesCopy := make([]*models.Node, len(nodes))
	copy(nodesCopy, nodes)

	// Fisher-Yates shuffle
	for i := len(nodesCopy) - 1; i > 0; i-- {
		j := nf.random.Intn(i + 1)
		nodesCopy[i], nodesCopy[j] = nodesCopy[j], nodesCopy[i]
	}

	return nodesCopy[:count]
}

// filterByZone filters nodes by zone number
func (nf *NodeFilter) filterByZone(nodes []*models.Node, zone uint16) []*models.Node {
	filtered := make([]*models.Node, 0)
	for _, node := range nodes {
		if uint16(node.Zone) == zone {
			filtered = append(filtered, node)
		}
	}
	return filtered
}

// filterByNetwork filters nodes by zone and network
func (nf *NodeFilter) filterByNetwork(nodes []*models.Node, zone, net uint16) []*models.Node {
	filtered := make([]*models.Node, 0)
	for _, node := range nodes {
		if uint16(node.Zone) == zone && uint16(node.Net) == net {
			filtered = append(filtered, node)
		}
	}
	return filtered
}

// filterByProtocol filters nodes that support a specific protocol
func (nf *NodeFilter) filterByProtocol(nodes []*models.Node, hasProtocol func(*models.Node) bool) []*models.Node {
	filtered := make([]*models.Node, 0)
	for _, node := range nodes {
		if hasProtocol(node) {
			filtered = append(filtered, node)
		}
	}
	return filtered
}

// filterOnlineOnly filters only nodes marked as online in the nodelist
func (nf *NodeFilter) filterOnlineOnly(nodes []*models.Node) []*models.Node {
	filtered := make([]*models.Node, 0)
	for _, node := range nodes {
		if node.IsOnline() {
			filtered = append(filtered, node)
		}
	}
	return filtered
}

// filterHubs filters only hub nodes (node number 0)
func (nf *NodeFilter) filterHubs(nodes []*models.Node) []*models.Node {
	filtered := make([]*models.Node, 0)
	for _, node := range nodes {
		if node.Node == 0 {
			filtered = append(filtered, node)
		}
	}
	return filtered
}

// filterHosts filters only host nodes (network coordinator nodes)
func (nf *NodeFilter) filterHosts(nodes []*models.Node) []*models.Node {
	filtered := make([]*models.Node, 0)
	for _, node := range nodes {
		// In FidoNet, hosts are typically nodes where Zone:Net/0
		// or nodes with Host flag
		if node.Node == 0 || node.IsHost() {
			filtered = append(filtered, node)
		}
	}
	return filtered
}

// filterMultiHostname filters nodes with multiple hostnames
func (nf *NodeFilter) filterMultiHostname(nodes []*models.Node) []*models.Node {
	filtered := make([]*models.Node, 0)
	for _, node := range nodes {
		if len(node.InternetHostnames) > 1 {
			filtered = append(filtered, node)
		}
	}
	return filtered
}

// FilterByPriority filters nodes based on priority criteria
func (nf *NodeFilter) FilterByPriority(nodes []*models.Node, priorityCriteria string) []*models.Node {
	switch priorityCriteria {
	case "failed":
		// Prioritize nodes that failed in recent tests
		// This would require access to recent test results
		return nodes // TODO: Implement when test history is available

	case "untested":
		// Prioritize nodes never tested
		// This would require access to test history
		return nodes // TODO: Implement when test history is available

	case "stale":
		// Prioritize nodes with oldest test results
		// This would require access to test history
		return nodes // TODO: Implement when test history is available

	default:
		return nodes
	}
}

// FilterByCapabilities filters nodes based on their capabilities
func (nf *NodeFilter) FilterByCapabilities(nodes []*models.Node, capabilities []string) []*models.Node {
	if len(capabilities) == 0 {
		return nodes
	}

	filtered := make([]*models.Node, 0)
	for _, node := range nodes {
		hasAllCapabilities := true

		for _, cap := range capabilities {
			switch strings.ToLower(cap) {
			case "ipv6":
				// Check if node has IPv6 capability - would need DNS resolution
				hasAllCapabilities = hasAllCapabilities && false // TODO: Implement IPv6 check
			case "secure":
				// Check if node supports secure protocols
				hasAllCapabilities = hasAllCapabilities && (node.HasProtocol("IBN") || node.HasProtocol("IFC"))
			case "24h":
				// Check if node is available 24 hours (check for CM flag)
				hasCM := false
				for _, flag := range node.Flags {
					if flag == "CM" {
						hasCM = true
						break
					}
				}
				hasAllCapabilities = hasAllCapabilities && hasCM
			default:
				// Unknown capability
			}
		}

		if hasAllCapabilities {
			filtered = append(filtered, node)
		}
	}

	return filtered
}