package daemon

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

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
	
	// Protocol testers
	binkpTester  protocols.Tester
	ifcicoTester protocols.Tester
	telnetTester protocols.Tester
	ftpTester    protocols.Tester
	vmodemTester protocols.Tester
	
	// Worker pool
	workerPool *WorkerPool
	
	// Statistics
	stats struct {
		sync.Mutex
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

	// Initialize services
	d.dnsResolver = services.NewDNSResolver(
		cfg.Services.DNS.Workers,
		cfg.Services.DNS.Timeout,
	)
	
	d.geolocator = services.NewGeolocation(
		cfg.Services.Geolocation.Provider,
		cfg.Services.Geolocation.APIKey,
	)

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

	return d, nil
}

// Run starts the daemon
func (d *Daemon) Run(ctx context.Context) error {
	log.Printf("Starting NodeTest Daemon with %d workers", d.config.Daemon.Workers)
	
	// Start CLI server if enabled
	if err := d.StartCLIServer(ctx); err != nil {
		return fmt.Errorf("failed to start CLI server: %w", err)
	}
	
	// Start worker pool
	d.workerPool.Start()
	defer d.workerPool.Stop()
	
	// Check if CLI-only mode is enabled
	if d.config.Daemon.CLIOnly {
		log.Println("CLI-only mode enabled - automatic testing disabled")
		log.Println("Use telnet interface on port", d.config.CLI.Port, "to test nodes")
		
		// Just wait for context cancellation
		<-ctx.Done()
		log.Println("Daemon stopping due to context cancellation")
		return ctx.Err()
	}
	
	// Run initial cycle immediately
	if err := d.runTestCycle(ctx); err != nil {
		log.Printf("Error in initial test cycle: %v", err)
	}
	
	// If run-once mode, exit after first cycle
	if d.config.Daemon.RunOnce {
		log.Println("Run-once mode completed")
		return nil
	}
	
	// Create ticker for periodic testing
	ticker := time.NewTicker(d.config.Daemon.TestInterval)
	defer ticker.Stop()
	
	log.Printf("Daemon started, will run tests every %v", d.config.Daemon.TestInterval)
	
	for {
		select {
		case <-ctx.Done():
			log.Println("Daemon stopping due to context cancellation")
			return ctx.Err()
			
		case <-ticker.C:
			if err := d.runTestCycle(ctx); err != nil {
				log.Printf("Error in test cycle: %v", err)
			}
		}
	}
}

// runTestCycle runs a complete test cycle
func (d *Daemon) runTestCycle(ctx context.Context) error {
	startTime := time.Now()
	
	log.Println("Starting test cycle")
	
	// Get nodes to test
	nodes, err := d.storage.GetNodesWithInternet(ctx, 0) // 0 = no limit
	if err != nil {
		return fmt.Errorf("failed to get nodes: %w", err)
	}
	
	log.Printf("Found %d nodes with internet connectivity", len(nodes))
	
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
		log.Printf("Processing batch %d-%d of %d nodes", i+1, end, len(nodes))
		
		// Create test requests for batch
		var wg sync.WaitGroup
		for _, node := range batch {
			wg.Add(1)
			
			// Submit test job to worker pool
			d.workerPool.Submit(func() {
				defer wg.Done()
				
				result := d.testNode(ctx, node)
				
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
				log.Printf("Failed to store batch results: %v", err)
			}
		}
	}
	
	// Calculate and store statistics
	stats := d.calculateStatistics(allResults)
	
	if !d.config.Daemon.DryRun {
		if err := d.storage.StoreDailyStats(ctx, stats); err != nil {
			log.Printf("Failed to store daily statistics: %v", err)
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
	log.Printf("Test cycle completed in %v: %d nodes tested, %d operational, %d with issues",
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
	// Similar implementation for Telnet
}

// testFTP tests FTP connectivity
func (d *Daemon) testFTP(ctx context.Context, node *models.Node, result *models.TestResult) {
	// Similar implementation for FTP
}

// testVModem tests VModem connectivity
func (d *Daemon) testVModem(ctx context.Context, node *models.Node, result *models.TestResult) {
	// Similar implementation for VModem
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
	
	if d.storage != nil {
		return d.storage.Close()
	}
	
	return nil
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
			log.Printf("Failed to store test result: %v", err)
		}
	}
	
	return result, nil
}