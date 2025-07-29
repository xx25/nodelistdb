package web

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// NodelistFile represents a nodelist file with metadata
type NodelistFile struct {
	Name         string
	Year         string
	DayNumber    int
	Date         time.Time
	Path         string
	Size         int64
	IsCompressed bool
}

// NodelistYear represents a year's worth of nodelist files
type NodelistYear struct {
	Year  string
	Files []NodelistFile
	Count int
}

// getNodelistPath returns the base path for nodelist files
func getNodelistPath() string {
	// Check if NODELIST_PATH environment variable is set
	if path := os.Getenv("NODELIST_PATH"); path != "" {
		return path
	}
	// Default to ~/nodelists
	home, err := os.UserHomeDir()
	if err != nil {
		return "/home/dp/nodelists" // fallback
	}
	return filepath.Join(home, "nodelists")
}

// scanNodelistDirectory scans the nodelist directory and returns organized files by year
func scanNodelistDirectory() ([]NodelistYear, error) {
	basePath := getNodelistPath()
	
	// Read year directories
	yearDirs, err := os.ReadDir(basePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read nodelist directory: %v", err)
	}
	
	var years []NodelistYear
	
	for _, yearDir := range yearDirs {
		if !yearDir.IsDir() {
			continue
		}
		
		// Check if directory name is a valid year (4 digits)
		yearName := yearDir.Name()
		if len(yearName) != 4 {
			continue
		}
		if _, err := strconv.Atoi(yearName); err != nil {
			continue
		}
		
		yearPath := filepath.Join(basePath, yearName)
		files, err := os.ReadDir(yearPath)
		if err != nil {
			continue
		}
		
		var nodelistFiles []NodelistFile
		
		for _, file := range files {
			if file.IsDir() {
				continue
			}
			
			name := file.Name()
			// Match nodelist.DDD or nodelist.DDD.gz
			if !strings.HasPrefix(strings.ToLower(name), "nodelist.") {
				continue
			}
			
			// Parse the file name
			parts := strings.Split(name, ".")
			if len(parts) < 2 {
				continue
			}
			
			// Extract day number
			dayStr := parts[1]
			if len(dayStr) != 3 {
				continue
			}
			dayNum, err := strconv.Atoi(dayStr)
			if err != nil {
				continue
			}
			
			// Get file info
			info, err := file.Info()
			if err != nil {
				continue
			}
			
			// Calculate date from year and day number
			yearInt, _ := strconv.Atoi(yearName)
			date := time.Date(yearInt, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, dayNum-1)
			
			// Hide .gz extension from display name
			displayName := name
			isCompressed := strings.HasSuffix(strings.ToLower(name), ".gz")
			if isCompressed {
				displayName = strings.TrimSuffix(name, ".gz")
			}
			
			nodelistFile := NodelistFile{
				Name:         displayName,
				Year:         yearName,
				DayNumber:    dayNum,
				Date:         date,
				Path:         filepath.Join(yearPath, name), // Keep real path with .gz
				Size:         info.Size(),
				IsCompressed: isCompressed,
			}
			
			nodelistFiles = append(nodelistFiles, nodelistFile)
		}
		
		if len(nodelistFiles) > 0 {
			// Sort files by day number (descending)
			sort.Slice(nodelistFiles, func(i, j int) bool {
				return nodelistFiles[i].DayNumber > nodelistFiles[j].DayNumber
			})
			
			years = append(years, NodelistYear{
				Year:  yearName,
				Files: nodelistFiles,
				Count: len(nodelistFiles),
			})
		}
	}
	
	// Sort years (descending)
	sort.Slice(years, func(i, j int) bool {
		return years[i].Year > years[j].Year
	})
	
	return years, nil
}

// findLatestNodelist finds the latest nodelist file across all years
func findLatestNodelist() (*NodelistFile, error) {
	years, err := scanNodelistDirectory()
	if err != nil {
		return nil, err
	}
	
	if len(years) == 0 {
		return nil, fmt.Errorf("no nodelist files found")
	}
	
	// The latest file is the first file in the first year (already sorted)
	if len(years[0].Files) > 0 {
		return &years[0].Files[0], nil
	}
	
	return nil, fmt.Errorf("no nodelist files found")
}

