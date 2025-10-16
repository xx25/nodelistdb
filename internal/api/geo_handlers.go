package api

import (
	"log"
	"net/http"
	"strconv"
)

// GetGeoHostingStats returns geographic hosting distribution statistics
func (s *Server) GetGeoHostingStats(w http.ResponseWriter, r *http.Request) {
	// Parse days parameter
	daysStr := r.URL.Query().Get("days")
	days := 365 // default
	if d, err := strconv.Atoi(daysStr); err == nil && d > 0 {
		days = d
	}

	// Get geo distribution from storage layer
	dist, err := s.storage.TestOps().GetGeoHostingDistribution(days)
	if err != nil {
		log.Printf("ERROR: GetGeoHostingDistribution failed: %v", err)
		WriteJSONError(w, "Failed to get geo hosting distribution", http.StatusInternalServerError)
		return
	}

	WriteJSONSuccess(w, dist)
}
