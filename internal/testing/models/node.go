package models

import (
	"fmt"
	"time"
)

// Node represents a FidoNet node from the database
type Node struct {
	Zone              int      `db:"zone"`
	Net               int      `db:"net"`
	Node              int      `db:"node"`
	SystemName        string   `db:"system_name"`
	SysopName         string   `db:"sysop_name"`
	Location          string   `db:"location"`
	InternetHostnames []string `db:"internet_hostnames"`
	InternetProtocols []string `db:"internet_protocols"`
	HasInet           bool     `db:"has_inet"`
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

// NodeTestRequest represents a request to test a node
type NodeTestRequest struct {
	Node              *Node
	ProtocolsToTest   []string
	ExpectedAddresses []string
	TestTime          time.Time
}