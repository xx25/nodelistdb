package daemon

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/nodelistdb/internal/cache"
	"github.com/nodelistdb/internal/testing/logging"
	"github.com/nodelistdb/internal/testing/protocols"
	"github.com/nodelistdb/internal/testing/services"
	"github.com/nodelistdb/internal/testing/storage"
)

// Daemon is the main testing daemon
type Daemon struct {
	config  *Config
	storage storage.Storage

	// Services
	dnsResolver *services.DNSResolver
	geolocator  *services.Geolocation

	// Persistent cache (optional) - now uses unified cache interface
	persistentCache cache.Cache

	// Protocol testers
	binkpTester  protocols.Tester
	ifcicoTester protocols.Tester
	telnetTester protocols.Tester
	ftpTester    protocols.Tester
	vmodemTester protocols.Tester

	// Worker pool
	workerPool *WorkerPool

	// Scheduler for intelligent test scheduling
	scheduler *Scheduler

	// Modular components
	testExecutor        *TestExecutor
	testAggregator      *TestAggregator
	statisticsCollector *StatisticsCollector
	nodeFilter          *NodeFilter

	// Control state
	pauseMu sync.RWMutex
	paused  bool

	// Debug mode
	debugMu sync.RWMutex
	debug   bool

	// Track nodelist updates
	lastNodelistDate time.Time
	nodelistMu       sync.RWMutex

	// Statistics
	stats struct {
		sync.Mutex
		startTime      time.Time
		cyclesRun      int
		totalTested    int
		totalSuccesses int
		totalFailures  int
		lastCycleTime  time.Time
	}
}

