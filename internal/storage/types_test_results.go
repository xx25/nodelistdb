package storage

import "time"

// Shapes returned by the reports over node_test_results - the test daemon's
// log of what it probed. NodeTestResult alone is 143 lines and is scanned
// positionally; the column list it must match lives in
// test_result_columns.go.

// NodeTestResult represents a test result for a node
type NodeTestResult struct {
	TestTime     time.Time `json:"test_time"`
	Zone         int       `json:"zone"`
	Net          int       `json:"net"`
	Node         int       `json:"node"`
	Address      string    `json:"address"`
	Hostname     string    `json:"hostname"`
	ResolvedIPv4 []string  `json:"resolved_ipv4"`
	ResolvedIPv6 []string  `json:"resolved_ipv6"`
	DNSError     string    `json:"dns_error"`

	// Geolocation
	Country     string  `json:"country"`
	CountryCode string  `json:"country_code"`
	City        string  `json:"city"`
	Region      string  `json:"region"`
	Latitude    float32 `json:"latitude"`
	Longitude   float32 `json:"longitude"`
	ISP         string  `json:"isp"`
	Org         string  `json:"org"`
	ASN         uint32  `json:"asn"`

	// BinkP Test Results
	BinkPTested       bool     `json:"binkp_tested"`
	BinkPSuccess      bool     `json:"binkp_success"`
	BinkPResponseMs   uint32   `json:"binkp_response_ms"`
	BinkPSystemName   string   `json:"binkp_system_name"`
	BinkPSysop        string   `json:"binkp_sysop"`
	BinkPLocation     string   `json:"binkp_location"`
	BinkPVersion      string   `json:"binkp_version"`
	BinkPAddresses    []string `json:"binkp_addresses"`
	BinkPCapabilities []string `json:"binkp_capabilities"`
	BinkPError        string   `json:"binkp_error"`

	// IFCICO Test Results
	IfcicoTested       bool     `json:"ifcico_tested"`
	IfcicoSuccess      bool     `json:"ifcico_success"`
	IfcicoResponseMs   uint32   `json:"ifcico_response_ms"`
	IfcicoMailerInfo   string   `json:"ifcico_mailer_info"`
	IfcicoSystemName   string   `json:"ifcico_system_name"`
	IfcicoAddresses    []string `json:"ifcico_addresses"`
	IfcicoResponseType string   `json:"ifcico_response_type"`
	IfcicoError        string   `json:"ifcico_error"`

	// Telnet Test Results
	TelnetTested     bool   `json:"telnet_tested"`
	TelnetSuccess    bool   `json:"telnet_success"`
	TelnetResponseMs uint32 `json:"telnet_response_ms"`
	TelnetError      string `json:"telnet_error"`

	// FTP Test Results
	FTPTested      bool   `json:"ftp_tested"`
	FTPSuccess     bool   `json:"ftp_success"`
	FTPResponseMs  uint32 `json:"ftp_response_ms"`
	FTPError       string `json:"ftp_error"`
	FTPAnonSuccess *bool  `json:"ftp_anon_success"` // nil=not attempted, true=success, false=rejected

	// VModem Test Results
	VModemTested      bool     `json:"vmodem_tested"`
	VModemSuccess     bool     `json:"vmodem_success"`
	VModemResponseMs  uint32   `json:"vmodem_response_ms"`
	VModemError       string   `json:"vmodem_error"`
	VModemVariant     string   `json:"vmodem_variant"`      // protocol actually observed on the IVM port
	VModemConformant  bool     `json:"vmodem_conformant"`   // true only for a genuine VMODEM (VMP) responder
	VModemSoftware    string   `json:"vmodem_software"`     // detected mailer/software
	VModemSystemName  string   `json:"vmodem_system_name"`  // remote system name (EMSI)
	VModemSysop       string   `json:"vmodem_sysop"`        // remote sysop name (EMSI)
	VModemLocation    string   `json:"vmodem_location"`     // remote location (EMSI)
	VModemAddresses   []string `json:"vmodem_addresses"`    // remote FTN addresses (EMSI)
	VModemDetail      string   `json:"vmodem_detail"`       // human-readable note behind the variant (how a VMP call ended)
	VModemCallOutcome string   `json:"vmodem_call_outcome"` // groupable outcome of a placed VMP call; "" when none was placed
	VModemBanner      string   `json:"vmodem_banner"`       // raw greeting, for variants identified only by their banner

	// IPv4-specific Test Results
	BinkPIPv4Tested      bool   `json:"binkp_ipv4_tested"`
	BinkPIPv4Success     bool   `json:"binkp_ipv4_success"`
	BinkPIPv4ResponseMs  uint32 `json:"binkp_ipv4_response_ms"`
	BinkPIPv4Address     string `json:"binkp_ipv4_address"`
	BinkPIPv4Error       string `json:"binkp_ipv4_error"`
	IfcicoIPv4Tested     bool   `json:"ifcico_ipv4_tested"`
	IfcicoIPv4Success    bool   `json:"ifcico_ipv4_success"`
	IfcicoIPv4ResponseMs uint32 `json:"ifcico_ipv4_response_ms"`
	IfcicoIPv4Address    string `json:"ifcico_ipv4_address"`
	IfcicoIPv4Error      string `json:"ifcico_ipv4_error"`
	TelnetIPv4Tested     bool   `json:"telnet_ipv4_tested"`
	TelnetIPv4Success    bool   `json:"telnet_ipv4_success"`
	TelnetIPv4ResponseMs uint32 `json:"telnet_ipv4_response_ms"`
	TelnetIPv4Address    string `json:"telnet_ipv4_address"`
	TelnetIPv4Error      string `json:"telnet_ipv4_error"`
	FTPIPv4Tested        bool   `json:"ftp_ipv4_tested"`
	FTPIPv4Success       bool   `json:"ftp_ipv4_success"`
	FTPIPv4ResponseMs    uint32 `json:"ftp_ipv4_response_ms"`
	FTPIPv4Address       string `json:"ftp_ipv4_address"`
	FTPIPv4Error         string `json:"ftp_ipv4_error"`
	VModemIPv4Tested     bool   `json:"vmodem_ipv4_tested"`
	VModemIPv4Success    bool   `json:"vmodem_ipv4_success"`
	VModemIPv4ResponseMs uint32 `json:"vmodem_ipv4_response_ms"`
	VModemIPv4Address    string `json:"vmodem_ipv4_address"`
	VModemIPv4Error      string `json:"vmodem_ipv4_error"`

	// IPv6-specific Test Results
	BinkPIPv6Tested      bool   `json:"binkp_ipv6_tested"`
	BinkPIPv6Success     bool   `json:"binkp_ipv6_success"`
	BinkPIPv6ResponseMs  uint32 `json:"binkp_ipv6_response_ms"`
	BinkPIPv6Address     string `json:"binkp_ipv6_address"`
	BinkPIPv6Error       string `json:"binkp_ipv6_error"`
	IfcicoIPv6Tested     bool   `json:"ifcico_ipv6_tested"`
	IfcicoIPv6Success    bool   `json:"ifcico_ipv6_success"`
	IfcicoIPv6ResponseMs uint32 `json:"ifcico_ipv6_response_ms"`
	IfcicoIPv6Address    string `json:"ifcico_ipv6_address"`
	IfcicoIPv6Error      string `json:"ifcico_ipv6_error"`
	TelnetIPv6Tested     bool   `json:"telnet_ipv6_tested"`
	TelnetIPv6Success    bool   `json:"telnet_ipv6_success"`
	TelnetIPv6ResponseMs uint32 `json:"telnet_ipv6_response_ms"`
	TelnetIPv6Address    string `json:"telnet_ipv6_address"`
	TelnetIPv6Error      string `json:"telnet_ipv6_error"`
	FTPIPv6Tested        bool   `json:"ftp_ipv6_tested"`
	FTPIPv6Success       bool   `json:"ftp_ipv6_success"`
	FTPIPv6ResponseMs    uint32 `json:"ftp_ipv6_response_ms"`
	FTPIPv6Address       string `json:"ftp_ipv6_address"`
	FTPIPv6Error         string `json:"ftp_ipv6_error"`
	VModemIPv6Tested     bool   `json:"vmodem_ipv6_tested"`
	VModemIPv6Success    bool   `json:"vmodem_ipv6_success"`
	VModemIPv6ResponseMs uint32 `json:"vmodem_ipv6_response_ms"`
	VModemIPv6Address    string `json:"vmodem_ipv6_address"`
	VModemIPv6Error      string `json:"vmodem_ipv6_error"`

	IsOperational         bool `json:"is_operational"`
	HasConnectivityIssues bool `json:"has_connectivity_issues"`
	AddressValidated      bool `json:"address_validated"`

	// Multi-network identity and AKA-derivation provenance
	Domain             string `json:"domain,omitempty"`               // FTN network of the tested identity
	DerivedFromAddress string `json:"derived_from_address,omitempty"` // non-empty: result derived from this node's direct test

	// Per-hostname testing fields (simplified migration)
	TestedHostname       string   `json:"tested_hostname"`       // Which hostname was tested
	HostnameIndex        int32    `json:"hostname_index"`        // -1=legacy, 0=primary, 1+=backup
	IsAggregated         bool     `json:"is_aggregated"`         // false=per-hostname, true=summary
	TotalHostnames       int32    `json:"total_hostnames"`       // Total number of hostnames for this node
	HostnamesTested      int32    `json:"hostnames_tested"`      // Number of hostnames actually tested
	HostnamesOperational int32    `json:"hostnames_operational"` // Number of operational hostnames
	AllHostnames         []string `json:"all_hostnames"`         // All hostnames for this node (for display)
}

