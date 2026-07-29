package api

import (
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/nodelistdb/internal/nodelistfs"
)

// LatestNodelistAPIHandler returns the newest nodelist of one FTN network:
// its metadata and a download link when the archive holds it compressed, the
// file itself when it does not.
//
// GET /api/nodelist/latest?domain=fsxnet
func (s *Server) LatestNodelistAPIHandler(w http.ResponseWriter, r *http.Request) {
	network := queryDomain(r)
	if !nodelistfs.ValidNetworkName(network) {
		WriteJSONError(w, "Invalid domain", http.StatusBadRequest)
		return
	}

	latest, err := nodelistfs.FindLatest(nodelistfs.NormalizeNetwork(network))
	if err != nil {
		WriteJSONError(w, "No nodelist files found", http.StatusNotFound)
		return
	}

	if !latest.IsCompressed {
		file, err := os.Open(latest.Path)
		if err != nil {
			WriteJSONError(w, "Failed to open file", http.StatusInternalServerError)
			return
		}
		defer file.Close()

		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", latest.Name))
		_, _ = io.Copy(w, file)
		return
	}

	// Compressed files are described rather than served: the caller asked an
	// API for the latest nodelist, and decompressing a whole year's worth of
	// archive into a JSON endpoint is not what it wants.
	WriteJSONSuccess(w, map[string]interface{}{
		"network": network,
		// Name is already the display name: the archive stores nodelist.191.gz,
		// the nodelist is nodelist.191, and the download route resolves either.
		"filename":     latest.Name,
		"year":         latest.Year,
		"day_number":   latest.DayNumber,
		"date":         latest.Date.Format("2006-01-02"),
		"compressed":   true,
		"download_url": latest.DownloadURL(),
	})
}
