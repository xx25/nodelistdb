package mocks

import (
	"context"
	"sync"

	"github.com/nodelistdb/internal/testing/models"
	"github.com/nodelistdb/internal/testing/protocols"
)

// MockStorage is a mock implementation of the Storage interface
type MockStorage struct {
	mu sync.Mutex

	StoreTestResultFunc    func(ctx context.Context, result *models.TestResult) error
	StoreTestResultsFunc   func(ctx context.Context, results []*models.TestResult) error
	GetNodesForTestingFunc func(ctx context.Context) ([]*models.Node, error)

	StoreCalls    []*models.TestResult
	StoreCallsErr error
}

func (m *MockStorage) StoreTestResult(ctx context.Context, result *models.TestResult) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.StoreCalls = append(m.StoreCalls, result)

	if m.StoreTestResultFunc != nil {
		return m.StoreTestResultFunc(ctx, result)
	}
	return m.StoreCallsErr
}

func (m *MockStorage) StoreTestResults(ctx context.Context, results []*models.TestResult) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.StoreCalls = append(m.StoreCalls, results...)

	if m.StoreTestResultsFunc != nil {
		return m.StoreTestResultsFunc(ctx, results)
	}
	return m.StoreCallsErr
}

func (m *MockStorage) GetNodesForTesting(ctx context.Context) ([]*models.Node, error) {
	if m.GetNodesForTestingFunc != nil {
		return m.GetNodesForTestingFunc(ctx)
	}
	return []*models.Node{}, nil
}

func (m *MockStorage) GetStoreCalls() []*models.TestResult {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make([]*models.TestResult, len(m.StoreCalls))
	copy(result, m.StoreCalls)
	return result
}

// MockDNSResolver is a mock implementation of DNSResolver
type MockDNSResolver struct {
	ResolveFunc func(ctx context.Context, hostname string) *models.DNSResult
	ResolveCalls []string
}

func (m *MockDNSResolver) Resolve(ctx context.Context, hostname string) *models.DNSResult {
	m.ResolveCalls = append(m.ResolveCalls, hostname)

	if m.ResolveFunc != nil {
		return m.ResolveFunc(ctx, hostname)
	}

	// Default: successful resolution
	return &models.DNSResult{
		Hostname:      hostname,
		IPv4Addresses: []string{"192.0.2.1"},
		IPv6Addresses: []string{"2001:db8::1"},
		Error:         nil,
	}
}

// MockProtocolTester is a mock implementation of protocol testers
type MockProtocolTester struct {
	TestFunc func(ctx context.Context, host string, port int, addr string) protocols.TestResult
	TestCalls []TestCall
}

type TestCall struct {
	Host string
	Port int
	Addr string
}

func (m *MockProtocolTester) Test(ctx context.Context, host string, port int, expectedAddress string) protocols.TestResult {
	m.TestCalls = append(m.TestCalls, TestCall{Host: host, Port: port, Addr: expectedAddress})

	if m.TestFunc != nil {
		return m.TestFunc(ctx, host, port, expectedAddress)
	}

	// Default: successful test
	return &protocols.BinkPTestResult{
		BaseTestResult: protocols.BaseTestResult{
			Success:    true,
			Error:      "",
			ResponseMs: 100,
		},
		SystemName:   "Test System",
		Sysop:        "Test Sysop",
		Location:     "Test Location",
		Version:      "1.0",
		Addresses:    []string{expectedAddress},
		AddressValid: true,
	}
}

func (m *MockProtocolTester) GetProtocolName() string {
	return "Mock"
}

// MockGeolocation is a mock implementation of Geolocation service
type MockGeolocation struct {
	GetLocationFunc func(ctx context.Context, ip string) *models.GeolocationResult
	GetLocationCalls []string
}

func (m *MockGeolocation) GetLocation(ctx context.Context, ip string) *models.GeolocationResult {
	m.GetLocationCalls = append(m.GetLocationCalls, ip)

	if m.GetLocationFunc != nil {
		return m.GetLocationFunc(ctx, ip)
	}

	// Default: US location
	return &models.GeolocationResult{
		IP:          ip,
		Country:     "United States",
		CountryCode: "US",
		City:        "New York",
		Region:      "NY",
		Latitude:    40.7128,
		Longitude:   -74.0060,
		ISP:         "Test ISP",
		Org:         "Test Org",
		ASN:         12345,
		Source:      "mock",
	}
}

// MockScheduler is a mock implementation of Scheduler
type MockScheduler struct {
	ShouldTestFunc func(node *models.Node, lastResult *models.TestResult) bool
	RecordResultFunc func(node *models.Node, result *models.TestResult)
}

func (m *MockScheduler) ShouldTest(node *models.Node, lastResult *models.TestResult) bool {
	if m.ShouldTestFunc != nil {
		return m.ShouldTestFunc(node, lastResult)
	}
	return true
}

func (m *MockScheduler) RecordResult(node *models.Node, result *models.TestResult) {
	if m.RecordResultFunc != nil {
		m.RecordResultFunc(node, result)
	}
}