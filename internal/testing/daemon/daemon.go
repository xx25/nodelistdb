package daemon

import (
	"context"
	"fmt"
	"sync"
	"time"

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
		store, err = storage.NewClickHouseStorage(
			cfg.Database.ClickHouse.Host,
			cfg.Database.ClickHouse.Port,
			cfg.Database.ClickHouse.Database,
			cfg.Database.ClickHouse.Username,
			cfg.Database.ClickHouse.Password,
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

	// Initialize persistent cache if configured
	if cfg.Cache.Enabled && cfg.Cache.Path != "" {
		pCache, err := storage.NewCache(cfg.Cache.Path)
		if err != nil {
			logging.Warnf("Failed to initialize persistent cache: %v", err)
			logging.Infof("Continuing with in-memory cache only")
		} else {
			d.persistentCache = pCache
			logging.Infof("Persistent cache initialized at %s", cfg.Cache.Path)
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

	// Initialize protocol testers
	if cfg.Protocols.BinkP.Enabled {
		d.binkpTester = protocols.NewBinkPTester(
			cfg.Protocols.BinkP.Timeout,
			cfg.Protocols.BinkP.OurAddress,
		)
	}
	
	if cfg.Protocols.Ifcico.Enabled {
		d.ifcicoTester = protocols.NewIfcicoTester(
			cfg.Protocols.Ifcico.Timeout,
			cfg.Protocols.Ifcico.OurAddress,
		)
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
	
	logging.Infof("Daemon started, will run tests every %v", d.config.Daemon.TestInterval)
	
	for {
		select {
		case <-ctx.Done():
			logging.Info("Daemon stopping due to context cancellation")
			return ctx.Err()
			
		case <-ticker.C:
			if err := d.runTestCycle(ctx); err != nil {
				logging.Errorf("Error in test cycle: %v", err)
			}
		}
	}
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
		// Fallback to getting all nodes if scheduler has no nodes ready
		logging.Infof("No nodes ready from scheduler, checking all nodes")
		allNodes, err := d.storage.GetNodesWithInternet(ctx, 0) // 0 = no limit
		if err != nil {
			return fmt.Errorf("failed to get nodes: %w", err)
		}
		nodes = allNodes
	}
	
	logging.Infof("Scheduler selected %d nodes for testing", len(nodes))
	
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
				logging.Infof("Failed to store batch results: %v", err)
			}
		}
	}
	
	// Calculate and store statistics
	stats := d.calculateStatistics(allResults)
	
	if !d.config.Daemon.DryRun {
		if err := d.storage.StoreDailyStats(ctx, stats); err != nil {
			logging.Infof("Failed to store daily statistics: %v", err)
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

// testBinkP tests BinkP connectivity
func (d *Daemon) testBinkP(ctx context.Context, node *models.Node, result *models.TestResult) {
	if d.binkpTester == nil {
		return
	}
	
	hostname := node.GetPrimaryHostname()
	if hostname == "" {
		return
	}
	
	// Get IP from DNS result
	var ip string
	if len(result.ResolvedIPv4) > 0 {
		ip = result.ResolvedIPv4[0]
	} else if len(result.ResolvedIPv6) > 0 {
		ip = result.ResolvedIPv6[0]
	} else {
		return
	}
	
	testResult := d.binkpTester.Test(ctx, ip, d.config.Protocols.BinkP.Port, node.Address())
	
	if binkpResult, ok := testResult.(*protocols.BinkPTestResult); ok {
		details := &models.BinkPTestDetails{
			SystemName:   binkpResult.SystemName,
			Sysop:        binkpResult.Sysop,
			Location:     binkpResult.Location,
			Version:      binkpResult.Version,
			Addresses:    binkpResult.Addresses,
			Capabilities: binkpResult.Capabilities,
		}
		result.SetBinkPResult(binkpResult.Success, binkpResult.ResponseMs, details, binkpResult.Error)
		
		// Use the AddressValid flag from BinkP tester
		if binkpResult.AddressValid {
			result.AddressValidated = true
		}
	}
}

// testIfcico tests IFCICO connectivity
func (d *Daemon) testIfcico(ctx context.Context, node *models.Node, result *models.TestResult) {
	if d.ifcicoTester == nil {
		return
	}
	
	hostname := node.GetPrimaryHostname()
	if hostname == "" {
		return
	}
	
	// Get IP from DNS result
	var ip string
	if len(result.ResolvedIPv4) > 0 {
		ip = result.ResolvedIPv4[0]
	} else if len(result.ResolvedIPv6) > 0 {
		ip = result.ResolvedIPv6[0]
	} else {
		return
	}
	
	testResult := d.ifcicoTester.Test(ctx, ip, d.config.Protocols.Ifcico.Port, node.Address())
	
	if ifcicoResult, ok := testResult.(*protocols.IfcicoTestResult); ok {
		details := &models.IfcicoTestDetails{
			MailerInfo:   ifcicoResult.MailerInfo,
			SystemName:   ifcicoResult.SystemName,
			Addresses:    ifcicoResult.Addresses,
			ResponseType: ifcicoResult.ResponseType,
		}
		result.SetIfcicoResult(ifcicoResult.Success, ifcicoResult.ResponseMs, details, ifcicoResult.Error)
	}
}

// testTelnet tests Telnet connectivity
func (d *Daemon) testTelnet(ctx context.Context, node *models.Node, result *models.TestResult) {
	if d.telnetTester == nil {
		return
	}
	
	hostname := node.GetPrimaryHostname()
	if hostname == "" {
		return
	}
	
	// Get IP from DNS result
	var ip string
	if len(result.ResolvedIPv4) > 0 {
		ip = result.ResolvedIPv4[0]
	} else if len(result.ResolvedIPv6) > 0 {
		ip = result.ResolvedIPv6[0]
	} else {
		return
	}
	
	testResult := d.telnetTester.Test(ctx, ip, d.config.Protocols.Telnet.Port, node.Address())
	
	if telnetResult, ok := testResult.(*protocols.TelnetTestResult); ok {
		result.SetTelnetResult(telnetResult.Success, telnetResult.ResponseMs, telnetResult.Banner, telnetResult.Error)
	}
}

// testFTP tests FTP connectivity
func (d *Daemon) testFTP(ctx context.Context, node *models.Node, result *models.TestResult) {
	if d.ftpTester == nil {
		return
	}
	
	hostname := node.GetPrimaryHostname()
	if hostname == "" {
		return
	}
	
	// Get IP from DNS result
	var ip string
	if len(result.ResolvedIPv4) > 0 {
		ip = result.ResolvedIPv4[0]
	} else if len(result.ResolvedIPv6) > 0 {
		ip = result.ResolvedIPv6[0]
	} else {
		return
	}
	
	testResult := d.ftpTester.Test(ctx, ip, d.config.Protocols.FTP.Port, node.Address())
	
	if ftpResult, ok := testResult.(*protocols.FTPTestResult); ok {
		result.SetFTPResult(ftpResult.Success, ftpResult.ResponseMs, ftpResult.Banner, ftpResult.Error)
	}
}

// testVModem tests VModem connectivity
func (d *Daemon) testVModem(ctx context.Context, node *models.Node, result *models.TestResult) {
	if d.vmodemTester == nil {
		return
	}
	
	hostname := node.GetPrimaryHostname()
	if hostname == "" {
		return
	}
	
	// Get IP from DNS result
	var ip string
	if len(result.ResolvedIPv4) > 0 {
		ip = result.ResolvedIPv4[0]
	} else if len(result.ResolvedIPv6) > 0 {
		ip = result.ResolvedIPv6[0]
	} else {
		return
	}
	
	testResult := d.vmodemTester.Test(ctx, ip, d.config.Protocols.VModem.Port, node.Address())
	
	if vmodemResult, ok := testResult.(*protocols.VModemTestResult); ok {
		result.SetVModemResult(vmodemResult.Success, vmodemResult.ResponseMs, vmodemResult.Error)
	}
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
		d.binkpTester = protocols.NewBinkPTester(
			newCfg.Protocols.BinkP.Timeout,
			newCfg.Protocols.BinkP.OurAddress,
		)
	} else {
		d.binkpTester = nil
	}
	
	if newCfg.Protocols.Ifcico.Enabled {
		d.ifcicoTester = protocols.NewIfcicoTester(
			newCfg.Protocols.Ifcico.Timeout,
			newCfg.Protocols.Ifcico.OurAddress,
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

// TestNodeDirect tests a specific node directly (for CLI)
func (d *Daemon) TestNodeDirect(ctx context.Context, zone, net, node uint16, hostname string) (*models.TestResult, error) {
	testNode := &models.Node{
		Zone: int(zone),
		Net:  int(net),
		Node: int(node),
		InternetHostnames: []string{hostname},
		HasInet: true,
		// Enable all protocols for CLI testing
		InternetProtocols: []string{"IBN", "IFC", "ITN", "IFT", "IVM"},
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