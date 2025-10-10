package daemon

import (
	"fmt"
	"time"

	"github.com/nodelistdb/internal/testing/models"
)

// TestAggregator handles aggregation of test results from multiple hostnames
type TestAggregator struct{}

// NewTestAggregator creates a new test aggregator
func NewTestAggregator() *TestAggregator {
	return &TestAggregator{}
}

// CreateAggregatedResult creates an aggregated test result from multiple hostname results
func (ta *TestAggregator) CreateAggregatedResult(node *models.Node, results []*models.TestResult) *models.TestResult {
	if len(results) == 0 {
		return nil
	}

	// Start with base information from the first result
	aggregated := &models.TestResult{
		Zone:          node.Zone,
		Net:           node.Net,
		Node:          node.Node,
		TestTime:      time.Now(),
		TestDate:      time.Now().Truncate(24 * time.Hour),
		IsOperational: false,
		IsAggregated:  true, // This is the complete aggregated result
		Address:       fmt.Sprintf("%d:%d/%d", node.Zone, node.Net, node.Node),
	}

	// Track successful hostnames and protocols
	var successfulHostnames []string  // DNS successful hostnames
	var failedHostnames []string
	var operationalHostnames []string  // Protocol successful hostnames
	hasAnyDNSSuccess := false
	hasAnyProtocolSuccess := false

	// Aggregate DNS results
	var allIPv4s []string
	var allIPv6s []string
	ipv4Map := make(map[string]bool)
	ipv6Map := make(map[string]bool)

	// Track protocol successes across all hostnames
	binkpSuccess := false
	emsiSuccess := false
	telnetSuccess := false
	ftpSuccess := false
	vmodemSuccess := false

	// Process each result
	for _, result := range results {
		if result == nil {
			continue
		}

		hostname := result.TestedHostname

		// DNS aggregation
		if result.DNSError == "" && (len(result.ResolvedIPv4) > 0 || len(result.ResolvedIPv6) > 0) {
			// DNS resolution succeeded
			hasAnyDNSSuccess = true
			successfulHostnames = append(successfulHostnames, hostname)

			// Collect unique IPs
			for _, ip := range result.ResolvedIPv4 {
				if !ipv4Map[ip] {
					ipv4Map[ip] = true
					allIPv4s = append(allIPv4s, ip)
				}
			}
			for _, ip := range result.ResolvedIPv6 {
				if !ipv6Map[ip] {
					ipv6Map[ip] = true
					allIPv6s = append(allIPv6s, ip)
				}
			}
		} else if result.DNSError != "" {
			// DNS resolution failed
			failedHostnames = append(failedHostnames, hostname)
		}

		// Protocol aggregation - track if ANY hostname succeeded
		if result.BinkPResult != nil && result.BinkPResult.Tested && result.BinkPResult.Success {
			binkpSuccess = true
			hasAnyProtocolSuccess = true
			if aggregated.BinkPResult == nil || !aggregated.BinkPResult.Success {
				aggregated.BinkPResult = result.BinkPResult
			}
		}

		if result.IfcicoResult != nil && result.IfcicoResult.Tested && result.IfcicoResult.Success {
			emsiSuccess = true
			hasAnyProtocolSuccess = true
			if aggregated.IfcicoResult == nil || !aggregated.IfcicoResult.Success {
				aggregated.IfcicoResult = result.IfcicoResult
			}
		}

		if result.TelnetResult != nil && result.TelnetResult.Tested && result.TelnetResult.Success {
			telnetSuccess = true
			hasAnyProtocolSuccess = true
			if aggregated.TelnetResult == nil || !aggregated.TelnetResult.Success {
				aggregated.TelnetResult = result.TelnetResult
			}
		}

		if result.FTPResult != nil && result.FTPResult.Tested && result.FTPResult.Success {
			ftpSuccess = true
			hasAnyProtocolSuccess = true
			if aggregated.FTPResult == nil || !aggregated.FTPResult.Success {
				aggregated.FTPResult = result.FTPResult
			}
		}

		if result.VModemResult != nil && result.VModemResult.Tested && result.VModemResult.Success {
			vmodemSuccess = true
			hasAnyProtocolSuccess = true
			if aggregated.VModemResult == nil || !aggregated.VModemResult.Success {
				aggregated.VModemResult = result.VModemResult
			}
		}

		// Any success means the node is reachable
		if result.IsOperational {
			hasAnyProtocolSuccess = true
			// Track this hostname as operational
			operationalHostnames = append(operationalHostnames, hostname)
		}

		// Use geolocation from first successful result
		if result.Country != "" && aggregated.Country == "" {
			aggregated.Country = result.Country
			aggregated.CountryCode = result.CountryCode
			aggregated.City = result.City
			aggregated.Region = result.Region
			aggregated.Latitude = result.Latitude
			aggregated.Longitude = result.Longitude
			aggregated.ISP = result.ISP
			aggregated.Org = result.Org
			aggregated.ASN = result.ASN
		}
	}

	// Set aggregated DNS results
	if hasAnyDNSSuccess {
		aggregated.ResolvedIPv4 = allIPv4s
		aggregated.ResolvedIPv6 = allIPv6s
		aggregated.DNSError = ""
	} else if len(failedHostnames) > 0 {
		aggregated.DNSError = "All hostnames failed DNS resolution"
	}

	// Set aggregated tested hostname info
	// Prefer a hostname with protocol success, fallback to DNS success
	if len(operationalHostnames) > 0 {
		aggregated.TestedHostname = operationalHostnames[0] // Primary operational hostname
	} else if len(successfulHostnames) > 0 {
		aggregated.TestedHostname = successfulHostnames[0] // Primary DNS successful hostname
	}

	// Fill in protocol results with failure for those that didn't succeed
	if node.HasProtocol("IBN") && !binkpSuccess && aggregated.BinkPResult == nil {
		aggregated.BinkPResult = &models.ProtocolTestResult{
			Tested:  true,
			Success: false,
			Error:   "Failed on all hostnames",
		}
	}

	if node.HasProtocol("IFC") && !emsiSuccess && aggregated.IfcicoResult == nil {
		aggregated.IfcicoResult = &models.ProtocolTestResult{
			Tested:  true,
			Success: false,
			Error:   "Failed on all hostnames",
		}
	}

	if node.HasProtocol("ITN") && !telnetSuccess && aggregated.TelnetResult == nil {
		aggregated.TelnetResult = &models.ProtocolTestResult{
			Tested:  true,
			Success: false,
			Error:   "Failed on all hostnames",
		}
	}

	if node.HasProtocol("IFT") && !ftpSuccess && aggregated.FTPResult == nil {
		aggregated.FTPResult = &models.ProtocolTestResult{
			Tested:  true,
			Success: false,
			Error:   "Failed on all hostnames",
		}
	}

	if node.HasProtocol("IVM") && !vmodemSuccess && aggregated.VModemResult == nil {
		aggregated.VModemResult = &models.ProtocolTestResult{
			Tested:  true,
			Success: false,
			Error:   "Failed on all hostnames",
		}
	}

	// Determine overall status
	if hasAnyProtocolSuccess {
		aggregated.IsOperational = true
		aggregated.HasConnectivityIssues = false
	} else if hasAnyDNSSuccess {
		aggregated.IsOperational = false
		aggregated.HasConnectivityIssues = true
	} else {
		aggregated.IsOperational = false
		aggregated.HasConnectivityIssues = false
	}

	// Set hostname count info
	aggregated.TotalHostnames = int32(len(node.InternetHostnames))
	aggregated.HostnamesTested = int32(len(results))
	aggregated.HostnamesOperational = int32(len(operationalHostnames))  // Count protocol-successful hostnames, not DNS-successful

	return aggregated
}