package daemon

import (
	"os"
	"path/filepath"
	"testing"
)

// testdaemon_cache used to be half-decorative: daemon.go called
// cache.NewBadgerCache directly and never read the type field, and max_disk_mb
// was not on the struct at all, so yaml dropped it. Production carried
// `type: memory` and `max_disk_mb: 512` while the daemon ran badger with a
// hardcoded 512. These pin the fields as load-bearing.

func loadDaemonConfig(t *testing.T, body string) *Config {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	return cfg
}

const minimalDaemonConfig = `
clickhouse:
  host: localhost
  database: testdb
protocols:
  binkp:
    enabled: true
`

// 512 is what the call site hardcoded before the field existed, so an omitted
// max_disk_mb has to keep meaning 512 rather than 0 ("no cap").
func TestTestdaemonCacheMaxDiskMBDefault(t *testing.T) {
	cfg := loadDaemonConfig(t, minimalDaemonConfig)

	if cfg.TestdaemonCache.MaxDiskMB != 512 {
		t.Errorf("testdaemon_cache.max_disk_mb = %d, want the previously hardcoded 512",
			cfg.TestdaemonCache.MaxDiskMB)
	}
}

func TestTestdaemonCacheMaxDiskMBHonoured(t *testing.T) {
	cfg := loadDaemonConfig(t, minimalDaemonConfig+`
testdaemon_cache:
  enabled: true
  type: badger
  path: /tmp/nodelistdb-test-daemon-cache
  max_disk_mb: 64
`)

	if cfg.TestdaemonCache.MaxDiskMB != 64 {
		t.Errorf("testdaemon_cache.max_disk_mb = %d, want 64 - an explicit value must not be dropped",
			cfg.TestdaemonCache.MaxDiskMB)
	}
	if cfg.TestdaemonCache.Type != "badger" {
		t.Errorf("testdaemon_cache.type = %q, want \"badger\"", cfg.TestdaemonCache.Type)
	}
}

func TestTestdaemonCacheTypeHonoured(t *testing.T) {
	cfg := loadDaemonConfig(t, minimalDaemonConfig+`
testdaemon_cache:
  enabled: true
  type: memory
  path: /tmp/nodelistdb-test-daemon-cache
`)

	if cfg.TestdaemonCache.Type != "memory" {
		t.Errorf("testdaemon_cache.type = %q, want %q", cfg.TestdaemonCache.Type, "memory")
	}
}
