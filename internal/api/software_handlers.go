package api

import (
	"encoding/json"
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
		http.Error(w, "Failed to get BinkP software distribution", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(dist)
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
		http.Error(w, "Failed to get IFCICO software distribution", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(dist)
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
		http.Error(w, "Failed to get detailed binkd statistics", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(dist)
}

// GetSoftwareTrends returns software usage trends over time
func (s *Server) GetSoftwareTrends(w http.ResponseWriter, r *http.Request) {
	// This feature is not yet implemented in the storage layer
	// Return empty response for now
	emptyTrends := make(map[string]interface{})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(emptyTrends)
}