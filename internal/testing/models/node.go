package models

import (
	"fmt"
	"regexp"
	"time"

	"github.com/nodelistdb/internal/testing/timeavail"
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
	Availability      *timeavail.NodeAvailability `db:"-"` // Time availability windows for calling
	Flags             []string           `db:"flags"` // Node flags from nodelist
	InfoFlags         []string           `db:"-"` // Information flags from InternetConfig (INO4, ICM)
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

	// If no hostname is configured, try to use system name as fallback
	// but only if it looks like a valid hostname (FQDN or hostname pattern)
	if n.SystemName != "" && n.SystemName != "-Unpublished-" {
		// Simple regex to check if system name looks like a hostname:
		// - Contains dots (FQDN like "bbs.example.com")
		// - Or contains only valid hostname characters
		hostnameRegex := regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?\.)*[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?$`)
		if hostnameRegex.MatchString(n.SystemName) {
			return n.SystemName
		}
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

// IsOnline checks if the node is considered online (has internet protocols)
func (n *Node) IsOnline() bool {
	return n.HasInet && len(n.InternetProtocols) > 0
}

// IsHost checks if the node has the Host flag
func (n *Node) IsHost() bool {
	for _, flag := range n.Flags {
		if flag == "HOST" || flag == "Host" {
			return true
		}
	}
	return false
}

// HasBinkP checks if node supports BinkP protocol
func (n *Node) HasBinkP() bool {
	return n.HasProtocol("IBN") || n.HasProtocol("IFC")
}

// HasIFCICO checks if node supports IFCICO (EMSI) protocol
func (n *Node) HasIFCICO() bool {
	return n.HasProtocol("IFC") || n.HasProtocol("ITN")
}

// HasTEL checks if node supports Telnet protocol
func (n *Node) HasTEL() bool {
	return n.HasProtocol("ITN") || n.HasProtocol("TEL")
}

// FTPAddress returns the FTP address if available
func (n *Node) FTPAddress() string {
	// Check if node has FTP protocol flag
	if !n.HasProtocol("IFT") {
		return ""
	}

	// Return primary hostname if available
	if hostname := n.GetPrimaryHostname(); hostname != "" {
		return hostname
	}

	return ""
}

// HasInfoFlag checks if node has a specific information flag (INO4, ICM, etc.)
func (n *Node) HasInfoFlag(flag string) bool {
	for _, f := range n.InfoFlags {
		if f == flag {
			return true
		}
	}
	return false
}

// HasINO4 checks if node has the INO4 flag (no IPv4 incoming connections)
// Per FTS-1038: "Indicates that an otherwise IP capable node is unable to
// accept incoming connections over IPv4"
func (n *Node) HasINO4() bool {
	return n.HasInfoFlag("INO4")
}

// ShouldTestIPv4 determines if IPv4 connectivity should be tested
// Returns false if the node has the INO4 flag set
func (n *Node) ShouldTestIPv4() bool {
	return !n.HasINO4()
}

// NodeTestRequest represents a request to test a node
type NodeTestRequest struct {
	Node              *Node
	ProtocolsToTest   []string
	ExpectedAddresses []string
	TestTime          time.Time
}