package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// LatestNodelistAPIHandler returns the latest nodelist file.
// GET /api/nodelist/latest
func (s *Server) LatestNodelistAPIHandler(w http.ResponseWriter, r *http.Request) {
	if !CheckMethod(w, r, http.MethodGet) {
		return
	}

	// Find the latest nodelist file
	latest, err := findLatestNodelistAPI()
	if err != nil {
		WriteJSONError(w, "No nodelist files found", http.StatusNotFound)
		return
	}

	// Open the file
	file, err := os.Open(latest.Path)
	if err != nil {
		WriteJSONError(w, "Failed to open file", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	// Check if file is gzipped
	if latest.IsCompressed {
		// For API, we'll return metadata about the file instead of decompressing
		response := map[string]interface{}{
			"filename":     strings.TrimSuffix(latest.Name, ".gz"),
			"year":         latest.Year,
			"day_number":   latest.DayNumber,
			"date":         latest.Date.Format("2006-01-02"),
			"compressed":   true,
			"download_url": fmt.Sprintf("/download/nodelist/%s/%s", latest.Year, latest.Name),
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	} else {
		// Return the file content directly
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", latest.Name))
		_, _ = io.Copy(w, file)
	}
}

// Helper function to find latest nodelist for API
func findLatestNodelistAPI() (*NodelistFileAPI, error) {
	basePath := getNodelistPathAPI()

	// Read year directories
	yearDirs, err := os.ReadDir(basePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read nodelist directory: %v", err)
	}

	var latestFile *NodelistFileAPI
	var latestYear int
	var latestDay int

	for _, yearDir := range yearDirs {
		if !yearDir.IsDir() {
			continue
		}

		yearName := yearDir.Name()
		if len(yearName) != 4 {
			continue
		}
		yearInt, err := strconv.Atoi(yearName)
		if err != nil {
			continue
		}

		yearPath := filepath.Join(basePath, yearName)
		files, err := os.ReadDir(yearPath)
		if err != nil {
			continue
		}

		for _, file := range files {
			if file.IsDir() {
				continue
			}

			name := file.Name()
			if !strings.HasPrefix(strings.ToLower(name), "nodelist.") {
				continue
			}

			parts := strings.Split(name, ".")
			if len(parts) < 2 {
				continue
			}

			dayStr := parts[1]
			if len(dayStr) != 3 {
				continue
			}
			dayNum, err := strconv.Atoi(dayStr)
			if err != nil {
				continue
			}

			// Check if this is the latest file
			if yearInt > latestYear || (yearInt == latestYear && dayNum > latestDay) {
				latestYear = yearInt
				latestDay = dayNum

				info, err := file.Info()
				if err != nil {
					// Skip files we can't stat
					continue
				}
				date := time.Date(yearInt, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, dayNum-1)

				latestFile = &NodelistFileAPI{
					Name:         name,
					Year:         yearName,
					DayNumber:    dayNum,
					Date:         date,
					Path:         filepath.Join(yearPath, name),
					Size:         info.Size(),
					IsCompressed: strings.HasSuffix(strings.ToLower(name), ".gz"),
				}
			}
		}
	}

	if latestFile == nil {
		return nil, fmt.Errorf("no nodelist files found")
	}

	return latestFile, nil
}

// NodelistFileAPI represents a nodelist file for API responses.
type NodelistFileAPI struct {
	Name         string
	Year         string
	DayNumber    int
	Date         time.Time
	Path         string
	Size         int64
	IsCompressed bool
}

// getNodelistPathAPI returns the base path for nodelist files.
func getNodelistPathAPI() string {
	if path := os.Getenv("NODELIST_PATH"); path != "" {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "/home/dp/nodelists"
	}
	return filepath.Join(home, "nodelists")
}
