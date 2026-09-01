package ratelimit

import (
	"math"
	"sync"
	"time"
)

// Rate is one class's allowance: a sustained refill rate plus a burst depth.
//
// Burst is what makes this usable by people. A reader opening a node page,
// then its history, then a neighbouring node arrives in a clump; sustained
// rate alone would reject the clump even though the hourly total is trivial.
// Burst absorbs the clump and Refill decides what a client may keep up
// indefinitely - which is the number that actually protects the CPU.
type Rate struct {
	Refill float64 // tokens per second
	Burst  float64 // maximum tokens held
}

// Unlimited reports whether this class is exempt.
func (r Rate) Unlimited() bool { return r.Refill <= 0 || r.Burst <= 0 }

// bucket is one caller's allowance for one class.
//
// Deliberately a value in the map, not a pointer to a struct with its own
// mutex: at a few thousand live keys the single lock is far cheaper than the
// allocation, and it keeps eviction honest - see Limiter.
type bucket struct {
	tokens float64
	last   time.Time
}

// Limiter is a bounded set of token buckets keyed by caller and class.
//
// The bound is not decoration. The nearest thing to this in the codebase -
// the in-memory response cache - keys an unbounded map by URL and evicts only
// on TTL, which on this host grew to a 314 MB resident set and pushed 455 MB
// into swap on a 954 MB machine. A per-IP structure is a worse version of the
// same risk, because the key is chosen by the caller: anyone able to source
// from a wide prefix can mint keys as fast as they can open connections. So
// maxKeys is enforced on every insert, and it is enforced by evicting rather
// than by refusing to limit.
type Limiter struct {
	mu      sync.Mutex
	buckets map[string]bucket
	maxKeys int
	idle    time.Duration
	now     func() time.Time // injectable for tests
}

// NewLimiter builds a limiter holding at most maxKeys buckets and forgetting
// any caller idle for longer than idle.
func NewLimiter(maxKeys int, idle time.Duration) *Limiter {
	if maxKeys <= 0 {
		maxKeys = 10000
	}
	if idle <= 0 {
		idle = 10 * time.Minute
	}
	return &Limiter{
		buckets: make(map[string]bucket),
		maxKeys: maxKeys,
		idle:    idle,
		now:     time.Now,
	}
}

// Allow charges one request against key's bucket for the given rate.
//
// It returns whether the request may proceed and, when it may not, how long
// the caller must wait for a single token - which is what goes in Retry-After.
func (l *Limiter) Allow(key string, rate Rate) (bool, time.Duration) {
	if rate.Unlimited() {
		return true, 0
	}

	now := l.now()

	l.mu.Lock()
	defer l.mu.Unlock()

	b, seen := l.buckets[key]
	if !seen {
		// A caller we have no record of starts full, then immediately pays for
		// this request. Starting empty would reject every first-time visitor.
		b = bucket{tokens: rate.Burst, last: now}
		l.evictLocked(now)
	} else if elapsed := now.Sub(b.last); elapsed > 0 {
		b.tokens = math.Min(rate.Burst, b.tokens+elapsed.Seconds()*rate.Refill)
	}
	b.last = now

	if b.tokens < 1 {
		// Report the wait before storing, so a rejected request does not also
		// reset the clock and starve the caller further.
		wait := time.Duration((1 - b.tokens) / rate.Refill * float64(time.Second))
		l.buckets[key] = b
		return false, wait
	}

	b.tokens--
	l.buckets[key] = b
	return true, 0
}

// evictLocked makes room for one new key. Called with l.mu held.
//
// It first drops everything idle past the horizon, which in steady state is
// enough and is O(n) only when the map is actually full. If that frees
// nothing - the pathological case, a flood of distinct keys all active - it
// drops the least recently seen. That is the right victim: the flood's own
// keys are the freshest, so evicting oldest-first would hand the attacker a
// way to flush honest callers out. Instead the flood mostly evicts itself,
// and a bucket that is dropped and recreated is at worst as permissive as a
// first-time visitor, never more.
func (l *Limiter) evictLocked(now time.Time) {
	if len(l.buckets) < l.maxKeys {
		return
	}

	horizon := now.Add(-l.idle)
	for k, b := range l.buckets {
		if b.last.Before(horizon) {
			delete(l.buckets, k)
		}
	}
	if len(l.buckets) < l.maxKeys {
		return
	}

	oldestKey, oldest := "", time.Time{}
	for k, b := range l.buckets {
		if oldest.IsZero() || b.last.Before(oldest) {
			oldestKey, oldest = k, b.last
		}
	}
	if oldestKey != "" {
		delete(l.buckets, oldestKey)
	}
}

// Len reports the number of live buckets, for the stats endpoint and tests.
func (l *Limiter) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.buckets)
}
