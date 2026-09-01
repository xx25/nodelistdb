package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nodelistdb/internal/api"
	"github.com/nodelistdb/internal/cache"
	"github.com/nodelistdb/internal/config"
	"github.com/nodelistdb/internal/database"
	"github.com/nodelistdb/internal/ftp"
	"github.com/nodelistdb/internal/links"
	"github.com/nodelistdb/internal/logging"
	"github.com/nodelistdb/internal/querybudget"
	"github.com/nodelistdb/internal/ratelimit"
	"github.com/nodelistdb/internal/storage"
	"github.com/nodelistdb/internal/version"
	"github.com/nodelistdb/internal/web"
)

// options are the command-line flags.
type options struct {
	configPath string
	host       string
	port       string
	debugSQL   bool
}

func main() {
	var (
		opts        options
		showVersion = flag.Bool("version", false, "Show version information")
	)
	flag.StringVar(&opts.configPath, "config", "config.yaml", "Path to configuration file")
	flag.StringVar(&opts.port, "port", "8080", "HTTP server port")
	flag.StringVar(&opts.host, "host", "localhost", "HTTP server host")
	flag.BoolVar(&opts.debugSQL, "debug-sql", false, "Enable SQL query debugging")
	flag.Parse()

	if *showVersion {
		fmt.Printf("NodelistDB Server %s\n", version.GetFullVersionInfo())
		return
	}

	// run() returns errors rather than calling logging.Fatalf, which is
	// os.Exit(1) and therefore skips every deferred Close on the way out -
	// including the Badger cache's, which is the one that matters: a cache
	// closed by process death leaves its value log to be recovered on the
	// next start.
	if err := run(opts); err != nil {
		logging.Error("Server failed", slog.Any("error", err))
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func run(opts options) error {
	cfg, err := config.LoadConfig(opts.configPath)
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}

	if err := logging.Initialize(logging.FromStruct(&cfg.ServerLogging)); err != nil {
		return fmt.Errorf("initializing logging: %w", err)
	}

	logging.Info("NodelistDB server starting",
		slog.String("version", version.GetFullVersionInfo()),
		slog.String("config", opts.configPath))

	fmt.Println("FidoNet Nodelist Server (ClickHouse)")
	fmt.Println("====================================")
	fmt.Printf("Database: %s@%s:%d/%s\n", cfg.ClickHouse.Username, cfg.ClickHouse.Host, cfg.ClickHouse.Port, cfg.ClickHouse.Database)
	fmt.Printf("Server: http://%s:%s\n", opts.host, opts.port)
	fmt.Println()

	if opts.debugSQL {
		fmt.Println("=== SQL DEBUGGING ENABLED ===")
		fmt.Println("All SQL queries will be logged to console")
		fmt.Println("============================")
		_ = os.Setenv("DEBUG_SQL", "true")
	}

	db, err := openDatabase(cfg)
	if err != nil {
		return err
	}
	defer db.Close()

	deps, closeStorage, err := buildStorage(cfg, db)
	if err != nil {
		return err
	}
	defer closeStorage()

	ftpServer, err := buildFTP(cfg)
	if err != nil {
		return err
	}

	apiServer, err := buildAPI(cfg, db, deps, ftpServer)
	if err != nil {
		return err
	}

	webServer, err := web.New(deps.storage, web.TemplatesFS, web.StaticFS)
	if err != nil {
		return fmt.Errorf("loading web templates: %w", err)
	}

	readBudget, analyticsBudget, err := buildQueryBudgets(cfg)
	if err != nil {
		return err
	}
	apiServer.SetQueryBudgets(api.Budgets{Read: readBudget, Analytics: analyticsBudget})
	webServer.SetQueryBudgets(web.Budgets{Read: readBudget, Analytics: analyticsBudget})
	if cfg.LinksFile != "" {
		linksLoader := links.NewLoader(cfg.LinksFile)
		defer linksLoader.Stop()
		webServer.SetLinksLoader(linksLoader)
		logging.Info("Links configuration enabled", slog.String("path", cfg.LinksFile))
	}

	limiter, err := buildRateLimiter(cfg.RateLimit)
	if err != nil {
		return fmt.Errorf("rate limiter: %w", err)
	}
	if limiter != nil {
		apiServer.SetRateLimitStatsHandler(rateLimitStatsHandler(limiter))
	}

	return serve(buildHTTPServer(opts, apiServer, webServer, longestBudget(readBudget, analyticsBudget), limiter), ftpServer)
}

// longestBudget returns whichever budget runs longest. Sizing WriteTimeout off
// the analytics budget alone would be wrong the moment a config sets
// query_budget.default above query_budget.analytics - an odd thing to write,
// but nothing forbids it, and the failure would be the silent
// connection-cut-before-the-deadline trap this sizing exists to avoid.
func longestBudget(budgets ...querybudget.Budget) querybudget.Budget {
	longest := querybudget.Budget{}
	for _, b := range budgets {
		if b.Duration() > longest.Duration() {
			longest = b
		}
	}
	return longest
}

// buildQueryBudgets turns the config into the two deadlines the routers apply.
// Both come back zero - meaning no deadline - when the feature is off or when
// ClickHouse is being spoken to over HTTP, where the driver would turn the
// deadline into a max_execution_time that a readonly user may not send.
func buildQueryBudgets(cfg *config.Config) (read, analytics querybudget.Budget, err error) {
	readDur, analyticsDur, err := cfg.QueryBudget.Durations()
	if err != nil {
		return querybudget.Budget{}, querybudget.Budget{}, err
	}
	read = querybudget.New(cfg.QueryBudget.Enabled, readDur, cfg.ClickHouse.Protocol)
	analytics = querybudget.New(cfg.QueryBudget.Enabled, analyticsDur, cfg.ClickHouse.Protocol)
	if read.Duration() > 0 || analytics.Duration() > 0 {
		logging.Info("Query budgets enabled",
			slog.Duration("read", read.Duration()),
			slog.Duration("analytics", analytics.Duration()))
	} else if cfg.QueryBudget.Enabled {
		logging.Info("Query budgets configured but withheld (clickhouse protocol is http)")
	}
	return read, analytics, nil
}

// serverDeps are the storage objects the rest of the server is built on.
type serverDeps struct {
	storage storage.Operations
	cache   cache.Cache // nil when caching is disabled
}

func openDatabase(cfg *config.Config) (*database.ClickHouseDB, error) {
	logging.Info("Initializing ClickHouse database")

	chConfig, err := cfg.ClickHouse.ToClickHouseDatabaseConfig()
	if err != nil {
		return nil, fmt.Errorf("invalid ClickHouse configuration: %w", err)
	}

	db, err := database.NewClickHouse(chConfig)
	if err != nil {
		return nil, fmt.Errorf("initializing database: %w", err)
	}

	if v, err := db.GetVersion(); err == nil {
		fmt.Printf("ClickHouse version: %s\n", v)
	}
	logging.Info("Database initialized successfully")
	return db, nil
}

// buildStorage builds the storage layer and, when enabled, the cache in front
// of it. The returned close function releases both in the right order.
func buildStorage(cfg *config.Config, db *database.ClickHouseDB) (serverDeps, func(), error) {
	storageLayer, err := storage.New(db)
	if err != nil {
		return serverDeps{}, nil, fmt.Errorf("initializing storage: %w", err)
	}

	deps := serverDeps{storage: storageLayer}
	closers := []func(){func() { _ = storageLayer.Close() }}
	closeAll := func() {
		for i := len(closers) - 1; i >= 0; i-- {
			closers[i]()
		}
	}

	if !cfg.Cache.Enabled {
		logging.Info("Cache is disabled")
		return deps, closeAll, nil
	}

	cacheConfig := cfg.Cache.ToCacheConfig()
	logging.Info("Initializing cache", slog.String("type", cacheConfig.Type))

	cacheImpl, err := cache.New(cacheConfig)
	if err != nil {
		closeAll()
		return serverDeps{}, nil, fmt.Errorf("initializing cache: %w", err)
	}
	closers = append(closers, func() { _ = cacheImpl.Close() })

	deps.cache = cacheImpl
	deps.storage = storage.NewCachedStorage(storageLayer, cacheImpl, cfg.Cache.ToCacheStorageConfig())

	logging.Info("Cache initialized successfully",
		slog.String("type", cacheConfig.Type),
		slog.Duration("node_ttl", cfg.Cache.NodeTTL),
		slog.Duration("stats_ttl", cfg.Cache.StatsTTL),
		slog.Duration("search_ttl", cfg.Cache.SearchTTL))

	return deps, closeAll, nil
}

func buildFTP(cfg *config.Config) (*ftp.Server, error) {
	if !cfg.FTP.Enabled {
		return nil, nil
	}

	ftpConfig := cfg.FTP.ToFTPConfig()
	server, err := ftp.New(ftpConfig)
	if err != nil {
		return nil, fmt.Errorf("initializing FTP server: %w", err)
	}

	logging.Info("FTP server configured",
		slog.String("host", cfg.FTP.Host),
		slog.Int("port", cfg.FTP.Port),
		slog.Int("mounts", len(ftpConfig.Mounts)))
	return server, nil
}

// buildAPI wires the API server and everything optional hanging off it. Every
// dependency is installed before SetupRouter runs, which router.go requires:
// the cache-stats, FTP-stats and modem routes are registered only when their
// field is already non-nil, so setting one afterwards silently loses routes.
func buildAPI(cfg *config.Config, db *database.ClickHouseDB, deps serverDeps, ftpServer *ftp.Server) (*api.Server, error) {
	apiServer := api.New(deps.storage)

	apiServer.SetHealthChecker(&serverHealthChecker{
		db:        db,
		storage:   deps.storage,
		cache:     deps.cache,
		ftpServer: ftpServer,
		startTime: time.Now(),
	})

	if cfg.ModemAPI.Enabled {
		logging.Info("Initializing modem testing API")
		apiServer.SetModemHandler(api.NewModemHandler(&cfg.ModemAPI, storage.NewModemResultOperations(db)))
		logging.Info("Modem API initialized", slog.Int("callers", len(cfg.ModemAPI.Callers)))
	}

	if deps.cache != nil {
		apiServer.SetCacheStatsHandler(cacheStatsHandler(deps.cache))
	}
	if ftpServer != nil {
		apiServer.SetFTPStatsHandler(ftpStatsHandler(ftpServer))
	}

	return apiServer, nil
}

func buildHTTPServer(opts options, apiServer *api.Server, webServer *web.Server, longest querybudget.Budget, limiter *ratelimit.Middleware) *http.Server {
	// The API is a Chi router mounted under /api/; the web pages are on a
	// plain ServeMux. One logging middleware wraps both.
	mux := http.NewServeMux()
	mux.Handle("/api/", apiServer.SetupRouter())
	webServer.SetupRoutes(mux)

	// WriteTimeout is a TCP-level deadline that does NOT cancel r.Context(),
	// so it silently caps every query budget: a route allowed 120s would still
	// have its connection cut at 60. Keep it strictly above the longest budget
	// rather than letting the two disagree.
	writeTimeout := 60 * time.Second
	if b := longest.Duration(); b+writeSlack > writeTimeout {
		writeTimeout = b + writeSlack
	}

	// The limiter sits inside the logging middleware, so a rejected request
	// still produces a log line with its 429 - without that, the only
	// evidence of throttling would be traffic that silently disappears.
	var handler http.Handler = mux
	if limiter != nil {
		handler = limiter.Wrap(handler)
	}

	return &http.Server{
		Addr:              fmt.Sprintf("%s:%s", opts.host, opts.port),
		Handler:           loggingMiddleware(handler),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       120 * time.Second,
	}
}

// writeSlack is the head-room WriteTimeout keeps over the longest query
// budget, so that a request which times out has room to render and send its
// own 503 instead of having the connection dropped underneath it.
const writeSlack = 15 * time.Second

// serve runs the HTTP (and optionally FTP) servers until a signal arrives,
// then shuts them down. It returns rather than exiting, so run's defers fire.
func serve(server *http.Server, ftpServer *ftp.Server) error {
	errs := make(chan error, 2)

	if ftpServer != nil {
		go func() {
			if err := ftpServer.Start(); err != nil {
				errs <- fmt.Errorf("FTP server: %w", err)
			}
		}()
	}

	go func() {
		logging.Info("Server listening", slog.String("address", "http://"+server.Addr))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errs <- fmt.Errorf("HTTP server: %w", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	var runErr error
	select {
	case runErr = <-errs:
	case <-quit:
	}

	logging.Info("Server shutting down")

	if ftpServer != nil {
		if err := ftpServer.Stop(); err != nil {
			logging.Error("FTP server shutdown error", slog.Any("error", err))
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logging.Error("Graceful shutdown timed out, forcing close", slog.Any("error", err))
		if err := server.Close(); err != nil {
			logging.Error("Server force close error", slog.Any("error", err))
		}
	}

	logging.Info("Server stopped")
	return runErr
}
