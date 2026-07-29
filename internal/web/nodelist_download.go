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

	"github.com/nodelistdb/internal/database"
	"github.com/nodelistdb/internal/nodelistfs"
	"github.com/nodelistdb/internal/version"
)

// recentNodelistLimit caps the "recent files" list on the downloads page.
const recentNodelistLimit = 40

func flattenNodelistFiles(years []nodelistfs.NodelistYear) []nodelistfs.NodelistFile {
	total := 0
	for _, year := range years {
		total += len(year.Files)
	}

	files := make([]nodelistfs.NodelistFile, 0, total)
	for _, year := range years {
		files = append(files, year.Files...)
	}

	sort.Slice(files, func(i, j int) bool {
		if files[i].Date.Equal(files[j].Date) {
			return files[i].DayNumber > files[j].DayNumber
		}
		return files[i].Date.After(files[j].Date)
	})

	return files
}

func totalNodelistFiles(years []nodelistfs.NodelistYear) int {
	total := 0
	for _, year := range years {
		total += year.Count
	}
	return total
}

func selectNodelistYear(years []nodelistfs.NodelistYear, selectedYear string) (nodelistfs.NodelistYear, bool) {
	for _, year := range years {
		if year.Year == selectedYear {
			return year, true
		}
	}
	return nodelistfs.NodelistYear{}, false
}

// requestNodelistNetwork resolves the network the downloads pages should be
// scoped to: explicit ?domain= wins, then the global switcher cookie, then
// fidonet — normalized so "" means the root-level fidonet archive.
func requestNodelistNetwork(r *http.Request) string {
	network := nodelistfs.NormalizeNetwork(requestDomain(r))
	if network != "" && !nodelistfs.ValidNetworkName(network) {
		return ""
	}
	return network
}

// NodelistHandler shows the nodelist download page, scoped to the globally
// selected FTN network.
func (s *Server) NodelistHandler(w http.ResponseWriter, r *http.Request) {
	network := requestNodelistNetwork(r)

	var years []nodelistfs.NodelistYear
	var err error
	if network == "" {
		years, err = nodelistfs.ScanFidonet()
	} else {
		// A selected network with no archived files (or no directory yet) is
		// not an error — render the empty state plus the other networks.
		years, _ = nodelistfs.ScanNetwork(network)
	}

	// Other networks' archives: everything except the selected one; when a
	// non-fidonet network is selected, fidonet joins this list.
	networks := make([]nodelistfs.NodelistNetwork, 0, 4)
	if network != "" {
		if fidoYears, ferr := nodelistfs.ScanFidonet(); ferr == nil && len(fidoYears) > 0 {
			networks = append(networks, nodelistfs.NodelistNetwork{Name: database.DefaultDomain, Years: fidoYears})
		}
	}
	for _, n := range nodelistfs.ListNetworks() {
		if n.Name != network {
			networks = append(networks, n)
		}
	}

	// Get base URL from request
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	}
	host := r.Host
	if host == "" {
		host = "localhost:8080"
	}
	baseURL := fmt.Sprintf("%s://%s", scheme, host)

	allFiles := flattenNodelistFiles(years)
	recentFiles := allFiles
	if len(recentFiles) > recentNodelistLimit {
		recentFiles = recentFiles[:recentNodelistLimit]
	}

	data := struct {
		Title         string
		ActivePage    string
		Network       string
		Years         []nodelistfs.NodelistYear
		Networks      []nodelistfs.NodelistNetwork
		RecentFiles   []nodelistfs.NodelistFile
		TotalFiles    int
		Error         error
		Latest        *nodelistfs.NodelistFile
		BaseURL       string
		Version       string
		HasPointlists bool
	}{
		Title:         "Downloads",
		ActivePage:    "nodelists",
		Network:       network,
		Years:         years,
		Networks:      networks,
		RecentFiles:   recentFiles,
		TotalFiles:    totalNodelistFiles(years),
		Error:         err,
		BaseURL:       baseURL,
		Version:       version.GetVersionInfo(),
		HasPointlists: len(listPointlistSources()) > 0,
	}

	// Find latest nodelist
	if err == nil && len(years) > 0 {
		data.Latest = &years[0].Files[0]
	}

	s.render(w, "nodelist_download", data)
}

