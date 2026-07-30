// Package nodelistfs reads the nodelist archive on disk: the directory layout,
// the filename conventions, and the metadata a file's name carries.
//
// The layout is per FTN network, one level per network:
// <root>/fidonet/2026/nodelist.191.gz, <root>/fsxnet/2026/fsxnet.191.gz.
//
// fidonet used to be spelled as the absence of a network - its year
// directories sat at the archive root and this package carried it as the empty
// string - while every other network got a directory. That asymmetry was
// visible to anyone browsing the archive over FTP, where /nodelists listed
// forty year directories next to a lone fsxnet/, and it disagreed with the
// pointlist tree next door, which always names its network. fidonet is now an
// ordinary network here and the network name is never empty.
//
// The old root layout is still readable: networkRoot falls back to <root> when
// <root>/fidonet does not exist, so a binary works either side of the one-time
// move, and the HTTP routes still accept their network-less spellings.
//
// Both the web download pages and the /api/nodelist/latest endpoint read this
// archive. They used to do it through two separate scanners, and the API copy
// never learned about networks - it hardcoded the fidonet prefix and the
// fidonet layout, so it answered with a FidoNet nodelist whatever was asked
// for. One scanner is the point of this package.
package nodelistfs

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/nodelistdb/internal/database"
)

// ValidNetworkName reports whether name is usable as a network path segment.
// The character class doubles as path-traversal protection: no dots, no
// separators.
func ValidNetworkName(name string) bool {
	return networkNameRe.MatchString(name)
}

// networkNameRe validates network path segments (also blocks path traversal)
var networkNameRe = regexp.MustCompile(`^[a-z0-9_-]+$`)

// NodelistNetwork groups a non-fidonet network's nodelist files
type NodelistNetwork struct {
	Name  string
	Years []NodelistYear
}

// NodelistFile represents a nodelist file with metadata. Network is the FTN
// network the file belongs to, always set for files this package scans.
type NodelistFile struct {
	Name         string
	Network      string
	Year         string
	DayNumber    int
	Date         time.Time
	Path         string
	Size         int64
	IsCompressed bool
}

// DownloadURL returns the direct download path for this file. The network is
// always named: the network-less spellings still resolve, but nothing this
// package hands to a template advertises them any more.
func (f NodelistFile) DownloadURL() string {
	return "/download/nodelist/" + NormalizeNetwork(f.Network) + "/" + f.Year + "/" + f.Name
}

// YearArchiveURL returns the tar.gz archive path for this file's year.
func (f NodelistFile) YearArchiveURL() string {
	return "/download/year/" + NormalizeNetwork(f.Network) + "/" + f.Year + ".tar.gz"
}

// NodelistYear represents a year's worth of nodelist files
type NodelistYear struct {
	Year         string
	Network      string
	Files        []NodelistFile
	PreviewFiles []NodelistFile
	NewestFile   NodelistFile
	OldestFile   NodelistFile
	Count        int
}

// BrowseURL returns the per-year file listing page for this year.
func (y NodelistYear) BrowseURL() string {
	return "/nodelists/" + NormalizeNetwork(y.Network) + "/" + y.Year
}

// ArchiveURL returns the tar.gz archive path for this year.
func (y NodelistYear) ArchiveURL() string {
	return "/download/year/" + NormalizeNetwork(y.Network) + "/" + y.Year + ".tar.gz"
}