// NodelistHandler shows the nodelist download page
func (s *Server) NodelistHandler(w http.ResponseWriter, r *http.Request) {
	years, err := scanNodelistDirectory()
	
	data := struct {
		Title  string
		Years  []NodelistYear
		Error  error
		Latest *NodelistFile
	}{
		Title: "Download Nodelists",
		Years: years,
		Error: err,
	}
	
	// Find latest nodelist
	if err == nil && len(years) > 0 {
		latest, _ := findLatestNodelist()
		data.Latest = latest
	}
	
	if err := s.templates["nodelist_download"].Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// NodelistDownloadHandler handles direct nodelist file downloads
func (s *Server) NodelistDownloadHandler(w http.ResponseWriter, r *http.Request) {
	// Extract year and filename from URL
	// Expected format: /download/nodelist/{year}/{filename}
	path := strings.TrimPrefix(r.URL.Path, "/download/nodelist/")
	parts := strings.SplitN(path, "/", 2)
	
	if len(parts) != 2 {
		http.Error(w, "Invalid download path", http.StatusBadRequest)
		return
	}
	
	year := parts[0]
	filename := parts[1]
	
	// Validate year
	if len(year) != 4 {
		http.Error(w, "Invalid year", http.StatusBadRequest)
		return
	}
	if _, err := strconv.Atoi(year); err != nil {
		http.Error(w, "Invalid year", http.StatusBadRequest)
		return
	}
	
	// Validate filename
	if !strings.HasPrefix(strings.ToLower(filename), "nodelist.") {
		http.Error(w, "Invalid filename", http.StatusBadRequest)
		return
	}
	
	// Construct full path - try both with and without .gz extension
	basePath := getNodelistPath()
	fullPath := filepath.Join(basePath, year, filename)
	
	// Check if file exists, if not try with .gz extension
	var actualPath string
	var isCompressed bool
	
	if _, err := os.Stat(fullPath); err == nil {
		actualPath = fullPath
		isCompressed = false
	} else if _, err := os.Stat(fullPath + ".gz"); err == nil {
		actualPath = fullPath + ".gz"
		isCompressed = true
	} else {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}
	
	// Security check - ensure the path is within the nodelist directory
	cleanPath := filepath.Clean(actualPath)
	if !strings.HasPrefix(cleanPath, filepath.Clean(basePath)) {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}
	
	// Open the file
	file, err := os.Open(actualPath)
	if err != nil {
		http.Error(w, "Failed to open file", http.StatusInternalServerError)
		return
	}
	defer file.Close()
	
	// Get file info
	stat, err := file.Stat()
	if err != nil {
		http.Error(w, "Failed to get file info", http.StatusInternalServerError)
		return
	}
	
	if isCompressed {
		// Decompress on the fly
		gzReader, err := gzip.NewReader(file)
		if err != nil {
			http.Error(w, "Failed to decompress file", http.StatusInternalServerError)
			return
		}
		defer gzReader.Close()
		
		// Set headers for uncompressed file
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
		// Note: We can't set Content-Length for decompressed data without reading it all first
		
		// Stream the decompressed content
		io.Copy(w, gzReader)
	} else {
		// Serve the file as-is
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
		w.Header().Set("Content-Length", strconv.FormatInt(stat.Size(), 10))
		
		// Stream the file
		io.Copy(w, file)
	}
}

// LatestNodelistHandler redirects to the latest nodelist download
func (s *Server) LatestNodelistHandler(w http.ResponseWriter, r *http.Request) {
	latest, err := findLatestNodelist()
	if err != nil {
		http.Error(w, "No nodelist files found", http.StatusNotFound)
		return
	}
	
	// Redirect to the download URL
	downloadURL := fmt.Sprintf("/download/nodelist/%s/%s", latest.Year, latest.Name)
	http.Redirect(w, r, downloadURL, http.StatusFound)
}

// YearArchiveHandler creates and serves a tar.gz archive of all nodelists for a specific year
func (s *Server) YearArchiveHandler(w http.ResponseWriter, r *http.Request) {
	// Extract year from URL: /download/year/2024.tar.gz
	path := strings.TrimPrefix(r.URL.Path, "/download/year/")
	year := strings.TrimSuffix(path, ".tar.gz")
	
	// Validate year
	if len(year) != 4 {
		http.Error(w, "Invalid year", http.StatusBadRequest)
		return
	}
	if _, err := strconv.Atoi(year); err != nil {
		http.Error(w, "Invalid year", http.StatusBadRequest)
		return
	}
	
	// Get base path and year directory
	basePath := getNodelistPath()
	yearPath := filepath.Join(basePath, year)
	
	// Check if year directory exists
	if _, err := os.Stat(yearPath); os.IsNotExist(err) {
		http.Error(w, "Year not found", http.StatusNotFound)
		return
	}
	
	// Set headers for download
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"nodelists-%s.tar.gz\"", year))
	
	// Create gzip writer
	gw := gzip.NewWriter(w)
	defer gw.Close()
	
	// Create tar writer
	tw := tar.NewWriter(gw)
	defer tw.Close()
	
	// Walk through year directory and add files to archive
	err := filepath.Walk(yearPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		
		// Skip directories
		if info.IsDir() {
			return nil
		}
		
		// Only include nodelist files
		name := info.Name()
		if !strings.HasPrefix(strings.ToLower(name), "nodelist.") {
			return nil
		}
		
		// Create tar header
		header := &tar.Header{
			Name:    filepath.Join(year, name),
			Mode:    0644,
			Size:    info.Size(),
			ModTime: info.ModTime(),
		}
		
		// Write header
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		
		// Open file
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		
		// Copy file content to tar
		_, err = io.Copy(tw, file)
		return err
	})
	
	if err != nil {
		http.Error(w, "Failed to create archive", http.StatusInternalServerError)
		return
	}
}

// URLListHandler generates a text file with all nodelist download URLs
func (s *Server) URLListHandler(w http.ResponseWriter, r *http.Request) {
	years, err := scanNodelistDirectory()
	if err != nil {
		http.Error(w, "Failed to scan nodelist directory", http.StatusInternalServerError)
		return
	}
	
	// Get base URL from request
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	baseURL := fmt.Sprintf("%s://%s", scheme, r.Host)
	
	// Set headers
	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("Content-Disposition", "attachment; filename=\"nodelist-urls.txt\"")
	
	// Write URLs
	for _, year := range years {
		for _, file := range year.Files {
			fmt.Fprintf(w, "%s/download/nodelist/%s/%s\n", baseURL, year.Year, file.Name)
		}
	}
}