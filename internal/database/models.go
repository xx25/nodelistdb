package database

import (
	"encoding/json"
	"fmt"
	"time"
)

// DefaultDomain is the FTN network assumed when none is specified.
const DefaultDomain = "fidonet"

// Node represents an FTN node entry
type Node struct {
	// Core identity
	Zone int `json:"zone"`
	Net  int `json:"net"`
	Node int `json:"node"`

	// FTN network this entry belongs to (fidonet, fsxnet, ...).
	// Zone numbers are reused across networks, so zone never identifies the network.
	Domain string `json:"domain,omitempty"`

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
	IsCM    bool `json:"is_cm"`
	IsMO    bool `json:"is_mo"`
	HasInet bool `json:"has_inet"` // Any internet connectivity

	// Raw flag arrays
	Flags      []string `json:"flags"`
	ModemFlags []string `json:"modem_flags"`

	// Internet configuration JSON
	InternetConfig json.RawMessage `json:"internet_config,omitempty"`

	// Conflict tracking
	ConflictSequence int  `json:"conflict_sequence"`
	HasConflict      bool `json:"has_conflict"`

	// FTS identifier
	FtsId string `json:"fts_id"`

	// Raw nodelist line (original format from file)
	RawLine string `json:"raw_line,omitempty"`
}

// ComputeFtsId generates the FTS identifier for this node.
// The fidonet format ("z:n/n@date#seq") is kept unchanged so existing rows stay
// valid; other domains carry an extra "@domain" suffix to keep the id unique
// when the same 3D address exists in several networks.
func (n *Node) ComputeFtsId() {
	n.FtsId = fmt.Sprintf("%d:%d/%d@%s#%d",
		n.Zone, n.Net, n.Node,
		n.NodelistDate.Format("2006-01-02"),
		n.ConflictSequence)
	if n.Domain != "" && n.Domain != DefaultDomain {
		n.FtsId += "@" + n.Domain
	}
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
	Protocols      map[string][]InternetProtocolDetail `json:"protocols,omitempty"`
	Defaults       map[string]string                   `json:"defaults,omitempty"`
	EmailProtocols map[string][]EmailProtocolDetail    `json:"email_protocols,omitempty"`
	InfoFlags      []string                            `json:"info_flags,omitempty"`
}

// NodeFilter represents search criteria for nodes
type NodeFilter struct {
	// Location filters
	Zone *int `json:"zone,omitempty"`
	Net  *int `json:"net,omitempty"`
	Node *int `json:"node,omitempty"`

	// FTN network filter (nil = all networks)
	Domain *string `json:"domain,omitempty"`

	// Date filters
	DateFrom *time.Time `json:"date_from,omitempty"`
	DateTo   *time.Time `json:"date_to,omitempty"`

	// Text filters
	SystemName *string `json:"system_name,omitempty"`
	Location   *string `json:"location,omitempty"`
	SysopName  *string `json:"sysop_name,omitempty"`
	NodeType   *string `json:"node_type,omitempty"`

	// Flag filters
	IsCM     *bool `json:"is_cm,omitempty"`
	IsMO     *bool `json:"is_mo,omitempty"`
	HasInet  *bool `json:"has_inet,omitempty"`  // Any internet connectivity
	HasBinkp *bool `json:"has_binkp,omitempty"` // Determined from JSON: protocols.IBN or protocols.BND exist

	// Result options
	LatestOnly *bool `json:"latest_only,omitempty"`

	// Pagination
	Limit  int `json:"limit,omitempty"`
	Offset int `json:"offset,omitempty"`
}

