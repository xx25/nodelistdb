package web

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// StaticHandler serves static files from the static directory
func (s *Server) StaticHandler(w http.ResponseWriter, r *http.Request) {
	// Extract the file path from the URL
	requestPath := strings.TrimPrefix(r.URL.Path, "/static/")
	
	// Security check: prevent directory traversal
	if strings.Contains(requestPath, "..") {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}
	
	// Build the full file path
	filePath := filepath.Join("static", requestPath)
	
	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		http.NotFound(w, r)
		return
	}
	
	// Set appropriate content type based on file extension
	switch filepath.Ext(requestPath) {
	case ".css":
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	case ".js":
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	case ".ico":
		w.Header().Set("Content-Type", "image/x-icon")
	case ".png":
		w.Header().Set("Content-Type", "image/png")
	case ".jpg", ".jpeg":
		w.Header().Set("Content-Type", "image/jpeg")
	case ".svg":
		w.Header().Set("Content-Type", "image/svg+xml")
	default:
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	}
	
	// Set cache headers for static assets
	w.Header().Set("Cache-Control", "public, max-age=31536000") // 1 year
	w.Header().Set("Expires", "Wed, 21 Oct 2025 07:28:00 GMT")
	
	// Serve the file
	http.ServeFile(w, r, filePath)
}

