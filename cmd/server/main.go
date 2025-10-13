package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/nodelistdb/internal/api"
	"github.com/nodelistdb/internal/cache"
	"github.com/nodelistdb/internal/config"
	"github.com/nodelistdb/internal/database"
	"github.com/nodelistdb/internal/ftp"
	"github.com/nodelistdb/internal/storage"
	"github.com/nodelistdb/internal/version"
	"github.com/nodelistdb/internal/web"
)

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

	// Load configuration
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Verify database configuration
	if cfg.ClickHouse.Host == "" {
		log.Fatalf("ClickHouse configuration is missing in %s", *configPath)
	}

	fmt.Println("FidoNet Nodelist Server (ClickHouse)")
	fmt.Println("====================================")
	fmt.Printf("Database: %s@%s:%d/%s\n", cfg.ClickHouse.Username, cfg.ClickHouse.Host, cfg.ClickHouse.Port, cfg.ClickHouse.Database)
	fmt.Printf("Server: http://%s:%s\n", *host, *port)
	fmt.Println()

	// Initialize ClickHouse database
	log.Println("Initializing ClickHouse database...")

	chConfig, err := cfg.ClickHouse.ToClickHouseDatabaseConfig()
	if err != nil {
		log.Fatalf("Invalid ClickHouse configuration: %v", err)
	}

	db, err := database.NewClickHouse(chConfig)

	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
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
	log.Println("Running in read-only mode - schema creation skipped")

	log.Println("Database initialized successfully")

	// Initialize storage layer
	storageLayer, err := storage.New(db)
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}
	defer storageLayer.Close()

	// Initialize cache if enabled
	var finalStorage storage.Operations = storageLayer
	var cacheImpl cache.Cache
	if cfg.Cache.Enabled {
		log.Println("Initializing BadgerCache...")

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
			log.Fatalf("Failed to initialize BadgerCache: %v", err)
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

		log.Printf("BadgerCache initialized successfully at %s", cfg.Cache.Path)
		log.Printf("Cache settings: Memory=%dMB, NodeTTL=%v, StatsTTL=%v, SearchTTL=%v",
			cfg.Cache.MaxMemoryMB, cfg.Cache.NodeTTL, cfg.Cache.StatsTTL, cfg.Cache.SearchTTL)
	} else {
		log.Println("Cache is disabled")
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
			log.Fatalf("Failed to initialize FTP server: %v", err)
		}
		log.Printf("FTP server configured on %s:%d", cfg.FTP.Host, cfg.FTP.Port)
	}

	// Initialize API and Web servers
	apiServer := api.New(finalStorage)
	webServer := web.New(finalStorage, web.TemplatesFS, web.StaticFS)

	// Set up HTTP routes
	mux := http.NewServeMux()

	// API routes
	apiServer.SetupRoutes(mux)

	// Web routes
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

	// Server configuration
	server := &http.Server{
		Addr:    fmt.Sprintf("%s:%s", *host, *port),
		Handler: mux,
	}

	// Start FTP server if enabled
	if ftpServer != nil {
		go func() {
			if err := ftpServer.Start(); err != nil {
				log.Fatalf("FTP server failed to start: %v", err)
			}
		}()
	}

	// Start server in a goroutine
	go func() {
		log.Printf("Server starting on http://%s:%s", *host, *port)
		log.Println("Available endpoints:")
		log.Println("  Web Interface:")
		log.Printf("    http://%s:%s/        - Home page\n", *host, *port)
		log.Printf("    http://%s:%s/search  - Search nodes\n", *host, *port)
		log.Printf("    http://%s:%s/stats   - Statistics\n", *host, *port)
		log.Println("  REST API:")
		log.Printf("    http://%s:%s/api/health          - API health check\n", *host, *port)
		log.Printf("    http://%s:%s/api/nodes           - Search nodes\n", *host, *port)
		log.Printf("    http://%s:%s/api/nodes/1/234/56  - Get specific node\n", *host, *port)
		log.Printf("    http://%s:%s/api/sysops          - List sysops\n", *host, *port)
		log.Printf("    http://%s:%s/api/sysops/{name}/nodes - Get nodes for sysop\n", *host, *port)
		log.Printf("    http://%s:%s/api/stats           - Network statistics\n", *host, *port)
		if cfg.Cache.Enabled {
			log.Printf("    http://%s:%s/api/cache/stats     - Cache statistics\n", *host, *port)
		}
		if ftpServer != nil {
			log.Printf("    http://%s:%s/api/ftp/stats       - FTP server statistics\n", *host, *port)
			log.Println("  FTP Server:")
			log.Printf("    ftp://%s:%d/                 - Anonymous FTP access (read-only)\n", *host, cfg.FTP.Port)
		}
		log.Println()

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Server shutting down...")

	// Stop FTP server first
	if ftpServer != nil {
		if err := ftpServer.Stop(); err != nil {
			log.Printf("FTP server shutdown error: %v", err)
		}
	}

	// Stop HTTP server
	if err := server.Close(); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	log.Println("Server stopped")
}
