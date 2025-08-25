package main

import (
	"flag"
	"fmt"
	"log"
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
	"github.com/nodelistdb/internal/storage"
	"github.com/nodelistdb/internal/version"
	"github.com/nodelistdb/internal/web"
)

func main() {
	// Command line flags
	var (
		configPath  = flag.String("config", "config.yaml", "Path to configuration file")
		dbPath      = flag.String("db", "", "Path to database file (overrides config)")
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

	// Override database path if specified via command line
	if *dbPath != "" && cfg.Database.Type == config.DatabaseTypeDuckDB {
		cfg.Database.DuckDB.Path = *dbPath
	}

	// Set read-only mode for server
	if cfg.Database.Type == config.DatabaseTypeDuckDB {
		cfg.Database.DuckDB.ReadOnly = true
	}

	fmt.Printf("FidoNet Nodelist Server (%s)\n", strings.ToUpper(string(cfg.Database.Type)))
	fmt.Println("===============================")
	switch cfg.Database.Type {
	case config.DatabaseTypeDuckDB:
		fmt.Printf("Database: %s\n", cfg.Database.DuckDB.Path)
	case config.DatabaseTypeClickHouse:
		fmt.Printf("Database: %s@%s:%d/%s\n", cfg.Database.ClickHouse.Username, cfg.Database.ClickHouse.Host, cfg.Database.ClickHouse.Port, cfg.Database.ClickHouse.Database)
	}
	fmt.Printf("Server: http://%s:%s\n", *host, *port)
	fmt.Println()

	// Initialize database based on configuration
	log.Printf("Initializing %s database in read-only mode...\n", cfg.Database.Type)

	var db database.DatabaseInterface
	switch cfg.Database.Type {
	case config.DatabaseTypeDuckDB:
		db, err = database.NewReadOnly(cfg.Database.DuckDB.Path)
	case config.DatabaseTypeClickHouse:
		// Convert config to ClickHouse config and create connection
		chConfig := &database.ClickHouseConfig{
			Host:         cfg.Database.ClickHouse.Host,
			Port:         cfg.Database.ClickHouse.Port,
			Database:     cfg.Database.ClickHouse.Database,
			Username:     cfg.Database.ClickHouse.Username,
			Password:     cfg.Database.ClickHouse.Password,
			UseSSL:       cfg.Database.ClickHouse.UseSSL,
			MaxOpenConns: cfg.Database.ClickHouse.MaxOpenConns,
			MaxIdleConns: cfg.Database.ClickHouse.MaxIdleConns,
		}

		// Parse timeout strings
		if cfg.Database.ClickHouse.DialTimeout != "" {
			if chConfig.DialTimeout, err = time.ParseDuration(cfg.Database.ClickHouse.DialTimeout); err != nil {
				log.Fatalf("Invalid dial timeout: %v", err)
			}
		} else {
			chConfig.DialTimeout = 30 * time.Second
		}

		if cfg.Database.ClickHouse.ReadTimeout != "" {
			if chConfig.ReadTimeout, err = time.ParseDuration(cfg.Database.ClickHouse.ReadTimeout); err != nil {
				log.Fatalf("Invalid read timeout: %v", err)
			}
		} else {
			chConfig.ReadTimeout = 5 * time.Minute
		}

		if cfg.Database.ClickHouse.WriteTimeout != "" {
			if chConfig.WriteTimeout, err = time.ParseDuration(cfg.Database.ClickHouse.WriteTimeout); err != nil {
				log.Fatalf("Invalid write timeout: %v", err)
			}
		} else {
			chConfig.WriteTimeout = 1 * time.Minute
		}

		chConfig.Compression = cfg.Database.ClickHouse.Compression

		db, err = database.NewClickHouseReadOnly(chConfig)
	default:
		log.Fatalf("Unsupported database type: %s", cfg.Database.Type)
	}

	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	// Get database version
	if version, err := db.GetVersion(); err == nil {
		fmt.Printf("%s version: %s\n", strings.ToUpper(string(cfg.Database.Type)), version)
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
		log.Printf("Initializing %s cache...", cfg.Cache.Type)
		
		// Create cache implementation
		switch cfg.Cache.Type {
		case "badger":
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
				log.Fatalf("Failed to initialize Badger cache: %v", err)
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
			
			log.Printf("Cache initialized successfully at %s", cfg.Cache.Path)
			log.Printf("Cache settings: Memory=%dMB, NodeTTL=%v, StatsTTL=%v, SearchTTL=%v",
				cfg.Cache.MaxMemoryMB, cfg.Cache.NodeTTL, cfg.Cache.StatsTTL, cfg.Cache.SearchTTL)
		default:
			log.Printf("Unknown cache type: %s, cache disabled", cfg.Cache.Type)
		}
	} else {
		log.Println("Cache is disabled")
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

	// Server configuration
	server := &http.Server{
		Addr:    fmt.Sprintf("%s:%s", *host, *port),
		Handler: mux,
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
	if err := server.Close(); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	log.Println("Server stopped")
}
