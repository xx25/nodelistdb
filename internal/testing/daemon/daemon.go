package daemon

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/nodelistdb/internal/testing/cli"
	"github.com/nodelistdb/internal/testing/logging"
	"github.com/nodelistdb/internal/testing/models"
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
	
	// Persistent cache (optional)
	persistentCache *storage.Cache
	
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
		Console:    true, // Always log to console as well
	}
	if err := logging.Initialize(logConfig); err != nil {
		return nil, fmt.Errorf("failed to initialize logging: %w", err)
	}

	// Initialize storage based on database type
	var store storage.Storage
	var err error
	
	switch cfg.Database.Type {
	case "duckdb":
		store, err = storage.NewDuckDBStorage(
			cfg.Database.DuckDB.NodesPath,
			cfg.Database.DuckDB.ResultsPath,
		)
	case "clickhouse":
		chConfig := &storage.ClickHouseConfig{
			MaxOpenConns:  cfg.Database.ClickHouse.MaxOpenConns,
			MaxIdleConns:  cfg.Database.ClickHouse.MaxIdleConns,
			DialTimeout:   cfg.Database.ClickHouse.DialTimeout,
			ReadTimeout:   cfg.Database.ClickHouse.ReadTimeout,
			WriteTimeout:  cfg.Database.ClickHouse.WriteTimeout,
			Compression:   cfg.Database.ClickHouse.Compression,
			BatchSize:     cfg.Database.ClickHouse.BatchSize,
			FlushInterval: cfg.Database.ClickHouse.FlushInterval,
		}
		store, err = storage.NewClickHouseStorageWithConfig(
			cfg.Database.ClickHouse.Host,
			cfg.Database.ClickHouse.Port,
			cfg.Database.ClickHouse.Database,
			cfg.Database.ClickHouse.Username,
			cfg.Database.ClickHouse.Password,
			chConfig,
		)
	default:
		return nil, fmt.Errorf("unsupported database type: %s", cfg.Database.Type)
	}
	
	if err != nil {
		return nil, fmt.Errorf("failed to initialize storage: %w", err)
	}

	d := &Daemon{
		config:  cfg,
		storage: store,
	}

	// Initialize persistent cache using testdaemon_cache configuration
	if cfg.TestdaemonCache.Enabled && cfg.TestdaemonCache.Path != "" {
		pCache, err := storage.NewCache(cfg.TestdaemonCache.Path)
		if err != nil {
			logging.Warnf("Failed to initialize persistent cache: %v", err)
			logging.Infof("Continuing with in-memory cache only")
		} else {
			d.persistentCache = pCache
			logging.Infof("Persistent cache initialized at %s", cfg.TestdaemonCache.Path)
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
		Strategy:          StrategyAdaptive,
		BaseInterval:      cfg.Daemon.TestInterval,
		FailureMultiplier: 2.0,
		MaxInterval:       24 * time.Hour,
		MaxBackoffLevel:   5,
		JitterPercent:     0.1,
		StaleTestThreshold: cfg.Daemon.StaleTestThreshold,
		FailedRetryInterval: cfg.Daemon.FailedRetryInterval,
	}, store)

	return d, nil
}

