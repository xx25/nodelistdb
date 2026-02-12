package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/nodelistdb/internal/storage"
)

// Server represents the API server
type Server struct {
	storage          storage.Operations
	modemHandler     *ModemHandler
	healthChecker    HealthChecker
	cacheStatsHandler http.HandlerFunc
	ftpStatsHandler   http.HandlerFunc
}

// SetModemHandler sets the modem handler for the server
func (s *Server) SetModemHandler(handler *ModemHandler) {
	s.modemHandler = handler
}

// SetHealthChecker sets the health checker for the server
func (s *Server) SetHealthChecker(hc HealthChecker) {
	s.healthChecker = hc
}

// SetCacheStatsHandler sets the handler for the /api/cache/stats endpoint
func (s *Server) SetCacheStatsHandler(h http.HandlerFunc) {
	s.cacheStatsHandler = h
}

// SetFTPStatsHandler sets the handler for the /api/ftp/stats endpoint
func (s *Server) SetFTPStatsHandler(h http.HandlerFunc) {
	s.ftpStatsHandler = h
}

// New creates a new API server
func New(storage storage.Operations) *Server {
	return &Server{
		storage: storage,
	}
}

// HealthHandler handles health check requests
func (s *Server) HealthHandler(w http.ResponseWriter, r *http.Request) {
	if s.healthChecker != nil {
		WriteJSONSuccess(w, s.healthChecker.CheckHealth())
		return
	}
	// Fallback: backward-compatible trivial response
	response := map[string]any{
		"status": "ok",
		"time":   time.Now().UTC(),
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