// getNodelistPath returns the base path for nodelist files
func Root() string {
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

// NormalizeNetwork resolves a requested network to a concrete name: an empty
// or unset network means fidonet. It used to do the opposite - fold fidonet
// down to the empty string - so callers that pass its result on to a URL or a
// path now get the network named rather than elided.
func NormalizeNetwork(network string) string {
	network = strings.ToLower(strings.TrimSpace(network))
	if network == "" {
		return database.DefaultDomain
	}
	return network
}

// NetworkRoots returns the directories that may hold a network's year
// directories, in precedence order.
//
// Canonically there is one: <root>/<network>. FidoNet has a second, the archive
// root itself, which is where its years lived before they were moved down a
// level. Both are searched rather than one being chosen, because "which layout
// is this?" has a third answer during the move: half of each. Moving 41 year
// directories is 41 renames, a sync run can land a new weekly in the canonical
// place before the rest follows, and an install may never migrate at all. A
// scanner that picked one root would answer a half-moved archive by hiding
// whichever half it did not pick.
//
// Callers that build a path into the archive must go through this rather than
// joining Root() themselves, or they will miss one location or the other.
func NetworkRoots(network string) []string {
	roots := []string{filepath.Join(Root(), network)}
	if network == database.DefaultDomain {
		roots = append(roots, Root())
	}
	return roots
}

// FindFile locates one nodelist in a network's archive, trying each root in
// precedence order and each of the compressed and plain spellings. It reports
// the path it found and whether that file is gzipped.
func FindFile(network, year, filename string) (path string, compressed bool, ok bool) {
	for _, root := range NetworkRoots(network) {
		candidate := filepath.Join(root, year, filename)
		// A path that escapes its root is not a file in the archive, whatever
		// the filesystem says.
		if !withinRoot(root, candidate) {
			continue
		}
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, false, true
		}
		if info, err := os.Stat(candidate + ".gz"); err == nil && !info.IsDir() {
			return candidate + ".gz", true, true
		}
	}
	return "", false, false
}

// FindYearDir locates a network's directory for one year.
func FindYearDir(network, year string) (string, bool) {
	for _, root := range NetworkRoots(network) {
		candidate := filepath.Join(root, year)
		if !withinRoot(root, candidate) {
			continue
		}
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate, true
		}
	}
	return "", false
}

// withinRoot reports whether path stays inside root. The separator boundary
// matters: a bare string prefix would also accept a sibling named <root>-other.
func withinRoot(root, path string) bool {
	cleanRoot := filepath.Clean(root)
	cleanPath := filepath.Clean(path)
	return cleanPath == cleanRoot || strings.HasPrefix(cleanPath, cleanRoot+string(os.PathSeparator))
}

// DisplayName returns the human spelling of a network name. Only FidoNet has
// one that differs from its path segment; it earned the capitals, and now that
// the default network is named rather than elided it would otherwise appear as
// "fidonet" in prose that used to read "FidoNet".
func DisplayName(network string) string {
	if NormalizeNetwork(network) == database.DefaultDomain {
		return "FidoNet"
	}
	return network
}

// FilePrefix returns the lowercase filename prefix a network's nodelists use.
// FidoNet's files are nodelist.DDD; every other network names its files after
// itself.
func FilePrefix(network string) string {
	if network == database.DefaultDomain {
		return "nodelist."
	}
	return network + "."
}

// ScanFidonet scans the FidoNet nodelist archive.
func ScanFidonet() ([]NodelistYear, error) {
	return ScanNetwork(database.DefaultDomain)
}

// ScanNetwork scans one network's nodelists, located at
// <root>/<network>/<year>/<network>.DDD[.gz] (and, for fidonet only, also at
// the pre-migration <root>/<year>/nodelist.DDD[.gz]).
//
// Where a network has more than one root, their years are merged, so an archive
// caught mid-move reads as one collection rather than as whichever half was
// looked at first. Within a year the canonical root wins per filename.
func ScanNetwork(network string) ([]NodelistYear, error) {
	network = NormalizeNetwork(network)
	if !networkNameRe.MatchString(network) {
		return nil, fmt.Errorf("invalid network name")
	}

	roots := NetworkRoots(network)
	prefix := FilePrefix(network)

	var merged []NodelistYear
	var firstErr error
	for _, root := range roots {
		years, err := scanYears(root, prefix, network)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		merged = mergeYears(merged, years)
	}

	// Only report an error when nothing was readable anywhere: a missing
	// pre-migration root is the normal state, not a failure.
	if len(merged) == 0 && firstErr != nil {
		return nil, firstErr
	}
	return sortYears(merged), nil
}

