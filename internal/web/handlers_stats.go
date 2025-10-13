package web

import (
	"fmt"
	"net/http"
	"time"

	"github.com/nodelistdb/internal/database"
	"github.com/nodelistdb/internal/version"
)

// StatsHandler handles statistics page
func (s *Server) StatsHandler(w http.ResponseWriter, r *http.Request) {
	var selectedDate time.Time
	var actualDate time.Time
	var err error
	var dateAdjusted bool
	var availableDates []time.Time

	// Get available dates for the dropdown
	availableDates, err = s.storage.StatsOps().GetAvailableDates()
	if err != nil {
		data := struct {
			Title          string
			ActivePage     string
			Stats          *database.NetworkStats
			Error          error
			NoData         bool
			AvailableDates []time.Time
			SelectedDate   string
			ActualDate     string
			DateAdjusted   bool
			Version        string
		}{
			Title:          "Network Statistics",
			ActivePage:     "stats",
			Stats:          nil,
			Error:          fmt.Errorf("Failed to get available dates: %v", err),
			NoData:         true,
			AvailableDates: []time.Time{},
			SelectedDate:   "",
			ActualDate:     "",
			DateAdjusted:   false,
			Version:        version.GetVersionInfo(),
		}

		if err := s.templates["stats"].Execute(w, data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	// Parse date parameter from query string
	dateStr := r.URL.Query().Get("date")
	if dateStr != "" {
		selectedDate, err = time.Parse("2006-01-02", dateStr)
		if err != nil {
			// Invalid date format, fall back to latest
			actualDate, err = s.storage.StatsOps().GetLatestStatsDate()
			if err != nil {
				data := struct {
					Title          string
					ActivePage     string
					Stats          *database.NetworkStats
					Error          error
					NoData         bool
					AvailableDates []time.Time
					SelectedDate   string
					ActualDate     string
					DateAdjusted   bool
					Version        string
				}{
					Title:          "Network Statistics",
					ActivePage:     "stats",
					Stats:          nil,
					Error:          fmt.Errorf("Invalid date format and failed to get latest date: %v", err),
					NoData:         true,
					AvailableDates: availableDates,
					SelectedDate:   dateStr,
					ActualDate:     "",
					DateAdjusted:   false,
					Version:        version.GetVersionInfo(),
				}

				if err := s.templates["stats"].Execute(w, data); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
				}
				return
			}
			dateAdjusted = true
		} else {
			// Find the nearest available date
			actualDate, err = s.storage.StatsOps().GetNearestAvailableDate(selectedDate)
			if err != nil {
				data := struct {
					Title          string
					ActivePage     string
					Stats          *database.NetworkStats
					Error          error
					NoData         bool
					AvailableDates []time.Time
					SelectedDate   string
					ActualDate     string
					DateAdjusted   bool
					Version        string
				}{
					Title:          "Network Statistics",
					ActivePage:     "stats",
					Stats:          nil,
					Error:          fmt.Errorf("Failed to find available date: %v", err),
					NoData:         true,
					AvailableDates: availableDates,
					SelectedDate:   dateStr,
					ActualDate:     "",
					DateAdjusted:   false,
					Version:        version.GetVersionInfo(),
				}

				if err := s.templates["stats"].Execute(w, data); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
				}
				return
			}
			dateAdjusted = !actualDate.Equal(selectedDate)
		}
	} else {
		// No date specified, use latest
		actualDate, err = s.storage.StatsOps().GetLatestStatsDate()
		if err != nil {
			data := struct {
				Title          string
				ActivePage     string
				Stats          *database.NetworkStats
				Error          error
				NoData         bool
				AvailableDates []time.Time
				SelectedDate   string
				ActualDate     string
				DateAdjusted   bool
				Version        string
			}{
				Title:          "Network Statistics",
				ActivePage:     "stats",
				Stats:          nil,
				Error:          fmt.Errorf("Failed to find latest nodelist date: %v", err),
				NoData:         true,
				AvailableDates: availableDates,
				SelectedDate:   "",
				ActualDate:     "",
				DateAdjusted:   false,
				Version:        version.GetVersionInfo(),
			}

			if err := s.templates["stats"].Execute(w, data); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}
	}

	// Get stats for the actual date
	stats, err := s.storage.StatsOps().GetStats(actualDate)

	data := struct {
		Title          string
		ActivePage     string
		Stats          *database.NetworkStats
		Error          error
		NoData         bool
		AvailableDates []time.Time
		SelectedDate   string
		ActualDate     string
		DateAdjusted   bool
		Version        string
	}{
		Title:          "Network Statistics",
		ActivePage:     "stats",
		Stats:          stats,
		Error:          err,
		NoData:         stats == nil || stats.TotalNodes == 0,
		AvailableDates: availableDates,
		SelectedDate:   dateStr,
		ActualDate:     actualDate.Format("2006-01-02"),
		DateAdjusted:   dateAdjusted,
		Version:        version.GetVersionInfo(),
	}

	if data.NoData && err == nil {
		data.Error = fmt.Errorf("No nodelist data available. Please import nodelist files first.")
	}

	if err := s.templates["stats"].Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
