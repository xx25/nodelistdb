package fixtures

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/nodelistdb/internal/database"
)

// NodeBuilder provides a fluent API for building test nodes
type NodeBuilder struct {
	node database.Node
}

// NewNodeBuilder creates a new NodeBuilder with sensible defaults
func NewNodeBuilder() *NodeBuilder {
	return &NodeBuilder{
		node: database.Node{
			Zone:           2,
			Net:            5001,
			Node:           100,
			SystemName:     "Test System",
			Location:       "Test Location, Country",
			SysopName:      "Test_Sysop",
			Phone:          "000-000-0000",
			NodeType:       "Pvt",
			Region:         nil,
			MaxSpeed:       33600,
			IsCM:           false,
			IsMO:           false,
			Flags:          []string{},
			ModemFlags:     []string{},
			NodelistDate:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			DayNumber:      1,
			HasInet:        false,
			InternetConfig: json.RawMessage("{}"),
		},
	}
}

// WithAddress sets the FidoNet address
func (b *NodeBuilder) WithAddress(zone, net, node int) *NodeBuilder {
	b.node.Zone = zone
	b.node.Net = net
	b.node.Node = node
	return b
}

// WithZone sets only the zone
func (b *NodeBuilder) WithZone(zone int) *NodeBuilder {
	b.node.Zone = zone
	return b
}

// WithNet sets only the net
func (b *NodeBuilder) WithNet(net int) *NodeBuilder {
	b.node.Net = net
	return b
}

// WithNode sets only the node number
func (b *NodeBuilder) WithNode(node int) *NodeBuilder {
	b.node.Node = node
	return b
}

// WithSystemName sets the system name
func (b *NodeBuilder) WithSystemName(name string) *NodeBuilder {
	b.node.SystemName = name
	return b
}

// WithLocation sets the location
func (b *NodeBuilder) WithLocation(location string) *NodeBuilder {
	b.node.Location = location
	return b
}

// WithSysopName sets the sysop name
func (b *NodeBuilder) WithSysopName(name string) *NodeBuilder {
	b.node.SysopName = name
	return b
}

// WithPhone sets the phone number
func (b *NodeBuilder) WithPhone(phone string) *NodeBuilder {
	b.node.Phone = phone
	return b
}

// WithNodeType sets the node type (Pvt, Hold, Down, etc.)
func (b *NodeBuilder) WithNodeType(nodeType string) *NodeBuilder {
	b.node.NodeType = nodeType
	return b
}

// WithDate sets the nodelist date
func (b *NodeBuilder) WithDate(date time.Time) *NodeBuilder {
	b.node.NodelistDate = date
	b.node.DayNumber = date.YearDay()
	return b
}

// WithDayNumber sets the day number
func (b *NodeBuilder) WithDayNumber(day int) *NodeBuilder {
	b.node.DayNumber = day
	return b
}

// WithFlags sets the flags
func (b *NodeBuilder) WithFlags(flags ...string) *NodeBuilder {
	b.node.Flags = flags
	return b
}

// AddFlag adds a single flag
func (b *NodeBuilder) AddFlag(flag string) *NodeBuilder {
	b.node.Flags = append(b.node.Flags, flag)
	return b
}

// WithModemFlags sets the modem flags
func (b *NodeBuilder) WithModemFlags(flags ...string) *NodeBuilder {
	b.node.ModemFlags = flags
	return b
}

// WithMaxSpeed sets the max speed
func (b *NodeBuilder) WithMaxSpeed(speed uint32) *NodeBuilder {
	b.node.MaxSpeed = speed
	return b
}

// WithCM marks the node as CM (Continuous Mail)
func (b *NodeBuilder) WithCM() *NodeBuilder {
	b.node.IsCM = true
	b.AddFlag("CM")
	return b
}

// WithMO marks the node as MO (Mail Only)
func (b *NodeBuilder) WithMO() *NodeBuilder {
	b.node.IsMO = true
	return b
}

// WithInternet adds internet connectivity
func (b *NodeBuilder) WithInternet(hostname string) *NodeBuilder {
	b.node.HasInet = true
	config := map[string]interface{}{
		"hostnames": []string{hostname},
	}
	jsonData, _ := json.Marshal(config)
	b.node.InternetConfig = jsonData
	return b
}

