package containers

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/nodelistdb/internal/database"
)

// ClickHouseContainer represents a test ClickHouse container
type ClickHouseContainer struct {
	Host     string
	Port     int
	Database string
	Config   database.ClickHouseConfig
	cleanup  func()
}

// NewClickHouseContainer creates a new ClickHouse test container
// Note: This is a placeholder implementation. For full testcontainer support,
// add testcontainers-go dependency and implement proper Docker container management.
func NewClickHouseContainer(t *testing.T) (*ClickHouseContainer, error) {
	t.Helper()

	// Check if we should skip Docker-based tests
	if testing.Short() {
		t.Skip("Skipping ClickHouse container test in short mode")
	}

	// For now, use environment variable or localhost
	// In production implementation, this would start a Docker container
	host := "localhost"
	port := 9000
	dbName := fmt.Sprintf("nodelistdb_test_%d", time.Now().Unix())

	config := database.ClickHouseConfig{
		Host:     host,
		Port:     port,
		Database: dbName,
		Username: "default",
		Password: "",
	}

	container := &ClickHouseContainer{
		Host:     host,
		Port:     port,
		Database: dbName,
		Config:   config,
	}

	// Setup cleanup
	container.cleanup = func() {
		// Clean up test database
		if db, err := database.NewClickHouse(&config); err == nil {
			conn := db.Conn()
			if conn != nil {
				_, _ = conn.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", dbName))
			}
			db.Close()
		}
	}

	// Create test database
	if err := container.createTestDatabase(); err != nil {
		container.cleanup()
		return nil, fmt.Errorf("failed to create test database: %w", err)
	}

	return container, nil
}

// createTestDatabase creates the test database
func (c *ClickHouseContainer) createTestDatabase() error {
	// Connect to default database to create test database
	defaultConfig := c.Config
	defaultConfig.Database = "default"

	// Use NewClickHouse directly instead of factory
	db, err := database.NewClickHouse(&defaultConfig)
	if err != nil {
		return fmt.Errorf("failed to connect to ClickHouse: %w", err)
	}
	defer db.Close()

	conn := db.Conn()
	if conn == nil {
		return fmt.Errorf("failed to get database connection")
	}

	// Create test database
	_, err = conn.Exec(fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s", c.Database))
	if err != nil {
		return fmt.Errorf("failed to create database: %w", err)
	}

	return nil
}

// GetDB returns a database connection to the test container
func (c *ClickHouseContainer) GetDB() (database.DatabaseInterface, error) {
	db, err := database.NewClickHouse(&c.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to test database: %w", err)
	}

	// Create schema
	if err := db.CreateSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create schema: %w", err)
	}

	return db, nil
}

// Cleanup cleans up the test container
func (c *ClickHouseContainer) Cleanup() {
	if c.cleanup != nil {
		c.cleanup()
	}
}

// WithClickHouse is a test helper that sets up a ClickHouse container,
// runs the test function, and cleans up
func WithClickHouse(t *testing.T, fn func(t *testing.T, db database.DatabaseInterface)) {
	t.Helper()

	container, err := NewClickHouseContainer(t)
	if err != nil {
		t.Fatalf("Failed to create ClickHouse container: %v", err)
	}
	defer container.Cleanup()

	db, err := container.GetDB()
	if err != nil {
		t.Fatalf("Failed to get database: %v", err)
	}
	defer db.Close()

	fn(t, db)
}

// WaitForClickHouse waits for ClickHouse to be ready
func WaitForClickHouse(config database.ClickHouseConfig, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for ClickHouse")
		case <-ticker.C:
			db, err := database.NewClickHouse(&config)
			if err != nil {
				continue
			}

			if err := db.Ping(); err != nil {
				db.Close()
				continue
			}

			db.Close()
			return nil
		}
	}
}

// TODO: Full testcontainer implementation example
//
// import (
//     "github.com/testcontainers/testcontainers-go"
//     "github.com/testcontainers/testcontainers-go/modules/clickhouse"
// )
//
// func NewClickHouseContainerFull(t *testing.T) (*ClickHouseContainer, error) {
//     ctx := context.Background()
//
//     clickhouseContainer, err := clickhouse.RunContainer(ctx,
//         testcontainers.WithImage("clickhouse/clickhouse-server:latest"),
//         clickhouse.WithUsername("default"),
//         clickhouse.WithPassword(""),
//         clickhouse.WithDatabase("nodelistdb_test"),
//     )
//     if err != nil {
//         return nil, err
//     }
//
//     host, err := clickhouseContainer.Host(ctx)
//     if err != nil {
//         return nil, err
//     }
//
//     port, err := clickhouseContainer.MappedPort(ctx, "9000")
//     if err != nil {
//         return nil, err
//     }
//
//     return &ClickHouseContainer{
//         Host:     host,
//         Port:     port.Int(),
//         Database: "nodelistdb_test",
//         cleanup: func() {
//             clickhouseContainer.Terminate(ctx)
//         },
//     }, nil
// }
