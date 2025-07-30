package storage

import (
	"fmt"
	"strings"

	"nodelistdb/internal/database"
)

// ClickHouseQueryBuilder provides ClickHouse-specific SQL query construction
type ClickHouseQueryBuilder struct {
	*QueryBuilder // Embed the base query builder for common functionality
}

// NewClickHouseQueryBuilder creates a new ClickHouse-specific QueryBuilder instance
func NewClickHouseQueryBuilder() *ClickHouseQueryBuilder {
	return &ClickHouseQueryBuilder{
		QueryBuilder: NewQueryBuilder(),
	}
}

// BuildDirectBatchInsertSQL creates a direct VALUES-based INSERT for ClickHouse with proper array handling
func (cqb *ClickHouseQueryBuilder) BuildDirectBatchInsertSQL(nodes []database.Node, rp *ResultParser) string {
	if len(nodes) == 0 {
		return ""
	}

	// Use ClickHouse-specific result parser for array formatting
	crp := NewClickHouseResultParser()

	var buf strings.Builder
	buf.WriteString(`INSERT INTO nodes (
		zone, net, node, nodelist_date, day_number,
		system_name, location, sysop_name, phone, node_type, region, max_speed,
		is_cm, is_mo, has_binkp, has_telnet, is_down, is_hold, is_pvt, is_active,
		flags, modem_flags, internet_protocols, internet_hostnames, internet_ports, internet_emails,
		conflict_sequence, has_conflict, has_inet, internet_config, fts_id
	) VALUES `)
	
	for i, node := range nodes {
		if i > 0 {
			buf.WriteByte(',')
		}
		
		// Compute FTS ID if not set
		if node.FtsId == "" {
			node.ComputeFtsId()
		}
		
		// Build direct VALUES clause for ClickHouse
		buf.WriteByte('(')
		
		// Core fields
		buf.WriteString(fmt.Sprintf("%d,%d,%d,'%s',%d,", 
			node.Zone, node.Net, node.Node, 
			node.NodelistDate.Format("2006-01-02"), node.DayNumber))
		
		// String fields (escaped)
		buf.WriteString(fmt.Sprintf("'%s','%s','%s','%s','%s',", 
			cqb.escapeClickHouseSQL(node.SystemName), cqb.escapeClickHouseSQL(node.Location), 
			cqb.escapeClickHouseSQL(node.SysopName), cqb.escapeClickHouseSQL(node.Phone), 
			cqb.escapeClickHouseSQL(node.NodeType)))
		
		// Region (nullable)
		if node.Region != nil {
			buf.WriteString(fmt.Sprintf("%d,", *node.Region))
		} else {
			buf.WriteString("NULL,")
		}
		
		// Max speed
		buf.WriteString(fmt.Sprintf("'%s',", cqb.escapeClickHouseSQL(node.MaxSpeed)))
		
		// Boolean flags
		buf.WriteString(fmt.Sprintf("%t,%t,%t,%t,%t,%t,%t,%t,", 
			node.IsCM, node.IsMO, node.HasBinkp, node.HasTelnet,
			node.IsDown, node.IsHold, node.IsPvt, node.IsActive))
		
		// Arrays (ClickHouse format)
		buf.WriteString(fmt.Sprintf("%s,%s,%s,%s,%s,%s,", 
			crp.formatArrayForDB(node.Flags),
			crp.formatArrayForDB(node.ModemFlags),
			crp.formatArrayForDB(node.InternetProtocols),
			crp.formatArrayForDB(node.InternetHostnames),
			crp.formatIntArrayForDB(node.InternetPorts),
			crp.formatArrayForDB(node.InternetEmails)))
		
		// Final fields
		buf.WriteString(fmt.Sprintf("%d,%t,%t,", 
			node.ConflictSequence, node.HasConflict, node.HasInet))
		
		// Internet config JSON
		if node.InternetConfig != nil && len(node.InternetConfig) > 0 {
			buf.WriteString(fmt.Sprintf("'%s',", cqb.escapeClickHouseSQL(string(node.InternetConfig))))
		} else {
			buf.WriteString("'{}',")
		}
		
		// FTS ID
		buf.WriteString(fmt.Sprintf("'%s')", cqb.escapeClickHouseSQL(node.FtsId)))
	}
	
	return buf.String()
}