// NodelistYearHandler shows all nodelist files for a specific year.
// Paths: /nodelists/{year} (fidonet) or /nodelists/{network}/{year}.
func (s *Server) NodelistYearHandler(w http.ResponseWriter, r *http.Request) {
	segments := pathSegments(r.URL.Path, "/nodelists/")

	network := ""
	year := ""
	switch len(segments) {
	case 1:
		year = segments[0]
	case 2:
		// /nodelists/fidonet/{year} is an alias for the root fidonet layout
		network = nodelistfs.NormalizeNetwork(strings.ToLower(segments[0]))
		year = segments[1]
	default:
		http.NotFound(w, r)
		return
	}

	if len(year) != 4 {
		http.NotFound(w, r)
		return
	}
	if _, err := strconv.Atoi(year); err != nil {
		http.NotFound(w, r)
		return
	}

	var years []nodelistfs.NodelistYear
	var err error
	if network == "" {
		years, err = nodelistfs.ScanFidonet()
	} else {
		years, err = nodelistfs.ScanNetwork(network)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	yearData, ok := selectNodelistYear(years, year)
	if !ok {
		http.NotFound(w, r)
		return
	}

	title := "Nodelists — " + year
	if network != "" {
		title = "Nodelists — " + network + " " + year
	}

	data := struct {
		Title      string
		ActivePage string
		Network    string
		Year       nodelistfs.NodelistYear
		Version    string
	}{
		Title:      title,
		ActivePage: "nodelists",
		Network:    network,
		Year:       yearData,
		Version:    version.GetVersionInfo(),
	}

	s.render(w, "nodelist_year", data)
}

// NodelistDownloadHandler handles direct nodelist file downloads
func (s *Server) NodelistDownloadHandler(w http.ResponseWriter, r *http.Request) {
	// Extract year and filename from URL
	// Expected formats: /download/nodelist/{year}/{filename} (fidonet) or
	// /download/nodelist/{network}/{year}/{filename}
	path := strings.TrimPrefix(r.URL.Path, "/download/nodelist/")
	parts := strings.SplitN(path, "/", 3)

	network := ""
	var year, filename string
	switch len(parts) {
	case 2:
		year, filename = parts[0], parts[1]
	case 3:
		network, year, filename = strings.ToLower(parts[0]), parts[1], parts[2]
		if !nodelistfs.ValidNetworkName(network) {
			http.Error(w, "Invalid network", http.StatusBadRequest)
			return
		}
		// /download/nodelist/fidonet/... is an alias for the root layout
		network = nodelistfs.NormalizeNetwork(network)
	default:
		http.Error(w, "Invalid download path", http.StatusBadRequest)
		return
	}

	// Validate year
	if len(year) != 4 {
		http.Error(w, "Invalid year", http.StatusBadRequest)
		return
	}
	if _, err := strconv.Atoi(year); err != nil {
		http.Error(w, "Invalid year", http.StatusBadRequest)
		return
	}

	// Filenames are always a single path element — SplitN leaves any extra
	// slashes in the trailing part, so reject separators and dot-dot outright
	// (blocks path traversal).
	if strings.ContainsAny(filename, "/\\") || strings.Contains(filename, "..") {
		http.Error(w, "Invalid filename", http.StatusBadRequest)
		return
	}

	// Validate filename against the network's naming convention
	expectedPrefix := "nodelist."
	if network != "" {
		expectedPrefix = network + "."
	}
	if !strings.HasPrefix(strings.ToLower(filename), expectedPrefix) {
		http.Error(w, "Invalid filename", http.StatusBadRequest)
		return
	}

	// Construct full path - try both with and without .gz extension
	basePath := nodelistfs.Root()
	if network != "" {
		basePath = filepath.Join(basePath, network)
	}
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

	// Security check - ensure the path is within the nodelist directory.
	// Require a path-separator boundary so a sibling like <base>-other can't
	// satisfy a bare string prefix.
	cleanPath := filepath.Clean(actualPath)
	cleanBase := filepath.Clean(basePath)
	if cleanPath != cleanBase && !strings.HasPrefix(cleanPath, cleanBase+string(os.PathSeparator)) {
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
		_, _ = io.Copy(w, gzReader)
	} else {
		// Serve the file as-is
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
		w.Header().Set("Content-Length", strconv.FormatInt(stat.Size(), 10))

		// Stream the file
		_, _ = io.Copy(w, file)
	}
}

// LatestNodelistHandler redirects to the latest nodelist download of the
// selected network (?domain= or the global switcher cookie; default fidonet,
// which also covers cookie-less scripted use).
func (s *Server) LatestNodelistHandler(w http.ResponseWriter, r *http.Request) {
	latest, err := nodelistfs.FindLatest(requestNodelistNetwork(r))
	if err != nil {
		http.Error(w, "No nodelist files found", http.StatusNotFound)
		return
	}

	http.Redirect(w, r, latest.DownloadURL(), http.StatusFound)
}

// YearArchiveHandler creates and serves a tar.gz archive of all nodelists for
// a specific year. Paths: /download/year/{year}.tar.gz (fidonet) or
// /download/year/{network}/{year}.tar.gz
func (s *Server) YearArchiveHandler(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/download/year/")
	path = strings.TrimSuffix(path, ".tar.gz")
	segments := strings.Split(path, "/")

	network := ""
	year := ""
	switch len(segments) {
	case 1:
		year = segments[0]
	case 2:
		network = strings.ToLower(segments[0])
		if !nodelistfs.ValidNetworkName(network) {
			http.Error(w, "Invalid network", http.StatusBadRequest)
			return
		}
		network = nodelistfs.NormalizeNetwork(network)
		year = segments[1]
	default:
		http.Error(w, "Invalid archive path", http.StatusBadRequest)
		return
	}

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
	basePath := nodelistfs.Root()
	filePrefix := "nodelist."
	archiveName := fmt.Sprintf("nodelists-%s.tar.gz", year)
	if network != "" {
		basePath = filepath.Join(basePath, network)
		filePrefix = network + "."
		archiveName = fmt.Sprintf("%s-nodelists-%s.tar.gz", network, year)
	}
	yearPath := filepath.Join(basePath, year)

	// Check if year directory exists
	if _, err := os.Stat(yearPath); os.IsNotExist(err) {
		http.Error(w, "Year not found", http.StatusNotFound)
		return
	}

	// Set headers for download
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", archiveName))

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

		// Only include this network's nodelist files
		name := info.Name()
		if !strings.HasPrefix(strings.ToLower(name), filePrefix) {
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
	years, err := nodelistfs.ScanFidonet()
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

	// Other networks' nodelists
	for _, network := range nodelistfs.ListNetworks() {
		for _, year := range network.Years {
			for _, file := range year.Files {
				fmt.Fprintf(w, "%s/download/nodelist/%s/%s/%s\n", baseURL, network.Name, year.Year, file.Name)
			}
		}
	}

	// Archived pointlists
	for _, source := range listPointlistSources() {
		for _, year := range source.Years {
			for _, file := range year.Files {
				fmt.Fprintf(w, "%s/download/pointlist/%s/%s/%s/%s\n", baseURL, source.Network, source.Source, year.Year, file.Name)
			}
		}
	}
}
