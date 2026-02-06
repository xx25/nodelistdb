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
	Hostname     string   // Primary hostname attempted
	TestedHostname string // Actual hostname that was tested (may be primary or backup)
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
	AddressValidatedIPv4  bool
	AddressValidatedIPv6  bool

	// IP version restrictions (FTS-1038)
	IPv4Skipped           bool   // IPv4 testing skipped due to INO4 flag

	// Per-hostname testing fields (simplified migration)
	HostnameIndex         int32  // -1=legacy, 0=primary, 1+=backup
	IsAggregated          bool   // false=per-hostname, true=summary
	TotalHostnames        int32  // Total number of hostnames for this node
	HostnamesTested       int32  // Number of hostnames actually tested
	HostnamesOperational  int32  // Number of operational hostnames
}

// ProtocolTestResult represents test result for a specific protocol
type ProtocolTestResult struct {
	// Overall results (backward compatible)
	Tested      bool
	Success     bool   // Success on ANY IP version
	ResponseMs  uint32 // Best response time from any IP version
	Error       string

	// IPv4 specific results
	IPv4Tested     bool
	IPv4Success    bool
	IPv4ResponseMs uint32
	IPv4Error      string
	IPv4Address    string // Which IPv4 address was used/succeeded

	// IPv6 specific results
	IPv6Tested     bool
	IPv6Success    bool
	IPv6ResponseMs uint32
	IPv6Error      string
	IPv6Address    string // Which IPv6 address was used/succeeded

	// Protocol-specific details
	Details map[string]interface{}

	// SoftwareSource indicates where software info came from
	// Values: "emsi_dat", "banner", ""
	SoftwareSource string
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

// WhoisResult holds the parsed WHOIS result for a domain
type WhoisResult struct {
	Domain         string
	ExpirationDate *time.Time
	CreationDate   *time.Time
	Registrar      string
	Status         string
	Error          string
	LookupTimeMs   int64
	Cached         bool
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

// HostnameTestResult represents test results for a specific hostname
type HostnameTestResult struct {
	Hostname      string
	DNSResolved   bool
	DNSError      string
	IPv4Addresses []string
	IPv6Addresses []string
	TestTime      time.Time

	// Protocol results for this specific hostname
	BinkPResult  *ProtocolTestResult
	IfcicoResult *ProtocolTestResult
	TelnetResult *ProtocolTestResult
	FTPResult    *ProtocolTestResult
	VModemResult *ProtocolTestResult

	// Summary for this hostname
	IsOperational bool
	ResponseMs    uint32 // Best response time from any protocol
}

// AggregatedTestResult represents test results aggregated across all hostnames
type AggregatedTestResult struct {
	// Node identification
	Zone     int
	Net      int
	Node     int
	Address  string
	TestTime time.Time

	// Per-hostname results
	HostnameResults map[string]*HostnameTestResult

	// Aggregate summary
	AnyHostnameOperational bool   // At least one hostname works
	AllHostnamesTested     bool   // All available hostnames were tested
	PrimaryHostname        string // The primary hostname configured
	WorkingHostnames       []string // List of operational hostnames
	FailedHostnames        []string // List of non-operational hostnames

	// Best results across all hostnames
	BestResponseMs uint32
	BestHostname   string // Which hostname had the best response
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
		// Initialize per-hostname fields with legacy values
		HostnameIndex:        -1,  // Mark as legacy by default
		IsAggregated:         true, // Legacy results are aggregated
		TotalHostnames:       1,
		HostnamesTested:      1,
		HostnamesOperational: 0,
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
		// Don't set HasConnectivityIssues here - DNS issues are separate from connectivity issues
		// HasConnectivityIssues will be set if protocols fail to connect
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

// SetIPv4Result updates IPv4-specific test results for a protocol
func (pr *ProtocolTestResult) SetIPv4Result(success bool, responseMs uint32, address string, err string) {
	pr.IPv4Tested = true
	pr.IPv4Success = success
	pr.IPv4ResponseMs = responseMs
	pr.IPv4Address = address
	pr.IPv4Error = err
	
	// Update overall success if IPv4 succeeded
	if success {
		pr.Success = true
		pr.Tested = true
		// Update overall response time to best (lowest) time
		if pr.ResponseMs == 0 || responseMs < pr.ResponseMs {
			pr.ResponseMs = responseMs
		}
	}
}

// SetIPv6Result updates IPv6-specific test results for a protocol
func (pr *ProtocolTestResult) SetIPv6Result(success bool, responseMs uint32, address string, err string) {
	pr.IPv6Tested = true
	pr.IPv6Success = success
	pr.IPv6ResponseMs = responseMs
	pr.IPv6Address = address
	pr.IPv6Error = err
	
	// Update overall success if IPv6 succeeded
	if success {
		pr.Success = true
		pr.Tested = true
		// Update overall response time to best (lowest) time
		if pr.ResponseMs == 0 || responseMs < pr.ResponseMs {
			pr.ResponseMs = responseMs
		}
	}
}

// GetConnectivityType returns a string describing the connectivity type
func (pr *ProtocolTestResult) GetConnectivityType() string {
	if pr.IPv4Success && pr.IPv6Success {
		return "dual-stack"
	} else if pr.IPv6Success {
		return "ipv6-only"
	} else if pr.IPv4Success {
		return "ipv4-only"
	} else if pr.IPv4Tested || pr.IPv6Tested {
		return "failed"
	}
	return "not-tested"
}

// NewAggregatedTestResult creates a new aggregated test result for a node
func NewAggregatedTestResult(node *Node) *AggregatedTestResult {
	return &AggregatedTestResult{
		Zone:            node.Zone,
		Net:             node.Net,
		Node:            node.Node,
		Address:         node.Address(),
		TestTime:        time.Now(),
		HostnameResults: make(map[string]*HostnameTestResult),
		PrimaryHostname: node.GetPrimaryHostname(),
	}
}

// AddHostnameResult adds test result for a specific hostname
func (atr *AggregatedTestResult) AddHostnameResult(result *HostnameTestResult) {
	if atr.HostnameResults == nil {
		atr.HostnameResults = make(map[string]*HostnameTestResult)
	}

	atr.HostnameResults[result.Hostname] = result

	// Update aggregate fields
	if result.IsOperational {
		atr.AnyHostnameOperational = true
		atr.WorkingHostnames = append(atr.WorkingHostnames, result.Hostname)

		// Track best response time
		if atr.BestResponseMs == 0 || result.ResponseMs < atr.BestResponseMs {
			atr.BestResponseMs = result.ResponseMs
			atr.BestHostname = result.Hostname
		}
	} else {
		atr.FailedHostnames = append(atr.FailedHostnames, result.Hostname)
	}
}

// GetPrimaryResult returns the test result for the primary hostname
func (atr *AggregatedTestResult) GetPrimaryResult() *HostnameTestResult {
	if atr.PrimaryHostname != "" && atr.HostnameResults != nil {
		return atr.HostnameResults[atr.PrimaryHostname]
	}
	return nil
}

// GetSuccessRate returns the percentage of successful hostnames
func (atr *AggregatedTestResult) GetSuccessRate() float64 {
	if len(atr.HostnameResults) == 0 {
		return 0
	}
	return float64(len(atr.WorkingHostnames)) / float64(len(atr.HostnameResults)) * 100
}