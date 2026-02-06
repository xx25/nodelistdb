package daemon

import (
	"context"
	"time"

	"github.com/nodelistdb/internal/domain"
	"github.com/nodelistdb/internal/testing/logging"
	"github.com/nodelistdb/internal/testing/models"
)

// TestExecutor handles test orchestration and execution
type TestExecutor struct {
	daemon *Daemon
}

// NewTestExecutor creates a new test executor
func NewTestExecutor(d *Daemon) *TestExecutor {
	return &TestExecutor{daemon: d}
}

// TestNode tests a node based on whether it has single or multiple hostnames
func (te *TestExecutor) TestNode(ctx context.Context, node *models.Node) *models.TestResult {
	// Note: Scheduling logic should be handled by the daemon before calling this method

	// If node has no hostnames but has a valid system name that can be used as hostname,
	// add it to the InternetHostnames temporarily for testing
	if len(node.InternetHostnames) == 0 {
		if hostname := node.GetPrimaryHostname(); hostname != "" {
			node.InternetHostnames = []string{hostname}
		}
	}

	// Check if node has multiple hostnames
	if len(node.InternetHostnames) > 1 {
		return te.testMultipleHostnameNode(ctx, node)
	}
	return te.testSingleHostnameNode(ctx, node)
}

// testSingleHostnameNode tests a node with a single hostname
func (te *TestExecutor) testSingleHostnameNode(ctx context.Context, node *models.Node) *models.TestResult {
	hostname := ""
	if len(node.InternetHostnames) > 0 {
		hostname = node.InternetHostnames[0]
	}

	// Perform testing with the single hostname (or empty string if no hostname)
	result := te.performTesting(ctx, node, hostname)

	// Storage is handled by the caller (daemon or CLI), not here
	return result
}

// testMultipleHostnameNode tests a node with multiple hostnames
func (te *TestExecutor) testMultipleHostnameNode(ctx context.Context, node *models.Node) *models.TestResult {
	nodeAddr := node.Address()
	logging.Infof("Testing node %s with %d hostnames", nodeAddr, len(node.InternetHostnames))

	var results []*models.TestResult

	// Test each hostname
	for i, hostname := range node.InternetHostnames {
		logging.Debugf("Testing hostname %d/%d: %s", i+1, len(node.InternetHostnames), hostname)

		// Perform testing for this specific hostname
		result := te.performTesting(ctx, node, hostname)
		if result != nil {
			// Mark this as a partial result for a specific hostname
			result.TestedHostname = hostname
			result.HostnameIndex = int32(i)
			result.IsAggregated = false

			// Store the partial result
			if err := te.daemon.storage.StoreTestResult(ctx, result); err != nil {
				logging.Errorf("Failed to store partial test result for %s (hostname: %s): %v",
					nodeAddr, hostname, err)
			}

			results = append(results, result)
		}

		// Add a small delay between hostname tests to avoid overwhelming the node
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(100 * time.Millisecond):
		}
	}

	// Create an aggregated result
	// Always return a non-nil result to prevent crashes in scheduler
	aggregated := NewTestAggregator().CreateAggregatedResult(node, results)

	// Storage is handled by the caller (daemon or CLI), not here
	return aggregated
}

