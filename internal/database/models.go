package database

import (
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
	MaxSpeed   string `json:"max_speed"`

	// Boolean flags (computed from raw flags)
	IsCM      bool `json:"is_cm"`
	IsMO      bool `json:"is_mo"`
	HasBinkp  bool `json:"has_binkp"`
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

	// File metadata
	RawLine   string    `json:"raw_line"`
	FilePath  string    `json:"file_path"`
	FileCRC   int       `json:"file_crc"`
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
	
	// Conflict tracking
	ConflictSequence int  `json:"conflict_sequence"`
	HasConflict      bool `json:"has_conflict"`
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

	// Pagination
	Limit  int `json:"limit,omitempty"`
	Offset int `json:"offset,omitempty"`
}

// NetworkStats represents aggregated network statistics
type NetworkStats struct {
	Date             time.Time `json:"date"`
	TotalNodes       int       `json:"total_nodes"`
	ActiveNodes      int       `json:"active_nodes"`
	CMNodes          int       `json:"cm_nodes"`
	MONodes          int       `json:"mo_nodes"`
	BinkpNodes       int       `json:"binkp_nodes"`
	TelnetNodes      int       `json:"telnet_nodes"`
	PvtNodes         int       `json:"pvt_nodes"`
	DownNodes        int       `json:"down_nodes"`
	HoldNodes        int       `json:"hold_nodes"`
	InternetNodes    int       `json:"internet_nodes"`
	ZoneDistribution map[int]int `json:"zone_distribution"`
}

// ProcessingResult represents the result of processing a nodelist file
type ProcessingResult struct {
	FilePath     string        `json:"file_path"`
	NodesFound   int          `json:"nodes_found"`
	NodesInserted int         `json:"nodes_inserted"`
	ProcessingTime time.Duration `json:"processing_time"`
	Error        error        `json:"error,omitempty"`
}