// New creates a new daemon instance
func New(cfg *Config) (*Daemon, error) {
	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	// Initialize logging
	logConfig := &logging.Config{
		Level:      cfg.Logging.Level,
		File:       cfg.Logging.File,
		MaxSize:    cfg.Logging.MaxSize,
		MaxBackups: cfg.Logging.MaxBackups,
		MaxAge:     cfg.Logging.MaxAge,
		Console:    cfg.Logging.Console, // Use config value instead of hardcoding
	}
	if err := logging.Initialize(logConfig); err != nil {
		return nil, fmt.Errorf("failed to initialize logging: %w", err)
	}

	// Log the configured log level for verification
	logging.Infof("Logging initialized with level: %s", cfg.Logging.Level)
	logging.Debugf("Debug logging test - if you see this, debug mode is working!")

	// Initialize ClickHouse storage (only supported database type)
	if cfg.ClickHouse == nil {
		return nil, fmt.Errorf("ClickHouse configuration is required")
	}

	chConfig := &storage.ClickHouseConfig{
		MaxOpenConns:  cfg.ClickHouse.MaxOpenConns,
		MaxIdleConns:  cfg.ClickHouse.MaxIdleConns,
		DialTimeout:   cfg.ClickHouse.DialTimeout,
		ReadTimeout:   cfg.ClickHouse.ReadTimeout,
		WriteTimeout:  cfg.ClickHouse.WriteTimeout,
		Compression:   cfg.ClickHouse.Compression,
		BatchSize:     cfg.ClickHouse.BatchSize,
		FlushInterval: cfg.ClickHouse.FlushInterval,
	}
	store, err := storage.NewClickHouseStorageWithConfig(
		cfg.ClickHouse.Host,
		cfg.ClickHouse.Port,
		cfg.ClickHouse.Database,
		cfg.ClickHouse.Username,
		cfg.ClickHouse.Password,
		chConfig,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to initialize storage: %w", err)
	}

	d := &Daemon{
		config:  cfg,
		storage: store,
	}

	// Initialize persistent cache using testdaemon_cache configuration
	// Now uses unified BadgerCache interface
	if cfg.TestdaemonCache.Enabled && cfg.TestdaemonCache.Path != "" {
		pCache, err := cache.NewBadgerCache(&cache.BadgerConfig{
			Path:              cfg.TestdaemonCache.Path,
			MaxMemoryMB:       256, // Default for testing daemon
			ValueLogMaxMB:     100,
			CompactL0OnClose:  true,
			NumGoroutines:     4,
			GCInterval:        10 * time.Minute,
			GCDiscardRatio:    0.5,
		})
		if err != nil {
			logging.Warnf("Failed to initialize persistent cache: %v", err)
			logging.Infof("Continuing with in-memory cache only")
		} else {
			d.persistentCache = pCache
			logging.Infof("Persistent BadgerCache initialized at %s", cfg.TestdaemonCache.Path)
		}
	}

	// Initialize services with configured TTLs
	d.dnsResolver = services.NewDNSResolverWithTTL(
		cfg.Services.DNS.Workers,
		cfg.Services.DNS.Timeout,
		cfg.Services.DNS.CacheTTL,
	)

	d.geolocator = services.NewGeolocationWithConfig(
		cfg.Services.Geolocation.Provider,
		cfg.Services.Geolocation.APIKey,
		cfg.Services.Geolocation.CacheTTL,
		cfg.Services.Geolocation.RateLimit,
	)

	// Wire persistent cache to services if available
	if d.persistentCache != nil {
		dnsCache := storage.NewDNSCache(d.persistentCache)
		d.dnsResolver.SetPersistentCache(dnsCache)

		geoCache := storage.NewGeolocationCache(d.persistentCache)
		d.geolocator.SetPersistentCache(geoCache)
	}

	// Set debug mode based on logging level
	debugMode := cfg.Logging.Level == "debug"
	d.debug = debugMode

	// Initialize protocol testers
	if cfg.Protocols.BinkP.Enabled {
		d.binkpTester = protocols.NewBinkPTesterWithInfo(
			cfg.Protocols.BinkP.Timeout,
			cfg.Protocols.BinkP.OurAddress,
			cfg.Protocols.BinkP.SystemName,
			cfg.Protocols.BinkP.Sysop,
			cfg.Protocols.BinkP.Location,
		)
		// Set debug mode if BinkP tester supports it
		if setter, ok := d.binkpTester.(protocols.DebugSetter); ok {
			setter.SetDebug(debugMode)
		}
	}

	if cfg.Protocols.Ifcico.Enabled {
		d.ifcicoTester = protocols.NewIfcicoTesterWithInfo(
			cfg.Protocols.Ifcico.Timeout,
			cfg.Protocols.Ifcico.OurAddress,
			cfg.Protocols.Ifcico.SystemName,
			cfg.Protocols.Ifcico.Sysop,
			cfg.Protocols.Ifcico.Location,
		)
		// Set debug mode for ifcico tester
		if setter, ok := d.ifcicoTester.(protocols.DebugSetter); ok {
			setter.SetDebug(debugMode)
		}
	}

	if cfg.Protocols.Telnet.Enabled {
		d.telnetTester = protocols.NewTelnetTester(
			cfg.Protocols.Telnet.Timeout,
		)
	}

	if cfg.Protocols.FTP.Enabled {
		d.ftpTester = protocols.NewFTPTester(
			cfg.Protocols.FTP.Timeout,
		)
	}

	if cfg.Protocols.VModem.Enabled {
		d.vmodemTester = protocols.NewVModemTester(
			cfg.Protocols.VModem.Timeout,
		)
	}

	// Initialize worker pool
	d.workerPool = NewWorkerPool(cfg.Daemon.Workers)

	// Initialize scheduler with adaptive strategy by default
	d.scheduler = NewScheduler(SchedulerConfig{
		Strategy:            StrategyAdaptive,
		BaseInterval:        cfg.Daemon.TestInterval,
		FailureMultiplier:   2.0,
		MaxInterval:         7 * 24 * time.Hour, // Allow up to 7 days to accommodate 72h test interval
		MaxBackoffLevel:     5,
		JitterPercent:       0.1,
		StaleTestThreshold:  cfg.Daemon.StaleTestThreshold,
		FailedRetryInterval: cfg.Daemon.FailedRetryInterval,
	}, store)

	// Initialize modular components
	d.testExecutor = NewTestExecutor(d)
	d.testAggregator = NewTestAggregator()
	d.statisticsCollector = NewStatisticsCollector()
	d.nodeFilter = NewNodeFilter()

	return d, nil
}

