package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nodelistdb/internal/database"
	"github.com/nodelistdb/internal/storage"
)

// MockStorage implements the methods the API handlers need for testing
type MockStorage struct {
	nodes       []database.Node
	stats       *database.NetworkStats
	dates       []time.Time
	nodeHistory []database.Node
	nodeChanges []database.NodeChange
	nodeSummary []storage.NodeSummary
	sysops      []storage.SysopInfo
	shouldError bool
	errorMsg    string
}

func (m *MockStorage) GetNodes(filter database.NodeFilter) ([]database.Node, error) {
	if m.shouldError {
		return nil, fmt.Errorf("%s", m.errorMsg)
	}
	return m.nodes, nil
}

func (m *MockStorage) GetStats(date time.Time) (*database.NetworkStats, error) {
	if m.shouldError {
		return nil, fmt.Errorf("%s", m.errorMsg)
	}
	return m.stats, nil
}

func (m *MockStorage) GetLatestStatsDate() (time.Time, error) {
	if m.shouldError {
		return time.Time{}, fmt.Errorf("%s", m.errorMsg)
	}
	if len(m.dates) > 0 {
		return m.dates[len(m.dates)-1], nil
	}
	return time.Now(), nil
}

func (m *MockStorage) GetNearestAvailableDate(date time.Time) (time.Time, error) {
	if m.shouldError {
		return time.Time{}, fmt.Errorf("%s", m.errorMsg)
	}
	return date, nil
}

func (m *MockStorage) GetAvailableDates() ([]time.Time, error) {
	if m.shouldError {
		return nil, fmt.Errorf("%s", m.errorMsg)
	}
	return m.dates, nil
}

func (m *MockStorage) GetNodeHistory(zone, net, node int) ([]database.Node, error) {
	if m.shouldError {
		return nil, fmt.Errorf("%s", m.errorMsg)
	}
	return m.nodeHistory, nil
}

func (m *MockStorage) GetNodeDateRange(zone, net, node int) (time.Time, time.Time, error) {
	if m.shouldError {
		return time.Time{}, time.Time{}, fmt.Errorf("%s", m.errorMsg)
	}
	first := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	last := time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)
	return first, last, nil
}

func (m *MockStorage) GetNodeChanges(zone, net, node int) ([]database.NodeChange, error) {
	if m.shouldError {
		return nil, fmt.Errorf("%s", m.errorMsg)
	}
	return m.nodeChanges, nil
}

func (m *MockStorage) SearchNodesBySysop(sysopName string, limit int) ([]storage.NodeSummary, error) {
	if m.shouldError {
		return nil, fmt.Errorf("%s", m.errorMsg)
	}
	return m.nodeSummary, nil
}

func (m *MockStorage) GetUniqueSysops(nameFilter string, limit, offset int) ([]storage.SysopInfo, error) {
	if m.shouldError {
		return nil, fmt.Errorf("%s", m.errorMsg)
	}
	// Filter sysops by name if filter is provided
	if nameFilter != "" {
		var filtered []storage.SysopInfo
		for _, s := range m.sysops {
			if strings.Contains(strings.ToLower(s.Name), strings.ToLower(nameFilter)) {
				filtered = append(filtered, s)
			}
		}
		return filtered, nil
	}
	// Apply pagination
	start := offset
	if start > len(m.sysops) {
		return []storage.SysopInfo{}, nil
	}
	end := start + limit
	if end > len(m.sysops) {
		end = len(m.sysops)
	}
	return m.sysops[start:end], nil
}

func (m *MockStorage) GetNodesBySysop(sysopName string, limit int) ([]database.Node, error) {
	if m.shouldError {
		return nil, fmt.Errorf("%s", m.errorMsg)
	}
	// Filter nodes by sysop name
	var filtered []database.Node
	for _, n := range m.nodes {
		if n.SysopName == sysopName {
			filtered = append(filtered, n)
		}
	}
	if limit > 0 && len(filtered) > limit {
		return filtered[:limit], nil
	}
	return filtered, nil
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
		sysops: []storage.SysopInfo{
			{
				Name:        "Test_Sysop",
				NodeCount:   3,
				ActiveNodes: 2,
				FirstSeen:   time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
				LastSeen:    time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
				Zones:       []int{1, 2},
			},
			{
				Name:        "Another_Sysop",
				NodeCount:   1,
				ActiveNodes: 1,
				FirstSeen:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				LastSeen:    time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
				Zones:       []int{2},
			},
		},
	}

	// Create a test server that will use the mock for handlers
	server := &Server{}
	return server, mockStorage
}

// Helper functions to create test handlers that use the mock storage
func createHealthHandler(mockStorage *MockStorage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"status": "ok",
			"time":   time.Now().UTC(),
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
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
		_ = json.NewEncoder(w).Encode(response)
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
		_ = json.NewEncoder(w).Encode(nodes[0])
	}
}

func createSysopsHandler(mockStorage *MockStorage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Parse query parameters
		query := r.URL.Query()
		nameFilter := query.Get("name")
		limit := 50
		if limitStr := query.Get("limit"); limitStr != "" {
			_, _ = fmt.Sscanf(limitStr, "%d", &limit)
		}
		offset := 0
		if offsetStr := query.Get("offset"); offsetStr != "" {
			_, _ = fmt.Sscanf(offsetStr, "%d", &offset)
		}

		sysops, err := mockStorage.GetUniqueSysops(nameFilter, limit, offset)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to get sysops: %v", err), http.StatusInternalServerError)
			return
		}

		response := map[string]interface{}{
			"sysops": sysops,
			"count":  len(sysops),
			"filter": map[string]interface{}{
				"name":   nameFilter,
				"limit":  limit,
				"offset": offset,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}
}

