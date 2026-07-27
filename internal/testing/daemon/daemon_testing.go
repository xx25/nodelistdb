package daemon

import (
	"context"
	"fmt"
	stdnet "net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nodelistdb/internal/testing/logging"
	"github.com/nodelistdb/internal/testing/models"
)

// protocolFlags maps a -test-proto value to the nodelist flag announcing it —
// the same pairing test_executor.go gates the scheduled cycle on, and the same
// one storage.protocolAnnouncementFlags gates the reports on.
var protocolFlags = map[string]string{
	"binkp":  "IBN",
	"ifcico": "IFC",
	"telnet": "ITN",
	"ftp":    "IFT",
	"vmodem": "IVM",
}

// protocolDefaultPorts is the fallback for a hand-run test: an ad-hoc host given
// without a port, or a node announcing the protocol with no port of its own.
// It is a fallback and not the first choice — the announced port wins, because
// probing 3141 on a node whose nodelist entry says IBN:24554 measures nothing
// about that node.
var protocolDefaultPorts = map[string]int{
	"binkp":  24554,
	"ifcico": 60179,
	"telnet": 23,
	"ftp":    21,
	"vmodem": 3141,
}

// Results for an ad-hoc `host:port` test belong to no node, so they are filed
// under zone 0. Zone 0 is not a valid FTN zone, so the address cannot collide
// with a real one, and node 0 keeps it out of every report that filters `node
// != 0` even before the nodelist gate rejects it.
const (
	adHocZone = 0
	adHocNet  = 0
	adHocNode = 0
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

	// Use scheduler to get nodes that need testing.
	//
	// A test limit names the nodes to test, so it selects from everything the
	// scheduler knows rather than from whatever happened to be due. Filtering
	// the due set instead made the flag mean "those of these nodes the regular
	// schedule had already picked" — so a sweep of a small population like the
	// IVM nodes usually matched nothing, and even naming one address did
	// nothing whenever that node was inside its test interval.
	var nodes []*models.Node
	forced := d.config.Daemon.TestLimit != ""
	if forced {
		nodes = d.scheduler.GetAllScheduledNodes()
	} else {
		nodes = d.scheduler.GetNodesForTesting(ctx, d.config.Daemon.BatchSize*d.config.Daemon.Workers)
	}
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
	if forced {
		nodes = d.nodeFilter.FilterByTestLimit(nodes, d.config.Daemon.TestLimit)
		if len(nodes) == 0 {
			logging.Warnf("No nodes match test limit filter: %s", d.config.Daemon.TestLimit)
			return nil
		}
		logging.Infof("Applied test limit filter '%s', testing %d node(s) regardless of when each was last tested", d.config.Daemon.TestLimit, len(nodes))
	} else {
		logging.Infof("Scheduler selected %d nodes for testing", len(nodes))
	}

	// Collapse cross-network AKA equivalence groups to one representative per
	// cycle: the deferred entries get covered by derived results instead of
	// direct tests
	nodes = d.collapseAKAGroups(nodes)

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

	// Each candidate identity gets at most one derived result per cycle
	cycle := newCycleCoverage()

	for i := 0; i < len(nodes); i += batchSize {
		end := i + batchSize
		if end > len(nodes) {
			end = len(nodes)
		}

		batch := nodes[i:end]
		logging.Infof("Processing batch %d-%d of %d nodes", i+1, end, len(nodes))

		// Track results for this batch separately to avoid slicing issues
		// when nil results are skipped (allResults can have fewer entries than batch).
		var batchResults []*models.TestResult
		var batchMu sync.Mutex

		// Create test requests for batch
		var wg sync.WaitGroup
		for _, node := range batch {
			wg.Add(1)

			// Capture node in closure to avoid race condition
			nodeToTest := node

			// Submit test job to worker pool
			d.workerPool.Submit(func() {
				defer wg.Done()

				result, partials := d.testExecutor.TestNodeWithPartials(ctx, nodeToTest)
				if result == nil {
					return
				}

				// Update scheduler immediately with correct pairing
				if d.scheduler != nil {
					d.scheduler.UpdateTestResult(ctx, nodeToTest, result)
				}

				// Cover the same host's entries in other networks with
				// derived results (stored via the same batch path)
				derived := d.deriveAKAResults(nodeToTest, result, partials, cycle)

				batchMu.Lock()
				batchResults = append(batchResults, result)
				batchResults = append(batchResults, derived...)
				batchMu.Unlock()
			})
		}

		// Wait for batch to complete
		wg.Wait()

		// Store batch results if not in dry-run mode
		if !d.config.Daemon.DryRun && len(batchResults) > 0 {
			if err := d.storage.StoreTestResults(ctx, batchResults); err != nil {
				logging.Errorf("Failed to store batch results: %v", err)
			}
		}

		mu.Lock()
		allResults = append(allResults, batchResults...)
		mu.Unlock()
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
	var nodeDomain string

	// Try to parse as FTN address first (e.g., "2:5053/56" or "21:1/100@fsxnet")
	if aka, ok := parseAKA(nodeSpec); ok {
		zone, net, node = uint16(aka.Zone), uint16(aka.Net), uint16(aka.Node)
		// It's an FTN address - look up node in database
		nodes, err := d.storage.GetNodesByZone(ctx, int(zone))
		if err != nil {
			return fmt.Errorf("failed to query nodes: %w", err)
		}

		// Find the specific node; without an @domain suffix the default
		// network is preferred when the address exists in several
		var targetNode *models.Node
		for _, n := range nodes {
			if n.Zone != int(zone) || n.Net != int(net) || n.Node != int(node) {
				continue
			}
			if aka.Domain != "" && aka.Domain != n.EffectiveDomain() {
				continue
			}
			if targetNode == nil || n.EffectiveDomain() == models.DefaultDomain {
				targetNode = n
			}
		}

		if targetNode == nil {
			return fmt.Errorf("node %s not found in database", nodeSpec)
		}
		nodeDomain = targetNode.EffectiveDomain()

		// Get hostname from node data (with system name fallback)
		hostname = targetNode.GetPrimaryHostname()
		if hostname == "" {
			return fmt.Errorf("node %s has no internet hostname or valid system name", nodeSpec)
		}

		// Honour what the node actually announces. Falling straight through to
		// the protocol's default port probed 2:221/1 on 3141 when its nodelist
		// entry offers only IBN:24554, and filed the refusal as this node's
		// VModem result. The scheduled cycle never does this: it reads the
		// announced port (daemon_protocols.go) and only tests a protocol the
		// node claims (test_executor.go).
		if flag, known := protocolFlags[protocol]; known {
			if announced := targetNode.GetProtocolPort(flag); announced != 0 {
				port = announced
			}
			if !targetNode.HasProtocol(flag) {
				logging.Warnf("%s does not announce %s; probing the default port anyway. "+
					"The result is stored, but /analytics/%s lists only nodes announcing %s, so it will not appear there.",
					nodeSpec, flag, protocol, flag)
			}
		}
	} else {
		// Try to parse as host:port or just host (bracket-aware for IPv6)
		if h, p, err := stdnet.SplitHostPort(nodeSpec); err == nil {
			hostname = h
			if pv, err := strconv.Atoi(p); err == nil {
				port = pv
			}
		} else {
			// Just a hostname or bare IPv6 literal (no port specified)
			hostname = strings.Trim(nodeSpec, "[]")
			port = 0
		}
		// An ad-hoc host is nobody's node, so it is filed under zone 0 — not a
		// valid FTN zone, so it can never collide with a real address.
		//
		// This used to be 2:5001/5001, which is a live node: 6479 nodelist rows,
		// present in today's issue, tested by the daemon every cycle. Every
		// `-test-node somehost:24554` run wrote a stranger's handshake into that
		// node's history, where nothing downstream could tell it apart from a
		// real result — the row is well-formed and the address is real.
		zone, net, node = adHocZone, adHocNet, adHocNode
		nodeDomain = models.DefaultDomain
		logging.Infof("%s is not an FTN address; filing results under the ad-hoc address %d:%d/%d, "+
			"which is outside the nodelist and excluded from the analytics reports",
			nodeSpec, adHocZone, adHocNet, adHocNode)
	}

	// Determine port based on protocol if not specified
	if port == 0 {
		def, ok := protocolDefaultPorts[protocol]
		if !ok {
			return fmt.Errorf("unsupported protocol: %s", protocol)
		}
		port = def
	}

	// Build hostname with port only if hostname doesn't already include port
	if port != 0 && hostname != "" && !containsPort(hostname) {
		hostname = stdnet.JoinHostPort(hostname, strconv.Itoa(port))
	}

	logging.Infof("Testing node %s via %s protocol at %s", nodeSpec, protocol, hostname)

	// Create test node
	testNode := &models.Node{
		Zone:              int(zone),
		Net:               int(net),
		Node:              int(node),
		Domain:            nodeDomain,
		InternetHostnames: []string{hostname},
		HasInet:           true,
		InternetProtocols: []string{},
	}

	// Set protocol flag based on requested protocol
	if flag, ok := protocolFlags[protocol]; ok {
		testNode.InternetProtocols = []string{flag}
	}

	// Run the test - but we need to handle the case where hostname is already an IP
	// The testNode function expects to do DNS resolution, but we may have an IP directly
	result := models.NewTestResult(testNode)

	// Extract hostname without port for DNS resolution (bracket-aware for IPv6)
	var hostOnly string
	if h, _, err := stdnet.SplitHostPort(hostname); err == nil {
		hostOnly = h
	} else {
		// No port — strip brackets from bare IPv6 literals
		hostOnly = strings.Trim(hostname, "[]")
	}

	// Check if hostname is already an IP address (zone IDs like %eth0 are valid IPv6 but ParseIP rejects them)
	hostForParse := hostOnly
	if idx := strings.Index(hostForParse, "%"); idx != -1 {
		hostForParse = hostForParse[:idx]
	}
	isIP := stdnet.ParseIP(hostForParse) != nil

	// If it's an IP, skip DNS resolution and set it directly
	if isIP {
		if stdnet.ParseIP(hostForParse).To4() != nil {
			result.ResolvedIPv4 = []string{hostOnly}
		} else {
			result.ResolvedIPv6 = []string{hostOnly}
		}
		result.Hostname = hostname

		// Get geolocation for the IP
		if geo := d.geolocator.GetLocation(ctx, hostOnly); geo != nil {
			result.SetGeolocation(geo)
		}
	} else {
		// Do DNS resolution (on hostname without port)
		logging.Debugf("Resolving DNS for hostname: %s (stripped from %s)", hostOnly, hostname)
		if dnsResult := d.dnsResolver.Resolve(ctx, hostOnly); dnsResult != nil {
			logging.Debugf("DNS resolution result: IPv4=%v, IPv6=%v", dnsResult.IPv4Addresses, dnsResult.IPv6Addresses)
			result.SetDNSResult(dnsResult)

			// Get geolocation for first resolved IP
			if len(dnsResult.IPv4Addresses) > 0 {
				if geo := d.geolocator.GetLocation(ctx, dnsResult.IPv4Addresses[0]); geo != nil {
					result.SetGeolocation(geo)
				}
			}
		} else {
			logging.Debugf("DNS resolution returned nil for hostname: %s", hostname)
		}
	}

	// Test the requested protocol
	logging.Debugf("Testing protocol: %s, DNS result - IPv4: %v, IPv6: %v", protocol, result.ResolvedIPv4, result.ResolvedIPv6)

	switch protocol {
	case "binkp":
		if d.config.Protocols.BinkP.Enabled && d.binkpTester != nil {
			logging.Debugf("Calling testBinkP")
			d.testBinkP(ctx, testNode, result)
		}
	case "ifcico":
		if d.config.Protocols.Ifcico.Enabled && d.ifcicoTester != nil {
			logging.Debugf("Calling testIfcico with node hostnames: %v", testNode.InternetHostnames)
			d.testIfcico(ctx, testNode, result)
		} else {
			logging.Debugf("NOT calling testIfcico - Enabled: %v, Tester nil: %v", d.config.Protocols.Ifcico.Enabled, d.ifcicoTester == nil)
		}
	case "telnet":
		if d.config.Protocols.Telnet.Enabled && d.telnetTester != nil {
			d.testTelnet(ctx, testNode, result)
		}
	case "ftp":
		if d.config.Protocols.FTP.Enabled && d.ftpTester != nil {
			d.testFTP(ctx, testNode, result)
		}
	case "vmodem":
		if d.config.Protocols.VModem.Enabled && d.vmodemTester != nil {
			// A VMP call rings the remote sysop's mailer for real, which is
			// exactly why this is worth having by hand: one node, on purpose.
			d.testVModem(ctx, testNode, result)
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

	if protocol == "vmodem" && result.VModemResult != nil {
		det := result.VModemResult.VModemDetails()
		if result.VModemResult.Success {
			logging.Infof("  VModem: Reachable")
		} else {
			logging.Infof("  VModem: Failed - %s", result.VModemResult.Error)
		}
		if det != nil {
			logging.Infof("    Protocol: %s (conformant VMODEM: %v)", det.Variant, det.Conformant)
			if det.CallOutcome != "" {
				logging.Infof("    VMP call: %s", det.CallOutcome)
			}
			if det.Detail != "" {
				logging.Infof("    Detail: %s", det.Detail)
			}
			if det.Software != "" {
				logging.Infof("    Software: %s", det.Software)
			}
			if det.SystemName != "" {
				logging.Infof("    System: %s", det.SystemName)
			}
			if det.Sysop != "" {
				logging.Infof("    Sysop: %s", det.Sysop)
			}
			if len(det.Addresses) > 0 {
				logging.Infof("    Addresses: %v (expected present: %v)", det.Addresses, det.AddressValid)
			}
			if det.Banner != "" {
				logging.Infof("    Banner: %s", det.Banner)
			}
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

// containsPort checks if a hostname string already contains a port.
// Uses net.SplitHostPort for correct IPv6 handling.
func containsPort(hostname string) bool {
	_, _, err := stdnet.SplitHostPort(hostname)
	return err == nil
}
