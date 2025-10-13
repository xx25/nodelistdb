package api

import (
	"log"
	"net/http"
	"strconv"
)

// GetBinkPSoftwareStats returns BinkP software distribution statistics
func (s *Server) GetBinkPSoftwareStats(w http.ResponseWriter, r *http.Request) {
	// Parse days parameter
	daysStr := r.URL.Query().Get("days")
	days := 365 // default
	if d, err := strconv.Atoi(daysStr); err == nil && d > 0 {
		days = d
	}

	// Get software distribution from storage layer
	dist, err := s.storage.GetBinkPSoftwareDistribution(days)
	if err != nil {
		log.Printf("ERROR: GetBinkPSoftwareDistribution failed: %v", err)
		WriteJSONError(w, "Failed to get BinkP software distribution", http.StatusInternalServerError)
		return
	}

	WriteJSONSuccess(w, dist)
}

// GetIFCICOSoftwareStats returns IFCICO software distribution statistics
func (s *Server) GetIFCICOSoftwareStats(w http.ResponseWriter, r *http.Request) {
	// Parse days parameter
	daysStr := r.URL.Query().Get("days")
	days := 365 // default
	if d, err := strconv.Atoi(daysStr); err == nil && d > 0 {
		days = d
	}

	// Get software distribution from storage layer
	dist, err := s.storage.GetIFCICOSoftwareDistribution(days)
	if err != nil {
		log.Printf("ERROR: GetIFCICOSoftwareDistribution failed: %v", err)
		WriteJSONError(w, "Failed to get IFCICO software distribution", http.StatusInternalServerError)
		return
	}

	WriteJSONSuccess(w, dist)
}

// GetBinkdDetailedStats returns detailed binkd statistics
func (s *Server) GetBinkdDetailedStats(w http.ResponseWriter, r *http.Request) {
	// Parse days parameter
	daysStr := r.URL.Query().Get("days")
	days := 365 // default
	if d, err := strconv.Atoi(daysStr); err == nil && d > 0 {
		days = d
	}

	// Get software distribution from storage layer
	dist, err := s.storage.GetBinkdDetailedStats(days)
	if err != nil {
		log.Printf("ERROR: GetBinkdDetailedStats failed: %v", err)
		WriteJSONError(w, "Failed to get detailed binkd statistics", http.StatusInternalServerError)
		return
	}

	WriteJSONSuccess(w, dist)
}

// GetSoftwareTrends returns software usage trends over time
func (s *Server) GetSoftwareTrends(w http.ResponseWriter, r *http.Request) {
	// This feature is not yet implemented in the storage layer
	// Return empty response for now
	emptyTrends := make(map[string]interface{})

	WriteJSONSuccess(w, emptyTrends)
}