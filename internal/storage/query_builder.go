package storage

import (
	"strings"

	"github.com/nodelistdb/internal/database"
)

// QueryBuilder provides base SQL query construction functionality
// This is the base builder that contains common utilities shared across all domain-specific builders
type QueryBuilder struct {
	// resultParser is used for formatting arrays and other ClickHouse-specific types
	resultParser *ResultParser
}

// NewQueryBuilder creates a new QueryBuilder instance
func NewQueryBuilder() *QueryBuilder {
	return &QueryBuilder{
		resultParser: NewClickHouseResultParser().ResultParser,
	}
}

// escapeSQL escapes strings for ClickHouse SQL literals
func (qb *QueryBuilder) escapeSQL(s string) string {
	// ClickHouse escape rules: single quotes are escaped with backslash
	s = strings.ReplaceAll(s, "\\", "\\\\") // Escape backslashes first
	s = strings.ReplaceAll(s, "'", "\\'")   // Escape single quotes
	return s
}

// Nodes returns a NodeQueryBuilder for node-related queries
func (qb *QueryBuilder) Nodes() *NodeQueryBuilder {
	return &NodeQueryBuilder{base: qb}
}

// Stats returns a StatsQueryBuilder for statistics queries
func (qb *QueryBuilder) Stats() *StatsQueryBuilder {
	return &StatsQueryBuilder{base: qb}
}

// Analytics returns an AnalyticsQueryBuilder for analytics queries
func (qb *QueryBuilder) Analytics() *AnalyticsQueryBuilder {
	return &AnalyticsQueryBuilder{base: qb}
}

// Dates returns a DateQueryBuilder for date-related queries
func (qb *QueryBuilder) Dates() *DateQueryBuilder {
	return &DateQueryBuilder{base: qb}
}

// Tests returns a TestQueryBuilder for test operations
func (qb *QueryBuilder) Tests(db database.DatabaseInterface) *TestQueryBuilder {
	return NewTestQueryBuilder(db)
}
