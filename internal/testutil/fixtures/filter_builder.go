package fixtures

import (
	"time"

	"github.com/nodelistdb/internal/database"
)

// FilterBuilder provides a fluent API for building test filters
type FilterBuilder struct {
	filter database.NodeFilter
}

// NewFilterBuilder creates a new FilterBuilder
func NewFilterBuilder() *FilterBuilder {
	return &FilterBuilder{
		filter: database.NodeFilter{},
	}
}

// WithZone sets the zone filter
func (b *FilterBuilder) WithZone(zone int) *FilterBuilder {
	b.filter.Zone = &zone
	return b
}

// WithNet sets the net filter
func (b *FilterBuilder) WithNet(net int) *FilterBuilder {
	b.filter.Net = &net
	return b
}

// WithNode sets the node filter
func (b *FilterBuilder) WithNode(node int) *FilterBuilder {
	b.filter.Node = &node
	return b
}

// WithSystemName sets the system name filter
func (b *FilterBuilder) WithSystemName(name string) *FilterBuilder {
	b.filter.SystemName = &name
	return b
}

// WithLocation sets the location filter
func (b *FilterBuilder) WithLocation(location string) *FilterBuilder {
	b.filter.Location = &location
	return b
}

// WithSysopName sets the sysop name filter
func (b *FilterBuilder) WithSysopName(name string) *FilterBuilder {
	b.filter.SysopName = &name
	return b
}

// WithNodeType sets the node type filter
func (b *FilterBuilder) WithNodeType(nodeType string) *FilterBuilder {
	b.filter.NodeType = &nodeType
	return b
}

// WithDateFrom sets the date from filter
func (b *FilterBuilder) WithDateFrom(date time.Time) *FilterBuilder {
	b.filter.DateFrom = &date
	return b
}

// WithDateTo sets the date to filter
func (b *FilterBuilder) WithDateTo(date time.Time) *FilterBuilder {
	b.filter.DateTo = &date
	return b
}

// WithDateRange sets the date range filter
func (b *FilterBuilder) WithDateRange(start, end time.Time) *FilterBuilder {
	b.filter.DateFrom = &start
	b.filter.DateTo = &end
	return b
}

// WithLatestOnly sets the latest only filter
func (b *FilterBuilder) WithLatestOnly(latest bool) *FilterBuilder {
	b.filter.LatestOnly = &latest
	return b
}

// LatestOnly is a convenience method that sets LatestOnly to true
func (b *FilterBuilder) LatestOnly() *FilterBuilder {
	latest := true
	b.filter.LatestOnly = &latest
	return b
}

// WithLimit sets the limit
func (b *FilterBuilder) WithLimit(limit int) *FilterBuilder {
	b.filter.Limit = limit
	return b
}

// WithOffset sets the offset
func (b *FilterBuilder) WithOffset(offset int) *FilterBuilder {
	b.filter.Offset = offset
	return b
}


// WithCM filters for CM nodes
func (b *FilterBuilder) WithCM(isCM bool) *FilterBuilder {
	b.filter.IsCM = &isCM
	return b
}

// WithMO filters for MO nodes
func (b *FilterBuilder) WithMO(isMO bool) *FilterBuilder {
	b.filter.IsMO = &isMO
	return b
}

// WithInternet filters for internet-enabled nodes
func (b *FilterBuilder) WithInternet(hasInet bool) *FilterBuilder {
	b.filter.HasInet = &hasInet
	return b
}

// WithBinkP filters for BinkP-enabled nodes
func (b *FilterBuilder) WithBinkP(hasBinkp bool) *FilterBuilder {
	b.filter.HasBinkp = &hasBinkp
	return b
}

// Build returns the constructed filter
func (b *FilterBuilder) Build() database.NodeFilter {
	return b.filter
}

// Clone creates a new builder with a copy of the current filter
func (b *FilterBuilder) Clone() *FilterBuilder {
	filterCopy := b.filter
	return &FilterBuilder{filter: filterCopy}
}

// Common filter presets

// EmptyFilter returns an empty filter
func EmptyFilter() database.NodeFilter {
	return NewFilterBuilder().Build()
}

// LatestNodesFilter returns a filter for latest nodes only
func LatestNodesFilter() database.NodeFilter {
	return NewFilterBuilder().LatestOnly().Build()
}

// Zone2Filter returns a filter for zone 2 nodes
func Zone2Filter() database.NodeFilter {
	return NewFilterBuilder().WithZone(2).Build()
}

// InternetNodesFilter returns a filter for internet-enabled nodes
func InternetNodesFilter() database.NodeFilter {
	hasInet := true
	return NewFilterBuilder().
		WithInternet(hasInet).
		LatestOnly().
		Build()
}

// CMNodesFilter returns a filter for CM nodes
func CMNodesFilter() database.NodeFilter {
	isCM := true
	return NewFilterBuilder().
		WithCM(isCM).
		LatestOnly().
		Build()
}

// DateRangeFilter returns a filter for a specific date range
func DateRangeFilter(start, end time.Time) database.NodeFilter {
	return NewFilterBuilder().
		WithDateRange(start, end).
		Build()
}

// PaginatedFilter returns a filter with pagination
func PaginatedFilter(limit, offset int) database.NodeFilter {
	return NewFilterBuilder().
		WithLimit(limit).
		WithOffset(offset).
		Build()
}
