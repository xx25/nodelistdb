package storage

import (
	"strings"
)

// QueryBuilder provides ClickHouse-specific SQL query construction
type QueryBuilder struct {
}

// NewQueryBuilder creates a new QueryBuilder instance
func NewQueryBuilder() *QueryBuilder {
	return &QueryBuilder{}
}

// escapeSQL escapes strings for ClickHouse SQL literals
func (qb *QueryBuilder) escapeSQL(s string) string {
	// ClickHouse escape rules: single quotes are escaped with backslash
	s = strings.ReplaceAll(s, "\\", "\\\\") // Escape backslashes first
	s = strings.ReplaceAll(s, "'", "\\'")   // Escape single quotes
	return s
}