// Run starts the daemon
func (d *Daemon) Run(ctx context.Context) error {
	// Log version if available
	if d.config.Version != "" {
		logging.Infof("Starting NodeTest Daemon %s with %d workers", d.config.Version, d.config.Daemon.Workers)
	} else {
		logging.Infof("Starting NodeTest Daemon with %d workers", d.config.Daemon.Workers)
	}

	// Record start time for uptime calculation
	d.stats.Lock()
	d.stats.startTime = time.Now()
	d.stats.Unlock()

	// Start CLI server if enabled
	if err := d.StartCLIServer(ctx); err != nil {
		return fmt.Errorf("failed to start CLI server: %w", err)
	}

	// Start worker pool
	d.workerPool.Start()
	defer d.workerPool.Stop()

	// Initialize scheduler with current nodes
	nodes, err := d.storage.GetNodesWithInternet(ctx, 0)
	if err != nil {
		logging.Errorf("Failed to initialize scheduler with nodes: %v", err)
	} else {
		if err := d.scheduler.InitializeSchedules(ctx, nodes); err != nil {
			logging.Errorf("Failed to initialize scheduler schedules: %v", err)
		} else {
			logging.Infof("Scheduler initialized with %d nodes", len(nodes))
		}
	}

	// Get initial nodelist date
	if nodelistDate, err := d.storage.GetLatestNodelistDate(ctx); err == nil {
		d.nodelistMu.Lock()
		d.lastNodelistDate = nodelistDate
		d.nodelistMu.Unlock()
		logging.Infof("Current nodelist date: %s", nodelistDate.Format("2006-01-02"))
	}

	// Check if CLI-only mode is enabled
	if d.config.Daemon.CLIOnly {
		logging.Info("CLI-only mode enabled - automatic testing disabled")
		logging.Infof("Use telnet interface on port %d to test nodes", d.config.CLI.Port)

		// Just wait for context cancellation
		<-ctx.Done()
		logging.Info("Daemon stopping due to context cancellation")
		return ctx.Err()
	}

	// Run initial cycle immediately
	if err := d.runTestCycle(ctx); err != nil {
		logging.Errorf("Error in initial test cycle: %v", err)
	}

	// If run-once mode, exit after first cycle
	if d.config.Daemon.RunOnce {
		logging.Info("Run-once mode completed")
		return nil
	}

	// Create ticker for periodic checking of nodes ready for testing
	// Check every minute to see if any nodes are due for testing
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	// Create ticker for checking nodelist updates (every 5 minutes)
	nodelistCheckTicker := time.NewTicker(5 * time.Minute)
	defer nodelistCheckTicker.Stop()

	logging.Info("Daemon started, checking for nodes to test every minute")
	logging.Infof("Test intervals: successful nodes=%v, failed nodes=%v", d.config.Daemon.TestInterval, d.config.Daemon.FailedRetryInterval)
	logging.Info("Will check for nodelist updates every 5 minutes")

	for {
		select {
		case <-ctx.Done():
			logging.Info("Daemon stopping due to context cancellation")
			return ctx.Err()

		case <-ticker.C:
			if err := d.runTestCycle(ctx); err != nil {
				logging.Errorf("Error in test cycle: %v", err)
			}

		case <-nodelistCheckTicker.C:
			// Check if nodelist has been updated
			if d.checkAndRefreshNodelist(ctx) {
				logging.Info("New nodelist detected - nodes refreshed from database")
			}
		}
	}
}

// Close closes the daemon and releases resources
func (d *Daemon) Close() error {
	if d.workerPool != nil {
		d.workerPool.Stop()
	}

	if d.persistentCache != nil {
		if err := d.persistentCache.Close(); err != nil {
			logging.Errorf("Error closing persistent cache: %v", err)
		}
	}

	// Close logging to flush any pending writes
	if err := logging.GetLogger().Close(); err != nil {
		// Can't log this error since we're closing the logger
		log.Printf("Error closing logger: %v\n", err)
	}

	if d.storage != nil {
		return d.storage.Close()
	}

	return nil
}
