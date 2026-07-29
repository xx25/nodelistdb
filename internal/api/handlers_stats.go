package api

import (
	"net/http"
	"time"
)

// StatsHandler handles statistics requests.
// GET /api/stats?date=2023-01-01&domain=fidonet
func (s *Server) StatsHandler(w http.ResponseWriter, r *http.Request) {
	domain := domainOrDefault(r)

	// Parse date parameter
	dateStr := r.URL.Query().Get("date")
	var date time.Time
	var err error
	var actualDate time.Time

	if dateStr != "" {
		date, err = time.Parse("2006-01-02", dateStr)
		if err != nil {
			WriteJSONError(w, "Invalid date format. Use YYYY-MM-DD", http.StatusBadRequest)
			return
		}
		// Find the nearest available date
		actualDate, err = s.storage.GetNearestAvailableDate(r.Context(), date, domain)
		if err != nil {
			writeStorageErrorf(w, "Failed to find available date", err)
			return
		}
	} else {
		// Default to latest available date
		actualDate, err = s.storage.GetLatestStatsDate(r.Context(), domain)
		if err != nil {
			writeStorageErrorf(w, "Failed to get latest date", err)
			return
		}
	}

	// Get statistics for the actual date
	stats, err := s.storage.GetStats(r.Context(), actualDate, domain)
	if err != nil {
		writeStorageErrorf(w, "Failed to get statistics", err)
		return
	}

	// Include information about date selection in the response
	response := map[string]interface{}{
		"stats":          stats,
		"domain":         domain,
		"requested_date": dateStr,
		"actual_date":    actualDate.Format("2006-01-02"),
		"date_adjusted":  dateStr != "" && actualDate.Format("2006-01-02") != dateStr,
	}

	WriteJSONSuccess(w, response)
}

// NetworksHandler lists the FTN networks present in the database with their
// latest nodelist date and node count.
// GET /api/networks
func (s *Server) NetworksHandler(w http.ResponseWriter, r *http.Request) {
	networks, err := s.storage.GetDomains(r.Context())
	if err != nil {
		writeStorageErrorf(w, "Failed to get networks", err)
		return
	}

	response := map[string]interface{}{
		"networks": networks,
		"count":    len(networks),
	}

	WriteJSONSuccess(w, response)
}

// GetAvailableDatesHandler returns all available dates for stats.
// GET /api/stats/dates?domain=fidonet
func (s *Server) GetAvailableDatesHandler(w http.ResponseWriter, r *http.Request) {
	dates, err := s.storage.GetAvailableDates(r.Context(), domainOrDefault(r))
	if err != nil {
		writeStorageErrorf(w, "Failed to get available dates", err)
		return
	}

	// Format dates as strings for JSON response
	formattedDates := make([]string, len(dates))
	for i, date := range dates {
		formattedDates[i] = date.Format("2006-01-02")
	}

	response := map[string]interface{}{
		"dates": formattedDates,
		"count": len(formattedDates),
	}

	WriteJSONSuccess(w, response)
}
