package cache

import (
	"testing"
	"time"
)

// A non-positive GC interval reaches time.NewTicker inside runGC, which panics
// on one - on a bare goroutine with no recover(), so it kills the process
// rather than the cache. Config validation rejects it for operators, but every
// caller except the server builds its BadgerConfig in code (see
// internal/testing/daemon/daemon.go), so the backends coerce it too.
//
// These tests are their own mutation check: with the `<=` guards reverted to
// `==`, the goroutine panics and takes the whole test binary down, so every
// test in the package fails rather than just these.

func TestBadgerNegativeGCIntervalCoerced(t *testing.T) {
	cfg := &BadgerConfig{
		Path:        t.TempDir(),
		MaxMemoryMB: 64,
		GCInterval:  -10 * time.Minute,
	}

	bc, err := NewBadgerCache(cfg)
	if err != nil {
		t.Fatalf("NewBadgerCache: %v", err)
	}
	defer bc.Close()

	if cfg.GCInterval != 10*time.Minute {
		t.Errorf("GCInterval = %s, want coerced to 10m", cfg.GCInterval)
	}

	// Give the GC goroutine a moment to reach NewTicker. Without it the test
	// can return before the panic that this guard prevents would have fired.
	time.Sleep(20 * time.Millisecond)
}

// Outside (0,1) badger's RunValueLogGC returns ErrInvalidRequest without doing
// anything, so GC silently never runs - quieter than the ticker panic, but it
// disarms the only thing that reclaims value log space.
func TestBadgerOutOfRangeGCDiscardRatioCoerced(t *testing.T) {
	for _, ratio := range []float64{-0.5, 0, 1, 1.5} {
		cfg := &BadgerConfig{
			Path:           t.TempDir(),
			MaxMemoryMB:    64,
			GCDiscardRatio: ratio,
		}

		bc, err := NewBadgerCache(cfg)
		if err != nil {
			t.Fatalf("NewBadgerCache(ratio=%v): %v", ratio, err)
		}
		bc.Close()

		if cfg.GCDiscardRatio != 0.5 {
			t.Errorf("GCDiscardRatio %v = %v, want coerced to 0.5", ratio, cfg.GCDiscardRatio)
		}
	}
}

func TestMemoryNegativeGCIntervalCoerced(t *testing.T) {
	cfg := &MemoryConfig{GCInterval: -5 * time.Minute}

	mc := NewMemoryCache(cfg)
	defer mc.Close()

	if cfg.GCInterval != 5*time.Minute {
		t.Errorf("GCInterval = %s, want coerced to 5m", cfg.GCInterval)
	}

	time.Sleep(20 * time.Millisecond)
}
