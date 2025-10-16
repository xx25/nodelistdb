package cli

import (
	"bufio"
	"fmt"
	"strings"
	"time"
)

type Formatter struct {
	writer *bufio.Writer
}

func NewFormatter(writer *bufio.Writer) *Formatter {
	return &Formatter{
		writer: writer,
	}
}

func (f *Formatter) WriteHeader(text string) {
	_, _ = f.writer.WriteString("\r\n")
	_, _ = f.writer.WriteString(text)
	_, _ = f.writer.WriteString("\r\n")
	_, _ = f.writer.WriteString(strings.Repeat("=", len(text)))
	_, _ = f.writer.WriteString("\r\n")
}

func (f *Formatter) WriteSubHeader(text string) {
	_, _ = f.writer.WriteString("\r\n")
	_, _ = f.writer.WriteString(text)
	_, _ = f.writer.WriteString("\r\n")
	_, _ = f.writer.WriteString(strings.Repeat("-", len(text)))
	_, _ = f.writer.WriteString("\r\n")
}

func (f *Formatter) WriteKeyValue(key, value string) {
	_, _ = f.writer.WriteString(fmt.Sprintf("%-20s: %s\r\n", key, value))
}

func (f *Formatter) WriteInfo(text string) {
	_, _ = f.writer.WriteString(text)
	_, _ = f.writer.WriteString("\r\n")
}

func (f *Formatter) WriteSuccess(text string) {
	_, _ = f.writer.WriteString("[SUCCESS] ")
	_, _ = f.writer.WriteString(text)
	_, _ = f.writer.WriteString("\r\n")
}

func (f *Formatter) WriteError(text string) {
	_, _ = f.writer.WriteString("[ERROR] ")
	_, _ = f.writer.WriteString(text)
	_, _ = f.writer.WriteString("\r\n")
}

func (f *Formatter) WriteWarning(text string) {
	_, _ = f.writer.WriteString("[WARNING] ")
	_, _ = f.writer.WriteString(text)
	_, _ = f.writer.WriteString("\r\n")
}

func (f *Formatter) WriteDebug(text string) {
	_, _ = f.writer.WriteString("[DEBUG] ")
	_, _ = f.writer.WriteString(text)
	_, _ = f.writer.WriteString("\r\n")
}

func (f *Formatter) WriteTimestamp(text string) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	_, _ = f.writer.WriteString(fmt.Sprintf("[%s] %s\r\n", timestamp, text))
}

func (f *Formatter) FormatTestResult(result *TestResult) {
	f.WriteSubHeader("Test Results")
	
	f.WriteKeyValue("Node", result.Address)
	f.WriteKeyValue("Test ID", result.TestID)
	f.WriteKeyValue("Started", result.StartTime.Format("2006-01-02 15:04:05"))
	f.WriteKeyValue("Duration", fmt.Sprintf("%dms", result.Duration.Milliseconds()))
	
	// DNS Information
	if result.Hostname != "" {
		f.WriteSubHeader("DNS Resolution")
		f.WriteKeyValue("Hostname", result.Hostname)
		if len(result.ResolvedIPs) > 0 {
			f.WriteKeyValue("Resolved IPs", strings.Join(result.ResolvedIPs, ", "))
		} else {
			f.WriteError("DNS resolution failed - no IPs resolved")
		}
	} else {
		f.WriteWarning("No hostname available for testing")
	}
	
	if result.Geolocation != nil {
		f.WriteSubHeader("Geolocation")
		f.WriteKeyValue("Country", fmt.Sprintf("%s (%s)", result.Geolocation.Country, result.Geolocation.CountryCode))
		f.WriteKeyValue("City", result.Geolocation.City)
		f.WriteKeyValue("ISP", result.Geolocation.ISP)
		if result.Geolocation.ASN != "" {
			f.WriteKeyValue("ASN", result.Geolocation.ASN)
		}
	}
	
	// Show protocol test results
	f.WriteSubHeader("Protocol Tests")
	
	if result.BinkPResult != nil {
		f.formatProtocolResult("BinkP (IBN)", result.BinkPResult)
	}
	
	if result.IFCICOResult != nil {
		f.formatProtocolResult("IFCICO (IFC)", result.IFCICOResult)
	}
	
	if result.TelnetResult != nil {
		f.formatProtocolResult("Telnet (ITN)", result.TelnetResult)
	}
	
	if result.FTPResult != nil {
		f.formatProtocolResult("FTP (IFT)", result.FTPResult)
	}
	
	if result.VModemResult != nil {
		f.formatProtocolResult("VModem (IVM)", result.VModemResult)
	}
	
	f.WriteSubHeader("Summary")
	
	// Count what was tested
	protocolsTested := 0
	protocolsSucceeded := 0
	
	if result.BinkPResult != nil && result.BinkPResult.Tested {
		protocolsTested++
		if result.BinkPResult.Success {
			protocolsSucceeded++
		}
	}
	if result.IFCICOResult != nil && result.IFCICOResult.Tested {
		protocolsTested++
		if result.IFCICOResult.Success {
			protocolsSucceeded++
		}
	}
	if result.TelnetResult != nil && result.TelnetResult.Tested {
		protocolsTested++
		if result.TelnetResult.Success {
			protocolsSucceeded++
		}
	}
	if result.FTPResult != nil && result.FTPResult.Tested {
		protocolsTested++
		if result.FTPResult.Success {
			protocolsSucceeded++
		}
	}
	if result.VModemResult != nil && result.VModemResult.Tested {
		protocolsTested++
		if result.VModemResult.Success {
			protocolsSucceeded++
		}
	}
	
	f.WriteKeyValue("Protocols Tested", fmt.Sprintf("%d", protocolsTested))
	f.WriteKeyValue("Protocols Succeeded", fmt.Sprintf("%d", protocolsSucceeded))
	
	if result.IsOperational {
		f.WriteSuccess("Node is OPERATIONAL")
	} else {
		if protocolsTested == 0 {
			f.WriteError("Node is NOT OPERATIONAL (no protocols could be tested)")
		} else if protocolsSucceeded == 0 {
			f.WriteError(fmt.Sprintf("Node is NOT OPERATIONAL (all %d tested protocols failed)", protocolsTested))
		} else {
			f.WriteError(fmt.Sprintf("Node is NOT OPERATIONAL (%d/%d protocols failed)", protocolsTested-protocolsSucceeded, protocolsTested))
		}
	}
	
	if result.HasConnectivityIssues {
		f.WriteWarning("Connectivity issues detected")
	}
	
	if result.AddressValidated {
		f.WriteSuccess("Address validated successfully")
	} else if result.ExpectedAddress != "" {
		f.WriteWarning(fmt.Sprintf("Address mismatch (expected: %s)", result.ExpectedAddress))
	}
}

