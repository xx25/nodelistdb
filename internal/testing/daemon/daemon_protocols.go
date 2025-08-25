package daemon

import (
	"context"

	"github.com/nodelistdb/internal/testing/logging"
	"github.com/nodelistdb/internal/testing/models"
	"github.com/nodelistdb/internal/testing/protocols"
)

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
	
	// Use custom port if specified, otherwise use default from config
	port := node.GetProtocolPort("IBN")
	if port == 0 {
		port = d.config.Protocols.BinkP.Port
	}
	
	testResult := d.binkpTester.Test(ctx, ip, port, node.Address())
	
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
	
	// Use custom port if specified, otherwise use default from config
	port := node.GetProtocolPort("IFC")
	if port == 0 {
		port = d.config.Protocols.Ifcico.Port
	}
	
	testResult := d.ifcicoTester.Test(ctx, ip, port, node.Address())
	
	if ifcicoResult, ok := testResult.(*protocols.IfcicoTestResult); ok {
		details := &models.IfcicoTestDetails{
			MailerInfo:   ifcicoResult.MailerInfo,
			SystemName:   ifcicoResult.SystemName,
			Addresses:    ifcicoResult.Addresses,
			ResponseType: ifcicoResult.ResponseType,
		}
		
		// Debug logging
		if ifcicoResult.Success && (details.MailerInfo == "" && details.SystemName == "") {
			logging.Warnf("IFCICO test successful but no details for %s: mailer=%s, system=%s, addresses=%v",
				node.Address(), details.MailerInfo, details.SystemName, details.Addresses)
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
	
	// Use custom port if specified, otherwise use default from config
	port := node.GetProtocolPort("ITN")
	if port == 0 {
		port = d.config.Protocols.Telnet.Port
	}
	
	testResult := d.telnetTester.Test(ctx, ip, port, node.Address())
	
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