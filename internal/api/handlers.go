package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/nodelistdb/internal/storage"
)

// Server represents the API server
type Server struct {
	storage storage.Operations
}

// New creates a new API server
func New(storage storage.Operations) *Server {
	return &Server{
		storage: storage,
	}
}

// HealthHandler handles health check requests
func (s *Server) HealthHandler(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"status": "ok",
		"time":   time.Now().UTC(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// SetupRoutes sets up HTTP routes for the API server
// Deprecated: Use SetupRouter() instead which returns a chi.Router with better routing capabilities
func (s *Server) SetupRoutes(mux *http.ServeMux) {
	// API routes
	mux.HandleFunc("/api/health", s.HealthHandler)
	mux.HandleFunc("/api/nodes", s.SearchNodesHandler)
	mux.HandleFunc("/api/stats", s.StatsHandler)
	mux.HandleFunc("/api/stats/dates", s.GetAvailableDatesHandler)
	mux.HandleFunc("/api/flags", s.FlagsDocumentationHandler)
	mux.HandleFunc("/api/sysops", s.SysopsHandler)
	mux.HandleFunc("/api/nodelist/latest", s.LatestNodelistAPIHandler)

	// Software analytics routes
	mux.HandleFunc("/api/software/binkp", s.GetBinkPSoftwareStats)
	mux.HandleFunc("/api/software/ifcico", s.GetIFCICOSoftwareStats)
	mux.HandleFunc("/api/software/binkd", s.GetBinkdDetailedStats)
	mux.HandleFunc("/api/software/trends", s.GetSoftwareTrends)

	// OpenAPI documentation routes
	mux.HandleFunc("/api/openapi.yaml", s.OpenAPISpecHandler)
	mux.HandleFunc("/api/docs", s.SwaggerUIHandler)

	// Sysop-specific routes
	mux.HandleFunc("/api/sysops/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		pathParts := strings.Split(strings.TrimPrefix(path, "/api/sysops/"), "/")

		// Check if this is /api/sysops/{name}/nodes pattern
		if len(pathParts) >= 2 && pathParts[1] == "nodes" {
			s.SysopNodesHandler(w, r)
		} else {
			// For /api/sysops or /api/sysops/, redirect to the base handler
			s.SysopsHandler(w, r)
		}
	})

	// Node lookup with path parameters
	mux.HandleFunc("/api/nodes/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		pathParts := strings.Split(strings.TrimPrefix(path, "/api/nodes/"), "/")

		// Route to appropriate handler based on path structure
		if len(pathParts) >= 4 && pathParts[3] == "history" {
			// /api/nodes/{zone}/{net}/{node}/history
			s.GetNodeHistoryHandler(w, r)
		} else if len(pathParts) >= 4 && pathParts[3] == "changes" {
			// /api/nodes/{zone}/{net}/{node}/changes
			s.GetNodeChangesHandler(w, r)
		} else if len(pathParts) >= 4 && pathParts[3] == "timeline" {
			// /api/nodes/{zone}/{net}/{node}/timeline
			s.GetNodeTimelineHandler(w, r)
		} else if strings.Count(path, "/") >= 5 {
			// /api/nodes/{zone}/{net}/{node}
			s.GetNodeHandler(w, r)
		} else {
			s.SearchNodesHandler(w, r)
		}
	})
}