func (f *Formatter) formatProtocolResult(name string, result *ProtocolResult) {
	f.WriteSubHeader(fmt.Sprintf("%s Test", name))
	
	if !result.Tested {
		f.WriteInfo("Not tested (protocol not enabled or not supported by node)")
		return
	}
	
	if result.Port > 0 {
		f.WriteKeyValue("Port", fmt.Sprintf("%d", result.Port))
	}
	
	if result.Success {
		f.WriteSuccess(fmt.Sprintf("Connection successful (%dms)", result.ResponseTime))
	} else {
		if result.Error != "" {
			f.WriteError(fmt.Sprintf("Connection failed: %s", result.Error))
		} else {
			f.WriteError("Connection failed (no error details available)")
		}
		return
	}
	
	if result.SystemName != "" {
		f.WriteKeyValue("System Name", result.SystemName)
	}
	if result.Sysop != "" {
		f.WriteKeyValue("Sysop", result.Sysop)
	}
	if result.Location != "" {
		f.WriteKeyValue("Location", result.Location)
	}
	if result.Version != "" {
		f.WriteKeyValue("Software", result.Version)
	}
	if len(result.Addresses) > 0 {
		f.WriteKeyValue("Addresses", strings.Join(result.Addresses, ", "))
	}
	if len(result.Capabilities) > 0 {
		f.WriteKeyValue("Capabilities", strings.Join(result.Capabilities, ", "))
	}
	if result.Port > 0 && result.Port != getDefaultPort(name) {
		f.WriteKeyValue("Port", fmt.Sprintf("%d", result.Port))
	}
}

func (f *Formatter) FormatLiveTestOutput(output TestOutput) {
	switch output.Type {
	case "info":
		f.WriteTimestamp(output.Message)
	case "success":
		f.WriteTimestamp(fmt.Sprintf("✓ %s", output.Message))
	case "error":
		f.WriteTimestamp(fmt.Sprintf("✗ %s", output.Message))
	case "debug":
		f.WriteTimestamp(fmt.Sprintf("  %s", output.Message))
	default:
		f.WriteInfo(output.Message)
	}
	_ = f.writer.Flush()
}

func getDefaultPort(protocol string) int {
	switch strings.ToLower(protocol) {
	case "binkp":
		return 24554
	case "ifcico":
		return 60179
	case "telnet":
		return 23
	case "ftp":
		return 21
	default:
		return 0
	}
}

func (f *Formatter) FormatNodeInfo(info *NodeInfo) {
	if !info.Found {
		f.WriteError(fmt.Sprintf("Node %s not found in database", info.Address))
		if info.ErrorMessage != "" {
			f.WriteInfo(fmt.Sprintf("Error: %s", info.ErrorMessage))
		}
		return
	}
	
	f.WriteHeader(fmt.Sprintf("Node Information: %s", info.Address))
	
	f.WriteSubHeader("Basic Information")
	f.WriteKeyValue("System Name", info.SystemName)
	f.WriteKeyValue("Sysop", info.SysopName)
	f.WriteKeyValue("Location", info.Location)
	if info.NodeType != "" {
		f.WriteKeyValue("Node Type", info.NodeType)
	}
	
	f.WriteSubHeader("Internet Configuration")
	if info.HasInternet {
		f.WriteSuccess("Internet connectivity enabled")
		if len(info.InternetHostnames) > 0 {
			f.WriteKeyValue("Hostnames", strings.Join(info.InternetHostnames, ", "))
		} else {
			f.WriteWarning("No hostnames configured")
		}
		if len(info.InternetProtocols) > 0 {
			f.WriteKeyValue("Protocols", strings.Join(info.InternetProtocols, ", "))
		} else {
			f.WriteWarning("No protocols configured")
		}
	} else {
		f.WriteInfo("No internet connectivity")
	}
	
	if len(info.Flags) > 0 {
		f.WriteSubHeader("Node Flags")
		f.WriteKeyValue("Flags", strings.Join(info.Flags, ", "))
	}
	
	if len(info.ModemFlags) > 0 {
		f.WriteKeyValue("Modem Flags", strings.Join(info.ModemFlags, ", "))
	}
	
	if !info.LastSeen.IsZero() {
		f.WriteSubHeader("Last Activity")
		f.WriteKeyValue("Last Seen", info.LastSeen.Format("2006-01-02"))
	}
}