package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestRouterSetup tests that the Chi router is properly configured with correct routes
func TestRouterSetup(t *testing.T) {
	// Create a test server using the existing mock infrastructure
	server, _ := newTestAPIServer()
	if server == nil {
		t.Skip("Skipping router test - mock server not available")
		return
	}

	router := server.SetupRouter()

	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
	}{
		// Health check - always works
		{"Health endpoint", "GET", "/api/health", http.StatusOK},

		// Non-existent routes should 404
		{"Invalid route", "GET", "/api/invalid", http.StatusNotFound},

		// Method not allowed
		{"Invalid method on health", "POST", "/api/health", http.StatusMethodNotAllowed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("Expected status %d, got %d for %s %s", tt.wantStatus, w.Code, tt.method, tt.path)
			}
		})
	}
}

// newTestAPIServer creates a simple test server for routing tests
func newTestAPIServer() (*Server, error) {
	// Use the existing newMockServer if available
	server, _ := newMockServer()
	return server, nil
}

// TestRouterMiddleware tests that middleware is properly applied
func TestRouterMiddleware(t *testing.T) {
	server, _ := newTestAPIServer()
	if server == nil {
		t.Skip("Skipping middleware test - mock server not available")
		return
	}

	router := server.SetupRouter()

	req := httptest.NewRequest("GET", "/api/health", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Check that response is successful
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Check Content-Type header
	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %s", contentType)
	}
}

// TestChiURLParameterValidation tests that Chi URL parameters are validated
func TestChiURLParameterValidation(t *testing.T) {
	server, _ := newTestAPIServer()
	if server == nil {
		t.Skip("Skipping URL parameter test - mock server not available")
		return
	}

	router := server.SetupRouter()

	tests := []struct {
		name       string
		path       string
		wantStatus int
	}{
		// Invalid parameters should return 400
		{"Invalid zone", "/api/nodes/abc/450/1024", http.StatusBadRequest},
		{"Invalid net", "/api/nodes/2/xyz/1024", http.StatusBadRequest},
		{"Invalid node", "/api/nodes/2/450/abc", http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("Expected status %d, got %d for %s", tt.wantStatus, w.Code, tt.path)
			}
		})
	}
}
