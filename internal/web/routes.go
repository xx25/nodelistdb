package web

import "net/http"

// SetupRoutes configures all HTTP routes
func (s *Server) SetupRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/", s.IndexHandler)
	mux.HandleFunc("/search", s.SearchHandler)
	mux.HandleFunc("/stats", s.StatsHandler)
	mux.HandleFunc("/nodelists", s.NodelistHandler)
	mux.HandleFunc("/download/nodelist/", s.NodelistDownloadHandler)
	mux.HandleFunc("/download/latest", s.LatestNodelistHandler)
	mux.HandleFunc("/download/year/", s.YearArchiveHandler)
	mux.HandleFunc("/download/urls.txt", s.URLListHandler)
	mux.HandleFunc("/api/help", s.APIHelpHandler)
	mux.HandleFunc("/node/", s.NodeHistoryHandler)
	mux.HandleFunc("/analytics", s.AnalyticsHandler)
	mux.HandleFunc("/analytics/flag", s.AnalyticsFlagHandler)
	mux.HandleFunc("/analytics/network", s.AnalyticsNetworkHandler)
	mux.HandleFunc("/analytics/ipv6", s.IPv6AnalyticsHandler)
	mux.HandleFunc("/analytics/ipv6-nonworking", s.IPv6NonWorkingAnalyticsHandler)
	mux.HandleFunc("/analytics/ipv6-advertised-ipv4-only", s.IPv6AdvertisedIPv4OnlyAnalyticsHandler)
	mux.HandleFunc("/analytics/ipv6-only", s.IPv6OnlyNodesHandler)
	mux.HandleFunc("/analytics/pure-ipv6-only", s.PureIPv6OnlyNodesHandler)
	mux.HandleFunc("/analytics/ipv6-weekly-news", s.IPv6WeeklyNewsHandler)
	mux.HandleFunc("/analytics/binkp", s.BinkPAnalyticsHandler)
	mux.HandleFunc("/analytics/ifcico", s.IfcicoAnalyticsHandler)
	mux.HandleFunc("/analytics/telnet", s.TelnetAnalyticsHandler)
	mux.HandleFunc("/analytics/vmodem", s.VModemAnalyticsHandler)
	mux.HandleFunc("/analytics/ftp", s.FTPAnalyticsHandler)
	mux.HandleFunc("/analytics/software/binkp", s.BinkPSoftwareHandler)
	mux.HandleFunc("/analytics/software/ifcico", s.IfcicoSoftwareHandler)
	mux.HandleFunc("/analytics/geo-hosting", s.GeoHostingAnalyticsHandler)
	mux.HandleFunc("/analytics/geo-hosting/country", s.GeoCountryNodesHandler)
	mux.HandleFunc("/analytics/geo-hosting/provider", s.GeoProviderNodesHandler)
	mux.HandleFunc("/analytics/pioneers", s.PioneersHandler)
	mux.HandleFunc("/reachability", s.ReachabilityHandler)
	mux.HandleFunc("/reachability/node", s.ReachabilityNodeHandler)
	mux.HandleFunc("/reachability/test", s.TestResultDetailHandler)

	// Serve static files
	mux.HandleFunc("/static/", s.StaticHandler)
}
