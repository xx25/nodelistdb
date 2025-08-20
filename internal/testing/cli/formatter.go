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
	f.writer.WriteString("\r\n")
	f.writer.WriteString(text)
	f.writer.WriteString("\r\n")
	f.writer.WriteString(strings.Repeat("=", len(text)))
	f.writer.WriteString("\r\n")
}

func (f *Formatter) WriteSubHeader(text string) {
	f.writer.WriteString("\r\n")
	f.writer.WriteString(text)
	f.writer.WriteString("\r\n")
	f.writer.WriteString(strings.Repeat("-", len(text)))
	f.writer.WriteString("\r\n")
}

func (f *Formatter) WriteKeyValue(key, value string) {
	f.writer.WriteString(fmt.Sprintf("%-20s: %s\r\n", key, value))
}

func (f *Formatter) WriteInfo(text string) {
	f.writer.WriteString(text)
	f.writer.WriteString("\r\n")
}

func (f *Formatter) WriteSuccess(text string) {
	f.writer.WriteString("[SUCCESS] ")
	f.writer.WriteString(text)
	f.writer.WriteString("\r\n")
}

func (f *Formatter) WriteError(text string) {
	f.writer.WriteString("[ERROR] ")
	f.writer.WriteString(text)
	f.writer.WriteString("\r\n")
}

func (f *Formatter) WriteWarning(text string) {
	f.writer.WriteString("[WARNING] ")
	f.writer.WriteString(text)
	f.writer.WriteString("\r\n")
}

func (f *Formatter) WriteTimestamp(text string) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	f.writer.WriteString(fmt.Sprintf("[%s] %s\r\n", timestamp, text))
}

func (f *Formatter) FormatTestResult(result *TestResult) {
	f.WriteSubHeader("Test Results")
	
	f.WriteKeyValue("Node", result.Address)
	f.WriteKeyValue("Test ID", result.TestID)
	f.WriteKeyValue("Started", result.StartTime.Format("2006-01-02 15:04:05"))
	f.WriteKeyValue("Duration", fmt.Sprintf("%dms", result.Duration.Milliseconds()))
	
	if result.Hostname != "" {
		f.WriteKeyValue("Hostname", result.Hostname)
	}
	
	if len(result.ResolvedIPs) > 0 {
		f.WriteKeyValue("Resolved IPs", strings.Join(result.ResolvedIPs, ", "))
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
	
	if result.BinkPResult != nil {
		f.formatProtocolResult("BinkP", result.BinkPResult)
	}
	
	if result.IFCICOResult != nil {
		f.formatProtocolResult("IFCICO", result.IFCICOResult)
	}
	
	if result.TelnetResult != nil {
		f.formatProtocolResult("Telnet", result.TelnetResult)
	}
	
	f.WriteSubHeader("Summary")
	if result.IsOperational {
		f.WriteSuccess("Node is OPERATIONAL")
	} else {
		f.WriteError("Node is NOT OPERATIONAL")
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
	f.WriteSubHeader(fmt.Sprintf("%s Test Results", name))
	
	if !result.Tested {
		f.WriteInfo("Not tested")
		return
	}
	
	if result.Success {
		f.WriteSuccess(fmt.Sprintf("Connection successful (%dms)", result.ResponseTime))
	} else {
		f.WriteError(fmt.Sprintf("Connection failed: %s", result.Error))
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
	f.writer.Flush()
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