// Run starts the daemon
func (d *Daemon) Run(ctx context.Context) error {
	logging.Infof("Starting NodeTest Daemon with %d workers", d.config.Daemon.Workers)
	
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
	
	// Create ticker for periodic testing
	ticker := time.NewTicker(d.config.Daemon.TestInterval)
	defer ticker.Stop()
	
	// Create ticker for checking nodelist updates (every 5 minutes)
	nodelistCheckTicker := time.NewTicker(5 * time.Minute)
	defer nodelistCheckTicker.Stop()
	
	logging.Infof("Daemon started, will run tests every %v", d.config.Daemon.TestInterval)
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

// filterNodesByTestLimit filters nodes based on the test limit string
func (d *Daemon) filterNodesByTestLimit(nodes []*models.Node, testLimit string) []*models.Node {
	// Parse the test limit address (e.g., "2:5001/100")
	var zone, net, node int
	if _, err := fmt.Sscanf(testLimit, "%d:%d/%d", &zone, &net, &node); err != nil {
		logging.Errorf("Invalid test limit format '%s': %v", testLimit, err)
		return nodes
	}
	
	// Filter nodes to only include the specified one
	var filtered []*models.Node
	for _, n := range nodes {
		if n.Zone == zone && n.Net == net && n.Node == node {
			filtered = append(filtered, n)
			break // Only need one match
		}
	}
	
	return filtered
}

// checkAndRefreshNodelist checks if the nodelist has been updated and refreshes if needed
func (d *Daemon) checkAndRefreshNodelist(ctx context.Context) bool {
	// Get current nodelist date from database
	currentDate, err := d.storage.GetLatestNodelistDate(ctx)
	if err != nil {
		logging.Errorf("Failed to check nodelist date: %v", err)
		return false
	}
	
	// Check if it's different from our last known date
	d.nodelistMu.RLock()
	lastDate := d.lastNodelistDate
	d.nodelistMu.RUnlock()
	
	if currentDate.After(lastDate) {
		// New nodelist detected!
		logging.Infof("New nodelist detected: %s (was %s)", 
			currentDate.Format("2006-01-02"),
			lastDate.Format("2006-01-02"))
		
		// Refresh the scheduler with new nodes
		// This will automatically schedule immediate retests for nodes with changed internet config
		if err := d.scheduler.RefreshNodes(ctx); err != nil {
			logging.Errorf("Failed to refresh nodes: %v", err)
			return false
		}
		
		// Update our last known date
		d.nodelistMu.Lock()
		d.lastNodelistDate = currentDate
		d.nodelistMu.Unlock()
		
		logging.Infof("Nodelist refresh complete. Nodes with changed internet config will be retested immediately.")
		
		return true
	}
	
	return false
}

// runTestCycle runs a complete test cycle
func (d *Daemon) runTestCycle(ctx context.Context) error {
	// Check if paused
	d.pauseMu.RLock()
	if d.paused {
		d.pauseMu.RUnlock()
		logging.Info("Test cycle skipped - daemon is paused")
		return nil
	}
	d.pauseMu.RUnlock()
	
	startTime := time.Now()
	
	logging.Info("Starting test cycle")
	
	// Use scheduler to get nodes that need testing
	nodes := d.scheduler.GetNodesForTesting(ctx, d.config.Daemon.BatchSize * d.config.Daemon.Workers)
	if len(nodes) == 0 {
		// Log scheduler status for debugging
		schedStatus := d.scheduler.GetScheduleStatus()
		logging.Infof("Scheduler status: total_nodes=%v, ready=%v, failing=%v", 
			schedStatus["total_nodes"], schedStatus["ready_for_test"], schedStatus["failing_nodes"])
		
		// Fallback to getting all nodes if scheduler has no nodes ready
		logging.Warnf("No nodes ready from scheduler, falling back to testing ALL nodes (this should not happen if tests were done recently)")
		allNodes, err := d.storage.GetNodesWithInternet(ctx, 0) // 0 = no limit
		if err != nil {
			return fmt.Errorf("failed to get nodes: %w", err)
		}
		nodes = allNodes
	}
	
	// Apply test limit filter if specified
	if d.config.Daemon.TestLimit != "" {
		nodes = d.filterNodesByTestLimit(nodes, d.config.Daemon.TestLimit)
		if len(nodes) == 0 {
			logging.Warnf("No nodes match test limit filter: %s", d.config.Daemon.TestLimit)
			return nil
		}
		logging.Infof("Applied test limit filter '%s', testing %d node(s)", d.config.Daemon.TestLimit, len(nodes))
	} else {
		logging.Infof("Scheduler selected %d nodes for testing", len(nodes))
	}
	
	// Process nodes in batches
	batchSize := d.config.Daemon.BatchSize
	var allResults []*models.TestResult
	var mu sync.Mutex
	
	for i := 0; i < len(nodes); i += batchSize {
		end := i + batchSize
		if end > len(nodes) {
			end = len(nodes)
		}
		
		batch := nodes[i:end]
		logging.Infof("Processing batch %d-%d of %d nodes", i+1, end, len(nodes))
		
		// Create test requests for batch
		var wg sync.WaitGroup
		for _, node := range batch {
			wg.Add(1)
			
			// Capture node in closure to avoid race condition
			nodeToTest := node
			
			// Submit test job to worker pool
			d.workerPool.Submit(func() {
				defer wg.Done()
				
				result := d.testNode(ctx, nodeToTest)
				
				// Update scheduler immediately with correct pairing
				if d.scheduler != nil {
					d.scheduler.UpdateTestResult(ctx, nodeToTest, result)
				}
				
				mu.Lock()
				allResults = append(allResults, result)
				mu.Unlock()
			})
		}
		
		// Wait for batch to complete
		wg.Wait()
		
		// Store batch results if not in dry-run mode
		if !d.config.Daemon.DryRun && len(allResults) > 0 {
			if err := d.storage.StoreTestResults(ctx, allResults[len(allResults)-len(batch):]); err != nil {
				logging.Errorf("Failed to store batch results: %v", err)
			}
		}
	}
	
	// Calculate and store statistics
	stats := d.calculateStatistics(allResults)
	
	if !d.config.Daemon.DryRun {
		if err := d.storage.StoreDailyStats(ctx, stats); err != nil {
			logging.Errorf("Failed to store daily statistics: %v", err)
		}
	}
	
	// Update daemon statistics
	d.stats.Lock()
	d.stats.cyclesRun++
	d.stats.totalTested += len(allResults)
	d.stats.totalSuccesses += int(stats.NodesOperational)
	d.stats.totalFailures += int(stats.NodesWithIssues)
	d.stats.lastCycleTime = startTime
	d.stats.Unlock()
	
	duration := time.Since(startTime)
	logging.Infof("Test cycle completed in %v: %d nodes tested, %d operational, %d with issues",
		duration, len(allResults), stats.NodesOperational, stats.NodesWithIssues)
	
	return nil
}

// testNode tests a single node
func (d *Daemon) testNode(ctx context.Context, node *models.Node) *models.TestResult {
	result := models.NewTestResult(node)
	
	// DNS resolution
	if hostname := node.GetPrimaryHostname(); hostname != "" {
		dnsResult := d.dnsResolver.Resolve(ctx, hostname)
		result.SetDNSResult(dnsResult)
		
		// Get geolocation for first resolved IP
		if len(dnsResult.IPv4Addresses) > 0 {
			if geo := d.geolocator.GetLocation(ctx, dnsResult.IPv4Addresses[0]); geo != nil {
				result.SetGeolocation(geo)
			}
		}
	}
	
	// Test protocols
	if d.config.Protocols.BinkP.Enabled && node.HasProtocol("IBN") {
		d.testBinkP(ctx, node, result)
	}
	
	if d.config.Protocols.Ifcico.Enabled && node.HasProtocol("IFC") {
		d.testIfcico(ctx, node, result)
	}
	
	if d.config.Protocols.Telnet.Enabled && node.HasProtocol("ITN") {
		d.testTelnet(ctx, node, result)
	}
	
	if d.config.Protocols.FTP.Enabled && node.HasProtocol("IFT") {
		d.testFTP(ctx, node, result)
	}
	
	if d.config.Protocols.VModem.Enabled && node.HasProtocol("IVM") {
		d.testVModem(ctx, node, result)
	}
	
	return result
}


// calculateStatistics calculates statistics from test results
func (d *Daemon) calculateStatistics(results []*models.TestResult) *models.TestStatistics {
	stats := &models.TestStatistics{
		Date:             time.Now().Truncate(24 * time.Hour),
		TotalNodesTested: uint32(len(results)),
		Countries:        make(map[string]uint32),
		ISPs:             make(map[string]uint32),
		ProtocolStats:    make(map[string]uint32),
		ErrorTypes:       make(map[string]uint32),
	}
	
	var binkpResponseTimes []float32
	var ifcicoResponseTimes []float32
	
	for _, r := range results {
		// Count operational nodes
		if r.IsOperational {
			stats.NodesOperational++
		}
		if r.HasConnectivityIssues {
			stats.NodesWithIssues++
		}
		if r.DNSError != "" {
			stats.NodesDNSFailed++
		}
		
		// Count by country
		if r.Country != "" {
			stats.Countries[r.Country]++
		}
		
		// Count by ISP
		if r.ISP != "" {
			stats.ISPs[r.ISP]++
		}
		
		// Protocol statistics
		if r.BinkPResult != nil && r.BinkPResult.Tested {
			stats.NodesWithBinkP++
			if r.BinkPResult.Success {
				stats.ProtocolStats["binkp_success"]++
				binkpResponseTimes = append(binkpResponseTimes, float32(r.BinkPResult.ResponseMs))
			} else {
				stats.ProtocolStats["binkp_failed"]++
				if r.BinkPResult.Error != "" {
					stats.ErrorTypes[r.BinkPResult.Error]++
				}
			}
		}
		
		if r.IfcicoResult != nil && r.IfcicoResult.Tested {
			stats.NodesWithIfcico++
			if r.IfcicoResult.Success {
				stats.ProtocolStats["ifcico_success"]++
				ifcicoResponseTimes = append(ifcicoResponseTimes, float32(r.IfcicoResult.ResponseMs))
			} else {
				stats.ProtocolStats["ifcico_failed"]++
			}
		}
	}
	
	// Calculate averages
	if len(binkpResponseTimes) > 0 {
		var sum float32
		for _, t := range binkpResponseTimes {
			sum += t
		}
		stats.AvgBinkPResponseMs = sum / float32(len(binkpResponseTimes))
	}
	
	if len(ifcicoResponseTimes) > 0 {
		var sum float32
		for _, t := range ifcicoResponseTimes {
			sum += t
		}
		stats.AvgIfcicoResponseMs = sum / float32(len(ifcicoResponseTimes))
	}
	
	return stats
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
		fmt.Printf("Error closing logger: %v\n", err)
	}
	
	if d.storage != nil {
		return d.storage.Close()
	}
	
	return nil
}

