package api

import (
	"log"
	"net/http"
	"strings"
)

// GetGeoHostingStats returns geographic hosting distribution statistics
func (s *Server) GetGeoHostingStats(w http.ResponseWriter, r *http.Request) {
	days := parseDaysParam(r.URL.Query(), 365)

	// Get geo distribution from storage layer
	// Optional ?domain= filter; empty keeps the pre-multi-network
	// all-networks aggregation
	domain := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("domain")))
	dist, err := s.storage.GetGeoHostingDistribution(days, domain)
	if err != nil {
		log.Printf("ERROR: GetGeoHostingDistribution failed: %v", err)
		WriteJSONError(w, "Failed to get geo hosting distribution", http.StatusInternalServerError)
		return
	}

	WriteJSONSuccess(w, dist)
}