// WithBinkP adds BinkP protocol
func (b *NodeBuilder) WithBinkP(hostname string) *NodeBuilder {
	b.AddFlag(fmt.Sprintf("IBN:%s", hostname))
	b.node.HasInet = true
	config := map[string]interface{}{
		"hostnames": []string{hostname},
		"protocols": []string{"binkp"},
	}
	jsonData, _ := json.Marshal(config)
	b.node.InternetConfig = jsonData
	return b
}

// WithTelnet adds Telnet protocol
func (b *NodeBuilder) WithTelnet(hostname string) *NodeBuilder {
	b.AddFlag(fmt.Sprintf("ITN:%s", hostname))
	b.node.HasInet = true
	return b
}

// WithConflictSequence sets the conflict sequence
func (b *NodeBuilder) WithConflictSequence(seq int) *NodeBuilder {
	b.node.ConflictSequence = seq
	if seq > 0 {
		b.node.HasConflict = true
	}
	return b
}

// WithRawLine sets the raw line
func (b *NodeBuilder) WithRawLine(line string) *NodeBuilder {
	b.node.RawLine = line
	return b
}

// Build returns the constructed node
func (b *NodeBuilder) Build() database.Node {
	// Compute FTS ID if not set
	if b.node.FtsId == "" {
		b.node.ComputeFtsId()
	}
	return b.node
}

// BuildPtr returns a pointer to the constructed node
func (b *NodeBuilder) BuildPtr() *database.Node {
	node := b.Build()
	return &node
}

// Clone creates a new builder with a copy of the current node
func (b *NodeBuilder) Clone() *NodeBuilder {
	nodeCopy := b.node
	// Deep copy slices
	nodeCopy.Flags = append([]string{}, b.node.Flags...)
	nodeCopy.ModemFlags = append([]string{}, b.node.ModemFlags...)
	return &NodeBuilder{node: nodeCopy}
}

// Predefined builder functions for common node types

// NewHub creates a hub node
func NewHub(zone, net, node int) *NodeBuilder {
	return NewNodeBuilder().
		WithAddress(zone, net, node).
		WithNodeType("Hub").
		WithSystemName("Hub System").
		WithCM()
}

// NewHost creates a host node
func NewHost(zone, net int) *NodeBuilder {
	return NewNodeBuilder().
		WithAddress(zone, net, 0).
		WithNodeType("Host").
		WithSystemName("Host System").
		WithCM()
}

// NewRegion creates a region node
func NewRegion(zone, region int) *NodeBuilder {
	return NewNodeBuilder().
		WithAddress(zone, region, 0).
		WithNodeType("Region").
		WithSystemName("Regional System").
		WithCM()
}

// NewZone creates a zone node
func NewZone(zone int) *NodeBuilder {
	return NewNodeBuilder().
		WithAddress(zone, zone, 0).
		WithNodeType("Zone").
		WithSystemName("Zone Coordinator").
		WithCM()
}

// NewPvt creates a private node
func NewPvt(zone, net, node int) *NodeBuilder {
	return NewNodeBuilder().
		WithAddress(zone, net, node).
		WithNodeType("Pvt").
		WithSystemName("Private System")
}

// NewDown creates a down node
func NewDown(zone, net, node int) *NodeBuilder {
	return NewNodeBuilder().
		WithAddress(zone, net, node).
		WithNodeType("Down").
		WithSystemName("Down System")
}

// NewHold creates a hold node
func NewHold(zone, net, node int) *NodeBuilder {
	return NewNodeBuilder().
		WithAddress(zone, net, node).
		WithNodeType("Hold").
		WithSystemName("Hold System")
}

// NewInternetNode creates a node with internet connectivity
func NewInternetNode(zone, net, node int, hostname string) *NodeBuilder {
	return NewNodeBuilder().
		WithAddress(zone, net, node).
		WithBinkP(hostname).
		WithCM()
}

// BatchBuild builds multiple nodes
func BatchBuild(builders ...*NodeBuilder) []database.Node {
	nodes := make([]database.Node, len(builders))
	for i, builder := range builders {
		nodes[i] = builder.Build()
	}
	return nodes
}