// ReloadConfig reloads the configuration from file
func (d *Daemon) ReloadConfig(configPath string) error {
	// Load new configuration
	newCfg, err := LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	
	// Validate new configuration
	if err := newCfg.Validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}
	
	// Update configuration (only safe fields)
	// Note: We don't change database connections or worker pool size
	d.config.Daemon.TestInterval = newCfg.Daemon.TestInterval
	d.config.Daemon.BatchSize = newCfg.Daemon.BatchSize
	d.config.Daemon.DryRun = newCfg.Daemon.DryRun
	
	// Update protocol settings
	d.config.Protocols = newCfg.Protocols
	
	// Update service settings
	d.config.Services = newCfg.Services
	
	// Reinitialize services with new configurations
	d.dnsResolver = services.NewDNSResolverWithTTL(
		newCfg.Services.DNS.Workers,
		newCfg.Services.DNS.Timeout,
		newCfg.Services.DNS.CacheTTL,
	)
	
	d.geolocator = services.NewGeolocationWithConfig(
		newCfg.Services.Geolocation.Provider,
		newCfg.Services.Geolocation.APIKey,
		newCfg.Services.Geolocation.CacheTTL,
		newCfg.Services.Geolocation.RateLimit,
	)
	
	// Re-wire persistent cache to new service instances if available
	if d.persistentCache != nil {
		dnsCache := storage.NewDNSCache(d.persistentCache)
		d.dnsResolver.SetPersistentCache(dnsCache)
		
		geoCache := storage.NewGeolocationCache(d.persistentCache)
		d.geolocator.SetPersistentCache(geoCache)
	}
	
	// Reinitialize protocol testers with new timeouts and settings
	if newCfg.Protocols.BinkP.Enabled {
		d.binkpTester = protocols.NewBinkPTesterWithInfo(
			newCfg.Protocols.BinkP.Timeout,
			newCfg.Protocols.BinkP.OurAddress,
			newCfg.Protocols.BinkP.SystemName,
			newCfg.Protocols.BinkP.Sysop,
			newCfg.Protocols.BinkP.Location,
		)
	} else {
		d.binkpTester = nil
	}
	
	if newCfg.Protocols.Ifcico.Enabled {
		d.ifcicoTester = protocols.NewIfcicoTesterWithInfo(
			newCfg.Protocols.Ifcico.Timeout,
			newCfg.Protocols.Ifcico.OurAddress,
			newCfg.Protocols.Ifcico.SystemName,
			newCfg.Protocols.Ifcico.Sysop,
			newCfg.Protocols.Ifcico.Location,
		)
	} else {
		d.ifcicoTester = nil
	}
	
	if newCfg.Protocols.Telnet.Enabled {
		d.telnetTester = protocols.NewTelnetTester(
			newCfg.Protocols.Telnet.Timeout,
		)
	} else {
		d.telnetTester = nil
	}
	
	if newCfg.Protocols.FTP.Enabled {
		d.ftpTester = protocols.NewFTPTester(
			newCfg.Protocols.FTP.Timeout,
		)
	} else {
		d.ftpTester = nil
	}
	
	if newCfg.Protocols.VModem.Enabled {
		d.vmodemTester = protocols.NewVModemTester(
			newCfg.Protocols.VModem.Timeout,
		)
	} else {
		d.vmodemTester = nil
	}
	
	// Update scheduler with new interval
	if d.scheduler != nil {
		d.scheduler.mu.Lock()
		d.scheduler.baseInterval = newCfg.Daemon.TestInterval
		d.scheduler.mu.Unlock()
	}
	
	// Reload logging configuration
	logConfig := &logging.Config{
		Level:      newCfg.Logging.Level,
		File:       newCfg.Logging.File,
		MaxSize:    newCfg.Logging.MaxSize,
		MaxBackups: newCfg.Logging.MaxBackups,
		MaxAge:     newCfg.Logging.MaxAge,
		Console:    true,
	}
	if err := logging.GetLogger().Reload(logConfig); err != nil {
		logging.Errorf("Failed to reload logging configuration: %v", err)
	}
	
	logging.Info("Configuration reloaded successfully")
	return nil
}

