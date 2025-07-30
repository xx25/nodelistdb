package database

import (
	"encoding/json"
	"fmt"
	"time"
)

// Node represents a FidoNet node entry
type Node struct {
	// Core identity
	Zone int `json:"zone"`
	Net  int `json:"net"`
	Node int `json:"node"`

	// Nodelist metadata
	NodelistDate time.Time `json:"nodelist_date"`
	DayNumber    int       `json:"day_number"`

	// Node information
	SystemName string `json:"system_name"`
	Location   string `json:"location"`
	SysopName  string `json:"sysop_name"`
	Phone      string `json:"phone"`
	NodeType   string `json:"node_type"` // Zone, Region, Host, Hub, Pvt, Down, Hold
	Region     *int   `json:"region,omitempty"`
	MaxSpeed   uint32 `json:"max_speed"`

	// Boolean flags (computed from raw flags)
	IsCM      bool `json:"is_cm"`
	IsMO      bool `json:"is_mo"`
	HasBinkp  bool `json:"has_binkp"`
	HasInet   bool `json:"has_inet"` // Any internet connectivity
	HasTelnet bool `json:"has_telnet"`
	IsDown    bool `json:"is_down"`
	IsHold    bool `json:"is_hold"`
	IsPvt     bool `json:"is_pvt"`
	IsActive  bool `json:"is_active"`

	// Raw flag arrays
	Flags             []string `json:"flags"`
	ModemFlags        []string `json:"modem_flags"`
	InternetProtocols []string `json:"internet_protocols"`
	InternetHostnames []string `json:"internet_hostnames"`
	InternetPorts     []int    `json:"internet_ports"`
	InternetEmails    []string `json:"internet_emails"`

	// Internet configuration JSON
	InternetConfig json.RawMessage `json:"internet_config,omitempty"`

	// Conflict tracking
	ConflictSequence int  `json:"conflict_sequence"`
	HasConflict      bool `json:"has_conflict"`
	
	// FTS identifier
	FtsId string `json:"fts_id"`
}

// ComputeFtsId generates the FTS identifier for this node
func (n *Node) ComputeFtsId() {
	n.FtsId = fmt.Sprintf("%d:%d/%d@%s#%d", 
		n.Zone, n.Net, n.Node, 
		n.NodelistDate.Format("2006-01-02"), 
		n.ConflictSequence)
}

// InternetProtocolDetail represents details for a specific protocol
type InternetProtocolDetail struct {
	Address string `json:"address,omitempty"`
	Port    int    `json:"port,omitempty"`
}

// EmailProtocolDetail represents details for email protocols
type EmailProtocolDetail struct {
	Email string `json:"email,omitempty"`
}

// InternetConfiguration represents the structured internet config
type InternetConfiguration struct {
	Protocols      map[string]InternetProtocolDetail `json:"protocols,omitempty"`
	Defaults       map[string]string                 `json:"defaults,omitempty"`
	EmailProtocols map[string]EmailProtocolDetail    `json:"email_protocols,omitempty"`
	InfoFlags      []string                          `json:"info_flags,omitempty"`
}

// NodeFilter represents search criteria for nodes
type NodeFilter struct {
	// Location filters
	Zone *int `json:"zone,omitempty"`
	Net  *int `json:"net,omitempty"`
	Node *int `json:"node,omitempty"`

	// Date filters
	DateFrom *time.Time `json:"date_from,omitempty"`
	DateTo   *time.Time `json:"date_to,omitempty"`

	// Text filters
	SystemName *string `json:"system_name,omitempty"`
	Location   *string `json:"location,omitempty"`
	SysopName  *string `json:"sysop_name,omitempty"`
	NodeType   *string `json:"node_type,omitempty"`

	// Flag filters
	IsCM      *bool `json:"is_cm,omitempty"`
	IsMO      *bool `json:"is_mo,omitempty"`
	HasBinkp  *bool `json:"has_binkp,omitempty"`
	HasTelnet *bool `json:"has_telnet,omitempty"`
	IsActive  *bool `json:"is_active,omitempty"`

	// Result options
	LatestOnly *bool `json:"latest_only,omitempty"`

	// Pagination
	Limit  int `json:"limit,omitempty"`
	Offset int `json:"offset,omitempty"`
}

// NetworkStats represents aggregated network statistics
// RegionInfo holds information about a region
type RegionInfo struct {
	Zone      int    `json:"zone"`
	Region    int    `json:"region"`
	NodeCount int    `json:"node_count"`
	Name      string `json:"name,omitempty"`
}

// NetInfo holds information about a net
type NetInfo struct {
	Zone      int    `json:"zone"`
	Net       int    `json:"net"`
	NodeCount int    `json:"node_count"`
	Name      string `json:"name,omitempty"`
}

type NetworkStats struct {
	Date             time.Time    `json:"date"`
	TotalNodes       int          `json:"total_nodes"`
	ActiveNodes      int          `json:"active_nodes"`
	CMNodes          int          `json:"cm_nodes"`
	MONodes          int          `json:"mo_nodes"`
	BinkpNodes       int          `json:"binkp_nodes"`
	TelnetNodes      int          `json:"telnet_nodes"`
	PvtNodes         int          `json:"pvt_nodes"`
	DownNodes        int          `json:"down_nodes"`
	HoldNodes        int          `json:"hold_nodes"`
	InternetNodes    int          `json:"internet_nodes"`
	ZoneDistribution map[int]int  `json:"zone_distribution"`
	LargestRegions   []RegionInfo `json:"largest_regions"`
	LargestNets      []NetInfo    `json:"largest_nets"`
}

// ProcessingResult represents the result of processing a nodelist file
type ProcessingResult struct {
	NodelistDate   time.Time     `json:"nodelist_date"`
	DayNumber      int           `json:"day_number"`
	NodesFound     int           `json:"nodes_found"`
	NodesInserted  int           `json:"nodes_inserted"`
	ProcessingTime time.Duration `json:"processing_time"`
	Error          error         `json:"error,omitempty"`
}

// NodeChange represents a change in node data between two dates
type NodeChange struct {
	Date       time.Time         `json:"date"`
	DayNumber  int               `json:"day_number"`
	ChangeType string            `json:"change_type"` // "added", "removed", "modified"
	Changes    map[string]string `json:"changes"`     // field -> "old value -> new value"
	OldNode    *Node             `json:"old_node,omitempty"`
	NewNode    *Node             `json:"new_node,omitempty"`
}
