package config

import (
	"strings"
	"testing"
)

// Both fields below share the shape that 0bc780c fixed for the TTLs: validate()
// backfills only when the value is exactly zero, so a negative one passes
// straight through. The TTL sweep in that commit stopped at the four TTL
// fields; gc_interval and max_disk_mb have the same hole, and gc_interval's
// consequence is worse than a stale cache - it reaches time.NewTicker on a
// goroutine with no recover(), which panics on a non-positive interval and
// takes the process with it.
//
// These use the real writeConfig/LoadConfig path rather than calling Validate()
// directly, because the ordering matters: validate() applies defaults first and
// Validate() checks second, and a negative has to survive the first to be
// caught by the second.

func loadCacheConfigErr(t *testing.T, cacheSection string) error {
	t.Helper()
	_, err := LoadConfig(writeConfig(t, baseConfig+cacheSection))
	return err
}

func TestNegativeGCIntervalRejected(t *testing.T) {
	err := loadCacheConfigErr(t, `
cache:
  enabled: true
  type: badger
  path: /tmp/nodelistdb-test-cache
  gc_interval: -10m
`)
	if err == nil {
		t.Fatal("negative cache.gc_interval accepted; it reaches time.NewTicker and panics the process")
	}
	if !strings.Contains(err.Error(), "gc_interval") {
		t.Errorf("error should name the offending field, got: %v", err)
	}
}

func TestNegativeMaxDiskMBRejected(t *testing.T) {
	err := loadCacheConfigErr(t, `
cache:
  enabled: true
  type: badger
  path: /tmp/nodelistdb-test-cache
  max_disk_mb: -512
`)
	if err == nil {
		t.Fatal("negative cache.max_disk_mb accepted; it is read as 'no limit', not as a limit")
	}
	if !strings.Contains(err.Error(), "max_disk_mb") {
		t.Errorf("error should name the offending field, got: %v", err)
	}
}

// The guard must reject negatives without rejecting the values that actually
// occur: an omitted max_disk_mb (0, meaning no cap) and an omitted gc_interval
// (backfilled to 10m before validation sees it). A `< 1` check instead of
// `< 0` would pass both tests above and fail this one.
func TestZeroMaxDiskMBAndOmittedGCIntervalAccepted(t *testing.T) {
	cfg, err := LoadConfig(writeConfig(t, baseConfig+`
cache:
  enabled: true
  type: badger
  path: /tmp/nodelistdb-test-cache
  max_disk_mb: 0
`))
	if err != nil {
		t.Fatalf("max_disk_mb: 0 must be accepted as 'no limit': %v", err)
	}

	if cfg.Cache.MaxDiskMB != 0 {
		t.Errorf("cache.max_disk_mb = %d, want 0 left alone", cfg.Cache.MaxDiskMB)
	}
	if want := DefaultCacheConfig().GCInterval; cfg.Cache.GCInterval != want {
		t.Errorf("cache.gc_interval = %s, want backfilled %s", cfg.Cache.GCInterval, want)
	}
}
