package web

import (
	"embed"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/nodelistdb/internal/database"
	"github.com/nodelistdb/internal/storage"
	"github.com/nodelistdb/internal/version"
)

// Server represents the web server
type Server struct {
	storage     storage.Operations
	templates   map[string]*template.Template
	templatesFS embed.FS
	staticFS    embed.FS
}

// parseNodeURLPath extracts zone, net, and node from URL path /node/{zone}/{net}/{node}
func parseNodeURLPath(path string) (zone, net, node int, err error) {
	path = strings.TrimPrefix(path, "/node/")
	parts := strings.Split(path, "/")

	if len(parts) < 3 {
		return 0, 0, 0, fmt.Errorf("invalid node address")
	}

	zone, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid zone")
	}

	net, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid net")
	}

	node, err = strconv.Atoi(parts[2])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid node")
	}

	return zone, net, node, nil
}


// NodeActivityInfo holds information about a node's activity
type NodeActivityInfo struct {
	FirstDate       time.Time
	LastDate        time.Time
	CurrentlyActive bool
}

// analyzeNodeActivity analyzes node history to determine activity information
func analyzeNodeActivity(history []database.Node) NodeActivityInfo {
	var info NodeActivityInfo

	if len(history) > 0 {
		info.FirstDate = history[0].NodelistDate
		info.LastDate = history[len(history)-1].NodelistDate

		// Check if currently active (last entry within 30 days)
		daysSinceLastSeen := time.Since(info.LastDate).Hours() / 24
		info.CurrentlyActive = daysSinceLastSeen <= 30
	}

	return info
}

// New creates a new web server
func New(storage storage.Operations, templatesFS embed.FS, staticFS embed.FS) *Server {
	server := &Server{
		storage:     storage,
		templates:   make(map[string]*template.Template),
		templatesFS: templatesFS,
		staticFS:    staticFS,
	}

	server.loadTemplates()
	return server
}

// IndexHandler handles the home page
func (s *Server) IndexHandler(w http.ResponseWriter, r *http.Request) {
	data := struct {
		Title      string
		ActivePage string
		Version    string
	}{
		Title:      "FidoNet Nodelist Database",
		ActivePage: "home",
		Version:    version.GetVersionInfo(),
	}

	if err := s.templates["index"].Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// APIHelpHandler shows API documentation
func (s *Server) APIHelpHandler(w http.ResponseWriter, r *http.Request) {
	// Determine the scheme (http or https)
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	// Check for X-Forwarded-Proto header (common with reverse proxies)
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	}

	// Get the host from the request
	host := r.Host
	if host == "" {
		host = "localhost:8080" // fallback
	}

	// Construct the base URL
	apiURL := fmt.Sprintf("%s://%s/api/", scheme, host)
	siteURL := fmt.Sprintf("%s://%s", scheme, host)

	data := struct {
		Title      string
		ActivePage string
		BaseURL    string
		SiteURL    string
		Version    string
	}{
		Title:      "API Documentation",
		ActivePage: "api",
		BaseURL:    apiURL,
		SiteURL:    siteURL,
		Version:    version.GetVersionInfo(),
	}

	if err := s.templates["api_help"].Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
