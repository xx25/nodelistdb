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
	"duckdb": func(config interface{}) (DatabaseInterface, error) {
		path, ok := config.(string)
		if !ok {
			return nil, fmt.Errorf("duckdb config must be a string path")
		}
		return New(path)
	},
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

// Ensure existing DB struct implements the interface
var _ DatabaseInterface = (*DB)(nil)

// Add Ping method to existing DB struct to satisfy interface
func (db *DB) Ping() error {
	db.mu.RLock()
	defer db.mu.RUnlock()
	
	if db.conn == nil {
		return fmt.Errorf("database connection is nil")
	}
	
	return db.conn.Ping()
}