// Pause pauses the daemon's automatic testing
func (d *Daemon) Pause() error {
	d.pauseMu.Lock()
	defer d.pauseMu.Unlock()
	
	if d.paused {
		return fmt.Errorf("daemon is already paused")
	}
	
	d.paused = true
	logging.Info("Daemon paused")
	return nil
}

// Resume resumes the daemon's automatic testing
func (d *Daemon) Resume() error {
	d.pauseMu.Lock()
	defer d.pauseMu.Unlock()
	
	if !d.paused {
		return fmt.Errorf("daemon is not paused")
	}
	
	d.paused = false
	logging.Info("Daemon resumed")
	return nil
}

// IsPaused returns whether the daemon is paused
func (d *Daemon) IsPaused() bool {
	d.pauseMu.RLock()
	defer d.pauseMu.RUnlock()
	return d.paused
}

// GetNodeInfo retrieves detailed information about a node from the database
func (d *Daemon) GetNodeInfo(ctx context.Context, zone, net, node uint16) (*cli.NodeInfo, error) {
	// Try to find the node in our storage
	nodes, err := d.storage.GetNodesByZone(ctx, int(zone))
	if err != nil {
		return &cli.NodeInfo{
			Address:      fmt.Sprintf("%d:%d/%d", zone, net, node),
			Found:        false,
			ErrorMessage: fmt.Sprintf("Failed to query database: %v", err),
		}, nil
	}
	
	// Find the specific node
	for _, n := range nodes {
		if n.Zone == int(zone) && n.Net == int(net) && n.Node == int(node) {
			return &cli.NodeInfo{
				Address:           fmt.Sprintf("%d:%d/%d", zone, net, node),
				SystemName:        n.SystemName,
				SysopName:         n.SysopName,
				Location:          n.Location,
				HasInternet:       n.HasInet,
				InternetHostnames: n.InternetHostnames,
				InternetProtocols: n.InternetProtocols,
				Found:             true,
			}, nil
		}
	}
	
	return &cli.NodeInfo{
		Address:      fmt.Sprintf("%d:%d/%d", zone, net, node),
		Found:        false,
		ErrorMessage: "Node not found in database",
	}, nil
}

