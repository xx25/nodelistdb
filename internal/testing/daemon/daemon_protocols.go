package daemon

import (
	"context"

	"github.com/nodelistdb/internal/testing/logging"
	"github.com/nodelistdb/internal/testing/models"
	"github.com/nodelistdb/internal/testing/protocols"
)

// testBinkP tests BinkP connectivity on both IPv4 and IPv6
func (d *Daemon) testBinkP(ctx context.Context, node *models.Node, result *models.TestResult) {
	if d.binkpTester == nil {
		return
	}
	
	hostname := node.GetPrimaryHostname()
	if hostname == "" {
		return
	}
	
	// Use custom port if specified, otherwise use default from config
	port := node.GetProtocolPort("IBN")
	if port == 0 {
		port = d.config.Protocols.BinkP.Port
	}
	
	// Initialize BinkP result
	if result.BinkPResult == nil {
		result.BinkPResult = &models.ProtocolTestResult{
			Details: make(map[string]interface{}),
		}
	}
	
	// Test IPv6 first (if available)
	if len(result.ResolvedIPv6) > 0 {
		for _, ipv6 := range result.ResolvedIPv6 {
			logging.Debugf("[%s]   Testing BinkP IPv6 %s:%d", node.Address(), ipv6, port)
			testResult := d.binkpTester.Test(ctx, ipv6, port, node.Address())

			if binkpResult, ok := testResult.(*protocols.BinkPTestResult); ok {
				result.BinkPResult.SetIPv6Result(
					binkpResult.Success,
					binkpResult.ResponseMs,
					ipv6,
					binkpResult.Error,
				)

				// Store details if successful
				if binkpResult.Success {
					logging.Debugf("[%s]     BinkP IPv6 success: %s (%dms)", node.Address(), binkpResult.SystemName, binkpResult.ResponseMs)
					details := &models.BinkPTestDetails{
						SystemName:   binkpResult.SystemName,
						Sysop:        binkpResult.Sysop,
						Location:     binkpResult.Location,
						Version:      binkpResult.Version,
						Addresses:    binkpResult.Addresses,
						Capabilities: binkpResult.Capabilities,
					}
					result.BinkPResult.Details["ipv6"] = details

					if binkpResult.AddressValid {
						result.AddressValidated = true
					}
					break // First successful IPv6 is enough
				} else if binkpResult.Error != "" {
					logging.Debugf("[%s]     BinkP IPv6 failed: %s", node.Address(), binkpResult.Error)
				} else {
					logging.Debugf("[%s]     BinkP IPv6 failed: timeout or connection refused", node.Address())
				}
			}
		}
	}
	
	// Test IPv4 (if available)
	if len(result.ResolvedIPv4) > 0 {
		for _, ipv4 := range result.ResolvedIPv4 {
			logging.Debugf("[%s]   Testing BinkP IPv4 %s:%d", node.Address(), ipv4, port)
			testResult := d.binkpTester.Test(ctx, ipv4, port, node.Address())

			if binkpResult, ok := testResult.(*protocols.BinkPTestResult); ok {
				result.BinkPResult.SetIPv4Result(
					binkpResult.Success,
					binkpResult.ResponseMs,
					ipv4,
					binkpResult.Error,
				)

				// Store details if successful
				if binkpResult.Success {
					logging.Debugf("[%s]     BinkP IPv4 success: %s (%dms)", node.Address(), binkpResult.SystemName, binkpResult.ResponseMs)
					details := &models.BinkPTestDetails{
						SystemName:   binkpResult.SystemName,
						Sysop:        binkpResult.Sysop,
						Location:     binkpResult.Location,
						Version:      binkpResult.Version,
						Addresses:    binkpResult.Addresses,
						Capabilities: binkpResult.Capabilities,
					}
					result.BinkPResult.Details["ipv4"] = details

					if binkpResult.AddressValid {
						result.AddressValidated = true
					}
					break // First successful IPv4 is enough
				} else if binkpResult.Error != "" {
					logging.Debugf("[%s]     BinkP IPv4 failed: %s", node.Address(), binkpResult.Error)
				} else {
					logging.Debugf("[%s]     BinkP IPv4 failed: timeout or connection refused", node.Address())
				}
			}
		}
	}
	
	// Update operational status if either IPv4 or IPv6 succeeded
	if result.BinkPResult.Success && !result.IsOperational {
		result.IsOperational = true
	}
}

