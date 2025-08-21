package daemon

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/nodelistdb/internal/testing/cli"
	"github.com/nodelistdb/internal/testing/models"
)

// StartCLIServer starts the CLI telnet server if enabled in config
func (d *Daemon) StartCLIServer(ctx context.Context) error {
	if !d.config.CLI.Enabled {
		return nil
	}
	
	// Create adapter
	adapter := &CLIAdapter{
		daemon:     d,
		configPath: d.config.ConfigPath, // We'll need to add this to config
	}
	
	// Create CLI server config
	cliConfig := cli.Config{
		Host:           d.config.CLI.Host,
		Port:           d.config.CLI.Port,
		MaxClients:     d.config.CLI.MaxClients,
		Timeout:        d.config.CLI.Timeout,
		Prompt:         d.config.CLI.Prompt,
		Welcome:        d.config.CLI.WelcomeMessage,
	}
	
	// Create and start CLI server
	cliServer := cli.NewServer(adapter, cliConfig)
	
	go func() {
		if err := cliServer.Start(ctx); err != nil {
			log.Printf("CLI server error: %v", err)
		}
	}()
	
	return nil
}

// CLIAdapter adapts the daemon to the CLI interface
type CLIAdapter struct {
	daemon     *Daemon
	configPath string
}

func (a *CLIAdapter) TestNode(ctx context.Context, zone, net, node uint16, hostname string, options cli.TestOptions) (*cli.TestResult, error) {
	// Call daemon's test method
	result, err := a.daemon.TestNodeDirect(ctx, zone, net, node, hostname)
	if err != nil {
		return nil, err
	}
	
	// Convert to CLI format
	return a.convertTestResult(result), nil
}

func (a *CLIAdapter) GetStatus() cli.DaemonStatus {
	a.daemon.stats.Lock()
	defer a.daemon.stats.Unlock()
	
	uptime := time.Since(a.daemon.stats.startTime)
	successRate := float64(0)
	if a.daemon.stats.totalTested > 0 {
		successRate = float64(a.daemon.stats.totalSuccesses) / float64(a.daemon.stats.totalTested) * 100
	}
	
	status := "running"
	if a.daemon.config.Daemon.CLIOnly {
		status = "cli-only mode"
	} else if a.daemon.IsPaused() {
		status = "paused"
	}
	
	return cli.DaemonStatus{
		Uptime:         uptime,
		TestsCompleted: a.daemon.stats.totalTested,
		SuccessRate:    successRate,
		ActiveWorkers:  a.daemon.config.Daemon.Workers,
		QueueSize:      0, // Placeholder
		Status:         status,
		NextCycle:      a.daemon.stats.lastCycleTime.Add(a.daemon.config.Daemon.TestInterval),
	}
}

func (a *CLIAdapter) GetWorkerStatus() cli.WorkerStatus {
	active := 0
	idle := a.daemon.config.Daemon.Workers
	queueLength := 0
	
	if a.daemon.workerPool != nil {
		// Get actual status from worker pool
		active = a.daemon.workerPool.GetActiveCount()
		idle = a.daemon.config.Daemon.Workers - active
		queueLength = a.daemon.workerPool.GetQueueSize()
	}
	
	return cli.WorkerStatus{
		TotalWorkers: a.daemon.config.Daemon.Workers,
		Active:       active,
		Idle:         idle,
		QueueLength:  queueLength,
		CurrentTasks: []cli.TaskInfo{},
	}
}

func (a *CLIAdapter) Pause() error {
	return a.daemon.Pause()
}

func (a *CLIAdapter) Resume() error {
	return a.daemon.Resume()
}

func (a *CLIAdapter) ReloadConfig() error {
	if a.configPath == "" {
		return fmt.Errorf("config path not set")
	}
	return a.daemon.ReloadConfig(a.configPath)
}

// SetConfigPath sets the config path for reload
func (a *CLIAdapter) SetConfigPath(path string) {
	a.configPath = path
}

func (a *CLIAdapter) convertTestResult(r *models.TestResult) *cli.TestResult {
	if r == nil {
		return nil
	}
	
	result := &cli.TestResult{
		TestID:                r.Address, // Use address as ID for now
		Address:               r.Address,
		Hostname:              r.Hostname,
		StartTime:             r.TestTime,
		Duration:              time.Since(r.TestTime),
		ResolvedIPs:           append(r.ResolvedIPv4, r.ResolvedIPv6...),
		IsOperational:         r.IsOperational,
		HasConnectivityIssues: r.HasConnectivityIssues,
		AddressValidated:      r.AddressValidated,
	}
	
	// Convert geolocation
	if r.Country != "" {
		result.Geolocation = &cli.GeolocationInfo{
			Country:     r.Country,
			CountryCode: r.CountryCode,
			City:        r.City,
			Region:      r.Region,
			ISP:         r.ISP,
			Latitude:    float64(r.Latitude),
			Longitude:   float64(r.Longitude),
		}
		if r.ASN > 0 {
			result.Geolocation.ASN = fmt.Sprintf("AS%d", r.ASN)
		}
	}
	
	// Convert BinkP results
	if r.BinkPResult != nil && r.BinkPResult.Tested {
		result.BinkPResult = &cli.ProtocolResult{
			Tested:       true,
			Success:      r.BinkPResult.Success,
			ResponseTime: int(r.BinkPResult.ResponseMs),
			Error:        r.BinkPResult.Error,
		}
		
		// Extract BinkP details from map if present
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
	
	// Convert IFCICO results
	if r.IfcicoResult != nil && r.IfcicoResult.Tested {
		result.IFCICOResult = &cli.ProtocolResult{
			Tested:       true,
			Success:      r.IfcicoResult.Success,
			ResponseTime: int(r.IfcicoResult.ResponseMs),
			Error:        r.IfcicoResult.Error,
		}
		
		// Extract IFCICO details from map if present
		if r.IfcicoResult.Details != nil {
			if mailerInfo, ok := r.IfcicoResult.Details["mailer_info"].(string); ok {
				result.IFCICOResult.Version = mailerInfo
			}
		}
	}
	
	// Convert Telnet results
	if r.TelnetResult != nil && r.TelnetResult.Tested {
		result.TelnetResult = &cli.ProtocolResult{
			Tested:       true,
			Success:      r.TelnetResult.Success,
			ResponseTime: int(r.TelnetResult.ResponseMs),
			Error:        r.TelnetResult.Error,
		}
	}
	
	return result
}