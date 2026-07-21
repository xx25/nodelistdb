package web

import "net/http"

// varyByCookie marks a page's content as dependent on request cookies (the
// global ftn_network switcher), so intermediary caches never serve one
// visitor's network view to another.
func varyByCookie(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Cookie")
		h(w, r)
	}
}

// SetupRoutes configures all HTTP routes
func (s *Server) SetupRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/", varyByCookie(s.IndexHandler))
	mux.HandleFunc("/search", varyByCookie(s.SearchHandler))
	mux.HandleFunc("/stats", varyByCookie(s.StatsHandler))
	mux.HandleFunc("/nodelists", varyByCookie(s.NodelistHandler))
	mux.HandleFunc("/nodelists/", varyByCookie(s.NodelistYearHandler))
	mux.HandleFunc("/pointlists", varyByCookie(s.PointlistIndexHandler))
	mux.HandleFunc("/pointlists/", varyByCookie(s.PointlistIndexHandler))
	mux.HandleFunc("/download/nodelist/", s.NodelistDownloadHandler)
	mux.HandleFunc("/download/pointlist/", s.PointlistDownloadHandler)
	mux.HandleFunc("/download/latest", varyByCookie(s.LatestNodelistHandler))
	mux.HandleFunc("/download/year/", s.YearArchiveHandler)
	mux.HandleFunc("/download/urls.txt", s.URLListHandler)
	mux.HandleFunc("/api/help", s.APIHelpHandler)
	mux.HandleFunc("/links", s.LinksHandler)
	mux.HandleFunc("/node/", varyByCookie(s.NodeHistoryHandler))
	mux.HandleFunc("/points/", varyByCookie(s.PointHistoryHandler))
	mux.HandleFunc("/browse", varyByCookie(s.BrowseZonesHandler))
	mux.HandleFunc("/browse/zone/", varyByCookie(s.BrowseZoneHandler))
	mux.HandleFunc("/browse/region/", varyByCookie(s.BrowseRegionHandler))
	mux.HandleFunc("/browse/net/", varyByCookie(s.BrowseNetHandler))
	mux.HandleFunc("/analytics", varyByCookie(s.AnalyticsHandler))
	mux.HandleFunc("/analytics/flag", varyByCookie(s.AnalyticsFlagHandler))
	mux.HandleFunc("/analytics/network", varyByCookie(s.AnalyticsNetworkHandler))
	mux.HandleFunc("/analytics/ipv6", varyByCookie(s.IPv6AnalyticsHandler))
	mux.HandleFunc("/analytics/ipv6-nonworking", varyByCookie(s.IPv6NonWorkingAnalyticsHandler))
	mux.HandleFunc("/analytics/ipv6-advertised-ipv4-only", varyByCookie(s.IPv6AdvertisedIPv4OnlyAnalyticsHandler))
	mux.HandleFunc("/analytics/ipv6-only", varyByCookie(s.IPv6OnlyNodesHandler))
	mux.HandleFunc("/analytics/pure-ipv6-only", varyByCookie(s.PureIPv6OnlyNodesHandler))
	mux.HandleFunc("/analytics/ipv6-weekly-news", varyByCookie(s.IPv6WeeklyNewsHandler))
	mux.HandleFunc("/analytics/ipv6-node-list", varyByCookie(s.IPv6NodeListHandler))
	mux.HandleFunc("/analytics/binkp", varyByCookie(s.BinkPAnalyticsHandler))
	mux.HandleFunc("/analytics/ifcico", varyByCookie(s.IfcicoAnalyticsHandler))
	mux.HandleFunc("/analytics/telnet", varyByCookie(s.TelnetAnalyticsHandler))
	mux.HandleFunc("/analytics/vmodem", varyByCookie(s.VModemAnalyticsHandler))
	mux.HandleFunc("/analytics/ftp", varyByCookie(s.FTPAnalyticsHandler))
	mux.HandleFunc("/analytics/aka-mismatch", varyByCookie(s.AKAMismatchAnalyticsHandler))
	mux.HandleFunc("/analytics/other-networks", varyByCookie(s.OtherNetworksAnalyticsHandler))
	mux.HandleFunc("/analytics/other-networks/nodes", varyByCookie(s.OtherNetworkNodesHandler))
	mux.HandleFunc("/analytics/pstn", varyByCookie(s.PSTNCMAnalyticsHandler))
	mux.HandleFunc("/analytics/pstn-accessible", varyByCookie(s.ModemAccessibleAnalyticsHandler))
	mux.HandleFunc("/analytics/pstn-no-answer", varyByCookie(s.ModemNoAnswerAnalyticsHandler))
	mux.HandleFunc("/analytics/file-request", varyByCookie(s.FileRequestAnalyticsHandler))
	mux.HandleFunc("/analytics/software/binkp", varyByCookie(s.BinkPSoftwareHandler))
	mux.HandleFunc("/analytics/software/ifcico", varyByCookie(s.IfcicoSoftwareHandler))
	mux.HandleFunc("/analytics/geo-hosting", varyByCookie(s.GeoHostingAnalyticsHandler))
	mux.HandleFunc("/analytics/geo-hosting/country", varyByCookie(s.GeoCountryNodesHandler))
	mux.HandleFunc("/analytics/geo-hosting/provider", varyByCookie(s.GeoProviderNodesHandler))
	mux.HandleFunc("/analytics/pioneers", varyByCookie(s.PioneersHandler))
	// The list pages scope by the ftn_network cookie, so their output varies
	// by cookie; the /nodes drill-down keys on the ?domain= URL param instead.
	mux.HandleFunc("/analytics/domain-expiration", varyByCookie(s.DomainExpirationHandler))
	mux.HandleFunc("/analytics/domain-expiration/nodes", s.DomainNodesHandler)
	mux.HandleFunc("/analytics/registrars", varyByCookie(s.RegistrarsHandler))
	mux.HandleFunc("/analytics/on-this-day", varyByCookie(s.OnThisDayHandler))
	mux.HandleFunc("/reachability", varyByCookie(s.ReachabilityHandler))
	mux.HandleFunc("/reachability/node", varyByCookie(s.ReachabilityNodeHandler))
	mux.HandleFunc("/reachability/test", varyByCookie(s.TestResultDetailHandler))
	mux.HandleFunc("/reachability/modem-test", varyByCookie(s.ModemTestDetailHandler))

	// Serve static files
	mux.HandleFunc("/static/", s.StaticHandler)
}
