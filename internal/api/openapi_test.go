package api

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAPISpecHandler(t *testing.T) {
	server := &Server{}

	req := httptest.NewRequest("GET", "/api/openapi.yaml", nil)
	w := httptest.NewRecorder()

	server.OpenAPISpecHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/x-yaml" {
		t.Errorf("Expected Content-Type application/x-yaml, got %s", contentType)
	}

	body := w.Body.String()
	if !strings.Contains(body, "openapi: 3.0.3") {
		t.Error("Expected OpenAPI spec to contain version 3.0.3")
	}

	if !strings.Contains(body, "NodelistDB API") {
		t.Error("Expected OpenAPI spec to contain title 'NodelistDB API'")
	}
}

func TestOpenAPISpecHandler_CORS(t *testing.T) {
	server := &Server{}

	req := httptest.NewRequest("OPTIONS", "/api/openapi.yaml", nil)
	w := httptest.NewRecorder()

	server.OpenAPISpecHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	corsOrigin := w.Header().Get("Access-Control-Allow-Origin")
	if corsOrigin != "*" {
		t.Errorf("Expected CORS origin *, got %s", corsOrigin)
	}

	corsMethods := w.Header().Get("Access-Control-Allow-Methods")
	if !strings.Contains(corsMethods, "GET") {
		t.Error("Expected CORS methods to contain GET")
	}
}

func TestSwaggerUIHandler(t *testing.T) {
	server := &Server{}

	req := httptest.NewRequest("GET", "/api/docs", nil)
	req.Host = "localhost:8080"
	w := httptest.NewRecorder()

	server.SwaggerUIHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if !strings.Contains(contentType, "text/html") {
		t.Errorf("Expected Content-Type to contain text/html, got %s", contentType)
	}

	body := w.Body.String()
	if !strings.Contains(body, "SwaggerUIBundle") {
		t.Error("Expected Swagger UI HTML to contain SwaggerUIBundle")
	}

	if !strings.Contains(body, "NodelistDB API Documentation") {
		t.Error("Expected page title to contain 'NodelistDB API Documentation'")
	}

	// Check that the spec URL is correctly embedded
	if !strings.Contains(body, "http://localhost:8080/api/openapi.yaml") {
		t.Error("Expected Swagger UI to reference the correct OpenAPI spec URL")
	}
}

func TestSwaggerUIHandler_HTTPS_Detection(t *testing.T) {
	server := &Server{}

	testCases := []struct {
		name           string
		setupRequest   func(*http.Request)
		expectedScheme string
	}{
		{
			name: "Direct HTTPS",
			setupRequest: func(req *http.Request) {
				// Simulate direct HTTPS connection
				req.TLS = &tls.ConnectionState{}
			},
			expectedScheme: "https",
		},
		{
			name: "Proxy X-Forwarded-Proto",
			setupRequest: func(req *http.Request) {
				req.Header.Set("X-Forwarded-Proto", "https")
			},
			expectedScheme: "https",
		},
		{
			name: "Proxy X-Forwarded-Ssl",
			setupRequest: func(req *http.Request) {
				req.Header.Set("X-Forwarded-Ssl", "on")
			},
			expectedScheme: "https",
		},
		{
			name: "Proxy X-Url-Scheme",
			setupRequest: func(req *http.Request) {
				req.Header.Set("X-Url-Scheme", "https")
			},
			expectedScheme: "https",
		},
		{
			name: "RFC 7239 Forwarded Header",
			setupRequest: func(req *http.Request) {
				req.Header.Set("Forwarded", "for=192.0.2.60;proto=https;by=203.0.113.43")
			},
			expectedScheme: "https",
		},
		{
			name: "No HTTPS indicators",
			setupRequest: func(req *http.Request) {
				// No special headers or TLS
			},
			expectedScheme: "http",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/docs", nil)
			req.Host = "nodelist.fidonet.cc"
			tc.setupRequest(req)

			w := httptest.NewRecorder()
			server.SwaggerUIHandler(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("Expected status 200, got %d", w.Code)
			}

			body := w.Body.String()
			expectedURL := fmt.Sprintf("%s://nodelist.fidonet.cc/api/openapi.yaml", tc.expectedScheme)

			if !strings.Contains(body, expectedURL) {
				t.Errorf("Expected Swagger UI to reference %s, but body doesn't contain it", expectedURL)
			}
		})
	}
}
