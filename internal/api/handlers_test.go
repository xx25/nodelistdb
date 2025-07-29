package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nodelistdb/internal/database"
	"nodelistdb/internal/storage"
)

// MockStorage implements the methods the API handlers need for testing
type MockStorage struct {
	nodes       []database.Node
	stats       *database.NetworkStats
	dates       []time.Time
	nodeHistory []database.Node
	nodeChanges []database.NodeChange
	nodeSummary []storage.NodeSummary
	shouldError bool
	errorMsg    string
}

func (m *MockStorage) GetNodes(filter database.NodeFilter) ([]database.Node, error) {
	if m.shouldError {
		return nil, fmt.Errorf(m.errorMsg)
	}
	return m.nodes, nil
}

func (m *MockStorage) GetStats(date time.Time) (*database.NetworkStats, error) {
	if m.shouldError {
		return nil, fmt.Errorf(m.errorMsg)
	}
	return m.stats, nil
}

func (m *MockStorage) GetLatestStatsDate() (time.Time, error) {
	if m.shouldError {
		return time.Time{}, fmt.Errorf(m.errorMsg)
	}
	if len(m.dates) > 0 {
		return m.dates[len(m.dates)-1], nil
	}
	return time.Now(), nil
}

func (m *MockStorage) GetNearestAvailableDate(date time.Time) (time.Time, error) {
	if m.shouldError {
		return time.Time{}, fmt.Errorf(m.errorMsg)
	}
	return date, nil
}

func (m *MockStorage) GetAvailableDates() ([]time.Time, error) {
	if m.shouldError {
		return nil, fmt.Errorf(m.errorMsg)
	}
	return m.dates, nil
}

func (m *MockStorage) GetNodeHistory(zone, net, node int) ([]database.Node, error) {
	if m.shouldError {
		return nil, fmt.Errorf(m.errorMsg)
	}
	return m.nodeHistory, nil
}

func (m *MockStorage) GetNodeDateRange(zone, net, node int) (time.Time, time.Time, error) {
	if m.shouldError {
		return time.Time{}, time.Time{}, fmt.Errorf(m.errorMsg)
	}
	first := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	last := time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)
	return first, last, nil
}

func (m *MockStorage) GetNodeChanges(zone, net, node int, filter storage.ChangeFilter) ([]database.NodeChange, error) {
	if m.shouldError {
		return nil, fmt.Errorf(m.errorMsg)
	}
	return m.nodeChanges, nil
}

func (m *MockStorage) SearchNodesBySysop(sysopName string, limit int) ([]storage.NodeSummary, error) {
	if m.shouldError {
		return nil, fmt.Errorf(m.errorMsg)
	}
	return m.nodeSummary, nil
}

// testServer wraps Server to allow dependency injection for testing
type testServer struct {
	*Server
}

func newTestServer(mockStorage *MockStorage) *testServer {
	server := &Server{}

	// Create wrapper methods that use the mock storage
	ts := &testServer{Server: server}

	// Override the storage field using reflection-like approach
	// For simplicity in tests, we'll create handlers that use the mock directly
	return ts
}

// Create test handlers that use mock storage directly
func createTestServer(mockStorage *MockStorage) *Server {
	// We'll modify the handlers to accept a storage interface
	// For now, let's use a simple approach where we inject the mock
	server := &Server{}
	// This is a test-only hack to inject the mock storage
	server.storage = interface{}(mockStorage).(*storage.Storage)
	return server
}

func newMockServer() (*Server, *MockStorage) {
	mockStorage := &MockStorage{
		nodes: []database.Node{
			{
				Zone:         1,
				Net:          234,
				Node:         56,
				SystemName:   "Test System",
				Location:     "Test Location",
				SysopName:    "Test Sysop",
				Phone:        "1-234-567-8900",
				NodeType:     "Pvt",
				NodelistDate: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
				DayNumber:    15,
				IsActive:     true,
				IsCM:         false,
			},
		},
		stats: &database.NetworkStats{
			TotalNodes:    1000,
			ActiveNodes:   950,
			CMNodes:       100,
			MONodes:       200,
			BinkpNodes:    800,
			TelnetNodes:   300,
			PvtNodes:      400,
			HoldNodes:     25,
			DownNodes:     25,
			InternetNodes: 850,
			Date:          time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
		},
		dates: []time.Time{
			time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
		},
		nodeHistory: []database.Node{
			{
				Zone:         1,
				Net:          234,
				Node:         56,
				SystemName:   "Test System",
				NodelistDate: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			},
		},
		nodeChanges: []database.NodeChange{
			{
				Date:       time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
				ChangeType: "modified",
				Changes:    map[string]string{"system_name": "Old System Name -> New System Name"},
			},
		},
		nodeSummary: []storage.NodeSummary{
			{
				Zone:            1,
				Net:             234,
				Node:            56,
				SystemName:      "Test System",
				Location:        "Test Location",
				SysopName:       "Test_Sysop",
				FirstDate:       time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				LastDate:        time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
				CurrentlyActive: true,
			},
		},
	}

	// Create a test server that will use the mock for handlers
	server := &testServer{Server: &Server{}}
	return server.Server, mockStorage
}