// TestNodeDirect tests a specific node directly (for CLI)
func (d *Daemon) TestNodeDirect(ctx context.Context, zone, net, node uint16, hostname string) (*models.TestResult, error) {
	var testNode *models.Node
	
	// If no hostname provided, try to look up the node from database
	if hostname == "" {
		// Try to find the node in our storage
		nodes, err := d.storage.GetNodesByZone(ctx, int(zone))
		if err == nil {
			// Find the specific node
			for _, n := range nodes {
				if n.Zone == int(zone) && n.Net == int(net) && n.Node == int(node) {
					testNode = n
					break
				}
			}
		}
		
		if testNode == nil {
			// Node not found in database, create a minimal node
			return nil, fmt.Errorf("node %d:%d/%d not found in database and no hostname provided", zone, net, node)
		}
	} else {
		// Hostname was provided, create node with that hostname
		testNode = &models.Node{
			Zone: int(zone),
			Net:  int(net),
			Node: int(node),
			InternetHostnames: []string{hostname},
			HasInet: true,
			// Enable all protocols for CLI testing when hostname is manually provided
			InternetProtocols: []string{"IBN", "IFC", "ITN", "IFT", "IVM"},
		}
	}
	
	result := d.testNode(ctx, testNode)
	
	// Store result if not in dry-run mode
	if !d.config.Daemon.DryRun {
		if err := d.storage.StoreTestResult(ctx, result); err != nil {
			logging.Infof("Failed to store test result: %v", err)
		}
	}
	
	return result, nil
}

