package web

import (
	"embed"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/nodelistdb/internal/querybudget"

	"github.com/nodelistdb/internal/database"
	"github.com/nodelistdb/internal/links"
	"github.com/nodelistdb/internal/version"
)

// Server represents the web server
type Server struct {
	storage     Storage
	budgets     Budgets
	templates   map[string]*template.Template
	templatesFS embed.FS
	staticFS    embed.FS
	linksLoader *links.Loader
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

// New creates a new web server. A template that will not parse is a
// deployment-stopping error, not something to discover on the first request:
// the loader used to call log.Fatalf from inside this constructor, which made
// every render test carry a comment about it.
func New(storage Storage, templatesFS embed.FS, staticFS embed.FS) (*Server, error) {
	server := &Server{
		storage:     storage,
		templates:   make(map[string]*template.Template),
		templatesFS: templatesFS,
		staticFS:    staticFS,
	}

	if err := server.loadTemplates(); err != nil {
		return nil, err
	}
	return server, nil
}

// Budgets are the per-path query deadlines SetupRoutes applies. The zero value
// is no deadline anywhere, which is what the server runs with until
// query_budget.enabled is set.
type Budgets struct {
	Read      querybudget.Budget // ordinary pages: search, browse, node, stats
	Analytics querybudget.Budget // the analytics and reachability reports
}

// SetQueryBudgets installs the deadlines SetupRoutes applies. It must be
// called before SetupRoutes, which reads them once while registering.
func (s *Server) SetQueryBudgets(b Budgets) {
	s.budgets = b
}

// SetLinksLoader sets the links loader for hot-reloadable links
func (s *Server) SetLinksLoader(loader *links.Loader) {
	s.linksLoader = loader
}

// IndexHandler handles the root page by serving search
func (s *Server) IndexHandler(w http.ResponseWriter, r *http.Request) {
	s.SearchHandler(w, r)
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

	s.render(w, "api_help", data)
}

// LinksHandler shows external FidoNet links
func (s *Server) LinksHandler(w http.ResponseWriter, r *http.Request) {
	var categories []links.Category
	if s.linksLoader != nil {
		config := s.linksLoader.GetConfig()
		if config != nil {
			categories = config.Categories
		}
	}

	data := struct {
		Title      string
		ActivePage string
		Version    string
		Categories []links.Category
	}{
		Title:      "FidoNet Links",
		ActivePage: "links",
		Version:    version.GetVersionInfo(),
		Categories: categories,
	}

	s.render(w, "links", data)
}
