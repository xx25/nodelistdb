package api

import (
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