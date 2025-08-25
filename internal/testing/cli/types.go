package cli

import (
	"time"
)

type TestOptions struct {
	Protocols []string
	Timeout   time.Duration
	Verbose   bool
	Port      int
}

type TestResult struct {
	TestID       string
	Address      string
	Hostname     string
	StartTime    time.Time
	Duration     time.Duration
	ResolvedIPs  []string
	ExpectedAddress string
	
	Geolocation *GeolocationInfo
	
	BinkPResult  *ProtocolResult
	IFCICOResult *ProtocolResult
	TelnetResult *ProtocolResult
	FTPResult    *ProtocolResult
	VModemResult *ProtocolResult
	
	IsOperational         bool
	HasConnectivityIssues bool
	AddressValidated      bool
}

type ProtocolResult struct {
	Tested       bool
	Success      bool
	ResponseTime int
	Port         int
	Error        string
	
	SystemName   string
	Sysop        string
	Location     string
	Version      string
	Addresses    []string
	Capabilities []string
}

type GeolocationInfo struct {
	Country     string
	CountryCode string
	City        string
	Region      string
	ISP         string
	ASN         string
	Latitude    float64
	Longitude   float64
}

type DaemonStatus struct {
	Uptime         time.Duration
	TestsCompleted int
	SuccessRate    float64
	ActiveWorkers  int
	QueueSize      int
	Status         string
	NextCycle      time.Time
}

type WorkerStatus struct {
	TotalWorkers int
	Active       int
	Idle         int
	QueueLength  int
	CurrentTasks []TaskInfo
}

type TaskInfo struct {
	Node      string
	StartTime time.Time
	Protocol  string
}

type TestOutput struct {
	Type    string
	Message string
	Time    time.Time
}

type NodeInfo struct {
	Address           string
	SystemName        string
	SysopName         string
	Location          string
	NodeType          string
	HasInternet       bool
	InternetHostnames []string
	InternetProtocols []string
	Flags             []string
	ModemFlags        []string
	LastSeen          time.Time
	Found             bool
	ErrorMessage      string
}