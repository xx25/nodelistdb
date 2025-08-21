package models

import (
	"time"
)

// TestResult represents the complete test result for a node
type TestResult struct {
	// Node identification
	TestTime time.Time
	TestDate time.Time
	Zone     int
	Net      int
	Node     int
	Address  string

	// DNS Resolution
	Hostname     string
	ResolvedIPv4 []string
	ResolvedIPv6 []string
	DNSError     string

	// Geolocation
	Country      string
	CountryCode  string
	City         string
	Region       string
	Latitude     float32
	Longitude    float32
	ISP          string
	Org          string
	ASN          uint32

	// Protocol Test Results
	BinkPResult  *ProtocolTestResult
	IfcicoResult *ProtocolTestResult
	TelnetResult *ProtocolTestResult
	FTPResult    *ProtocolTestResult
	VModemResult *ProtocolTestResult

	// Summary flags
	IsOperational         bool
	HasConnectivityIssues bool
	AddressValidated      bool
}

// ProtocolTestResult represents test result for a specific protocol
type ProtocolTestResult struct {
	Tested      bool
	Success     bool
	ResponseMs  uint32
	Error       string
	
	// Protocol-specific details
	Details map[string]interface{}
}

// BinkPTestDetails contains BinkP-specific test details
type BinkPTestDetails struct {
	SystemName   string
	Sysop        string
	Location     string
	Version      string
	Addresses    []string
	Capabilities []string
}

// IfcicoTestDetails contains IFCICO-specific test details
type IfcicoTestDetails struct {
	MailerInfo   string
	SystemName   string
	Addresses    []string
	ResponseType string // REQ/ACK/NAK/CLI/HBT
}

// DNSResult represents DNS resolution result
type DNSResult struct {
	Hostname     string
	IPv4Addresses []string
	IPv6Addresses []string
	Error        error
	ResolutionMs int64
}

// GeolocationResult represents IP geolocation data
type GeolocationResult struct {
	IP          string
	Country     string
	CountryCode string
	City        string
	Region      string
	Latitude    float32
	Longitude   float32
	ISP         string
	Org         string
	ASN         uint32
	Timezone    string
	Source      string // Provider name
}

// TestStatistics represents aggregated test statistics
type TestStatistics struct {
	Date                time.Time
	TotalNodesTested    uint32
	NodesWithBinkP      uint32
	NodesWithIfcico     uint32
	NodesOperational    uint32
	NodesWithIssues     uint32
	NodesDNSFailed      uint32
	AvgBinkPResponseMs  float32
	AvgIfcicoResponseMs float32
	Countries           map[string]uint32
	ISPs                map[string]uint32
	ProtocolStats       map[string]uint32
	ErrorTypes          map[string]uint32
}

// NewTestResult creates a new test result for a node
func NewTestResult(node *Node) *TestResult {
	return &TestResult{
		TestTime: time.Now(),
		TestDate: time.Now().Truncate(24 * time.Hour),
		Zone:     node.Zone,
		Net:      node.Net,
		Node:     node.Node,
		Address:  node.Address(),
		Hostname: node.GetPrimaryHostname(),
	}
}

// SetBinkPResult sets BinkP test result with details
func (tr *TestResult) SetBinkPResult(success bool, responseMs uint32, details *BinkPTestDetails, err string) {
	tr.BinkPResult = &ProtocolTestResult{
		Tested:     true,
		Success:    success,
		ResponseMs: responseMs,
		Error:      err,
		Details:    make(map[string]interface{}),
	}
	
	if details != nil {
		tr.BinkPResult.Details["system_name"] = details.SystemName
		tr.BinkPResult.Details["sysop"] = details.Sysop
		tr.BinkPResult.Details["location"] = details.Location
		tr.BinkPResult.Details["version"] = details.Version
		tr.BinkPResult.Details["addresses"] = details.Addresses
		tr.BinkPResult.Details["capabilities"] = details.Capabilities
		
		// Address validation is now handled by the BinkP tester which sets AddressValid flag
	}
	
	if success {
		tr.IsOperational = true
	}
}

// SetIfcicoResult sets IFCICO test result with details
func (tr *TestResult) SetIfcicoResult(success bool, responseMs uint32, details *IfcicoTestDetails, err string) {
	tr.IfcicoResult = &ProtocolTestResult{
		Tested:     true,
		Success:    success,
		ResponseMs: responseMs,
		Error:      err,
		Details:    make(map[string]interface{}),
	}
	
	if details != nil {
		tr.IfcicoResult.Details["mailer_info"] = details.MailerInfo
		tr.IfcicoResult.Details["system_name"] = details.SystemName
		tr.IfcicoResult.Details["addresses"] = details.Addresses
		tr.IfcicoResult.Details["response_type"] = details.ResponseType
	}
	
	if success && !tr.IsOperational {
		tr.IsOperational = true
	}
}

// SetDNSResult sets DNS resolution results
func (tr *TestResult) SetDNSResult(result *DNSResult) {
	if result == nil {
		return
	}
	
	tr.ResolvedIPv4 = result.IPv4Addresses
	tr.ResolvedIPv6 = result.IPv6Addresses
	
	if result.Error != nil {
		tr.DNSError = result.Error.Error()
		tr.HasConnectivityIssues = true
	}
}

// SetGeolocation sets geolocation data
func (tr *TestResult) SetGeolocation(geo *GeolocationResult) {
	if geo == nil {
		return
	}
	
	tr.Country = geo.Country
	tr.CountryCode = geo.CountryCode
	tr.City = geo.City
	tr.Region = geo.Region
	tr.Latitude = geo.Latitude
	tr.Longitude = geo.Longitude
	tr.ISP = geo.ISP
	tr.Org = geo.Org
	tr.ASN = geo.ASN
}

// SetTelnetResult sets Telnet test result
func (tr *TestResult) SetTelnetResult(success bool, responseMs uint32, banner string, err string) {
	tr.TelnetResult = &ProtocolTestResult{
		Tested:     true,
		Success:    success,
		ResponseMs: responseMs,
		Error:      err,
		Details:    make(map[string]interface{}),
	}
	
	if banner != "" {
		tr.TelnetResult.Details["banner"] = banner
	}
	
	if success && !tr.IsOperational {
		tr.IsOperational = true
	}
}

// SetFTPResult sets FTP test result
func (tr *TestResult) SetFTPResult(success bool, responseMs uint32, welcome string, err string) {
	tr.FTPResult = &ProtocolTestResult{
		Tested:     true,
		Success:    success,
		ResponseMs: responseMs,
		Error:      err,
		Details:    make(map[string]interface{}),
	}
	
	if welcome != "" {
		tr.FTPResult.Details["welcome"] = welcome
	}
	
	if success && !tr.IsOperational {
		tr.IsOperational = true
	}
}

// SetVModemResult sets VModem test result
func (tr *TestResult) SetVModemResult(success bool, responseMs uint32, err string) {
	tr.VModemResult = &ProtocolTestResult{
		Tested:     true,
		Success:    success,
		ResponseMs: responseMs,
		Error:      err,
		Details:    make(map[string]interface{}),
	}
	
	if success && !tr.IsOperational {
		tr.IsOperational = true
	}
}