// SetDebugMode enables or disables debug mode for protocol testers
func (d *Daemon) SetDebugMode(enabled bool) error {
	d.debugMu.Lock()
	defer d.debugMu.Unlock()
	
	d.debug = enabled
	
	// Update all protocol testers if they support debug mode
	if d.binkpTester != nil {
		if setter, ok := d.binkpTester.(protocols.DebugSetter); ok {
			setter.SetDebug(enabled)
		}
	}
	if d.ifcicoTester != nil {
		if setter, ok := d.ifcicoTester.(protocols.DebugSetter); ok {
			setter.SetDebug(enabled)
		}
	}
	if d.telnetTester != nil {
		if setter, ok := d.telnetTester.(protocols.DebugSetter); ok {
			setter.SetDebug(enabled)
		}
	}
	if d.ftpTester != nil {
		if setter, ok := d.ftpTester.(protocols.DebugSetter); ok {
			setter.SetDebug(enabled)
		}
	}
	if d.vmodemTester != nil {
		if setter, ok := d.vmodemTester.(protocols.DebugSetter); ok {
			setter.SetDebug(enabled)
		}
	}
	
	if enabled {
		logging.Info("Debug mode enabled for protocol testers")
	} else {
		logging.Info("Debug mode disabled for protocol testers")
	}
	
	return nil
}

// GetDebugMode returns the current debug mode status
func (d *Daemon) GetDebugMode() bool {
	d.debugMu.RLock()
	defer d.debugMu.RUnlock()
	return d.debug
}