// performTesting performs the actual testing of a node
func (te *TestExecutor) performTesting(ctx context.Context, node *models.Node, hostname string) *models.TestResult {
	result := &models.TestResult{
		Zone:           node.Zone,
		Net:            node.Net,
		Node:           node.Node,
		Address:        node.Address(),
		TestTime:       time.Now(),
		TestDate:       time.Now().Truncate(24 * time.Hour),
		Hostname:       hostname,
		TestedHostname: hostname,
		IPv4Skipped:    node.HasINO4(), // Mark if IPv4 testing will be skipped
	}

	nodeAddr := node.Address()

	// Set initial operational status
	result.IsOperational = false

	// DNS resolution if hostname is provided
	if hostname != "" {
		// Enqueue WHOIS lookup for the domain (non-blocking, runs regardless of DNS result)
		if regDomain := domain.ExtractRegistrableDomain(hostname); regDomain != "" {
			te.daemon.whoisWorker.Enqueue(regDomain)
		}

		logging.Debugf("Starting DNS resolution for %s", hostname)
		dnsResult := te.daemon.dnsResolver.Resolve(ctx, hostname)

		if dnsResult.Error != nil {
			result.DNSError = dnsResult.Error.Error()
			logging.Infof("DNS resolution failed for %s (%s): %s", nodeAddr, hostname, dnsResult.Error)
			te.daemon.logConnectivitySummary(nodeAddr, node, result)
			return result
		}

		// Store resolved IPs
		result.ResolvedIPv4 = dnsResult.IPv4Addresses
		result.ResolvedIPv6 = dnsResult.IPv6Addresses

		// Geolocation (optional, don't fail if it doesn't work)
		if len(dnsResult.IPv4Addresses) > 0 || len(dnsResult.IPv6Addresses) > 0 {
			var ipForGeo string
			if len(dnsResult.IPv4Addresses) > 0 {
				ipForGeo = dnsResult.IPv4Addresses[0]
			} else if len(dnsResult.IPv6Addresses) > 0 {
				ipForGeo = dnsResult.IPv6Addresses[0]
			}

			if ipForGeo != "" {
				logging.Debugf("Performing geolocation for %s", ipForGeo)
				geoResult := te.daemon.geolocator.GetLocation(ctx, ipForGeo)
				if geoResult != nil {
					result.Country = geoResult.Country
					result.CountryCode = geoResult.CountryCode
					result.City = geoResult.City
					result.Region = geoResult.Region
					result.Latitude = geoResult.Latitude
					result.Longitude = geoResult.Longitude
					result.ISP = geoResult.ISP
					result.Org = geoResult.Org
					result.ASN = geoResult.ASN
				}
			}
		}
	} else {
		logging.Debugf("No hostname available for %s, skipping DNS resolution", nodeAddr)
	}

	// Protocol tests (only if we have connectivity)
	if hostname != "" && result.DNSError == "" && (len(result.ResolvedIPv4) > 0 || len(result.ResolvedIPv6) > 0) {
		// Delegate to daemon's protocol testing methods that handle dual-stack properly
		// These methods test both IPv4 and IPv6 addresses and try multiple IPs

		// Binkp test
		if node.HasProtocol("IBN") && te.daemon.binkpTester != nil {
			logging.Debugf("Testing Binkp for %s", nodeAddr)
			te.daemon.testBinkP(ctx, node, result)
		}

		// IFCico/EMSI test
		if node.HasProtocol("IFC") && te.daemon.ifcicoTester != nil {
			logging.Debugf("Testing IFCico/EMSI for %s", nodeAddr)
			te.daemon.testIfcico(ctx, node, result)
		}

		// Telnet test
		if node.HasProtocol("ITN") && te.daemon.telnetTester != nil {
			logging.Debugf("Testing Telnet for %s", nodeAddr)
			te.daemon.testTelnet(ctx, node, result)
		}

		// FTP test
		if node.HasProtocol("IFT") && te.daemon.ftpTester != nil {
			logging.Debugf("Testing FTP for %s", nodeAddr)
			te.daemon.testFTP(ctx, node, result)
		}

		// Vmodem test
		if node.HasProtocol("IVM") && te.daemon.vmodemTester != nil {
			logging.Debugf("Testing Vmodem for %s", nodeAddr)
			te.daemon.testVModem(ctx, node, result)
		}
	}

	// Determine overall operational status based on test results
	result.IsOperational = te.determineOperationalStatus(result)

	// Log connectivity summary using daemon's logging method
	te.daemon.logConnectivitySummary(nodeAddr, node, result)

	return result
}

// determineOperationalStatus determines if the node is operational based on test results
func (te *TestExecutor) determineOperationalStatus(result *models.TestResult) bool {
	// Check if any protocol test succeeded
	if result.BinkPResult != nil && result.BinkPResult.Tested && result.BinkPResult.Success {
		return true
	}

	if result.IfcicoResult != nil && result.IfcicoResult.Tested && result.IfcicoResult.Success {
		return true
	}

	if result.TelnetResult != nil && result.TelnetResult.Tested && result.TelnetResult.Success {
		return true
	}

	if result.FTPResult != nil && result.FTPResult.Tested && result.FTPResult.Success {
		return true
	}

	if result.VModemResult != nil && result.VModemResult.Tested && result.VModemResult.Success {
		return true
	}

	return false
}

