package api

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/nodelistdb/internal/database"
)

// queryDomain returns the ?domain= query parameter, defaulting to fidonet so
// pre-multi-network URLs keep returning FidoNet-only data.
func queryDomain(r *http.Request) string {
	if d := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("domain"))); d != "" {
		return d
	}
	return database.DefaultDomain
}

// StatsHandler handles statistics requests.
// GET /api/stats?date=2023-01-01&domain=fidonet
func (s *Server) StatsHandler(w http.ResponseWriter, r *http.Request) {
	if !CheckMethod(w, r, http.MethodGet) {
		return
	}

	domain := queryDomain(r)

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
		actualDate, err = s.storage.StatsOps().GetNearestAvailableDate(date, domain)
		if err != nil {
			WriteJSONError(w, fmt.Sprintf("Failed to find available date: %v", err), http.StatusInternalServerError)
			return
		}
	} else {
		// Default to latest available date
		actualDate, err = s.storage.StatsOps().GetLatestStatsDate(domain)
		if err != nil {
			WriteJSONError(w, fmt.Sprintf("Failed to get latest date: %v", err), http.StatusInternalServerError)
			return
		}
	}

	// Get statistics for the actual date
	stats, err := s.storage.StatsOps().GetStats(actualDate, domain)
	if err != nil {
		WriteJSONError(w, fmt.Sprintf("Failed to get statistics: %v", err), http.StatusInternalServerError)
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
	if !CheckMethod(w, r, http.MethodGet) {
		return
	}

	networks, err := s.storage.GetDomains()
	if err != nil {
		WriteJSONError(w, fmt.Sprintf("Failed to get networks: %v", err), http.StatusInternalServerError)
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
	if !CheckMethod(w, r, http.MethodGet) {
		return
	}

	dates, err := s.storage.StatsOps().GetAvailableDates(queryDomain(r))
	if err != nil {
		WriteJSONError(w, fmt.Sprintf("Failed to get available dates: %v", err), http.StatusInternalServerError)
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
