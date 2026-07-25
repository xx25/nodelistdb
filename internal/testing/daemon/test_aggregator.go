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

// mergeProtocolResult folds one hostname's per-IP-family result into the
// aggregated summary, tracking IPv4 and IPv6 independently so a success on
// one hostname's IPv6 test isn't lost when another hostname's IPv4 result is
// merged in later (or vice versa), and so a per-family failure keeps its
// real error message instead of being genericized.
func mergeProtocolResult(dst, src *models.ProtocolTestResult) {
	if src == nil {
		return
	}

	if src.IPv4Tested {
		dst.IPv4Tested = true
		if src.IPv4Success {
			if !dst.IPv4Success || src.IPv4ResponseMs < dst.IPv4ResponseMs {
				dst.IPv4ResponseMs = src.IPv4ResponseMs
				dst.IPv4Address = src.IPv4Address
			}
			dst.IPv4Success = true
			dst.IPv4Error = ""
		} else if !dst.IPv4Success && src.IPv4Error != "" {
			dst.IPv4Error = src.IPv4Error
		}
	}

	if src.IPv6Tested {
		dst.IPv6Tested = true
		if src.IPv6Success {
			if !dst.IPv6Success || src.IPv6ResponseMs < dst.IPv6ResponseMs {
				dst.IPv6ResponseMs = src.IPv6ResponseMs
				dst.IPv6Address = src.IPv6Address
			}
			dst.IPv6Success = true
			dst.IPv6Error = ""
		} else if !dst.IPv6Success && src.IPv6Error != "" {
			dst.IPv6Error = src.IPv6Error
		}
	}

	// Details/software identification aren't per-family; keep the richest
	// set, preferring the first hostname that fully succeeded.
	if src.Success && dst.Details == nil {
		dst.Details = src.Details
		dst.SoftwareSource = src.SoftwareSource
	}
}

// keepFailureDetails gives the aggregated row the diagnosis of a hostname that
// FAILED, when no hostname has succeeded. mergeProtocolResult deliberately only
// takes details from a success, which leaves an all-failed multi-hostname node
// with an aggregated row that says nothing while its per-hostname rows explain
// exactly what went wrong (the VModem tester records the variant it found and
// how a VMP call ended even when the probe failed). Analytics read the
// aggregated row, so it should carry that too — and a later successful hostname
// still wins, because mergeProtocolResult runs first and overwrites nothing
// only when a success already filled it in.
func keepFailureDetails(dst, src *models.ProtocolTestResult, sawSuccess *bool) {
	if src == nil || dst == nil {
		return
	}
	if src.Success {
		if !*sawSuccess && src.Details != nil {
			// First successful hostname: replace whatever a failed one left.
			dst.Details = src.Details
			dst.SoftwareSource = src.SoftwareSource
			*sawSuccess = true
		}
		return
	}
	if !*sawSuccess && dst.Details == nil && src.Details != nil {
		dst.Details = src.Details
		dst.SoftwareSource = src.SoftwareSource
	}
}

// finalizeProtocolResult derives the overall Tested/Success/ResponseMs/Error
// fields from the merged per-IP-family results, so a node that succeeded via
// IPv6 on one hostname and failed IPv4 on another reports success overall,
// and a protocol never actually attempted on any hostname is left untested
// rather than mislabeled as failed.
func finalizeProtocolResult(pr *models.ProtocolTestResult) {
	if pr == nil {
		return
	}

	pr.Tested = pr.IPv4Tested || pr.IPv6Tested
	pr.Success = pr.IPv4Success || pr.IPv6Success

	pr.ResponseMs = 0
	if pr.IPv4Success {
		pr.ResponseMs = pr.IPv4ResponseMs
	}
	if pr.IPv6Success && (pr.ResponseMs == 0 || pr.IPv6ResponseMs < pr.ResponseMs) {
		pr.ResponseMs = pr.IPv6ResponseMs
	}

	switch {
	case pr.Success:
		pr.Error = ""
	case pr.IPv4Error != "":
		pr.Error = pr.IPv4Error
	case pr.IPv6Error != "":
		pr.Error = pr.IPv6Error
	case pr.Tested:
		pr.Error = "Failed on all hostnames"
	default:
		pr.Error = ""
	}
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
		Domain:        node.EffectiveDomain(),
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
	// Tracks whether the VModem details on the aggregated row came from a
	// hostname that succeeded; see keepFailureDetails.
	vmodemDetailsFromSuccess := false

	// Aggregate DNS results
	var allIPv4s []string
	var allIPv6s []string
	ipv4Map := make(map[string]bool)
	ipv6Map := make(map[string]bool)

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

		// Protocol aggregation - merge per-IP-family results from every
		// hostname instead of keeping only the first success (see
		// mergeProtocolResult for why that used to hide/mislabel results).
		if result.BinkPResult != nil {
			if aggregated.BinkPResult == nil {
				aggregated.BinkPResult = &models.ProtocolTestResult{}
			}
			mergeProtocolResult(aggregated.BinkPResult, result.BinkPResult)
			if result.BinkPResult.Success {
				hasAnyProtocolSuccess = true
			}
		}

		if result.IfcicoResult != nil {
			if aggregated.IfcicoResult == nil {
				aggregated.IfcicoResult = &models.ProtocolTestResult{}
			}
			mergeProtocolResult(aggregated.IfcicoResult, result.IfcicoResult)
			if result.IfcicoResult.Success {
				hasAnyProtocolSuccess = true
			}
		}

		if result.TelnetResult != nil {
			if aggregated.TelnetResult == nil {
				aggregated.TelnetResult = &models.ProtocolTestResult{}
			}
			mergeProtocolResult(aggregated.TelnetResult, result.TelnetResult)
			if result.TelnetResult.Success {
				hasAnyProtocolSuccess = true
			}
		}

		if result.FTPResult != nil {
			if aggregated.FTPResult == nil {
				aggregated.FTPResult = &models.ProtocolTestResult{}
			}
			mergeProtocolResult(aggregated.FTPResult, result.FTPResult)
			if result.FTPResult.Success {
				hasAnyProtocolSuccess = true
			}
		}

		if result.VModemResult != nil {
			if aggregated.VModemResult == nil {
				aggregated.VModemResult = &models.ProtocolTestResult{}
			}
			mergeProtocolResult(aggregated.VModemResult, result.VModemResult)
			keepFailureDetails(aggregated.VModemResult, result.VModemResult, &vmodemDetailsFromSuccess)
			if result.VModemResult.Success {
				hasAnyProtocolSuccess = true
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

	// Derive each protocol's overall Tested/Success/ResponseMs/Error from the
	// merged per-IP-family results now that every hostname has been folded in.
	finalizeProtocolResult(aggregated.BinkPResult)
	finalizeProtocolResult(aggregated.IfcicoResult)
	finalizeProtocolResult(aggregated.TelnetResult)
	finalizeProtocolResult(aggregated.FTPResult)
	finalizeProtocolResult(aggregated.VModemResult)

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