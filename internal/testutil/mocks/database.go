package mocks

import (
	"database/sql"
	"fmt"
	"sync"

	"github.com/nodelistdb/internal/database"
)

// MockDatabase is a mock implementation of DatabaseInterface for testing
type MockDatabase struct {
	mu sync.RWMutex

	// Mock data
	version    string
	pingError  error
	closeError error
	conn       *sql.DB

	// Call tracking
	CreateSchemaCalled     bool
	CreateFTSIndexesCalled bool
	DropFTSIndexesCalled   bool
	PingCalled             bool
	CloseCalled            bool
	GetVersionCalled       bool

	// Configurable behavior
	ShouldFailCreateSchema     bool
	ShouldFailCreateFTSIndexes bool
	ShouldFailDropFTSIndexes   bool
	ShouldFailPing             bool
	ShouldFailClose            bool
	ShouldFailGetVersion       bool
}

// NewMockDatabase creates a new mock database
func NewMockDatabase() *MockDatabase {
	return &MockDatabase{
		version: "test-version-1.0.0",
	}
}

// Close mocks the Close method
func (m *MockDatabase) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.CloseCalled = true

	if m.ShouldFailClose {
		return fmt.Errorf("mock close error")
	}

	return m.closeError
}

// Conn mocks the Conn method
func (m *MockDatabase) Conn() *sql.DB {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.conn
}

// CreateSchema mocks the CreateSchema method
func (m *MockDatabase) CreateSchema() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.CreateSchemaCalled = true

	if m.ShouldFailCreateSchema {
		return fmt.Errorf("mock create schema error")
	}

	return nil
}

// GetVersion mocks the GetVersion method
func (m *MockDatabase) GetVersion() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.GetVersionCalled = true

	if m.ShouldFailGetVersion {
		return "", fmt.Errorf("mock get version error")
	}

	return m.version, nil
}

// CreateFTSIndexes mocks the CreateFTSIndexes method
func (m *MockDatabase) CreateFTSIndexes() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.CreateFTSIndexesCalled = true

	if m.ShouldFailCreateFTSIndexes {
		return fmt.Errorf("mock create FTS indexes error")
	}

	return nil
}

// DropFTSIndexes mocks the DropFTSIndexes method
func (m *MockDatabase) DropFTSIndexes() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.DropFTSIndexesCalled = true

	if m.ShouldFailDropFTSIndexes {
		return fmt.Errorf("mock drop FTS indexes error")
	}

	return nil
}

// Ping mocks the Ping method
func (m *MockDatabase) Ping() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.PingCalled = true

	if m.ShouldFailPing {
		return fmt.Errorf("mock ping error")
	}

	return m.pingError
}

// SetVersion sets the version returned by GetVersion
func (m *MockDatabase) SetVersion(version string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.version = version
}

// SetConn sets the connection returned by Conn
func (m *MockDatabase) SetConn(conn *sql.DB) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.conn = conn
}

// SetPingError sets the error returned by Ping
func (m *MockDatabase) SetPingError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pingError = err
}

// SetCloseError sets the error returned by Close
func (m *MockDatabase) SetCloseError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closeError = err
}

// Reset resets all call tracking flags
func (m *MockDatabase) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.CreateSchemaCalled = false
	m.CreateFTSIndexesCalled = false
	m.DropFTSIndexesCalled = false
	m.PingCalled = false
	m.CloseCalled = false
	m.GetVersionCalled = false

	m.ShouldFailCreateSchema = false
	m.ShouldFailCreateFTSIndexes = false
	m.ShouldFailDropFTSIndexes = false
	m.ShouldFailPing = false
	m.ShouldFailClose = false
	m.ShouldFailGetVersion = false
}

// ClickHouseConfig returns a test ClickHouse configuration pointer
func ClickHouseConfig() *database.ClickHouseConfig {
	return &database.ClickHouseConfig{
		Host:     "localhost",
		Port:     9000,
		Database: "nodelistdb_test",
		Username: "default",
		Password: "",
	}
}