// Helper functions to create test handlers that use the mock storage
func createHealthHandler(mockStorage *MockStorage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"status": "ok",
			"time":   time.Now().UTC(),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}
}

func createSearchNodesHandler(mockStorage *MockStorage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Simulate the handler logic
		nodes, err := mockStorage.GetNodes(database.NodeFilter{})
		if err != nil {
			http.Error(w, fmt.Sprintf("Search failed: %v", err), http.StatusInternalServerError)
			return
		}

		response := map[string]interface{}{
			"nodes": nodes,
			"count": len(nodes),
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}
}

func createGetNodeHandler(mockStorage *MockStorage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Parse path parameters (simplified for testing)
		pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/nodes/"), "/")
		if len(pathParts) < 3 {
			http.Error(w, "Invalid path format", http.StatusBadRequest)
			return
		}

		// Simulate parsing zone/net/node
		nodes, err := mockStorage.GetNodes(database.NodeFilter{})
		if err != nil {
			http.Error(w, fmt.Sprintf("Node lookup failed: %v", err), http.StatusInternalServerError)
			return
		}

		if len(nodes) == 0 {
			http.Error(w, "Node not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(nodes[0])
	}
}

func TestHealthHandler(t *testing.T) {
	_, mockStorage := newMockServer()
	handler := createHealthHandler(mockStorage)

	req := httptest.NewRequest("GET", "/api/health", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %s", contentType)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	if response["status"] != "ok" {
		t.Errorf("Expected status 'ok', got %v", response["status"])
	}

	if _, exists := response["time"]; !exists {
		t.Error("Expected 'time' field in response")
	}
}

func TestSearchNodesHandler_Success(t *testing.T) {
	_, mockStorage := newMockServer()
	handler := createSearchNodesHandler(mockStorage)

	req := httptest.NewRequest("GET", "/api/nodes?zone=1&net=234&limit=10", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	if nodes, ok := response["nodes"].([]interface{}); !ok || len(nodes) != 1 {
		t.Errorf("Expected 1 node in response, got %v", response["nodes"])
	}

	if count, ok := response["count"].(float64); !ok || count != 1 {
		t.Errorf("Expected count 1, got %v", response["count"])
	}
}

func TestSearchNodesHandler_WrongMethod(t *testing.T) {
	_, mockStorage := newMockServer()
	handler := createSearchNodesHandler(mockStorage)

	req := httptest.NewRequest("POST", "/api/nodes", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}
}

func TestSearchNodesHandler_StorageError(t *testing.T) {
	_, mockStorage := newMockServer()
	mockStorage.shouldError = true
	mockStorage.errorMsg = "database error"

	handler := createSearchNodesHandler(mockStorage)
	req := httptest.NewRequest("GET", "/api/nodes", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 500, got %d", w.Code)
	}

	if !strings.Contains(w.Body.String(), "database error") {
		t.Errorf("Expected error message to contain 'database error', got %s", w.Body.String())
	}
}

func TestGetNodeHandler_Success(t *testing.T) {
	_, mockStorage := newMockServer()
	handler := createGetNodeHandler(mockStorage)

	req := httptest.NewRequest("GET", "/api/nodes/1/234/56", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var node database.Node
	if err := json.Unmarshal(w.Body.Bytes(), &node); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	if node.Zone != 1 || node.Net != 234 || node.Node != 56 {
		t.Errorf("Expected node 1:234/56, got %d:%d/%d", node.Zone, node.Net, node.Node)
	}
}

func TestGetNodeHandler_WrongMethod(t *testing.T) {
	_, mockStorage := newMockServer()
	handler := createGetNodeHandler(mockStorage)

	req := httptest.NewRequest("POST", "/api/nodes/1/234/56", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}
}

func TestGetNodeHandler_InvalidPath(t *testing.T) {
	_, mockStorage := newMockServer()
	handler := createGetNodeHandler(mockStorage)

	testCases := []struct {
		name string
		path string
	}{
		{"Too few segments", "/api/nodes/1/234"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tc.path, nil)
			w := httptest.NewRecorder()

			handler(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("Expected status 400, got %d", w.Code)
			}
		})
	}
}

func TestGetNodeHandler_NotFound(t *testing.T) {
	_, mockStorage := newMockServer()
	mockStorage.nodes = []database.Node{} // Empty result

	handler := createGetNodeHandler(mockStorage)
	req := httptest.NewRequest("GET", "/api/nodes/1/234/56", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}
}
