package web

import (
	"io"
	"net/http"
	"path/filepath"
	"strings"
)

// StaticHandler serves static files from the embedded filesystem
func (s *Server) StaticHandler(w http.ResponseWriter, r *http.Request) {
	// Extract the file path from the URL
	requestPath := strings.TrimPrefix(r.URL.Path, "/static/")

	// Security check: prevent directory traversal
	if strings.Contains(requestPath, "..") {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	// Build the full file path for embedded filesystem
	filePath := filepath.Join("static", requestPath)

	// Try to open file from embedded filesystem
	file, err := s.staticFS.Open(filePath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()

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

	// Serve the file content
	io.Copy(w, file)
}
