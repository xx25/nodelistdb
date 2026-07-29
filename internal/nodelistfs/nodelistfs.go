// Package nodelistfs reads the nodelist archive on disk: the directory layout,
// the filename conventions, and the metadata a file's name carries.
//
// The layout is per FTN network. fidonet's year directories sit at the archive
// root for backward compatibility (<root>/2026/nodelist.191.gz); every other
// network lives one level down, under its own name
// (<root>/fsxnet/2026/fsxnet.191.gz).
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

// NodelistFile represents a nodelist file with metadata. Network is empty for
// fidonet (whose files live at the archive root) and the network name for
// other FTN networks.
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

// DownloadURL returns the direct download path for this file.
func (f NodelistFile) DownloadURL() string {
	if f.Network != "" {
		return "/download/nodelist/" + f.Network + "/" + f.Year + "/" + f.Name
	}
	return "/download/nodelist/" + f.Year + "/" + f.Name
}

// YearArchiveURL returns the tar.gz archive path for this file's year.
func (f NodelistFile) YearArchiveURL() string {
	if f.Network != "" {
		return "/download/year/" + f.Network + "/" + f.Year + ".tar.gz"
	}
	return "/download/year/" + f.Year + ".tar.gz"
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
	if y.Network != "" {
		return "/nodelists/" + y.Network + "/" + y.Year
	}
	return "/nodelists/" + y.Year
}

// ArchiveURL returns the tar.gz archive path for this year.
func (y NodelistYear) ArchiveURL() string {
	if y.Network != "" {
		return "/download/year/" + y.Network + "/" + y.Year + ".tar.gz"
	}
	return "/download/year/" + y.Year + ".tar.gz"
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

// normalizeNetwork maps the default domain (fidonet) to the empty string used
// throughout this file for the root-level fidonet archive layout.
func NormalizeNetwork(network string) string {
	if network == database.DefaultDomain {
		return ""
	}
	return network
}

// scanNodelistDirectory scans the fidonet nodelist directory (year directories
// directly under the root, for backward compatibility) and returns organized
// files by year.
func ScanFidonet() ([]NodelistYear, error) {
	return scanYears(Root(), "nodelist.", "")
}

// scanNetworkNodelistDirectory scans a non-fidonet network's nodelists located
// at <root>/<network>/<year>/<network>.DDD[.gz].
func ScanNetwork(network string) ([]NodelistYear, error) {
	if !networkNameRe.MatchString(network) {
		return nil, fmt.Errorf("invalid network name")
	}
	return scanYears(filepath.Join(Root(), network), network+".", network)
}

// listNodelistNetworks returns the non-fidonet networks that have nodelist
// files under <root>/<network>/.
func ListNetworks() []NodelistNetwork {
	entries, err := os.ReadDir(Root())
	if err != nil {
		return nil
	}

	var networks []NodelistNetwork
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if len(name) == 4 {
			if _, err := strconv.Atoi(name); err == nil {
				continue // fidonet year directory
			}
		}
		if !networkNameRe.MatchString(name) || name == database.DefaultDomain {
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
			// Sort files by day number (descending)
			sort.Slice(nodelistFiles, func(i, j int) bool {
				return nodelistFiles[i].DayNumber > nodelistFiles[j].DayNumber
			})

			years = append(years, NodelistYear{
				Year:         yearName,
				Network:      network,
				Files:        nodelistFiles,
				PreviewFiles: append([]NodelistFile(nil), nodelistFiles[:min(3, len(nodelistFiles))]...),
				NewestFile:   nodelistFiles[0],
				OldestFile:   nodelistFiles[len(nodelistFiles)-1],
				Count:        len(nodelistFiles),
			})
		}
	}

	// Sort years (descending)
	sort.Slice(years, func(i, j int) bool {
		return years[i].Year > years[j].Year
	})

	return years, nil
}

// findLatestNodelist finds the latest nodelist file across all years of the
// given network ("" = fidonet).
func FindLatest(network string) (*NodelistFile, error) {
	var years []NodelistYear
	var err error
	if network == "" {
		years, err = ScanFidonet()
	} else {
		years, err = ScanNetwork(network)
	}
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
