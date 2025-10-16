package daemon

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/nodelistdb/internal/testing/logging"
	"github.com/nodelistdb/internal/testing/models"
)

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
	nodes := d.scheduler.GetNodesForTesting(ctx, d.config.Daemon.BatchSize*d.config.Daemon.Workers)
	if len(nodes) == 0 {
		// Log scheduler status for debugging
		schedStatus := d.scheduler.GetScheduleStatus()
		logging.Infof("Scheduler status: total_nodes=%v, ready=%v, failing=%v, pending_first_test=%v",
			schedStatus["total_nodes"], schedStatus["ready_for_test"], schedStatus["failing_nodes"], schedStatus["pending_first_test"])

		// This is normal after a restart if all nodes were tested recently
		logging.Infof("No nodes ready for testing at this time. All nodes are within their test intervals.")
		return nil // Skip this test cycle
	}

	// Apply test limit filter if specified
	if d.config.Daemon.TestLimit != "" {
		nodes = d.nodeFilter.FilterByTestLimit(nodes, d.config.Daemon.TestLimit)
		if len(nodes) == 0 {
			logging.Warnf("No nodes match test limit filter: %s", d.config.Daemon.TestLimit)
			return nil
		}
		logging.Infof("Applied test limit filter '%s', testing %d node(s)", d.config.Daemon.TestLimit, len(nodes))
	} else {
		logging.Infof("Scheduler selected %d nodes for testing", len(nodes))
	}

	// Log breakdown of test reasons
	reasonCounts := make(map[string]int)
	for _, node := range nodes {
		reason := node.TestReason
		if reason == "" {
			reason = "unknown"
		}
		reasonCounts[reason]++
	}

	if len(reasonCounts) > 0 {
		var reasons []string
		for reason, count := range reasonCounts {
			reasons = append(reasons, fmt.Sprintf("%s=%d", reason, count))
		}
		logging.Infof("Test reasons: %s", strings.Join(reasons, ", "))
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

				result := d.testExecutor.TestNode(ctx, nodeToTest)

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
	stats := d.statisticsCollector.CalculateStatistics(allResults)

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

// TestSingleNode tests a single node and returns immediately
// nodeSpec can be in format "zone:net/node" or "host:port" or "host"
func (d *Daemon) TestSingleNode(ctx context.Context, nodeSpec, protocol string) error {
	// Enable debug mode if configured
	if d.config.Logging.Level == "debug" {
		_ = d.SetDebugMode(true)
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

		// Get hostname from node data (with system name fallback)
		hostname = targetNode.GetPrimaryHostname()
		if hostname == "" {
			return fmt.Errorf("node %s has no internet hostname or valid system name", nodeSpec)
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
		Zone:              int(zone),
		Net:               int(net),
		Node:              int(node),
		InternetHostnames: []string{hostname},
		HasInet:           true,
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