// Point represents one FTN pointlist entry (FTS-5002).
// Points live in their own table: they have their own axes (list_source,
// per-source cadence) and would otherwise pollute every node-level stat.
type Point struct {
	// 4D identity: boss node + point number
	Zone     int `json:"zone"`
	Net      int `json:"net"`
	Node     int `json:"node"` // boss node number
	PointNum int `json:"point"`

	// FTN network this entry belongs to (fidonet, ...)
	Domain string `json:"domain,omitempty"`

	// Pointlist issue metadata
	PointlistDate time.Time `json:"pointlist_date"`
	DayNumber     int       `json:"day_number"`

	// Source provenance: which pointlist series this row came from.
	// Sources overlap (z2 aggregates the regionals) and publish on different
	// days; every file's rows are stored verbatim and readers resolve overlap
	// at query time via source_priority.
	ListSource     string `json:"list_source"`     // 'r24', 'z2', 'net244', 'nodelist', ...
	SourcePriority uint8  `json:"source_priority"` // 0 net-level, 10 regional, 20 zone rollup
	SourceFormat   string `json:"source_format"`   // 'boss', 'poss', 'pvt', 'point', 'fakenet'

	// Point information (same shape as a node line)
	SystemName string `json:"system_name"`
	Location   string `json:"location"`
	SysopName  string `json:"sysop_name"`
	Phone      string `json:"phone"`
	MaxSpeed   uint32 `json:"max_speed"`

	// Boolean flags (computed from raw flags)
	IsCM    bool `json:"is_cm"`
	IsMO    bool `json:"is_mo"`
	HasInet bool `json:"has_inet"`

	// Raw flag arrays
	Flags      []string `json:"flags"`
	ModemFlags []string `json:"modem_flags"`

	// Internet configuration JSON
	InternetConfig json.RawMessage `json:"internet_config,omitempty"`

	// Conflict tracking (same 4D address twice in ONE file)
	ConflictSequence int  `json:"conflict_sequence"`
	HasConflict      bool `json:"has_conflict"`

	// FTS identifier
	FtsId string `json:"fts_id"`

	// Raw pointlist line (decoded to UTF-8)
	RawLine string `json:"raw_line,omitempty"`
}

// ComputeFtsId generates the FTS identifier for this point:
// "z:n/n.p@date#seq", with an extra "@domain" suffix outside fidonet
// (mirrors Node.ComputeFtsId).
func (p *Point) ComputeFtsId() {
	p.FtsId = fmt.Sprintf("%d:%d/%d.%d@%s#%d",
		p.Zone, p.Net, p.Node, p.PointNum,
		p.PointlistDate.Format("2006-01-02"),
		p.ConflictSequence)
	if p.Domain != "" && p.Domain != DefaultDomain {
		p.FtsId += "@" + p.Domain
	}
}

// PointFilter represents search criteria for points
type PointFilter struct {
	// Address filters
	Zone     *int `json:"zone,omitempty"`
	Net      *int `json:"net,omitempty"`
	Node     *int `json:"node,omitempty"`
	PointNum *int `json:"point,omitempty"`

	// FTN network filter (nil = all networks)
	Domain *string `json:"domain,omitempty"`

	// Source filter
	ListSource *string `json:"list_source,omitempty"`

	// Date filters
	DateFrom *time.Time `json:"date_from,omitempty"`
	DateTo   *time.Time `json:"date_to,omitempty"`

	// Text filters
	SystemName *string `json:"system_name,omitempty"`
	Location   *string `json:"location,omitempty"`
	SysopName  *string `json:"sysop_name,omitempty"`

	// Result options
	LatestOnly *bool `json:"latest_only,omitempty"`

	// Pagination
	Limit  int `json:"limit,omitempty"`
	Offset int `json:"offset,omitempty"`
}

// PointlistFile describes one imported pointlist file (the import gate row)
type PointlistFile struct {
	Domain        string    `json:"domain"`
	ListSource    string    `json:"list_source"`
	PointlistDate time.Time `json:"pointlist_date"`
	DayNumber     int       `json:"day_number"`
	Filename      string    `json:"filename"`
	SourceFormat  string    `json:"source_format"`
	PointsCount   uint32    `json:"points_count"`
	BossesCount   uint32    `json:"bosses_count"`
	ImportedAt    time.Time `json:"imported_at"`
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
	HubNodes         int          `json:"hub_nodes"`
	ZoneNodes        int          `json:"zone_nodes"`
	RegionNodes      int          `json:"region_nodes"`
	HostNodes        int          `json:"host_nodes"`
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
