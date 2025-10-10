package testutil

import (
	"time"

	"github.com/nodelistdb/internal/testing/models"
)

// NodeBuilder helps create test nodes
type NodeBuilder struct {
	node *models.Node
}

// NewNodeBuilder creates a new node builder with defaults
func NewNodeBuilder() *NodeBuilder {
	return &NodeBuilder{
		node: &models.Node{
			Zone:              1,
			Net:               1,
			Node:              1,
			SystemName:        "Test BBS",
			SysopName:         "Test Sysop",
			Location:          "Test City",
			InternetHostnames: []string{"test.example.com"},
			InternetProtocols: []string{"IBN"},
			HasInet:           true,
			Flags:             []string{},
		},
	}
}

func (b *NodeBuilder) WithAddress(zone, net, node int) *NodeBuilder {
	b.node.Zone = zone
	b.node.Net = net
	b.node.Node = node
	return b
}

func (b *NodeBuilder) WithHostnames(hostnames ...string) *NodeBuilder {
	b.node.InternetHostnames = hostnames
	return b
}

func (b *NodeBuilder) WithProtocols(protocols ...string) *NodeBuilder {
	b.node.InternetProtocols = protocols
	return b
}

func (b *NodeBuilder) WithSystemName(name string) *NodeBuilder {
	b.node.SystemName = name
	return b
}

func (b *NodeBuilder) WithNoHostnames() *NodeBuilder {
	b.node.InternetHostnames = []string{}
	return b
}

func (b *NodeBuilder) WithFlags(flags ...string) *NodeBuilder {
	b.node.Flags = flags
	return b
}

func (b *NodeBuilder) Build() *models.Node {
	return b.node
}

// TestResultBuilder helps create test results
type TestResultBuilder struct {
	result *models.TestResult
}

// NewTestResultBuilder creates a new result builder with defaults
func NewTestResultBuilder() *TestResultBuilder {
	return &TestResultBuilder{
		result: &models.TestResult{
			TestTime:      time.Now(),
			TestDate:      time.Now().Truncate(24 * time.Hour),
			Zone:          1,
			Net:           1,
			Node:          1,
			IsOperational: false,
			Address:       "1:1/1",
		},
	}
}

func (b *TestResultBuilder) ForNode(node *models.Node) *TestResultBuilder {
	b.result.Zone = node.Zone
	b.result.Net = node.Net
	b.result.Node = node.Node
	b.result.Address = node.Address()
	return b
}

func (b *TestResultBuilder) WithDNS(ipv4, ipv6 []string) *TestResultBuilder {
	b.result.ResolvedIPv4 = ipv4
	b.result.ResolvedIPv6 = ipv6
	return b
}

func (b *TestResultBuilder) WithDNSError(err string) *TestResultBuilder {
	b.result.DNSError = err
	return b
}

func (b *TestResultBuilder) WithBinkPSuccess() *TestResultBuilder {
	b.result.BinkPResult = &models.ProtocolTestResult{
		Tested:     true,
		Success:    true,
		ResponseMs: 100,
	}
	b.result.IsOperational = true
	return b
}

func (b *TestResultBuilder) WithBinkPFailure(err string) *TestResultBuilder {
	b.result.BinkPResult = &models.ProtocolTestResult{
		Tested:  true,
		Success: false,
		Error:   err,
	}
	return b
}

func (b *TestResultBuilder) WithOperational(operational bool) *TestResultBuilder {
	b.result.IsOperational = operational
	return b
}

func (b *TestResultBuilder) WithHostname(hostname string) *TestResultBuilder {
	b.result.Hostname = hostname
	b.result.TestedHostname = hostname
	return b
}

func (b *TestResultBuilder) Build() *models.TestResult {
	return b.result
}