// mergeYears folds newer into base, keeping base's file on a filename clash
// because base was scanned from the higher-precedence root.
func mergeYears(base, incoming []NodelistYear) []NodelistYear {
	if len(base) == 0 {
		return incoming
	}

	byYear := make(map[string]int, len(base))
	for i, y := range base {
		byYear[y.Year] = i
	}

	for _, year := range incoming {
		idx, ok := byYear[year.Year]
		if !ok {
			byYear[year.Year] = len(base)
			base = append(base, year)
			continue
		}

		seen := make(map[string]bool, len(base[idx].Files))
		for _, f := range base[idx].Files {
			seen[strings.ToLower(f.Name)] = true
		}
		files := base[idx].Files
		for _, f := range year.Files {
			if !seen[strings.ToLower(f.Name)] {
				files = append(files, f)
			}
		}
		base[idx] = summarizeYear(year.Year, year.Network, files)
	}

	return base
}

// ListNetworks returns every FTN network that has archived nodelist files,
// fidonet included. Networks live at <root>/<network>/; fidonet is scanned
// separately because under the pre-migration layout it has no directory of its
// own, and is deduplicated against the directory walk once it does.
func ListNetworks() []NodelistNetwork {
	var networks []NodelistNetwork

	if years, err := ScanFidonet(); err == nil && len(years) > 0 {
		networks = append(networks, NodelistNetwork{Name: database.DefaultDomain, Years: years})
	}

	entries, err := os.ReadDir(Root())
	if err != nil {
		return networks
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == database.DefaultDomain {
			continue // already scanned above, wherever its files live
		}
		if len(name) == 4 {
			if _, err := strconv.Atoi(name); err == nil {
				continue // pre-migration fidonet year directory
			}
		}
		if !networkNameRe.MatchString(name) {
			continue
		}
		years, err := ScanNetwork(name)
		if err != nil || len(years) == 0 {
			continue
		}
		networks = append(networks, NodelistNetwork{Name: name, Years: years})
	}

	sort.Slice(networks, func(i, j int) bool { return networks[i].Name < networks[j].Name })
	return networks
}

// scanNetworkDirectory scans year directories under basePath for files whose
// lowercase name starts with filePrefix, and returns them organized by year.
// network is stamped onto every file/year ("" = fidonet) so templates can
// build network-aware URLs.
func scanYears(basePath, filePrefix, network string) ([]NodelistYear, error) {

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
			// Match <prefix>DDD or <prefix>DDD.gz
			if !strings.HasPrefix(strings.ToLower(name), filePrefix) {
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
				Network:      network,
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
			years = append(years, summarizeYear(yearName, network, nodelistFiles))
		}
	}

	return sortYears(years), nil
}

// summarizeYear orders a year's files newest-first and derives the counts and
// previews the templates read.
func summarizeYear(year, network string, files []NodelistFile) NodelistYear {
	sort.Slice(files, func(i, j int) bool { return files[i].DayNumber > files[j].DayNumber })

	return NodelistYear{
		Year:         year,
		Network:      network,
		Files:        files,
		PreviewFiles: append([]NodelistFile(nil), files[:min(3, len(files))]...),
		NewestFile:   files[0],
		OldestFile:   files[len(files)-1],
		Count:        len(files),
	}
}

// sortYears orders years newest-first, which is the order every caller wants
// and the order FindLatest depends on.
func sortYears(years []NodelistYear) []NodelistYear {
	sort.Slice(years, func(i, j int) bool { return years[i].Year > years[j].Year })
	return years
}

// FindLatest finds the latest nodelist file across all years of the given
// network (empty = fidonet).
func FindLatest(network string) (*NodelistFile, error) {
	years, err := ScanNetwork(network)
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
