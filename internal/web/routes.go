package web

import (
	"net/http"
	"strings"
)

// varyByCookie marks a page's content as dependent on request cookies (the
// global ftn_network switcher), so intermediary caches never serve one
// visitor's network view to another.
func varyByCookie(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Cookie")
		h(w, r)
	}
}

// SetupRoutes configures all HTTP routes.
//
// handle picks each page's query budget from its path rather than applying one
// at the mux: context.WithDeadline can only shorten a deadline it inherits, so
// a single budget here would silently cap the analytics pages at whatever the
// ordinary pages got. See internal/querybudget. Both budgets are no-ops until
// query_budget.enabled is set.
//
// /static/ and /download/ are registered directly - they serve files, have no
// database work to bound, and the archive builder can legitimately outrun any
// budget sized for a page.
func (s *Server) SetupRoutes(mux *http.ServeMux) {
	handle := func(pattern string, h http.HandlerFunc) {
		budget := s.budgets.Read
		if strings.HasPrefix(pattern, "/analytics") || strings.HasPrefix(pattern, "/reachability") {
			budget = s.budgets.Analytics
		}
		mux.Handle(pattern, budget.Wrap(h))
	}

	handle("/", varyByCookie(s.IndexHandler))
	handle("/search", varyByCookie(s.SearchHandler))
	handle("/stats", varyByCookie(s.StatsHandler))
	handle("/nodelists", varyByCookie(s.NodelistHandler))
	handle("/nodelists/", varyByCookie(s.NodelistYearHandler))
	handle("/pointlists", varyByCookie(s.PointlistIndexHandler))
	handle("/pointlists/", varyByCookie(s.PointlistIndexHandler))
	mux.HandleFunc("/download/nodelist/", s.NodelistDownloadHandler)
	mux.HandleFunc("/download/pointlist/", s.PointlistDownloadHandler)
	mux.HandleFunc("/download/latest", varyByCookie(s.LatestNodelistHandler))
	mux.HandleFunc("/download/year/", s.YearArchiveHandler)
	mux.HandleFunc("/download/urls.txt", s.URLListHandler)
	handle("/api/help", s.APIHelpHandler)
	handle("/links", s.LinksHandler)
	handle("/node/", varyByCookie(s.NodeHistoryHandler))
	handle("/points/", varyByCookie(s.PointHistoryHandler))
	handle("/browse", varyByCookie(s.BrowseZonesHandler))
	handle("/browse/zone/", varyByCookie(s.BrowseZoneHandler))
	handle("/browse/region/", varyByCookie(s.BrowseRegionHandler))
	handle("/browse/net/", varyByCookie(s.BrowseNetHandler))
	handle("/analytics", varyByCookie(s.AnalyticsHandler))
	handle("/analytics/flag", varyByCookie(s.AnalyticsFlagHandler))
	handle("/analytics/network", varyByCookie(s.AnalyticsNetworkHandler))
	handle("/analytics/ipv6", varyByCookie(s.IPv6AnalyticsHandler))
	handle("/analytics/ipv6-nonworking", varyByCookie(s.IPv6NonWorkingAnalyticsHandler))
	handle("/analytics/ipv6-advertised-ipv4-only", varyByCookie(s.IPv6AdvertisedIPv4OnlyAnalyticsHandler))
	handle("/analytics/ipv6-only", varyByCookie(s.IPv6OnlyNodesHandler))
	handle("/analytics/pure-ipv6-only", varyByCookie(s.PureIPv6OnlyNodesHandler))
	handle("/analytics/ipv6-weekly-news", varyByCookie(s.IPv6WeeklyNewsHandler))
	handle("/analytics/ipv6-node-list", varyByCookie(s.IPv6NodeListHandler))
	handle("/analytics/binkp", varyByCookie(s.BinkPAnalyticsHandler))
	handle("/analytics/ifcico", varyByCookie(s.IfcicoAnalyticsHandler))
	handle("/analytics/telnet", varyByCookie(s.TelnetAnalyticsHandler))
	handle("/analytics/vmodem", varyByCookie(s.VModemAnalyticsHandler))
	handle("/analytics/vmodem-unavailable", varyByCookie(s.VModemUnavailableAnalyticsHandler))
	handle("/analytics/ftp", varyByCookie(s.FTPAnalyticsHandler))
	handle("/analytics/aka-mismatch", varyByCookie(s.AKAMismatchAnalyticsHandler))
	handle("/analytics/other-networks", varyByCookie(s.OtherNetworksAnalyticsHandler))
	handle("/analytics/other-networks/nodes", varyByCookie(s.OtherNetworkNodesHandler))
	handle("/analytics/pstn", varyByCookie(s.PSTNCMAnalyticsHandler))
	handle("/analytics/pstn-accessible", varyByCookie(s.ModemAccessibleAnalyticsHandler))
	handle("/analytics/pstn-no-answer", varyByCookie(s.ModemNoAnswerAnalyticsHandler))
	handle("/analytics/file-request", varyByCookie(s.FileRequestAnalyticsHandler))
	handle("/analytics/email", varyByCookie(s.EmailAnalyticsHandler))
	handle("/analytics/software/binkp", varyByCookie(s.BinkPSoftwareHandler))
	handle("/analytics/software/ifcico", varyByCookie(s.IfcicoSoftwareHandler))
	handle("/analytics/geo-hosting", varyByCookie(s.GeoHostingAnalyticsHandler))
	handle("/analytics/geo-hosting/country", varyByCookie(s.GeoCountryNodesHandler))
	handle("/analytics/geo-hosting/provider", varyByCookie(s.GeoProviderNodesHandler))
	handle("/analytics/pioneers", varyByCookie(s.PioneersHandler))
	// The list pages scope by the ftn_network cookie, so their output varies
	// by cookie; the /nodes drill-down keys on the ?domain= URL param instead.
	handle("/analytics/domain-expiration", varyByCookie(s.DomainExpirationHandler))
	handle("/analytics/domain-expiration/nodes", s.DomainNodesHandler)
	handle("/analytics/registrars", varyByCookie(s.RegistrarsHandler))
	handle("/analytics/on-this-day", varyByCookie(s.OnThisDayHandler))
	handle("/reachability", varyByCookie(s.ReachabilityHandler))
	handle("/reachability/node", varyByCookie(s.ReachabilityNodeHandler))
	handle("/reachability/test", varyByCookie(s.TestResultDetailHandler))
	handle("/reachability/modem-test", varyByCookie(s.ModemTestDetailHandler))

	// Serve static files
	mux.HandleFunc("/static/", s.StaticHandler)
}
