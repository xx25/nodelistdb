package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// SetupRouter creates and configures a Chi router with all API routes
func (s *Server) SetupRouter() http.Handler {
	r := chi.NewRouter()

	// Built-in Chi middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))

	// Note: LoggingMiddleware is now applied at the top level in cmd/server/main.go
	// to capture both API and web routes, so we don't need it here anymore

	// Health check endpoint
	r.Get("/api/health", s.HealthHandler)

	// Node routes
	r.Route("/api/nodes", func(r chi.Router) {
		r.Get("/", s.SearchNodesHandler)
		r.Get("/pstn", s.GetPSTNNodesHandler)
		r.Get("/pstn/recent-success", s.GetRecentModemSuccessPhonesHandler)
		r.Get("/{zone}/{net}/{node}", s.GetNodeHandler)
		r.Get("/{zone}/{net}/{node}/history", s.GetNodeHistoryHandler)
		r.Get("/{zone}/{net}/{node}/changes", s.GetNodeChangesHandler)
		r.Get("/{zone}/{net}/{node}/timeline", s.GetNodeTimelineHandler)
	})

	// Statistics routes
	r.Get("/api/stats", s.StatsHandler)
	r.Get("/api/stats/dates", s.GetAvailableDatesHandler)

	// Sysop routes
	r.Route("/api/sysops", func(r chi.Router) {
		r.Get("/", s.SysopsHandler)
		r.Get("/{name}/nodes", s.SysopNodesHandler)
	})

	// Software analytics routes
	r.Route("/api/software", func(r chi.Router) {
		r.Get("/binkp", s.GetBinkPSoftwareStats)
		r.Get("/ifcico", s.GetIFCICOSoftwareStats)
		r.Get("/binkd", s.GetBinkdDetailedStats)
		r.Get("/trends", s.GetSoftwareTrends)
	})

	// Geographic analytics routes
	r.Route("/api/analytics", func(r chi.Router) {
		r.Get("/geo-hosting", s.GetGeoHostingStats)
	})

	// Documentation routes
	r.Get("/api/flags", s.FlagsDocumentationHandler)
	r.Get("/api/openapi.yaml", s.OpenAPISpecHandler)
	r.Get("/api/docs", s.SwaggerUIHandler)

	// Nodelist routes
	r.Get("/api/nodelist/latest", s.LatestNodelistAPIHandler)

	// Modem testing API routes (authenticated with size limits)
	if s.modemHandler != nil {
		r.Route("/api/modem", func(r chi.Router) {
			r.Use(s.modemHandler.SizeLimitMiddleware())
			r.Use(s.modemHandler.AuthMiddleware())
			r.Get("/nodes", s.modemHandler.GetAssignedNodes)
			r.Post("/in-progress", s.modemHandler.MarkInProgress)
			r.Post("/results", s.modemHandler.SubmitResults)
			r.Post("/results/direct", s.modemHandler.SubmitResultsDirect)
			r.Post("/heartbeat", s.modemHandler.Heartbeat)
			r.Post("/release", s.modemHandler.ReleaseNodes)
			r.Get("/stats", s.modemHandler.GetQueueStats) // Admin endpoint
		})
	}

	return r
}
