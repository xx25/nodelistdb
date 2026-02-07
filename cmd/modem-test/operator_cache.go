// Package main provides operator caching for modem testing with failover.
// The cache stores working operator configurations per phone number using BadgerDB,
// allowing the system to remember which operator worked for a given destination.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dgraph-io/badger/v4"
)

// OperatorCacheConfig contains configuration for the operator cache.
type OperatorCacheConfig struct {
	Enabled bool     `yaml:"enabled"` // Enable operator caching (default: true when multiple operators)
	Path    string   `yaml:"path"`    // Cache directory path (default: ~/.modem-test/operator_cache)
	TTL     Duration `yaml:"ttl"`     // Cache entry TTL (default: 8640h = 360 days)
}

// CachedOperator stores the cached operator information for a phone number.
type CachedOperator struct {
	OperatorName   string    `json:"operator_name"`
	OperatorPrefix string    `json:"operator_prefix"`
	LastSuccess    time.Time `json:"last_success"`
}

// OperatorCache provides BadgerDB-based caching for phone → operator mappings.
type OperatorCache struct {
	db     *badger.DB
	ttl    time.Duration
	log    *TestLogger
	stopGC chan struct{}
}

// NewOperatorCache creates a new operator cache from configuration.
func NewOperatorCache(cfg OperatorCacheConfig, log *TestLogger) (*OperatorCache, error) {
	if !cfg.Enabled {
		return nil, nil
	}

	// Determine cache path
	path := cfg.Path
	if path == "" {
		// Default to ~/.modem-test/operator_cache
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		path = filepath.Join(homeDir, ".modem-test", "operator_cache")
	} else if strings.HasPrefix(path, "~/") {
		// Expand ~ to home directory
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		path = filepath.Join(homeDir, path[2:])
	}

	// Create directory if it doesn't exist
	if err := os.MkdirAll(path, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory %s: %w", path, err)
	}

	// Configure BadgerDB options
	opts := badger.DefaultOptions(path)
	opts = opts.WithNumVersionsToKeep(1)
	opts = opts.WithNumLevelZeroTables(2)
	opts = opts.WithNumLevelZeroTablesStall(4)
	opts = opts.WithLoggingLevel(badger.WARNING)
	// Disable compression for small values
	opts = opts.WithCompression(0)

	// Open database
	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open operator cache database: %w", err)
	}

	// Determine TTL
	ttl := cfg.TTL.Duration()
	if ttl == 0 {
		ttl = 360 * 24 * time.Hour // Default: 360 days
	}

	cache := &OperatorCache{
		db:     db,
		ttl:    ttl,
		log:    log,
		stopGC: make(chan struct{}),
	}

	// Start background GC
	go cache.runGC()

	if log != nil {
		log.Info("Operator cache enabled: %s (TTL: %v)", path, ttl)
	}

	return cache, nil
}

// keyForPhone returns the BadgerDB key for a phone number.
func (c *OperatorCache) keyForPhone(phone string) []byte {
	return []byte("operator:" + phone)
}

// Get retrieves the cached operator for a phone number.
// Returns (nil, false) if no cache entry exists.
func (c *OperatorCache) Get(phone string) (*CachedOperator, bool) {
	if c == nil || c.db == nil {
		return nil, false
	}

	var cached *CachedOperator

	err := c.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(c.keyForPhone(phone))
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			cached = &CachedOperator{}
			return json.Unmarshal(val, cached)
		})
	})

	if err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil, false
		}
		if c.log != nil {
			c.log.Warn("Operator cache read error for %s: %v", phone, err)
		}
		return nil, false
	}

	return cached, true
}

// Set stores the working operator for a phone number.
func (c *OperatorCache) Set(phone string, op OperatorConfig) error {
	if c == nil || c.db == nil {
		return nil
	}

	cached := CachedOperator{
		OperatorName:   op.Name,
		OperatorPrefix: op.Prefix,
		LastSuccess:    time.Now(),
	}

	data, err := json.Marshal(cached)
	if err != nil {
		return fmt.Errorf("failed to marshal operator cache entry: %w", err)
	}

	err = c.db.Update(func(txn *badger.Txn) error {
		entry := badger.NewEntry(c.keyForPhone(phone), data)
		entry = entry.WithTTL(c.ttl)
		return txn.SetEntry(entry)
	})

	if err != nil {
		return fmt.Errorf("failed to write operator cache entry: %w", err)
	}

	if c.log != nil {
		c.log.Info("Cached operator %q for phone %s", op.Name, phone)
	}

	return nil
}

