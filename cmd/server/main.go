package main

import (
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/nodelistdb/internal/api"
	"github.com/nodelistdb/internal/cache"
	"github.com/nodelistdb/internal/config"
	"github.com/nodelistdb/internal/database"
	"github.com/nodelistdb/internal/ftp"
	"github.com/nodelistdb/internal/logging"
	"github.com/nodelistdb/internal/storage"
	"github.com/nodelistdb/internal/version"
	"github.com/nodelistdb/internal/web"
)

// loggingMiddleware wraps an http.Handler to log all HTTP requests
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap response writer to capture status code
		wrapped := &loggingResponseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		// Call the next handler
		next.ServeHTTP(wrapped, r)

		// Get real client IP (handles reverse proxy headers)
		clientIP := getRealIP(r)

		// Log the request
		logging.Info("HTTP request",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", wrapped.statusCode),
			slog.Duration("duration", time.Since(start)),
			slog.String("ip", clientIP),
		)
	})
}

// getRealIP extracts the real client IP from request headers when behind a reverse proxy
// Checks common proxy headers in order of preference
func getRealIP(r *http.Request) string {
	// X-Real-IP is set by many reverse proxies (including Caddy by default)
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}

	// X-Forwarded-For may contain multiple IPs (client, proxy1, proxy2...)
	// The first one is the original client
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP if there are multiple
		if idx := strings.Index(xff, ","); idx != -1 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}

	// Cloudflare uses CF-Connecting-IP
	if ip := r.Header.Get("CF-Connecting-IP"); ip != "" {
		return ip
	}

	// Fallback to RemoteAddr (direct connection or proxy not configured)
	return r.RemoteAddr
}

// loggingResponseWriter wraps http.ResponseWriter to capture the status code
type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (lrw *loggingResponseWriter) WriteHeader(code int) {
	lrw.statusCode = code
	lrw.ResponseWriter.WriteHeader(code)
}