// TestSingleNode tests a single node and returns immediately
// nodeSpec can be in format "zone:net/node" or "host:port" or "host"
func (d *Daemon) TestSingleNode(ctx context.Context, nodeSpec, protocol string) error {
	// Enable debug mode if configured
	if d.config.Logging.Level == "debug" {
		d.SetDebugMode(true)
	}
	
	// Parse the node specification
	var zone, net, node uint16
	var hostname string
	var port int
	
	// Try to parse as FTN address first (e.g., "2:5053/56")
	if _, err := fmt.Sscanf(nodeSpec, "%d:%d/%d", &zone, &net, &node); err == nil {
		// It's an FTN address - look up node in database
		nodes, err := d.storage.GetNodesByZone(ctx, int(zone))
		if err != nil {
			return fmt.Errorf("failed to query nodes: %w", err)
		}
		
		// Find the specific node
		var targetNode *models.Node
		for _, n := range nodes {
			if n.Zone == int(zone) && n.Net == int(net) && n.Node == int(node) {
				targetNode = n
				break
			}
		}
		
		if targetNode == nil {
			return fmt.Errorf("node %s not found in database", nodeSpec)
		}
		
		// Get hostname from node data
		if len(targetNode.InternetHostnames) > 0 {
			hostname = targetNode.InternetHostnames[0]
		} else {
			return fmt.Errorf("node %s has no internet hostname", nodeSpec)
		}
	} else {
		// Try to parse as host:port or just host
		var parsedHost string
		if n, err := fmt.Sscanf(nodeSpec, "%[^:]:%d", &parsedHost, &port); n == 2 && err == nil {
			// Successfully parsed host:port
			hostname = parsedHost
		} else {
			// Just a hostname (no port specified)
			hostname = nodeSpec
			port = 0
		}
		// We'll need to create a synthetic node for testing
		zone, net, node = 2, 5001, 5001 // Default testing address
	}
	
	// Determine port based on protocol if not specified
	if port == 0 {
		switch protocol {
		case "binkp":
			port = 24554
		case "ifcico":
			port = 60179
		case "telnet":
			port = 23
		case "ftp":
			port = 21
		default:
			return fmt.Errorf("unsupported protocol: %s", protocol)
		}
	}
	
	// Build hostname with port only if hostname doesn't already include port
	if port != 0 && hostname != "" && !containsPort(hostname) {
		hostname = fmt.Sprintf("%s:%d", hostname, port)
	}
	
	logging.Infof("Testing node %s via %s protocol at %s", nodeSpec, protocol, hostname)
	
	// Create test node
	testNode := &models.Node{
		Zone: int(zone),
		Net:  int(net),
		Node: int(node),
		InternetHostnames: []string{hostname},
		HasInet: true,
		InternetProtocols: []string{},
	}
	
	// Set protocol flag based on requested protocol
	switch protocol {
	case "binkp":
		testNode.InternetProtocols = []string{"IBN"}
	case "ifcico":
		testNode.InternetProtocols = []string{"IFC"}
	case "telnet":
		testNode.InternetProtocols = []string{"ITN"}
	case "ftp":
		testNode.InternetProtocols = []string{"IFT"}
	}
	
	// Run the test - but we need to handle the case where hostname is already an IP
	// The testNode function expects to do DNS resolution, but we may have an IP directly
	result := models.NewTestResult(testNode)
	
	// Check if hostname is already an IP address
	isIP := false
	if parts := strings.Split(hostname, ":"); len(parts) > 0 {
		// Remove port if present
		hostOnly := parts[0]
		// Simple check for IP address (contains dots and all parts are numeric)
		if strings.Count(hostOnly, ".") == 3 {
			ipParts := strings.Split(hostOnly, ".")
			isIP = true
			for _, part := range ipParts {
				if part == "" {
					isIP = false
					break
				}
				for _, ch := range part {
					if ch < '0' || ch > '9' {
						isIP = false
						break
					}
				}
			}
		}
	}
	
	// If it's an IP, skip DNS resolution and set it directly
	if isIP {
		parts := strings.Split(hostname, ":")
		ip := parts[0]
		result.ResolvedIPv4 = []string{ip}
		result.Hostname = hostname
		
		// Get geolocation for the IP
		if geo := d.geolocator.GetLocation(ctx, ip); geo != nil {
			result.SetGeolocation(geo)
		}
	} else {
		// Do DNS resolution
		if dnsResult := d.dnsResolver.Resolve(ctx, hostname); dnsResult != nil {
			result.SetDNSResult(dnsResult)
			
			// Get geolocation for first resolved IP
			if len(dnsResult.IPv4Addresses) > 0 {
				if geo := d.geolocator.GetLocation(ctx, dnsResult.IPv4Addresses[0]); geo != nil {
					result.SetGeolocation(geo)
				}
			}
		}
	}
	
	// Test the requested protocol
	switch protocol {
	case "binkp":
		if d.config.Protocols.BinkP.Enabled && d.binkpTester != nil {
			d.testBinkP(ctx, testNode, result)
		}
	case "ifcico":
		if d.config.Protocols.Ifcico.Enabled && d.ifcicoTester != nil {
			d.testIfcico(ctx, testNode, result)
		}
	case "telnet":
		if d.config.Protocols.Telnet.Enabled && d.telnetTester != nil {
			d.testTelnet(ctx, testNode, result)
		}
	case "ftp":
		if d.config.Protocols.FTP.Enabled && d.ftpTester != nil {
			d.testFTP(ctx, testNode, result)
		}
	}
	
	// Display results
	logging.Infof("Test completed for %s", nodeSpec)
	logging.Infof("  Success: %v", result.IsOperational)
	
	if protocol == "binkp" && result.BinkPResult != nil {
		if result.BinkPResult.Success {
			logging.Infof("  BinkP: Connected successfully")
			if result.BinkPResult.Details != nil {
				if systemName, ok := result.BinkPResult.Details["SystemName"].(string); ok && systemName != "" {
					logging.Infof("    System: %s", systemName)
				}
				if sysop, ok := result.BinkPResult.Details["Sysop"].(string); ok && sysop != "" {
					logging.Infof("    Sysop: %s", sysop)
				}
				if addresses, ok := result.BinkPResult.Details["Addresses"].([]string); ok && len(addresses) > 0 {
					logging.Infof("    Addresses: %v", addresses)
				}
			}
		} else {
			logging.Infof("  BinkP: Failed - %s", result.BinkPResult.Error)
		}
	}
	
	if protocol == "ifcico" && result.IfcicoResult != nil {
		if result.IfcicoResult.Success {
			logging.Infof("  IFCICO: Connected successfully")
			if result.IfcicoResult.Details != nil {
				if systemName, ok := result.IfcicoResult.Details["SystemName"].(string); ok && systemName != "" {
					logging.Infof("    System: %s", systemName)
				}
				if mailerInfo, ok := result.IfcicoResult.Details["MailerInfo"].(string); ok && mailerInfo != "" {
					logging.Infof("    Mailer: %s", mailerInfo)
				}
				if addresses, ok := result.IfcicoResult.Details["Addresses"].([]string); ok && len(addresses) > 0 {
					logging.Infof("    Addresses: %v", addresses)
				}
			}
		} else {
			logging.Infof("  IFCICO: Failed - %s", result.IfcicoResult.Error)
		}
	}
	
	// Store result if not in dry-run mode
	if !d.config.Daemon.DryRun {
		if err := d.storage.StoreTestResult(ctx, result); err != nil {
			logging.Warnf("Failed to store test result: %v", err)
		}
	}
	
	return nil
}

// containsPort checks if a hostname string already contains a port
func containsPort(hostname string) bool {
	// Check if the hostname contains a colon followed by digits (port number)
	// But be careful with IPv6 addresses which also contain colons
	if strings.Contains(hostname, "[") && strings.Contains(hostname, "]") {
		// IPv6 address format like [::1]:8080
		lastColon := strings.LastIndex(hostname, ":")
		lastBracket := strings.LastIndex(hostname, "]")
		return lastColon > lastBracket
	}
	// For regular hostnames/IPv4, check if there's a colon followed by port
	parts := strings.Split(hostname, ":")
	return len(parts) == 2 && parts[1] != ""
}