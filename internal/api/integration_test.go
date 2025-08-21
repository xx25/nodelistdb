package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nodelistdb/internal/database"
)

func TestAPIIntegration_BasicFlow(t *testing.T) {
	// Create mock storage
	_, mockStorage := newMockServer()

	// Test health endpoint
	t.Run("Health Check", func(t *testing.T) {
		handler := createHealthHandler(mockStorage)
		req := httptest.NewRequest("GET", "/api/health", nil)
		w := httptest.NewRecorder()

		handler(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}

		var health map[string]interface{}
		if err := json.NewDecoder(w.Body).Decode(&health); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if health["status"] != "ok" {
			t.Errorf("Expected status 'ok', got %v", health["status"])
		}
	})

	// Test search nodes endpoint
	t.Run("Search Nodes", func(t *testing.T) {
		handler := createSearchNodesHandler(mockStorage)
		req := httptest.NewRequest("GET", "/api/nodes?zone=1&net=234", nil)
		w := httptest.NewRecorder()

		handler(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}

		var searchResp map[string]interface{}
		if err := json.NewDecoder(w.Body).Decode(&searchResp); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if nodes, ok := searchResp["nodes"].([]interface{}); !ok || len(nodes) != 1 {
			t.Errorf("Expected 1 node, got %v", searchResp["nodes"])
		}
	})

	// Test get specific node
	t.Run("Get Specific Node", func(t *testing.T) {
		handler := createGetNodeHandler(mockStorage)
		req := httptest.NewRequest("GET", "/api/nodes/1/234/56", nil)
		w := httptest.NewRecorder()

		handler(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}

		var node database.Node
		if err := json.NewDecoder(w.Body).Decode(&node); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if node.Zone != 1 || node.Net != 234 || node.Node != 56 {
			t.Errorf("Expected node 1:234/56, got %d:%d/%d", node.Zone, node.Net, node.Node)
		}
	})
}

func TestAPIIntegration_ErrorHandling(t *testing.T) {
	// Create mock storage that returns errors
	_, mockStorage := newMockServer()
	mockStorage.shouldError = true
	mockStorage.errorMsg = "integration test error"

	testCases := []struct {
		name           string
		handlerCreator func(*MockStorage) http.HandlerFunc
		path           string
		expectedStatus int
	}{
		{"Search Nodes Error", createSearchNodesHandler, "/api/nodes", http.StatusInternalServerError},
		{"Get Node Error", createGetNodeHandler, "/api/nodes/1/234/56", http.StatusInternalServerError},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			handler := tc.handlerCreator(mockStorage)
			req := httptest.NewRequest("GET", tc.path, nil)
			w := httptest.NewRecorder()

			handler(w, req)

			if w.Code != tc.expectedStatus {
				t.Errorf("Expected status %d, got %d", tc.expectedStatus, w.Code)
			}
		})
	}
}

func TestAPIIntegration_HTTPMethods(t *testing.T) {
	_, mockStorage := newMockServer()

	testCases := []struct {
		name           string
		handlerCreator func(*MockStorage) http.HandlerFunc
		path           string
		method         string
		expected       int
	}{
		// Valid GET requests
		{"Search GET", createSearchNodesHandler, "/api/nodes", "GET", http.StatusOK},
		{"Node GET", createGetNodeHandler, "/api/nodes/1/234/56", "GET", http.StatusOK},

		// Invalid methods
		{"Search POST", createSearchNodesHandler, "/api/nodes", "POST", http.StatusMethodNotAllowed},
		{"Node POST", createGetNodeHandler, "/api/nodes/1/234/56", "POST", http.StatusMethodNotAllowed},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			handler := tc.handlerCreator(mockStorage)
			req := httptest.NewRequest(tc.method, tc.path, nil)
			w := httptest.NewRecorder()

			handler(w, req)

			if w.Code != tc.expected {
				t.Errorf("Expected status %d, got %d", tc.expected, w.Code)
			}
		})
	}
}

func TestAPIIntegration_ContentTypes(t *testing.T) {
	_, mockStorage := newMockServer()

	testCases := []struct {
		name           string
		handlerCreator func(*MockStorage) http.HandlerFunc
		path           string
		expectedCT     string
	}{
		{"Health", createHealthHandler, "/api/health", "application/json"},
		{"Search Nodes", createSearchNodesHandler, "/api/nodes", "application/json"},
		{"Get Node", createGetNodeHandler, "/api/nodes/1/234/56", "application/json"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			handler := tc.handlerCreator(mockStorage)
			req := httptest.NewRequest("GET", tc.path, nil)
			w := httptest.NewRecorder()

			handler(w, req)

			contentType := w.Header().Get("Content-Type")
			if contentType != tc.expectedCT {
				t.Errorf("Expected Content-Type %s, got %s", tc.expectedCT, contentType)
			}
		})
	}
}

func TestAPIIntegration_LargeResponses(t *testing.T) {
	// Create large dataset
	largeNodes := make([]database.Node, 100)
	for i := 0; i < 100; i++ {
		largeNodes[i] = database.Node{
			Zone:         1,
			Net:          i,
			Node:         1,
			SystemName:   "Test System",
			NodelistDate: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
		}
	}

	_, mockStorage := newMockServer()
	mockStorage.nodes = largeNodes

	t.Run("Large Node Search", func(t *testing.T) {
		handler := createSearchNodesHandler(mockStorage)
		req := httptest.NewRequest("GET", "/api/nodes", nil)
		w := httptest.NewRecorder()

		handler(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}

		var searchResp map[string]interface{}
		if err := json.NewDecoder(w.Body).Decode(&searchResp); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if nodes, ok := searchResp["nodes"].([]interface{}); !ok || len(nodes) != 100 {
			t.Errorf("Expected 100 nodes, got %d", len(nodes))
		}
	})
}
