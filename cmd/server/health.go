package main

import (
	"time"

	"github.com/nodelistdb/internal/api"
	"github.com/nodelistdb/internal/cache"
	"github.com/nodelistdb/internal/database"
	"github.com/nodelistdb/internal/ftp"
	"github.com/nodelistdb/internal/storage"
	"github.com/nodelistdb/internal/version"
)

// serverHealthChecker implements api.HealthChecker using concrete server dependencies.
type serverHealthChecker struct {
	db        database.DatabaseInterface
	storage   storage.Operations
	cache     cache.Cache
	ftpServer *ftp.Server
	startTime time.Time
}

func (h *serverHealthChecker) CheckHealth() *api.HealthStatus {
	now := time.Now().UTC()
	uptime := now.Sub(h.startTime)

	status := &api.HealthStatus{
		Status:    "ok",
		Time:      now,
		Uptime:    uptime.Truncate(time.Second).String(),
		UptimeSec: uptime.Seconds(),
		Version: api.VersionInfo{
			Version:   version.Version,
			GitCommit: version.GitCommit,
			BuildTime: version.BuildTime,
		},
	}

	// ClickHouse ping (uses existing 5s context timeout)
	pingStart := time.Now()
	err := h.db.Ping()
	status.Database = api.DatabaseHealth{
		Connected:  err == nil,
		ResponseMs: time.Since(pingStart).Milliseconds(),
	}
	if err != nil {
		status.Database.Error = err.Error()
		status.Status = "degraded"
	}

	// Node count from latest nodelist
	if latestDate, err := h.storage.StatsOps().GetLatestStatsDate(); err == nil {
		if count, err := h.storage.NodeOps().CountNodes(latestDate); err == nil {
			status.Nodes = api.NodeCountInfo{
				LatestDate: latestDate.Format("2006-01-02"),
				Count:      count,
			}
		}
	}

	// Cache stats (if enabled)
	if h.cache != nil {
		metrics := h.cache.GetMetrics()
		status.Cache = &api.CacheHealth{
			Enabled: true,
			Keys:    metrics.Keys,
			HitRate: metrics.HitRate(),
		}
	}

	// FTP stats (if enabled)
	if h.ftpServer != nil {
		ftpStats := h.ftpServer.GetStats()
		port, _ := ftpStats["port"].(int)
		status.FTP = &api.FTPHealth{
			Enabled: true,
			Port:    port,
		}
	}

	return status
}
