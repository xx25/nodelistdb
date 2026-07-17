package web

import (
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/nodelistdb/internal/version"
)

// Pointlist file distribution. Archived weeklies live at
// <pointlist_root>/<network>/<source>/<year>/NAME.DDD[.gz] — the layout
// sync_nodelists.sh writes (EXTRA_POINTLISTS). Unlike nodelists there is no
// fixed filename prefix; the 3-digit day extension identifies list files.

// pointlistFileRe validates pointlist file names in download requests
// (also blocks path traversal — no separators, no leading dot).
var pointlistFileRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// PointlistSourceDir groups one pointlist series' archived files
type PointlistSourceDir struct {
	Network string
	Source  string
	Years   []NodelistYear
	Count   int
}

// getPointlistPath returns the base path for archived pointlist files
func getPointlistPath() string {
	if path := os.Getenv("POINTLIST_PATH"); path != "" {
		return path
	}
	return filepath.Join(getNodelistPath(), "pointlists")
}

// scanPointlistYearDir lists the pointlist files of one year directory. Any
// file whose (pre-.gz) extension is a 3-digit day number counts.
func scanPointlistYearDir(yearPath, yearName string) []NodelistFile {
	entries, err := os.ReadDir(yearPath)
	if err != nil {
		return nil
	}
	yearNum, err := strconv.Atoi(yearName)
	if err != nil {
		return nil
	}

	var files []NodelistFile
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		base := strings.TrimSuffix(strings.ToLower(name), ".gz")
		dot := strings.LastIndex(base, ".")
		if dot < 0 {
			continue
		}
		dayStr := base[dot+1:]
		if len(dayStr) != 3 {
			continue
		}
		dayNum, err := strconv.Atoi(dayStr)
		if err != nil || dayNum < 1 || dayNum > 366 {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, NodelistFile{
			Name:         name,
			Year:         yearName,
			DayNumber:    dayNum,
			Date:         time.Date(yearNum, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, dayNum-1),
			Path:         filepath.Join(yearPath, name),
			Size:         info.Size(),
			IsCompressed: strings.HasSuffix(strings.ToLower(name), ".gz"),
		})
	}

	sort.Slice(files, func(i, j int) bool { return files[i].DayNumber > files[j].DayNumber })
	return files
}

// scanPointlistSourceDir organizes one series' year directories, newest first.
func scanPointlistSourceDir(network, source string) []NodelistYear {
	basePath := filepath.Join(getPointlistPath(), network, source)
	yearDirs, err := os.ReadDir(basePath)
	if err != nil {
		return nil
	}

	var years []NodelistYear
	for _, yearDir := range yearDirs {
		if !yearDir.IsDir() {
			continue
		}
		yearName := yearDir.Name()
		if len(yearName) != 4 {
			continue
		}
		if _, err := strconv.Atoi(yearName); err != nil {
			continue
		}
		files := scanPointlistYearDir(filepath.Join(basePath, yearName), yearName)
		if len(files) == 0 {
			continue
		}
		years = append(years, NodelistYear{
			Year:       yearName,
			Files:      files,
			NewestFile: files[0],
			OldestFile: files[len(files)-1],
			Count:      len(files),
		})
	}

	sort.Slice(years, func(i, j int) bool { return years[i].Year > years[j].Year })
	return years
}

// listPointlistSources walks <pointlist_root>/<network>/<source>/.
func listPointlistSources() []PointlistSourceDir {
	root := getPointlistPath()
	networkDirs, err := os.ReadDir(root)
	if err != nil {
		return nil
	}

	var sources []PointlistSourceDir
	for _, networkDir := range networkDirs {
		if !networkDir.IsDir() || !networkNameRe.MatchString(networkDir.Name()) {
			continue
		}
		network := networkDir.Name()
		sourceDirs, err := os.ReadDir(filepath.Join(root, network))
		if err != nil {
			continue
		}
		for _, sourceDir := range sourceDirs {
			if !sourceDir.IsDir() || !networkNameRe.MatchString(sourceDir.Name()) {
				continue
			}
			source := sourceDir.Name()
			years := scanPointlistSourceDir(network, source)
			if len(years) == 0 {
				continue
			}
			count := 0
			for _, y := range years {
				count += y.Count
			}
			sources = append(sources, PointlistSourceDir{
				Network: network,
				Source:  source,
				Years:   years,
				Count:   count,
			})
		}
	}

	sort.Slice(sources, func(i, j int) bool {
		if sources[i].Network != sources[j].Network {
			return sources[i].Network < sources[j].Network
		}
		return sources[i].Source < sources[j].Source
	})
	return sources
}