// testIfcico tests IFCICO connectivity on both IPv4 and IPv6
func (d *Daemon) testIfcico(ctx context.Context, node *models.Node, result *models.TestResult) {
	if d.ifcicoTester == nil {
		return
	}
	
	hostname := node.GetPrimaryHostname()
	if hostname == "" {
		return
	}
	
	// Use custom port if specified, otherwise use default from config
	port := node.GetProtocolPort("IFC")
	if port == 0 {
		port = d.config.Protocols.Ifcico.Port
	}
	
	// Initialize IFCICO result
	if result.IfcicoResult == nil {
		result.IfcicoResult = &models.ProtocolTestResult{
			Details: make(map[string]interface{}),
		}
	}
	
	// Test IPv6 first (if available)
	if len(result.ResolvedIPv6) > 0 {
		for _, ipv6 := range result.ResolvedIPv6 {
			logging.Debugf("[%s]   Testing IFCICO IPv6 %s:%d", node.Address(), ipv6, port)
			testResult := d.ifcicoTester.Test(ctx, ipv6, port, node.Address())

			if ifcicoResult, ok := testResult.(*protocols.IfcicoTestResult); ok {
				result.IfcicoResult.SetIPv6Result(
					ifcicoResult.Success,
					ifcicoResult.ResponseMs,
					ipv6,
					ifcicoResult.Error,
				)

				// Store details if successful
				if ifcicoResult.Success {
					logging.Debugf("[%s]     IFCICO IPv6 success: %s (%dms)", node.Address(), ifcicoResult.SystemName, ifcicoResult.ResponseMs)
					details := &models.IfcicoTestDetails{
						MailerInfo:   ifcicoResult.MailerInfo,
						SystemName:   ifcicoResult.SystemName,
						Addresses:    ifcicoResult.Addresses,
						ResponseType: ifcicoResult.ResponseType,
					}
					result.IfcicoResult.Details["ipv6"] = details

					if ifcicoResult.AddressValid {
						result.AddressValidated = true
					}
					break // First successful IPv6 is enough
				} else if ifcicoResult.Error != "" {
					logging.Debugf("[%s]     IFCICO IPv6 failed: %s", node.Address(), ifcicoResult.Error)
				} else {
					logging.Debugf("[%s]     IFCICO IPv6 failed: timeout or connection refused", node.Address())
				}
			}
		}
	}
	
	// Test IPv4 (if available)
	if len(result.ResolvedIPv4) > 0 {
		for _, ipv4 := range result.ResolvedIPv4 {
			logging.Debugf("[%s]   Testing IFCICO IPv4 %s:%d", node.Address(), ipv4, port)
			testResult := d.ifcicoTester.Test(ctx, ipv4, port, node.Address())

			if ifcicoResult, ok := testResult.(*protocols.IfcicoTestResult); ok {
				result.IfcicoResult.SetIPv4Result(
					ifcicoResult.Success,
					ifcicoResult.ResponseMs,
					ipv4,
					ifcicoResult.Error,
				)

				// Store details if successful
				if ifcicoResult.Success {
					logging.Debugf("[%s]     IFCICO IPv4 success: %s (%dms)", node.Address(), ifcicoResult.SystemName, ifcicoResult.ResponseMs)
					details := &models.IfcicoTestDetails{
						MailerInfo:   ifcicoResult.MailerInfo,
						SystemName:   ifcicoResult.SystemName,
						Addresses:    ifcicoResult.Addresses,
						ResponseType: ifcicoResult.ResponseType,
					}
					result.IfcicoResult.Details["ipv4"] = details

					if ifcicoResult.AddressValid {
						result.AddressValidated = true
					}
					break // First successful IPv4 is enough
				} else if ifcicoResult.Error != "" {
					logging.Debugf("[%s]     IFCICO IPv4 failed: %s", node.Address(), ifcicoResult.Error)
				} else {
					logging.Debugf("[%s]     IFCICO IPv4 failed: timeout or connection refused", node.Address())
				}
			}
		}
	}
	
	// Update operational status if either IPv4 or IPv6 succeeded
	if result.IfcicoResult.Success && !result.IsOperational {
		result.IsOperational = true
	}
}

// testTelnet tests Telnet connectivity on both IPv4 and IPv6
func (d *Daemon) testTelnet(ctx context.Context, node *models.Node, result *models.TestResult) {
	if d.telnetTester == nil {
		return
	}
	
	hostname := node.GetPrimaryHostname()
	if hostname == "" {
		return
	}
	
	// Use custom port if specified, otherwise use default from config
	port := node.GetProtocolPort("ITN")
	if port == 0 {
		port = d.config.Protocols.Telnet.Port
	}
	
	// Initialize Telnet result
	if result.TelnetResult == nil {
		result.TelnetResult = &models.ProtocolTestResult{
			Details: make(map[string]interface{}),
		}
	}
	
	// Test IPv6 first (if available)
	if len(result.ResolvedIPv6) > 0 {
		for _, ipv6 := range result.ResolvedIPv6 {
			testResult := d.telnetTester.Test(ctx, ipv6, port, node.Address())
			
			if telnetResult, ok := testResult.(*protocols.TelnetTestResult); ok {
				result.TelnetResult.SetIPv6Result(
					telnetResult.Success,
					telnetResult.ResponseMs,
					ipv6,
					telnetResult.Error,
				)
				
				// Store banner if successful
				if telnetResult.Success && telnetResult.Banner != "" {
					result.TelnetResult.Details["ipv6_banner"] = telnetResult.Banner
					break // First successful IPv6 is enough
				}
			}
		}
	}
	
	// Test IPv4 (if available)
	if len(result.ResolvedIPv4) > 0 {
		for _, ipv4 := range result.ResolvedIPv4 {
			testResult := d.telnetTester.Test(ctx, ipv4, port, node.Address())
			
			if telnetResult, ok := testResult.(*protocols.TelnetTestResult); ok {
				result.TelnetResult.SetIPv4Result(
					telnetResult.Success,
					telnetResult.ResponseMs,
					ipv4,
					telnetResult.Error,
				)
				
				// Store banner if successful
				if telnetResult.Success && telnetResult.Banner != "" {
					result.TelnetResult.Details["ipv4_banner"] = telnetResult.Banner
					break // First successful IPv4 is enough
				}
			}
		}
	}
	
	// Update operational status if either IPv4 or IPv6 succeeded
	if result.TelnetResult.Success && !result.IsOperational {
		result.IsOperational = true
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
	
	// Use custom port if specified, otherwise use default from config
	port := node.GetProtocolPort("IFT")
	if port == 0 {
		port = d.config.Protocols.FTP.Port
	}
	
	testResult := d.ftpTester.Test(ctx, ip, port, node.Address())
	
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