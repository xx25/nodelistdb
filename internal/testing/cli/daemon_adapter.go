package cli

import (
	"context"
	"fmt"
	"time"
	
	"github.com/google/uuid"
	"github.com/nodelistdb/internal/testing/models"
)

type DaemonAdapter struct {
	testNode     func(ctx context.Context, node *models.Node, hostname string) (*models.TestResult, error)
	getStatus    func() DaemonStatus
	getWorkers   func() WorkerStatus
	pause        func() error
	resume       func() error
	reloadConfig func() error
}

func NewDaemonAdapter() *DaemonAdapter {
	return &DaemonAdapter{}
}

func (a *DaemonAdapter) SetTestNodeFunc(f func(ctx context.Context, node *models.Node, hostname string) (*models.TestResult, error)) {
	a.testNode = f
}

func (a *DaemonAdapter) SetGetStatusFunc(f func() DaemonStatus) {
	a.getStatus = f
}

func (a *DaemonAdapter) SetGetWorkersFunc(f func() WorkerStatus) {
	a.getWorkers = f
}

func (a *DaemonAdapter) SetPauseFunc(f func() error) {
	a.pause = f
}

func (a *DaemonAdapter) SetResumeFunc(f func() error) {
	a.resume = f
}

func (a *DaemonAdapter) SetReloadConfigFunc(f func() error) {
	a.reloadConfig = f
}

func (a *DaemonAdapter) TestNode(ctx context.Context, zone, net, node uint16, hostname string, options TestOptions) (*TestResult, error) {
	if a.testNode == nil {
		return nil, fmt.Errorf("test node function not configured")
	}
	
	testNode := &models.Node{
		Zone: int(zone),
		Net:  int(net),
		Node: int(node),
	}
	
	if hostname == "" {
		return nil, fmt.Errorf("hostname is required for testing")
	}
	
	modelResult, err := a.testNode(ctx, testNode, hostname)
	if err != nil {
		return nil, err
	}
	
	return a.convertTestResult(modelResult), nil
}

func (a *DaemonAdapter) GetStatus() DaemonStatus {
	if a.getStatus != nil {
		return a.getStatus()
	}
	return DaemonStatus{
		Status: "not configured",
	}
}

func (a *DaemonAdapter) GetWorkerStatus() WorkerStatus {
	if a.getWorkers != nil {
		return a.getWorkers()
	}
	return WorkerStatus{}
}

func (a *DaemonAdapter) Pause() error {
	if a.pause != nil {
		return a.pause()
	}
	return fmt.Errorf("pause function not configured")
}

func (a *DaemonAdapter) Resume() error {
	if a.resume != nil {
		return a.resume()
	}
	return fmt.Errorf("resume function not configured")
}

func (a *DaemonAdapter) ReloadConfig() error {
	if a.reloadConfig != nil {
		return a.reloadConfig()
	}
	return fmt.Errorf("reload config function not configured")
}

func (a *DaemonAdapter) convertTestResult(r *models.TestResult) *TestResult {
	if r == nil {
		return nil
	}
	
	result := &TestResult{
		TestID:                uuid.New().String(),
		Address:               r.Address,
		Hostname:              r.Hostname,
		StartTime:             r.TestTime,
		Duration:              time.Since(r.TestTime),
		ResolvedIPs:           append(r.ResolvedIPv4, r.ResolvedIPv6...),
		IsOperational:         r.IsOperational,
		HasConnectivityIssues: r.HasConnectivityIssues,
		AddressValidated:      r.AddressValidated,
	}
	
	if r.Country != "" {
		result.Geolocation = &GeolocationInfo{
			Country:     r.Country,
			CountryCode: r.CountryCode,
			City:        r.City,
			Region:      r.Region,
			ISP:         r.ISP,
			ASN:         fmt.Sprintf("AS%d", r.ASN),
			Latitude:    float64(r.Latitude),
			Longitude:   float64(r.Longitude),
		}
	}
	
	if r.BinkPResult != nil && r.BinkPResult.Tested {
		result.BinkPResult = &ProtocolResult{
			Tested:       true,
			Success:      r.BinkPResult.Success,
			ResponseTime: int(r.BinkPResult.ResponseMs),
			Error:        r.BinkPResult.Error,
		}
		
		// Extract BinkP details if present
		if r.BinkPResult.Details != nil {
			if sysName, ok := r.BinkPResult.Details["system_name"].(string); ok {
				result.BinkPResult.SystemName = sysName
			}
			if sysop, ok := r.BinkPResult.Details["sysop"].(string); ok {
				result.BinkPResult.Sysop = sysop
			}
			if location, ok := r.BinkPResult.Details["location"].(string); ok {
				result.BinkPResult.Location = location
			}
			if version, ok := r.BinkPResult.Details["version"].(string); ok {
				result.BinkPResult.Version = version
			}
			if addresses, ok := r.BinkPResult.Details["addresses"].([]string); ok {
				result.BinkPResult.Addresses = addresses
			}
			if capabilities, ok := r.BinkPResult.Details["capabilities"].([]string); ok {
				result.BinkPResult.Capabilities = capabilities
			}
		}
	}
	
	if r.IfcicoResult != nil && r.IfcicoResult.Tested {
		result.IFCICOResult = &ProtocolResult{
			Tested:       true,
			Success:      r.IfcicoResult.Success,
			ResponseTime: int(r.IfcicoResult.ResponseMs),
			Error:        r.IfcicoResult.Error,
		}
		
		// Extract IFCICO details if present
		if r.IfcicoResult.Details != nil {
			if mailerInfo, ok := r.IfcicoResult.Details["mailer_info"].(string); ok {
				result.IFCICOResult.Version = mailerInfo
			}
		}
	}
	
	if r.TelnetResult != nil && r.TelnetResult.Tested {
		result.TelnetResult = &ProtocolResult{
			Tested:       true,
			Success:      r.TelnetResult.Success,
			ResponseTime: int(r.TelnetResult.ResponseMs),
			Error:        r.TelnetResult.Error,
		}
	}
	
	return result
}

type RealDaemon interface {
	TestNodeDirect(ctx context.Context, node *models.Node, hostname string) (*models.TestResult, error)
	GetStatistics() (int, int, int, time.Time)
	IsPaused() bool
	SetPaused(paused bool)
	GetWorkerPoolStatus() (int, int, int)
}