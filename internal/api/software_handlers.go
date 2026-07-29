package api

import (
	"net/http"
)

// GetBinkPSoftwareStats returns BinkP software distribution statistics
func (s *Server) GetBinkPSoftwareStats(w http.ResponseWriter, r *http.Request) {
	days := parseDaysParam(r.URL.Query(), 365)

	// Get software distribution from storage layer
	dist, err := s.storage.GetBinkPSoftwareDistribution(r.Context(), days, domainOrAll(r))
	if err != nil {
		writeStorageError(w, "Failed to get BinkP software distribution", err)
		return
	}

	WriteJSONSuccess(w, dist)
}

// GetIFCICOSoftwareStats returns IFCICO software distribution statistics
func (s *Server) GetIFCICOSoftwareStats(w http.ResponseWriter, r *http.Request) {
	days := parseDaysParam(r.URL.Query(), 365)

	// Get software distribution from storage layer
	dist, err := s.storage.GetIFCICOSoftwareDistribution(r.Context(), days, domainOrAll(r))
	if err != nil {
		writeStorageError(w, "Failed to get IFCICO software distribution", err)
		return
	}

	WriteJSONSuccess(w, dist)
}

// GetBinkdDetailedStats returns detailed binkd statistics
func (s *Server) GetBinkdDetailedStats(w http.ResponseWriter, r *http.Request) {
	days := parseDaysParam(r.URL.Query(), 365)

	// Get software distribution from storage layer
	dist, err := s.storage.GetBinkdDetailedStats(r.Context(), days, domainOrAll(r))
	if err != nil {
		writeStorageError(w, "Failed to get detailed binkd statistics", err)
		return
	}

	WriteJSONSuccess(w, dist)
}