// escapeClickHouseSQL escapes strings for ClickHouse SQL literals
func (cqb *ClickHouseQueryBuilder) escapeClickHouseSQL(s string) string {
	// ClickHouse escape rules: single quotes are escaped with backslash
	s = strings.ReplaceAll(s, "\\", "\\\\") // Escape backslashes first
	s = strings.ReplaceAll(s, "'", "\\'")   // Escape single quotes
	return s
}

// BuildClickHouseFTSQuery builds a ClickHouse-compatible FTS query
func (cqb *ClickHouseQueryBuilder) BuildClickHouseFTSQuery(filter database.NodeFilter) (string, []interface{}, bool) {
	// ClickHouse doesn't have FTS indexes like DuckDB, so we use LIKE with bloom filters
	var conditions []string
	var args []interface{}
	usedFTS := false

	baseQuery := cqb.NodeSelectSQL()

	// Text search using ClickHouse LIKE (bloom filter indexes help performance)
	if filter.SystemName != nil && *filter.SystemName != "" {
		conditions = append(conditions, "system_name ILIKE ?")
		args = append(args, "%"+*filter.SystemName+"%")
		usedFTS = true
	}

	if filter.Location != nil && *filter.Location != "" {
		conditions = append(conditions, "location ILIKE ?")
		args = append(args, "%"+*filter.Location+"%")
		usedFTS = true
	}

	if filter.SysopName != nil && *filter.SysopName != "" {
		conditions = append(conditions, "sysop_name ILIKE ?")
		args = append(args, "%"+*filter.SysopName+"%")
		usedFTS = true
	}

	// Add other non-text filters
	if filter.Zone != nil {
		conditions = append(conditions, "zone = ?")
		args = append(args, *filter.Zone)
	}

	if filter.Net != nil {
		conditions = append(conditions, "net = ?")
		args = append(args, *filter.Net)
	}

	if filter.Node != nil {
		conditions = append(conditions, "node = ?")
		args = append(args, *filter.Node)
	}

	if filter.NodeType != nil {
		conditions = append(conditions, "node_type = ?")
		args = append(args, *filter.NodeType)
	}

	// Boolean filters
	if filter.IsCM != nil {
		conditions = append(conditions, "is_cm = ?")
		args = append(args, *filter.IsCM)
	}

	if filter.IsMO != nil {
		conditions = append(conditions, "is_mo = ?")
		args = append(args, *filter.IsMO)
	}

	if filter.HasBinkp != nil {
		conditions = append(conditions, "has_binkp = ?")
		args = append(args, *filter.HasBinkp)
	}

	if filter.HasTelnet != nil {
		conditions = append(conditions, "has_telnet = ?")
		args = append(args, *filter.HasTelnet)
	}

	if filter.IsActive != nil {
		conditions = append(conditions, "is_active = ?")
		args = append(args, *filter.IsActive)
	}

	// Date filters
	if filter.DateFrom != nil {
		conditions = append(conditions, "nodelist_date >= ?")
		args = append(args, *filter.DateFrom)
	}

	if filter.DateTo != nil {
		conditions = append(conditions, "nodelist_date <= ?")
		args = append(args, *filter.DateTo)
	}

	// Latest only
	if filter.LatestOnly != nil && *filter.LatestOnly {
		conditions = append(conditions, "nodelist_date = (SELECT MAX(nodelist_date) FROM nodes)")
	}

	// Build final query
	query := baseQuery
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	// Order by for consistent results
	query += " ORDER BY zone, net, node, nodelist_date DESC, conflict_sequence"

	// Limit and offset
	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", filter.Limit)
		if filter.Offset > 0 {
			query += fmt.Sprintf(" OFFSET %d", filter.Offset)
		}
	}

	return query, args, usedFTS
}

// NodeSelectSQL returns the ClickHouse-compatible base SELECT statement for nodes
func (cqb *ClickHouseQueryBuilder) NodeSelectSQL() string {
	return `
	SELECT zone, net, node, nodelist_date, day_number,
		system_name, location, sysop_name, phone, node_type, region, max_speed,
		is_cm, is_mo, has_binkp, has_telnet, is_down, is_hold, is_pvt, is_active,
		flags, modem_flags, internet_protocols, internet_hostnames, internet_ports, internet_emails,
		conflict_sequence, has_conflict, has_inet, internet_config, fts_id
	FROM nodes`
}

