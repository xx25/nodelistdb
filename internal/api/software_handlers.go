package api

import (
	"net/http"

	"github.com/nodelistdb/internal/logging"
)

// GetBinkPSoftwareStats returns BinkP software distribution statistics
func (s *Server) GetBinkPSoftwareStats(w http.ResponseWriter, r *http.Request) {
	days := parseDaysParam(r.URL.Query(), 365)

	// Get software distribution from storage layer
	dist, err := s.storage.GetBinkPSoftwareDistribution(days, domainOrAll(r))
	if err != nil {
		logging.Errorf("ERROR: GetBinkPSoftwareDistribution failed: %v", err)
		WriteJSONError(w, "Failed to get BinkP software distribution", http.StatusInternalServerError)
		return
	}

	WriteJSONSuccess(w, dist)
}

// GetIFCICOSoftwareStats returns IFCICO software distribution statistics
func (s *Server) GetIFCICOSoftwareStats(w http.ResponseWriter, r *http.Request) {
	days := parseDaysParam(r.URL.Query(), 365)

	// Get software distribution from storage layer
	dist, err := s.storage.GetIFCICOSoftwareDistribution(days, domainOrAll(r))
	if err != nil {
		logging.Errorf("ERROR: GetIFCICOSoftwareDistribution failed: %v", err)
		WriteJSONError(w, "Failed to get IFCICO software distribution", http.StatusInternalServerError)
		return
	}

	WriteJSONSuccess(w, dist)
}

// GetBinkdDetailedStats returns detailed binkd statistics
func (s *Server) GetBinkdDetailedStats(w http.ResponseWriter, r *http.Request) {
	days := parseDaysParam(r.URL.Query(), 365)

	// Get software distribution from storage layer
	dist, err := s.storage.GetBinkdDetailedStats(days, domainOrAll(r))
	if err != nil {
		logging.Errorf("ERROR: GetBinkdDetailedStats failed: %v", err)
		WriteJSONError(w, "Failed to get detailed binkd statistics", http.StatusInternalServerError)
		return
	}

	WriteJSONSuccess(w, dist)
}
