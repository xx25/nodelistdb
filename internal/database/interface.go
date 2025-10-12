package database

import (
	"database/sql"
	"fmt"
)

// DatabaseInterface defines the common interface for all database backends
type DatabaseInterface interface {
	// Connection management
	Close() error
	Conn() *sql.DB

	// Schema management
	CreateSchema() error
	GetVersion() (string, error)

	// Full-Text Search
	CreateFTSIndexes() error
	DropFTSIndexes() error

	// Health check
	Ping() error
}

// Factory function type for creating database instances
type DatabaseFactory func(config interface{}) (DatabaseInterface, error)

// DatabaseRegistry holds factory functions for different database types
var DatabaseRegistry = map[string]DatabaseFactory{
	// ClickHouse factory is registered in clickhouse.go
}

// RegisterDatabase registers a new database type with its factory function
func RegisterDatabase(dbType string, factory DatabaseFactory) {
	DatabaseRegistry[dbType] = factory
}

// CreateDatabase creates a database instance based on type and configuration
func CreateDatabase(dbType string, config interface{}) (DatabaseInterface, error) {
	factory, exists := DatabaseRegistry[dbType]
	if !exists {
		return nil, fmt.Errorf("unsupported database type: %s", dbType)
	}

	return factory(config)
}
