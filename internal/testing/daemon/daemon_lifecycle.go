package daemon

import (
	"fmt"

	"github.com/nodelistdb/internal/testing/logging"
	"github.com/nodelistdb/internal/testing/protocols"
	"github.com/nodelistdb/internal/testing/services"
	"github.com/nodelistdb/internal/testing/storage"
)

// ReloadConfig reloads the configuration from file
func (d *Daemon) ReloadConfig(configPath string) error {
	// Load new configuration
	newCfg, err := LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Validate new configuration
	if err := newCfg.Validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	// Update configuration (only safe fields)
	// Note: We don't change database connections or worker pool size
	d.config.Daemon.TestInterval = newCfg.Daemon.TestInterval
	d.config.Daemon.BatchSize = newCfg.Daemon.BatchSize
	d.config.Daemon.DryRun = newCfg.Daemon.DryRun

	// Update protocol settings
	d.config.Protocols = newCfg.Protocols

	// Update service settings
	d.config.Services = newCfg.Services

	// Reinitialize services with new configurations
	d.dnsResolver = services.NewDNSResolverWithTTL(
		newCfg.Services.DNS.Workers,
		newCfg.Services.DNS.Timeout,
		newCfg.Services.DNS.CacheTTL,
	)

	d.geolocator = services.NewGeolocationWithConfig(
		newCfg.Services.Geolocation.Provider,
		newCfg.Services.Geolocation.APIKey,
		newCfg.Services.Geolocation.CacheTTL,
		newCfg.Services.Geolocation.RateLimit,
	)

	// Re-wire persistent cache to new service instances if available
	if d.persistentCache != nil {
		dnsCache := storage.NewDNSCache(d.persistentCache)
		d.dnsResolver.SetPersistentCache(dnsCache)

		geoCache := storage.NewGeolocationCache(d.persistentCache)
		d.geolocator.SetPersistentCache(geoCache)
	}

	// Reinitialize protocol testers with new timeouts and settings
	if newCfg.Protocols.BinkP.Enabled {
		d.binkpTester = protocols.NewBinkPTesterWithInfo(
			newCfg.Protocols.BinkP.Timeout,
			newCfg.Protocols.BinkP.OurAddress,
			newCfg.Protocols.BinkP.SystemName,
			newCfg.Protocols.BinkP.Sysop,
			newCfg.Protocols.BinkP.Location,
		)
	} else {
		d.binkpTester = nil
	}

	if newCfg.Protocols.Ifcico.Enabled {
		d.ifcicoTester = protocols.NewIfcicoTesterWithInfo(
			newCfg.Protocols.Ifcico.Timeout,
			newCfg.Protocols.Ifcico.OurAddress,
			newCfg.Protocols.Ifcico.SystemName,
			newCfg.Protocols.Ifcico.Sysop,
			newCfg.Protocols.Ifcico.Location,
		)
	} else {
		d.ifcicoTester = nil
	}

	if newCfg.Protocols.Telnet.Enabled {
		d.telnetTester = protocols.NewTelnetTester(
			newCfg.Protocols.Telnet.Timeout,
		)
	} else {
		d.telnetTester = nil
	}

	if newCfg.Protocols.FTP.Enabled {
		d.ftpTester = protocols.NewFTPTester(
			newCfg.Protocols.FTP.Timeout,
		)
	} else {
		d.ftpTester = nil
	}

	if newCfg.Protocols.VModem.Enabled {
		d.vmodemTester = protocols.NewVModemTester(
			newCfg.Protocols.VModem.Timeout,
		)
	} else {
		d.vmodemTester = nil
	}

	// Update scheduler with new intervals
	if d.scheduler != nil {
		d.scheduler.mu.Lock()
		d.scheduler.baseInterval = newCfg.Daemon.TestInterval
		d.scheduler.failedRetryInterval = newCfg.Daemon.FailedRetryInterval
		d.scheduler.staleTestThreshold = newCfg.Daemon.StaleTestThreshold
		d.scheduler.mu.Unlock()
	}

	// Reload logging configuration
	logConfig := &logging.Config{
		Level:      newCfg.Logging.Level,
		File:       newCfg.Logging.File,
		MaxSize:    newCfg.Logging.MaxSize,
		MaxBackups: newCfg.Logging.MaxBackups,
		MaxAge:     newCfg.Logging.MaxAge,
		Console:    true,
	}
	if err := logging.GetLogger().Reload(logConfig); err != nil {
		logging.Errorf("Failed to reload logging configuration: %v", err)
	}

	logging.Info("Configuration reloaded successfully")
	return nil
}

// Pause pauses the daemon's automatic testing
func (d *Daemon) Pause() error {
	d.pauseMu.Lock()
	defer d.pauseMu.Unlock()

	if d.paused {
		return fmt.Errorf("daemon is already paused")
	}

	d.paused = true
	logging.Info("Daemon paused")
	return nil
}

// Resume resumes the daemon's automatic testing
func (d *Daemon) Resume() error {
	d.pauseMu.Lock()
	defer d.pauseMu.Unlock()

	if !d.paused {
		return fmt.Errorf("daemon is not paused")
	}

	d.paused = false
	logging.Info("Daemon resumed")
	return nil
}

// IsPaused returns whether the daemon is paused
func (d *Daemon) IsPaused() bool {
	d.pauseMu.RLock()
	defer d.pauseMu.RUnlock()
	return d.paused
}

// SetDebugMode enables or disables debug mode for protocol testers
func (d *Daemon) SetDebugMode(enabled bool) error {
	d.debugMu.Lock()
	defer d.debugMu.Unlock()

	d.debug = enabled

	// Update all protocol testers if they support debug mode
	if d.binkpTester != nil {
		if setter, ok := d.binkpTester.(protocols.DebugSetter); ok {
			setter.SetDebug(enabled)
		}
	}
	if d.ifcicoTester != nil {
		if setter, ok := d.ifcicoTester.(protocols.DebugSetter); ok {
			setter.SetDebug(enabled)
		}
	}
	if d.telnetTester != nil {
		if setter, ok := d.telnetTester.(protocols.DebugSetter); ok {
			setter.SetDebug(enabled)
		}
	}
	if d.ftpTester != nil {
		if setter, ok := d.ftpTester.(protocols.DebugSetter); ok {
			setter.SetDebug(enabled)
		}
	}
	if d.vmodemTester != nil {
		if setter, ok := d.vmodemTester.(protocols.DebugSetter); ok {
			setter.SetDebug(enabled)
		}
	}

	if enabled {
		logging.Info("Debug mode enabled for protocol testers")
	} else {
		logging.Info("Debug mode disabled for protocol testers")
	}

	return nil
}

// GetDebugMode returns the current debug mode status
func (d *Daemon) GetDebugMode() bool {
	d.debugMu.RLock()
	defer d.debugMu.RUnlock()
	return d.debug
}
