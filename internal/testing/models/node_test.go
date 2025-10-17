package models

import (
	"testing"
)

func TestNodeAddress(t *testing.T) {
	tests := []struct {
		name     string
		node     *Node
		expected string
	}{
		{
			name:     "standard address",
			node:     &Node{Zone: 2, Net: 450, Node: 1024},
			expected: "2:450/1024",
		},
		{
			name:     "zone 1",
			node:     &Node{Zone: 1, Net: 1, Node: 1},
			expected: "1:1/1",
		},
		{
			name:     "large numbers",
			node:     &Node{Zone: 99, Net: 9999, Node: 999999},
			expected: "99:9999/999999",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.node.Address()
			if result != tt.expected {
				t.Errorf("Address() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestNodeHasProtocol(t *testing.T) {
	node := &Node{
		Zone:              2,
		Net:               450,
		Node:              1024,
		InternetProtocols: []string{"IBN", "IFC", "ITN"},
	}

	tests := []struct {
		name     string
		protocol string
		expected bool
	}{
		{"has IBN", "IBN", true},
		{"has IFC", "IFC", true},
		{"has ITN", "ITN", true},
		{"doesn't have IFT", "IFT", false},
		{"doesn't have IVM", "IVM", false},
		{"empty protocol", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := node.HasProtocol(tt.protocol)
			if result != tt.expected {
				t.Errorf("HasProtocol(%s) = %v, want %v", tt.protocol, result, tt.expected)
			}
		})
	}
}

func TestNodeGetPrimaryHostname(t *testing.T) {
	tests := []struct {
		name     string
		node     *Node
		expected string
	}{
		{
			name: "has hostname",
			node: &Node{
				InternetHostnames: []string{"bbs.example.com", "backup.example.com"},
			},
			expected: "bbs.example.com",
		},
		{
			name: "single hostname",
			node: &Node{
				InternetHostnames: []string{"bbs.example.com"},
			},
			expected: "bbs.example.com",
		},
		{
			name: "no hostname, valid system name as FQDN",
			node: &Node{
				InternetHostnames: []string{},
				SystemName:        "bbs.example.com",
			},
			expected: "bbs.example.com",
		},
		{
			name: "no hostname, valid system name as hostname",
			node: &Node{
				InternetHostnames: []string{},
				SystemName:        "myserver",
			},
			expected: "myserver",
		},
		{
			name: "no hostname, unpublished system name",
			node: &Node{
				InternetHostnames: []string{},
				SystemName:        "-Unpublished-",
			},
			expected: "",
		},
		{
			name: "no hostname, invalid system name with spaces",
			node: &Node{
				InternetHostnames: []string{},
				SystemName:        "My BBS System",
			},
			expected: "",
		},
		{
			name: "no hostname, empty system name",
			node: &Node{
				InternetHostnames: []string{},
				SystemName:        "",
			},
			expected: "",
		},
		{
			name:     "empty node",
			node:     &Node{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.node.GetPrimaryHostname()
			if result != tt.expected {
				t.Errorf("GetPrimaryHostname() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestNodeGetProtocolPort(t *testing.T) {
	node := &Node{
		Zone: 2,
		Net:  450,
		Node: 1024,
		ProtocolPorts: map[string]int{
			"IBN": 24555,
			"IFC": 5983,
		},
	}

	tests := []struct {
		name     string
		protocol string
		expected int
	}{
		{"has custom IBN port", "IBN", 24555},
		{"has custom IFC port", "IFC", 5983},
		{"no custom ITN port", "ITN", 0},
		{"no custom IFT port", "IFT", 0},
		{"empty protocol", "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := node.GetProtocolPort(tt.protocol)
			if result != tt.expected {
				t.Errorf("GetProtocolPort(%s) = %v, want %v", tt.protocol, result, tt.expected)
			}
		})
	}
}

func TestNodeGetProtocolPortNilMap(t *testing.T) {
	node := &Node{
		Zone:          2,
		Net:           450,
		Node:          1024,
		ProtocolPorts: nil,
	}

	result := node.GetProtocolPort("IBN")
	if result != 0 {
		t.Errorf("GetProtocolPort with nil map should return 0, got %v", result)
	}
}

func TestNodeIsOnline(t *testing.T) {
	tests := []struct {
		name     string
		node     *Node
		expected bool
	}{
		{
			name: "online with protocols",
			node: &Node{
				HasInet:           true,
				InternetProtocols: []string{"IBN", "IFC"},
			},
			expected: true,
		},
		{
			name: "has inet but no protocols",
			node: &Node{
				HasInet:           true,
				InternetProtocols: []string{},
			},
			expected: false,
		},
		{
			name: "has protocols but not inet",
			node: &Node{
				HasInet:           false,
				InternetProtocols: []string{"IBN"},
			},
			expected: false,
		},
		{
			name: "offline",
			node: &Node{
				HasInet:           false,
				InternetProtocols: []string{},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.node.IsOnline()
			if result != tt.expected {
				t.Errorf("IsOnline() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestNodeIsHost(t *testing.T) {
	tests := []struct {
		name     string
		node     *Node
		expected bool
	}{
		{
			name: "has HOST flag uppercase",
			node: &Node{
				Flags: []string{"CM", "HOST", "XA"},
			},
			expected: true,
		},
		{
			name: "has Host flag mixed case",
			node: &Node{
				Flags: []string{"CM", "Host", "XA"},
			},
			expected: true,
		},
		{
			name: "doesn't have HOST flag",
			node: &Node{
				Flags: []string{"CM", "XA", "MO"},
			},
			expected: false,
		},
		{
			name: "no flags",
			node: &Node{
				Flags: []string{},
			},
			expected: false,
		},
		{
			name:     "nil flags",
			node:     &Node{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.node.IsHost()
			if result != tt.expected {
				t.Errorf("IsHost() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestNodeHasBinkP(t *testing.T) {
	tests := []struct {
		name     string
		node     *Node
		expected bool
	}{
		{
			name: "has IBN",
			node: &Node{
				InternetProtocols: []string{"IBN"},
			},
			expected: true,
		},
		{
			name: "has IFC",
			node: &Node{
				InternetProtocols: []string{"IFC"},
			},
			expected: true,
		},
		{
			name: "has both IBN and IFC",
			node: &Node{
				InternetProtocols: []string{"IBN", "IFC"},
			},
			expected: true,
		},
		{
			name: "has neither",
			node: &Node{
				InternetProtocols: []string{"ITN"},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.node.HasBinkP()
			if result != tt.expected {
				t.Errorf("HasBinkP() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestNodeHasIFCICO(t *testing.T) {
	tests := []struct {
		name     string
		node     *Node
		expected bool
	}{
		{
			name: "has IFC",
			node: &Node{
				InternetProtocols: []string{"IFC"},
			},
			expected: true,
		},
		{
			name: "has ITN",
			node: &Node{
				InternetProtocols: []string{"ITN"},
			},
			expected: true,
		},
		{
			name: "has both IFC and ITN",
			node: &Node{
				InternetProtocols: []string{"IFC", "ITN"},
			},
			expected: true,
		},
		{
			name: "has neither",
			node: &Node{
				InternetProtocols: []string{"IBN"},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.node.HasIFCICO()
			if result != tt.expected {
				t.Errorf("HasIFCICO() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestNodeHasTEL(t *testing.T) {
	tests := []struct {
		name     string
		node     *Node
		expected bool
	}{
		{
			name: "has ITN",
			node: &Node{
				InternetProtocols: []string{"ITN"},
			},
			expected: true,
		},
		{
			name: "has TEL",
			node: &Node{
				InternetProtocols: []string{"TEL"},
			},
			expected: true,
		},
		{
			name: "has both ITN and TEL",
			node: &Node{
				InternetProtocols: []string{"ITN", "TEL"},
			},
			expected: true,
		},
		{
			name: "has neither",
			node: &Node{
				InternetProtocols: []string{"IBN"},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.node.HasTEL()
			if result != tt.expected {
				t.Errorf("HasTEL() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestNodeFTPAddress(t *testing.T) {
	tests := []struct {
		name     string
		node     *Node
		expected string
	}{
		{
			name: "has FTP protocol and hostname",
			node: &Node{
				InternetProtocols: []string{"IFT"},
				InternetHostnames: []string{"ftp.example.com"},
			},
			expected: "ftp.example.com",
		},
		{
			name: "has FTP protocol but no hostname",
			node: &Node{
				InternetProtocols: []string{"IFT"},
				InternetHostnames: []string{},
			},
			expected: "",
		},
		{
			name: "has hostname but no FTP protocol",
			node: &Node{
				InternetProtocols: []string{"IBN"},
				InternetHostnames: []string{"bbs.example.com"},
			},
			expected: "",
		},
		{
			name: "has FTP protocol and system name as fallback",
			node: &Node{
				InternetProtocols: []string{"IFT"},
				InternetHostnames: []string{},
				SystemName:        "ftp.example.com",
			},
			expected: "ftp.example.com",
		},
		{
			name: "no FTP protocol, no hostname",
			node: &Node{
				InternetProtocols: []string{"IBN"},
				InternetHostnames: []string{},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.node.FTPAddress()
			if result != tt.expected {
				t.Errorf("FTPAddress() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestNodeTestReason(t *testing.T) {
	node := &Node{
		Zone:       2,
		Net:        450,
		Node:       1024,
		TestReason: "stale",
	}

	if node.TestReason != "stale" {
		t.Errorf("TestReason should be 'stale', got %s", node.TestReason)
	}
}