// Delete removes the cached operator for a phone number.
func (c *OperatorCache) Delete(phone string) error {
	if c == nil || c.db == nil {
		return nil
	}

	err := c.db.Update(func(txn *badger.Txn) error {
		return txn.Delete(c.keyForPhone(phone))
	})

	if err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
		return fmt.Errorf("failed to delete operator cache entry: %w", err)
	}

	if c.log != nil {
		c.log.Info("Cleared cached operator for phone %s", phone)
	}

	return nil
}

// Close closes the operator cache database.
func (c *OperatorCache) Close() error {
	if c == nil || c.db == nil {
		return nil
	}

	close(c.stopGC)
	return c.db.Close()
}

// runGC runs periodic garbage collection on the BadgerDB database.
func (c *OperatorCache) runGC() {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.performGC()
		case <-c.stopGC:
			return
		}
	}
}

// performGC runs a single GC cycle.
func (c *OperatorCache) performGC() {
	for {
		err := c.db.RunValueLogGC(0.5)
		if err != nil {
			// ErrNoRewrite means nothing to GC, which is expected
			if !errors.Is(err, badger.ErrNoRewrite) && c.log != nil {
				c.log.Warn("Operator cache GC error: %v", err)
			}
			break
		}
	}
}

// FindOperatorByName looks up an operator by name in the given list.
// Returns (OperatorConfig, index, found).
func FindOperatorByName(operators []OperatorConfig, name string) (OperatorConfig, int, bool) {
	for i, op := range operators {
		if op.Name == name {
			return op, i, true
		}
	}
	return OperatorConfig{}, -1, false
}

// ReorderOperatorsWithCached returns a new operator list with the cached operator first.
// If no cached operator exists or it's not in the list, returns the original order.
func ReorderOperatorsWithCached(operators []OperatorConfig, cached *CachedOperator) []OperatorConfig {
	if cached == nil || len(operators) == 0 {
		return operators
	}

	// Find cached operator in the list
	_, idx, found := FindOperatorByName(operators, cached.OperatorName)
	if !found {
		return operators // Cached operator no longer in config
	}

	// If already first, return as-is
	if idx == 0 {
		return operators
	}

	// Reorder: put cached operator first
	reordered := make([]OperatorConfig, len(operators))
	reordered[0] = operators[idx]
	copy(reordered[1:idx+1], operators[:idx])
	copy(reordered[idx+1:], operators[idx+1:])

	return reordered
}

// isUserBusy checks if the CDR indicates the destination is genuinely busy
// (as opposed to a routing/network failure that should trigger operator failover).
// Only Q.931 cause 17 definitively indicates the destination line is occupied.
// BUSY disposition alone is not sufficient - it could be a routing/operator issue.
func isUserBusy(asteriskCDR *AsteriskCDRData) bool {
	if asteriskCDR == nil {
		return false
	}

	// Q.931 cause code 17 = "User busy" - destination line is occupied
	// This is the only definitive indicator that the destination itself is busy,
	// not a routing or operator issue.
	return asteriskCDR.HangupCause == 17
}

// FailoverResult contains the outcome of a failover test sequence.
type FailoverResult struct {
	Success          bool            // Overall success
	SuccessOperator  *OperatorConfig // Which operator worked (nil if none)
	LastOperator     *OperatorConfig // Last operator tried (for attribution on failure)
	LastResult       testResult      // Last test result
	TriedOperators   int             // How many operators were tried
	UserBusy         bool            // True if stopped due to user busy (not operator failure)
	AllOperatorsFail bool            // True if all operators failed
	WindowClosed     bool            // True if call window closed during test, node should be retried later
}

// OperatorResultCallback is called when an intermediate operator attempt completes
// (success or failure) before trying the next operator. This allows recording
// each operator's result to CSV, databases, and the NodelistDB API.
type OperatorResultCallback func(result testResult, operatorName, operatorPrefix string)

