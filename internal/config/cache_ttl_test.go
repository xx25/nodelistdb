package config

import (
	"strings"
	"testing"
	"time"
)

// A TTL of zero means "never expires" in both cache backends, not "expires
// immediately" - they each gate expiry on ttl > 0. Since LoadConfig unmarshals
// onto a zero-value Config, every cache section that omits a TTL used to
// produce exactly that, silently and permanently. These tests pin the backfill
// that closes it, using the same writeConfig/LoadConfig harness as
// networks_test.go so they exercise the real load path rather than validate()
// in isolation.

// ttlFields names the three TTLs that reach a cache.Set call, with the value
// DefaultCacheConfig promises for each. They are deliberately distinct, so a
// backfill that flattens them all to one value cannot pass.
func ttlFields(c *CacheConfig) map[string]struct {
	got  time.Duration
	want time.Duration
} {
	d := DefaultCacheConfig()
	return map[string]struct {
		got  time.Duration
		want time.Duration
	}{
		"node_ttl":   {c.NodeTTL, d.NodeTTL},
		"stats_ttl":  {c.StatsTTL, d.StatsTTL},
		"search_ttl": {c.SearchTTL, d.SearchTTL},
	}
}

func TestCacheTTLsBackfilledFromPartialConfig(t *testing.T) {
	cfg, err := LoadConfig(writeConfig(t, baseConfig+`
cache:
  enabled: true
  type: memory
  path: /tmp/nodelistdb-test-cache
`))
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	// Assert the exact per-field default, not merely non-zero: an
	// implementation that filled all three from default_ttl would satisfy a
	// non-zero check while quietly collapsing 15m/1h/5m into one value.
	for name, f := range ttlFields(&cfg.Cache) {
		if f.got != f.want {
			t.Errorf("cache.%s = %s, want its own default %s", name, f.got, f.want)
		}
	}

	if cfg.Cache.DefaultTTL != DefaultCacheConfig().DefaultTTL {
		t.Errorf("cache.default_ttl = %s, want %s", cfg.Cache.DefaultTTL, DefaultCacheConfig().DefaultTTL)
	}
}

// default_ttl is the documented catch-all. Before this change it was parsed,
// copied through to the storage layer and read by nothing at all.
func TestCacheDefaultTTLFeedsEveryUnsetTTL(t *testing.T) {
	cfg, err := LoadConfig(writeConfig(t, baseConfig+`
cache:
  enabled: true
  type: memory
  path: /tmp/nodelistdb-test-cache
  default_ttl: 42m
`))
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	want := 42 * time.Minute
	for name, f := range ttlFields(&cfg.Cache) {
		if f.got != want {
			t.Errorf("cache.%s = %s, want default_ttl %s", name, f.got, want)
		}
	}
}

// The precedence case: an explicit TTL must survive alongside default_ttl.
// Reverting the backfill entirely leaves this passing, so it is the one case
// the mutation check cannot substitute for.
func TestCacheExplicitTTLBeatsDefaultTTL(t *testing.T) {
	cfg, err := LoadConfig(writeConfig(t, baseConfig+`
cache:
  enabled: true
  type: memory
  path: /tmp/nodelistdb-test-cache
  default_ttl: 42m
  node_ttl: 7m
  stats_ttl: 8m
  search_ttl: 9m
`))
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	for name, want := range map[string]time.Duration{
		"node_ttl":   7 * time.Minute,
		"stats_ttl":  8 * time.Minute,
		"search_ttl": 9 * time.Minute,
	} {
		if got := ttlFields(&cfg.Cache)[name].got; got != want {
			t.Errorf("cache.%s = %s, want the explicit %s", name, got, want)
		}
	}
}

// A negative duration slips past a zero-check, and both backends only apply
// expiry when ttl > 0 - so it lands in the same never-expires state the
// backfill exists to prevent. Reject it instead of coercing a typo.
func TestCacheNegativeTTLRejected(t *testing.T) {
	for _, field := range []string{"default_ttl", "node_ttl", "stats_ttl", "search_ttl"} {
		t.Run(field, func(t *testing.T) {
			_, err := LoadConfig(writeConfig(t, baseConfig+`
cache:
  enabled: true
  type: memory
  path: /tmp/nodelistdb-test-cache
  `+field+`: -5m
`))
			if err == nil {
				t.Fatalf("negative cache.%s was accepted", field)
			}
			if !strings.Contains(err.Error(), field) {
				t.Errorf("error should name the offending field %q, got: %v", field, err)
			}
		})
	}
}

// Nothing above should apply to a disabled cache section that is simply absent.
func TestCacheTTLsBackfilledEvenWhenDisabled(t *testing.T) {
	cfg, err := LoadConfig(writeConfig(t, baseConfig))
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	// The cache is off here, but the values must still be coherent: enabling it
	// later via a flag or a second config must not resurrect the zero TTLs.
	for name, f := range ttlFields(&cfg.Cache) {
		if f.got != f.want {
			t.Errorf("cache.%s = %s, want %s", name, f.got, f.want)
		}
	}
}