func main() {
	// Command line flags
	var (
		configPath  = flag.String("config", "config.yaml", "Path to configuration file")
		port        = flag.String("port", "8080", "HTTP server port")
		host        = flag.String("host", "localhost", "HTTP server host")
		showVersion = flag.Bool("version", false, "Show version information")
		debugSQL    = flag.Bool("debug-sql", false, "Enable SQL query debugging")
	)
	flag.Parse()

	// Handle version flag
	if *showVersion {
		fmt.Printf("NodelistDB Server %s\n", version.GetFullVersionInfo())
		os.Exit(0)
	}

	// Load configuration first
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Initialize logging from configuration (using server-specific logging config)
	if err := logging.Initialize(logging.FromStruct(&cfg.ServerLogging)); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logging: %v\n", err)
		os.Exit(1)
	}

	logging.Info("NodelistDB server starting",
		slog.String("version", version.GetFullVersionInfo()),
		slog.String("config", *configPath))

	// Verify database configuration
	if cfg.ClickHouse.Host == "" {
		logging.Fatalf("ClickHouse configuration is missing in %s", *configPath)
	}

	fmt.Println("FidoNet Nodelist Server (ClickHouse)")
	fmt.Println("====================================")
	fmt.Printf("Database: %s@%s:%d/%s\n", cfg.ClickHouse.Username, cfg.ClickHouse.Host, cfg.ClickHouse.Port, cfg.ClickHouse.Database)
	fmt.Printf("Server: http://%s:%s\n", *host, *port)
	fmt.Println()

	// Initialize ClickHouse database
	logging.Info("Initializing ClickHouse database")

	chConfig, err := cfg.ClickHouse.ToClickHouseDatabaseConfig()
	if err != nil {
		logging.Fatalf("Invalid ClickHouse configuration: %v", err)
	}

	db, err := database.NewClickHouse(chConfig)

	if err != nil {
		logging.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	// Get database version
	if version, err := db.GetVersion(); err == nil {
		fmt.Printf("ClickHouse version: %s\n", version)
	}

	// Enable SQL debugging if requested
	if *debugSQL {
		fmt.Println("=== SQL DEBUGGING ENABLED ===")
		fmt.Println("All SQL queries will be logged to console")
		fmt.Println("============================")
		os.Setenv("DEBUG_SQL", "true")
	}

	// Skip schema creation in read-only mode
	logging.Info("Running in read-only mode - schema creation skipped")

	logging.Info("Database initialized successfully")

	// Initialize storage layer
	storageLayer, err := storage.New(db)
	if err != nil {
		logging.Fatalf("Failed to initialize storage: %v", err)
	}
	defer storageLayer.Close()

	// Initialize cache if enabled
	var finalStorage storage.Operations = storageLayer
	var cacheImpl cache.Cache
	if cfg.Cache.Enabled {
		logging.Info("Initializing BadgerCache")

		// Create BadgerCache
		badgerConfig := &cache.BadgerConfig{
			Path:              cfg.Cache.Path,
			MaxMemoryMB:       cfg.Cache.MaxMemoryMB,
			ValueLogMaxMB:     cfg.Cache.ValueLogMaxMB,
			CompactL0OnClose:  cfg.Cache.CompactOnClose,
			NumGoroutines:     4,
			GCInterval:        cfg.Cache.GCInterval,
			GCDiscardRatio:    cfg.Cache.GCDiscardRatio,
		}
		cacheImpl, err = cache.NewBadgerCache(badgerConfig)
		if err != nil {
			logging.Fatalf("Failed to initialize BadgerCache: %v", err)
		}
		defer cacheImpl.Close()

		// Wrap storage with cache
		cacheStorageConfig := &storage.CacheStorageConfig{
			Enabled:          true,
			DefaultTTL:       cfg.Cache.DefaultTTL,
			NodeTTL:          cfg.Cache.NodeTTL,
			StatsTTL:         cfg.Cache.StatsTTL,
			SearchTTL:        cfg.Cache.SearchTTL,
			MaxSearchResults: cfg.Cache.MaxSearchResults,
			WarmupOnStart:    cfg.Cache.WarmupOnStart,
		}
		cachedStorage := storage.NewCachedStorage(storageLayer, cacheImpl, cacheStorageConfig)
		finalStorage = cachedStorage

		logging.Info("BadgerCache initialized successfully",
			slog.String("path", cfg.Cache.Path),
			slog.Int("memory_mb", cfg.Cache.MaxMemoryMB),
			slog.Duration("node_ttl", cfg.Cache.NodeTTL),
			slog.Duration("stats_ttl", cfg.Cache.StatsTTL),
			slog.Duration("search_ttl", cfg.Cache.SearchTTL))
	} else {
		logging.Info("Cache is disabled")
	}

	// Initialize FTP server if enabled
	var ftpServer *ftp.Server
	if cfg.FTP.Enabled {
		ftpConfig := &ftp.Config{
			Enabled:        cfg.FTP.Enabled,
			Host:           cfg.FTP.Host,
			Port:           cfg.FTP.Port,
			NodelistPath:   cfg.FTP.NodelistPath,
			MaxConnections: cfg.FTP.MaxConnections,
			PassivePortMin: cfg.FTP.PassivePortMin,
			PassivePortMax: cfg.FTP.PassivePortMax,
			IdleTimeout:    cfg.FTP.IdleTimeout,
			PublicHost:     cfg.FTP.PublicHost,
		}

		var err error
		ftpServer, err = ftp.New(ftpConfig)
		if err != nil {
			logging.Fatalf("Failed to initialize FTP server: %v", err)
		}
		logging.Info("FTP server configured",
			slog.String("host", cfg.FTP.Host),
			slog.Int("port", cfg.FTP.Port))
	}

	// Initialize API and Web servers
	apiServer := api.New(finalStorage)
	webServer := web.New(finalStorage, web.TemplatesFS, web.StaticFS)

	// Set up HTTP routes using Chi router
	apiRouter := apiServer.SetupRouter()

	// Set up combined routes - wrap Chi router with ServeMux for web routes
	mux := http.NewServeMux()

	// Mount API routes under Chi router
	mux.Handle("/api/", apiRouter)

	// Web routes (keep existing setup)
	webServer.SetupRoutes(mux)

	// Cache stats endpoint if cache is enabled
	if cfg.Cache.Enabled && cacheImpl != nil {
		mux.HandleFunc("/api/cache/stats", func(w http.ResponseWriter, r *http.Request) {
			metrics := cacheImpl.GetMetrics()
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"hits":%d,"misses":%d,"sets":%d,"deletes":%d,"size":%d,"keys":%d,"hit_rate":%.2f}`,
				metrics.Hits, metrics.Misses, metrics.Sets, metrics.Deletes,
				metrics.Size, metrics.Keys,
				float64(metrics.Hits)/float64(metrics.Hits+metrics.Misses+1)*100)
		})
	}

	// FTP stats endpoint if FTP is enabled
	if ftpServer != nil {
		mux.HandleFunc("/api/ftp/stats", func(w http.ResponseWriter, r *http.Request) {
			stats := ftpServer.GetStats()
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"enabled":%t,"host":"%s","port":%d,"max_connections":%d}`,
				stats["enabled"].(bool),
				stats["host"].(string),
				stats["port"].(int),
				stats["max_connections"].(int))
		})
	}

	// Wrap entire mux with logging middleware to capture all requests (API + Web)
	loggingHandler := loggingMiddleware(mux)

	// Server configuration
	server := &http.Server{
		Addr:    fmt.Sprintf("%s:%s", *host, *port),
		Handler: loggingHandler,
	}

	// Start FTP server if enabled
	if ftpServer != nil {
		go func() {
			if err := ftpServer.Start(); err != nil {
				logging.Fatalf("FTP server failed to start: %v", err)
			}
		}()
	}

	// Start server in a goroutine
	go func() {
		logging.Info("Server starting", slog.String("address", fmt.Sprintf("http://%s:%s", *host, *port)))
		logging.Info("Available endpoints:")
		logging.Info("  Web Interface:")
		logging.Infof("    http://%s:%s/        - Home page", *host, *port)
		logging.Infof("    http://%s:%s/search  - Search nodes", *host, *port)
		logging.Infof("    http://%s:%s/stats   - Statistics", *host, *port)
		logging.Info("  REST API:")
		logging.Infof("    http://%s:%s/api/health          - API health check", *host, *port)
		logging.Infof("    http://%s:%s/api/nodes           - Search nodes", *host, *port)
		logging.Infof("    http://%s:%s/api/nodes/1/234/56  - Get specific node", *host, *port)
		logging.Infof("    http://%s:%s/api/sysops          - List sysops", *host, *port)
		logging.Infof("    http://%s:%s/api/sysops/{name}/nodes - Get nodes for sysop", *host, *port)
		logging.Infof("    http://%s:%s/api/stats           - Network statistics", *host, *port)
		if cfg.Cache.Enabled {
			logging.Infof("    http://%s:%s/api/cache/stats     - Cache statistics", *host, *port)
		}
		if ftpServer != nil {
			logging.Infof("    http://%s:%s/api/ftp/stats       - FTP server statistics", *host, *port)
			logging.Info("  FTP Server:")
			logging.Infof("    ftp://%s:%d/                 - Anonymous FTP access (read-only)", *host, cfg.FTP.Port)
		}

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logging.Fatalf("Server failed to start: %v", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logging.Info("Server shutting down")

	// Stop FTP server first
	if ftpServer != nil {
		if err := ftpServer.Stop(); err != nil {
			logging.Error("FTP server shutdown error", slog.Any("error", err))
		}
	}

	// Stop HTTP server
	if err := server.Close(); err != nil {
		logging.Error("Server shutdown error", slog.Any("error", err))
	}

	logging.Info("Server stopped")
}