// Implement QueryBuilderInterface methods by delegating to embedded QueryBuilder
// where appropriate, overriding only ClickHouse-specific methods

func (cqb *ClickHouseQueryBuilder) InsertNodeSQL() string {
	return cqb.QueryBuilder.InsertNodeSQL()
}

func (cqb *ClickHouseQueryBuilder) BuildBatchInsertSQL(batchSize int) string {
	return cqb.QueryBuilder.BuildBatchInsertSQL(batchSize)
}

func (cqb *ClickHouseQueryBuilder) BuildNodesQuery(filter database.NodeFilter) (string, []interface{}) {
	return cqb.QueryBuilder.BuildNodesQuery(filter)
}

func (cqb *ClickHouseQueryBuilder) BuildFTSQuery(filter database.NodeFilter) (string, []interface{}, bool) {
	// Use ClickHouse-specific FTS query
	return cqb.BuildClickHouseFTSQuery(filter)
}

func (cqb *ClickHouseQueryBuilder) StatsSQL() string {
	return cqb.QueryBuilder.StatsSQL()
}

func (cqb *ClickHouseQueryBuilder) ZoneDistributionSQL() string {
	return cqb.QueryBuilder.ZoneDistributionSQL()
}

func (cqb *ClickHouseQueryBuilder) LargestRegionsSQL() string {
	return cqb.QueryBuilder.LargestRegionsSQL()
}

func (cqb *ClickHouseQueryBuilder) LargestNetsSQL() string {
	return cqb.QueryBuilder.LargestNetsSQL()
}

func (cqb *ClickHouseQueryBuilder) OptimizedLargestRegionsSQL() string {
	return cqb.QueryBuilder.OptimizedLargestRegionsSQL()
}

func (cqb *ClickHouseQueryBuilder) OptimizedLargestNetsSQL() string {
	return cqb.QueryBuilder.OptimizedLargestNetsSQL()
}

func (cqb *ClickHouseQueryBuilder) NodeHistorySQL() string {
	return cqb.QueryBuilder.NodeHistorySQL()
}

func (cqb *ClickHouseQueryBuilder) NodeDateRangeSQL() string {
	return cqb.QueryBuilder.NodeDateRangeSQL()
}

func (cqb *ClickHouseQueryBuilder) SysopSearchSQL() string {
	return cqb.QueryBuilder.SysopSearchSQL()
}

func (cqb *ClickHouseQueryBuilder) ConflictCheckSQL() string {
	return cqb.QueryBuilder.ConflictCheckSQL()
}

func (cqb *ClickHouseQueryBuilder) MarkConflictSQL() string {
	return cqb.QueryBuilder.MarkConflictSQL()
}

func (cqb *ClickHouseQueryBuilder) IsProcessedSQL() string {
	return cqb.QueryBuilder.IsProcessedSQL()
}

func (cqb *ClickHouseQueryBuilder) LatestDateSQL() string {
	return cqb.QueryBuilder.LatestDateSQL()
}

func (cqb *ClickHouseQueryBuilder) AvailableDatesSQL() string {
	return cqb.QueryBuilder.AvailableDatesSQL()
}

func (cqb *ClickHouseQueryBuilder) ExactDateExistsSQL() string {
	return cqb.QueryBuilder.ExactDateExistsSQL()
}

func (cqb *ClickHouseQueryBuilder) NearestDateBeforeSQL() string {
	return cqb.QueryBuilder.NearestDateBeforeSQL()
}

func (cqb *ClickHouseQueryBuilder) NearestDateAfterSQL() string {
	return cqb.QueryBuilder.NearestDateAfterSQL()
}

func (cqb *ClickHouseQueryBuilder) ConsecutiveNodelistCheckSQL() string {
	return cqb.QueryBuilder.ConsecutiveNodelistCheckSQL()
}

func (cqb *ClickHouseQueryBuilder) NextNodelistDateSQL() string {
	return cqb.QueryBuilder.NextNodelistDateSQL()
}