// runTestWithFailover executes tests with operator failover.
// It tries operators in order (cached first if available), caches successful operators,
// and handles "User Busy" specially by not switching operators.
//
// Parameters:
//   - ctx: context for cancellation
//   - job: the phone job to process
//   - operators: full list of operators to try
//   - cache: operator cache (may be nil)
//   - onRetryAttempt: callback for retry tracking
//   - onOperatorResult: callback to emit intermediate operator results
//
// Returns FailoverResult with the outcome.
func (w *ModemWorker) runTestWithFailover(
	ctx context.Context,
	job phoneJob,
	operators []OperatorConfig,
	cache *OperatorCache,
	onRetryAttempt RetryAttemptCallback,
	onOperatorResult OperatorResultCallback,
) FailoverResult {
	if len(operators) == 0 {
		// No operators configured - run test directly
		result := w.runTest(ctx, job.testNum, job.phone, job.phone, onRetryAttempt, job.nodeAvailability)
		return FailoverResult{
			Success:      result.success,
			LastResult:   result,
			WindowClosed: result.windowClosed,
		}
	}

	// Check cache for known working operator
	orderedOperators := operators
	if cache != nil {
		cached, found := cache.Get(job.phone)
		if found {
			// Verify cached operator still exists in config before using
			_, _, exists := FindOperatorByName(operators, cached.OperatorName)
			if exists {
				w.log.Info("Using cached operator %q for %s", cached.OperatorName, job.phone)
				orderedOperators = ReorderOperatorsWithCached(operators, cached)
			} else {
				w.log.Info("Cached operator %q no longer in config, ignoring", cached.OperatorName)
			}
		}
	}

	// Pre-dial availability check before starting operator loop
	if job.nodeAvailability != nil && !job.nodeAvailability.IsCallableNow(time.Now().UTC()) {
		w.log.Warn("Node %s: outside call window, deferring", job.nodeAddress)
		return FailoverResult{
			WindowClosed: true,
			LastResult:   testResult{windowClosed: true, message: fmt.Sprintf("Node %s: deferred - outside call window", job.nodeAddress)},
		}
	}

	// Try operators in order
	var lastResult testResult
	var lastOperator *OperatorConfig

	for i, op := range orderedOperators {
		currentOp := op // Capture for closure and tracking
		lastOperator = &currentOp

		select {
		case <-ctx.Done():
			return FailoverResult{
				Success:        false,
				LastOperator:   lastOperator,
				LastResult:     testResult{success: false, message: "cancelled"},
				TriedOperators: i,
			}
		default:
		}

		// Log operator being tried
		if op.Name != "" {
			if i == 0 {
				w.log.Info("Trying operator: %s (prefix: %q)", op.Name, op.Prefix)
			} else {
				w.log.Info("Failover to operator: %s (prefix: %q)", op.Name, op.Prefix)
			}
		}

		// Dial with this operator's prefix
		dialPhone := op.Prefix + job.phone

		// Create retry callback that includes operator info
		// Note: runTest passes empty operator strings - we override with actual operator from closure
		opRetryCallback := func(attempt int, dialTime time.Duration, reason, _, _ string) {
			if onRetryAttempt != nil {
				// Prefix reason with operator name for clarity
				prefixedReason := reason
				if currentOp.Name != "" {
					prefixedReason = fmt.Sprintf("[%s] %s", currentOp.Name, reason)
				}
				// Pass actual operator info from current iteration
				onRetryAttempt(attempt, dialTime, prefixedReason, currentOp.Name, currentOp.Prefix)
			}
		}

		// Run test with this operator
		lastResult = w.runTest(ctx, job.testNum, dialPhone, job.phone, opRetryCallback, job.nodeAvailability)

		// If call window closed during test, stop immediately
		if lastResult.windowClosed {
			return FailoverResult{
				WindowClosed:   true,
				LastOperator:   lastOperator,
				LastResult:     lastResult,
				TriedOperators: i + 1,
			}
		}

		if lastResult.success {
			// Success! Cache this operator
			if cache != nil {
				if err := cache.Set(job.phone, op); err != nil {
					w.log.Warn("Failed to cache operator: %v", err)
				}
			}
			return FailoverResult{
				Success:         true,
				SuccessOperator: lastOperator,
				LastOperator:    lastOperator,
				LastResult:      lastResult,
				TriedOperators:  i + 1,
			}
		}

		// Continue to next operator if available
		if i < len(orderedOperators)-1 {
			// Check availability before trying next operator
			if job.nodeAvailability != nil && !job.nodeAvailability.IsCallableNow(time.Now().UTC()) {
				w.log.Warn("Node %s: call window closed between operators, deferring", job.nodeAddress)
				return FailoverResult{
					WindowClosed:   true,
					LastOperator:   lastOperator,
					LastResult:     testResult{windowClosed: true, message: fmt.Sprintf("Node %s: deferred - call window closed between operators", job.nodeAddress)},
					TriedOperators: i + 1,
				}
			}

			// Emit this operator's result before trying the next one
			if onOperatorResult != nil {
				onOperatorResult(lastResult, currentOp.Name, currentOp.Prefix)
			}
			w.log.Warn("Operator %q failed, will try next", op.Name)
		}
	}

	// All operators failed - clear cache entry if it existed
	if cache != nil {
		if err := cache.Delete(job.phone); err != nil {
			w.log.Warn("Failed to clear operator cache: %v", err)
		}
	}

	return FailoverResult{
		Success:          false,
		LastOperator:     lastOperator,
		LastResult:       lastResult,
		TriedOperators:   len(orderedOperators),
		AllOperatorsFail: true,
	}
}
