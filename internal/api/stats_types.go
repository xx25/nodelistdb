package api

// The bodies of the operational endpoints cmd/server registers on this
// router. They live here rather than in package main so the schema test can
// hold them against openapi.yaml: main cannot be imported, and it imports
// this package.

// CacheStats is the /api/cache/stats body.
type CacheStats struct {
	Hits    uint64  `json:"hits"`
	Misses  uint64  `json:"misses"`
	Sets    uint64  `json:"sets"`
	Deletes uint64  `json:"deletes"`
	Size    uint64  `json:"size"`
	Keys    uint64  `json:"keys"`
	HitRate float64 `json:"hit_rate"`
}

// FTPStats is the /api/ftp/stats body.
type FTPStats struct {
	Enabled        bool   `json:"enabled"`
	Host           string `json:"host"`
	Port           int    `json:"port"`
	MaxConnections int    `json:"max_connections"`
}
