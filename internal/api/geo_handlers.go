package api

import (
	"net/http"
)

// GetGeoHostingStats returns geographic hosting distribution statistics
func (s *Server) GetGeoHostingStats(w http.ResponseWriter, r *http.Request) {
	days := parseDaysParam(r.URL.Query(), 365)

	dist, err := s.storage.GetGeoHostingDistribution(r.Context(), days, domainOrAll(r))
	if err != nil {
		writeStorageError(w, "Failed to get geo hosting distribution", err)
		return
	}

	WriteJSONSuccess(w, dist)
}
