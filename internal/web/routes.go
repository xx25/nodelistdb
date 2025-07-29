package web

import "net/http"

// SetupRoutes configures all HTTP routes
func (s *Server) SetupRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/", s.IndexHandler)
	mux.HandleFunc("/search", s.SearchHandler)
	mux.HandleFunc("/search/sysop", s.SysopSearchHandler)
	mux.HandleFunc("/stats", s.StatsHandler)
	mux.HandleFunc("/nodelists", s.NodelistHandler)
	mux.HandleFunc("/download/nodelist/", s.NodelistDownloadHandler)
	mux.HandleFunc("/download/latest", s.LatestNodelistHandler)
	mux.HandleFunc("/download/year/", s.YearArchiveHandler)
	mux.HandleFunc("/download/urls.txt", s.URLListHandler)
	mux.HandleFunc("/api/help", s.APIHelpHandler)
	mux.HandleFunc("/node/", s.NodeHistoryHandler)

	// Serve static files
	mux.HandleFunc("/static/", s.StaticHandler)
}
