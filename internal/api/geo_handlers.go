package api

import (
	"net/http"

	"github.com/nodelistdb/internal/logging"
)

// GetGeoHostingStats returns geographic hosting distribution statistics
func (s *Server) GetGeoHostingStats(w http.ResponseWriter, r *http.Request) {
	days := parseDaysParam(r.URL.Query(), 365)

	dist, err := s.storage.GetGeoHostingDistribution(days, domainOrAll(r))
	if err != nil {
		logging.Errorf("ERROR: GetGeoHostingDistribution failed: %v", err)
		WriteJSONError(w, "Failed to get geo hosting distribution", http.StatusInternalServerError)
		return
	}

	WriteJSONSuccess(w, dist)
}