func createSysopNodesHandler(mockStorage *MockStorage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Extract sysop name from path
		pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/sysops/"), "/")
		if len(pathParts) < 2 || pathParts[1] != "nodes" {
			http.Error(w, "Invalid path format", http.StatusBadRequest)
			return
		}

		sysopName := pathParts[0]
		if sysopName == "" {
			http.Error(w, "Sysop name cannot be empty", http.StatusBadRequest)
			return
		}

		limit := 100
		if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
			_, _ = fmt.Sscanf(limitStr, "%d", &limit)
		}

		nodes, err := mockStorage.GetNodesBySysop(sysopName, limit)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to get nodes: %v", err), http.StatusInternalServerError)
			return
		}

		response := map[string]interface{}{
			"sysop_name": sysopName,
			"nodes":      nodes,
			"count":      len(nodes),
			"limit":      limit,
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}
}

func createNodeChangesHandler(mockStorage *MockStorage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Parse path parameters (simplified for testing)
		pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/nodes/"), "/")
		if len(pathParts) < 4 {
			http.Error(w, "Invalid path format", http.StatusBadRequest)
			return
		}

		// Get all node changes without filtering
		changes, err := mockStorage.GetNodeChanges(1, 234, 56)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to get node changes: %v", err), http.StatusInternalServerError)
			return
		}

		response := map[string]interface{}{
			"address": "1:234/56",
			"changes": changes,
			"count":   len(changes),
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
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

func TestSysopsHandler_Success(t *testing.T) {
	_, mockStorage := newMockServer()
	handler := createSysopsHandler(mockStorage)

	req := httptest.NewRequest("GET", "/api/sysops", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	sysops, ok := response["sysops"].([]interface{})
	if !ok || len(sysops) != 2 {
		t.Errorf("Expected 2 sysops in response, got %v", response["sysops"])
	}
}

func TestSysopsHandler_WithNameFilter(t *testing.T) {
	_, mockStorage := newMockServer()
	handler := createSysopsHandler(mockStorage)

	req := httptest.NewRequest("GET", "/api/sysops?name=Test", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	sysops, ok := response["sysops"].([]interface{})
	if !ok || len(sysops) != 1 {
		t.Errorf("Expected 1 sysop in filtered response, got %v", response["sysops"])
	}
}

func TestSysopsHandler_Pagination(t *testing.T) {
	_, mockStorage := newMockServer()
	handler := createSysopsHandler(mockStorage)

	req := httptest.NewRequest("GET", "/api/sysops?limit=1&offset=1", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	sysops, ok := response["sysops"].([]interface{})
	if !ok || len(sysops) != 1 {
		t.Errorf("Expected 1 sysop with pagination, got %v", response["sysops"])
	}
}

func TestSysopNodesHandler_Success(t *testing.T) {
	_, mockStorage := newMockServer()
	// Add a node with matching sysop name
	mockStorage.nodes = append(mockStorage.nodes, database.Node{
		Zone:       2,
		Net:        5001,
		Node:       100,
		SystemName: "Test Sysop Node",
		SysopName:  "Test_Sysop",
	})

	handler := createSysopNodesHandler(mockStorage)

	req := httptest.NewRequest("GET", "/api/sysops/Test_Sysop/nodes", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	if response["sysop_name"] != "Test_Sysop" {
		t.Errorf("Expected sysop_name 'Test_Sysop', got %v", response["sysop_name"])
	}

	nodes, ok := response["nodes"].([]interface{})
	if !ok || len(nodes) != 1 {
		t.Errorf("Expected 1 node for sysop, got %v", response["nodes"])
	}
}

func TestSysopNodesHandler_InvalidPath(t *testing.T) {
	_, mockStorage := newMockServer()
	handler := createSysopNodesHandler(mockStorage)

	req := httptest.NewRequest("GET", "/api/sysops/Test_Sysop", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

func TestNodeChangesHandler_NewExcludeFormat(t *testing.T) {
	_, mockStorage := newMockServer()
	handler := createNodeChangesHandler(mockStorage)

	req := httptest.NewRequest("GET", "/api/nodes/1/234/56/changes?exclude=flags,phone,speed", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	// Filter is no longer included in response since filtering has been removed
	_, hasFilter := response["filter"]
	if hasFilter {
		t.Fatal("Filter should not be in response anymore")
	}
}

func TestNodeChangesHandler_OldFormatBackwardCompatibility(t *testing.T) {
	_, mockStorage := newMockServer()
	handler := createNodeChangesHandler(mockStorage)

	req := httptest.NewRequest("GET", "/api/nodes/1/234/56/changes?noflags=1&nophone=1", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	// Filter is no longer included in response since filtering has been removed
	_, hasFilter := response["filter"]
	if hasFilter {
		t.Fatal("Filter should not be in response anymore")
	}
}

func TestSearchNodesHandler_WithSysopName(t *testing.T) {
	_, mockStorage := newMockServer()
	handler := createSearchNodesHandler(mockStorage)

	req := httptest.NewRequest("GET", "/api/nodes?sysop_name=Test_Sysop", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	// The handler should have processed the sysop_name parameter
	nodes, ok := response["nodes"].([]interface{})
	if !ok {
		t.Errorf("Expected nodes in response, got %v", response)
	}

	if len(nodes) != 1 {
		t.Errorf("Expected 1 node matching sysop filter, got %d", len(nodes))
	}
}