// PointlistIndexHandler shows the pointlist download index.
// Path: /pointlists
func (s *Server) PointlistIndexHandler(w http.ResponseWriter, r *http.Request) {
	if strings.Trim(strings.TrimPrefix(r.URL.Path, "/pointlists"), "/") != "" {
		s.PointlistYearHandler(w, r)
		return
	}

	data := struct {
		Title      string
		ActivePage string
		Sources    []PointlistSourceDir
		Version    string
	}{
		Title:      "Pointlist Downloads",
		ActivePage: "nodelists",
		Sources:    listPointlistSources(),
		Version:    version.GetVersionInfo(),
	}

	if err := s.templates["pointlist_download"].Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// PointlistYearHandler lists one series' files for one year.
// Path: /pointlists/{network}/{source}/{year}
func (s *Server) PointlistYearHandler(w http.ResponseWriter, r *http.Request) {
	segments := pathSegments(r.URL.Path, "/pointlists/")
	if len(segments) != 3 {
		http.NotFound(w, r)
		return
	}
	network, source, year := strings.ToLower(segments[0]), strings.ToLower(segments[1]), segments[2]
	if !networkNameRe.MatchString(network) || !networkNameRe.MatchString(source) || len(year) != 4 {
		http.NotFound(w, r)
		return
	}
	if _, err := strconv.Atoi(year); err != nil {
		http.NotFound(w, r)
		return
	}

	yearData, ok := selectNodelistYear(scanPointlistSourceDir(network, source), year)
	if !ok {
		http.NotFound(w, r)
		return
	}

	data := struct {
		Title      string
		ActivePage string
		Network    string
		Source     string
		Year       NodelistYear
		Version    string
	}{
		Title:      fmt.Sprintf("Pointlists — %s %s %s", network, source, year),
		ActivePage: "nodelists",
		Network:    network,
		Source:     source,
		Year:       yearData,
		Version:    version.GetVersionInfo(),
	}

	if err := s.templates["pointlist_year"].Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// PointlistDownloadHandler serves one archived pointlist file, decompressing
// .gz on the fly. Path: /download/pointlist/{network}/{source}/{year}/{file}
func (s *Server) PointlistDownloadHandler(w http.ResponseWriter, r *http.Request) {
	segments := pathSegments(r.URL.Path, "/download/pointlist/")
	if len(segments) != 4 {
		http.Error(w, "Invalid download path", http.StatusBadRequest)
		return
	}
	network, source, year, filename := strings.ToLower(segments[0]), strings.ToLower(segments[1]), segments[2], segments[3]

	if !networkNameRe.MatchString(network) || !networkNameRe.MatchString(source) {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}
	if len(year) != 4 {
		http.Error(w, "Invalid year", http.StatusBadRequest)
		return
	}
	if _, err := strconv.Atoi(year); err != nil {
		http.Error(w, "Invalid year", http.StatusBadRequest)
		return
	}
	if !pointlistFileRe.MatchString(filename) || strings.Contains(filename, "..") {
		http.Error(w, "Invalid filename", http.StatusBadRequest)
		return
	}

	basePath := filepath.Join(getPointlistPath(), network, source)
	fullPath := filepath.Join(basePath, year, filename)

	var actualPath string
	var isCompressed bool
	if _, err := os.Stat(fullPath); err == nil {
		actualPath = fullPath
		isCompressed = strings.HasSuffix(strings.ToLower(filename), ".gz")
	} else if _, err := os.Stat(fullPath + ".gz"); err == nil {
		actualPath = fullPath + ".gz"
		isCompressed = true
	} else {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	// Security check — the resolved path must stay inside the series directory
	if !strings.HasPrefix(filepath.Clean(actualPath), filepath.Clean(basePath)) {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	file, err := os.Open(actualPath)
	if err != nil {
		http.Error(w, "Failed to open file", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	downloadName := strings.TrimSuffix(filename, ".gz")
	if isCompressed {
		gzReader, err := gzip.NewReader(file)
		if err != nil {
			http.Error(w, "Failed to decompress file", http.StatusInternalServerError)
			return
		}
		defer gzReader.Close()

		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", downloadName))
		_, _ = io.Copy(w, gzReader)
		return
	}

	stat, err := file.Stat()
	if err != nil {
		http.Error(w, "Failed to get file info", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", downloadName))
	w.Header().Set("Content-Length", strconv.FormatInt(stat.Size(), 10))
	_, _ = io.Copy(w, file)
}
