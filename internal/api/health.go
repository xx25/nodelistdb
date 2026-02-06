package api

import "time"

// HealthChecker provides health check data to the API server.
type HealthChecker interface {
	CheckHealth() *HealthStatus
}

// HealthStatus is the full health check response.
type HealthStatus struct {
	Status    string         `json:"status"`          // "ok" or "degraded"
	Time      time.Time      `json:"time"`
	Uptime    string         `json:"uptime"`          // human-readable
	UptimeSec float64        `json:"uptime_seconds"`  // machine-readable
	Version   VersionInfo    `json:"version"`
	Database  DatabaseHealth `json:"database"`
	Cache     *CacheHealth   `json:"cache,omitempty"`
	FTP       *FTPHealth     `json:"ftp,omitempty"`
	Nodes     NodeCountInfo  `json:"nodes"`
}

// VersionInfo contains build version details.
type VersionInfo struct {
	Version   string `json:"version"`
	GitCommit string `json:"git_commit"`
	BuildTime string `json:"build_time"`
}

// DatabaseHealth reports ClickHouse connectivity.
type DatabaseHealth struct {
	Connected  bool   `json:"connected"`
	ResponseMs int64  `json:"response_ms"`
	Error      string `json:"error,omitempty"`
}

// CacheHealth reports BadgerDB cache status.
type CacheHealth struct {
	Enabled bool    `json:"enabled"`
	Keys    uint64  `json:"keys"`
	HitRate float64 `json:"hit_rate"`
}

// FTPHealth reports FTP server status.
type FTPHealth struct {
	Enabled bool `json:"enabled"`
	Port    int  `json:"port"`
}

// NodeCountInfo reports nodelist data availability.
type NodeCountInfo struct {
	LatestDate string `json:"latest_date"`
	Count      int    `json:"count"`
}