// IsConfirmedVMODEM reports whether the announced IVM port was confirmed to run
// a genuine VMODEM (Gwinn VMP) responder.
//
// This is deliberately narrower than VModemSuccess, which is true for anything
// recognizable on the port — an EMSI mailer over telnet or raw TCP, binkd, even
// a bare telnet login prompt. Reaching one of those proves the port is alive but
// says nothing about VMODEM, so anywhere a surface claims to be about VMODEM it
// should ask this instead. It is also false for rows written before the tester
// classified variants, whose bare success carries no evidence at all.
func (r *NodeTestResult) IsConfirmedVMODEM() bool {
	return r != nil && r.VModemConformant && r.VModemVariant == "vmp"
}

// NodeReachabilityStats represents aggregated reachability statistics for a node
type NodeReachabilityStats struct {
	Zone                  int       `json:"zone"`
	Net                   int       `json:"net"`
	Node                  int       `json:"node"`
	TotalTests            int       `json:"total_tests"`
	FullySuccessfulTests  int       `json:"fully_successful_tests"`
	PartiallyFailedTests  int       `json:"partially_failed_tests"`
	FailedTests           int       `json:"failed_tests"`
	SuccessfulTests       int       `json:"successful_tests"` // For backward compatibility (operational)
	SuccessRate           float64   `json:"success_rate"`
	AverageResponseMs     float64   `json:"average_response_ms"`
	LastTestTime          time.Time `json:"last_test_time"`
	LastStatus            string    `json:"last_status"`
	BinkPSuccessRate      float64   `json:"binkp_success_rate"`       // Combined (IPv4 OR IPv6)
	IfcicoSuccessRate     float64   `json:"ifcico_success_rate"`      // Combined (IPv4 OR IPv6)
	TelnetSuccessRate     float64   `json:"telnet_success_rate"`      // Combined (IPv4 OR IPv6)
	BinkPIPv4SuccessRate  float64   `json:"binkp_ipv4_success_rate"`  // IPv4-only
	IfcicoIPv4SuccessRate float64   `json:"ifcico_ipv4_success_rate"` // IPv4-only
	TelnetIPv4SuccessRate float64   `json:"telnet_ipv4_success_rate"` // IPv4-only
	BinkPIPv6SuccessRate  float64   `json:"binkp_ipv6_success_rate"`  // IPv6-only
	IfcicoIPv6SuccessRate float64   `json:"ifcico_ipv6_success_rate"` // IPv6-only
	TelnetIPv6SuccessRate float64   `json:"telnet_ipv6_success_rate"` // IPv6-only
}

// ReachabilityTrend represents reachability trend over time
type ReachabilityTrend struct {
	Date             time.Time `json:"date"`
	TotalNodes       int       `json:"total_nodes"`
	OperationalNodes int       `json:"operational_nodes"`
	FailedNodes      int       `json:"failed_nodes"`
	SuccessRate      float64   `json:"success_rate"`
	AvgResponseMs    float64   `json:"avg_response_ms"`
}
