package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// SetupRouter creates and configures a Chi router with all API routes.
//
// The query budgets are applied per route group rather than once at the top.
// That is not a style choice: context.WithDeadline can only shorten a deadline
// it inherits, so a single budget here would cap every group below it and make
// the longer analytics budget unreachable. See internal/querybudget.
func (s *Server) SetupRouter() http.Handler {
	r := chi.NewRouter()

	// read is the ordinary budget; heavy is the one the analytics reports get.
	// Both are no-ops until query_budget.enabled is set, and on protocol: http
	// they stay no-ops regardless.
	read, heavy := s.budgets.Read.Wrap, s.budgets.Analytics.Wrap

	// Built-in Chi middleware
	r.Use(middleware.RequestID)
	// No middleware.RealIP here: cmd/server derives the client address once,
	// for logging, without rewriting r.RemoteAddr. Running RealIP on this
	// router as well gave the API and the web pages different answers to the
	// same question, and its rewrite is unconditional - a forged
	// X-Forwarded-For became indistinguishable from a real peer address.
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))

	// Note: LoggingMiddleware is now applied at the top level in cmd/server/main.go
	// to capture both API and web routes, so we don't need it here anymore

	// Health check endpoint
	r.Get("/api/health", s.HealthHandler)

	// Node routes
	r.Route("/api/nodes", func(r chi.Router) {
		r.Use(read)
		r.Get("/", s.SearchNodesHandler)
		r.Get("/pstn", s.GetPSTNNodesHandler)
		r.Get("/pstn/dead", s.ListPSTNDeadHandler)
		r.Get("/pstn/recent-success", s.GetRecentModemSuccessPhonesHandler)
		r.Get("/{zone}/{net}/{node}", s.GetNodeHandler)
		r.Get("/{zone}/{net}/{node}/history", s.GetNodeHistoryHandler)
		r.Get("/{zone}/{net}/{node}/changes", s.GetNodeChangesHandler)
		r.Get("/{zone}/{net}/{node}/timeline", s.GetNodeTimelineHandler)
		r.Get("/{zone}/{net}/{node}/points", s.GetNodePointsHandler)
	})

	// Point (FTS-5002 pointlist) routes
	r.Route("/api/points", func(r chi.Router) {
		r.Use(read)
		r.Get("/", s.SearchPointsHandler)
		r.Get("/{zone}/{net}/{node}/{point}", s.GetPointHandler)
		r.Get("/{zone}/{net}/{node}/{point}/history", s.GetPointHistoryHandler)
	})

	// Pointlist metadata routes
	r.Route("/api/pointlists", func(r chi.Router) {
		r.Use(read)
		r.Get("/dates", s.PointlistDatesHandler)
		r.Get("/sources", s.PointlistSourcesHandler)
	})

	// Network (FTN domain) routes
	r.With(read).Get("/api/networks", s.NetworksHandler)

	// Statistics routes
	r.With(read).Get("/api/stats", s.StatsHandler)
	r.With(read).Get("/api/stats/dates", s.GetAvailableDatesHandler)

	// Sysop routes
	r.Route("/api/sysops", func(r chi.Router) {
		r.Use(read)
		r.Get("/", s.SysopsHandler)
		r.Get("/{name}/nodes", s.SysopNodesHandler)
	})

	// Software analytics routes
	r.Route("/api/software", func(r chi.Router) {
		r.Use(heavy)
		r.Get("/binkp", s.GetBinkPSoftwareStats)
		r.Get("/ifcico", s.GetIFCICOSoftwareStats)
		r.Get("/binkd", s.GetBinkdDetailedStats)
	})

	// Geographic analytics routes
	r.Route("/api/analytics", func(r chi.Router) {
		r.Use(heavy)
		r.Get("/geo-hosting", s.GetGeoHostingStats)
	})

	// Documentation routes
	r.Get("/api/flags", s.FlagsDocumentationHandler)
	r.Get("/api/openapi.yaml", s.OpenAPISpecHandler)
	r.Get("/api/docs", s.SwaggerUIHandler)

	// Nodelist routes
	r.Get("/api/nodelist/latest", s.LatestNodelistAPIHandler)

	// Cache stats endpoint (if configured)
	if s.cacheStatsHandler != nil {
		r.Get("/api/cache/stats", s.cacheStatsHandler)
	}

	// FTP stats endpoint (if configured)
	if s.rateLimitStatsHandler != nil {
		r.Get("/api/ratelimit/stats", s.rateLimitStatsHandler)
	}

	if s.ftpStatsHandler != nil {
		r.Get("/api/ftp/stats", s.ftpStatsHandler)
	}

	// Modem testing API routes (authenticated with size limits)
	if s.modemHandler != nil {
		r.Route("/api/modem", func(r chi.Router) {
			r.Use(s.modemHandler.SizeLimitMiddleware())
			r.Use(s.modemHandler.AuthMiddleware())
			r.Post("/results/direct", s.modemHandler.SubmitResultsDirect)
			r.Post("/pstn-dead", s.MarkPSTNDeadHandler)
			r.Delete("/pstn-dead", s.UnmarkPSTNDeadHandler)
		})
	}

	return r
}
