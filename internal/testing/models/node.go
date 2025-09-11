package models

import (
	"fmt"
	"time"
)

// Node represents a FidoNet node from the database
type Node struct {
	Zone              int                `db:"zone"`
	Net               int                `db:"net"`
	Node              int                `db:"node"`
	SystemName        string             `db:"system_name"`
	SysopName         string             `db:"sysop_name"`
	Location          string             `db:"location"`
	InternetHostnames []string           `db:"internet_hostnames"`
	InternetProtocols []string           `db:"internet_protocols"`
	ProtocolPorts     map[string]int     `db:"-"` // Custom ports for protocols (e.g., "IFC" -> 5983)
	InternetConfig    map[string]interface{} `db:"internet_config"` // Raw JSON config from database
	HasInet           bool               `db:"has_inet"`
	TestReason        string             `db:"-"` // Reason for current test: "stale", "new", "config_changed", "scheduled", "failed_retry"
}

// Address returns the FidoNet address string
func (n *Node) Address() string {
	return fmt.Sprintf("%d:%d/%d", n.Zone, n.Net, n.Node)
}

// HasProtocol checks if node supports a specific protocol
func (n *Node) HasProtocol(protocol string) bool {
	for _, p := range n.InternetProtocols {
		if p == protocol {
			return true
		}
	}
	return false
}

// GetPrimaryHostname returns the first hostname or empty string
func (n *Node) GetPrimaryHostname() string {
	if len(n.InternetHostnames) > 0 {
		return n.InternetHostnames[0]
	}
	return ""
}

// GetProtocolPort returns the custom port for a protocol, or 0 if using default
func (n *Node) GetProtocolPort(protocol string) int {
	if n.ProtocolPorts != nil {
		if port, ok := n.ProtocolPorts[protocol]; ok {
			return port
		}
	}
	return 0 // Return 0 to indicate default port should be used
}

// NodeTestRequest represents a request to test a node
type NodeTestRequest struct {
	Node              *Node
	ProtocolsToTest   []string
	ExpectedAddresses []string
	TestTime          time.Time
}