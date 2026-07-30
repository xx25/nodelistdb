// Package main provides operator failover for modem test calls: operators are
// tried in order (cached working operator first) until one connects.
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/nodelistdb/internal/testing/timeavail"
)

// FailoverResult contains the outcome of a failover test sequence.
type FailoverResult struct {
	Success          bool            // Overall success
	SuccessOperator  *OperatorConfig // Which operator worked (nil if none)
	LastOperator     *OperatorConfig // Last operator tried (for attribution on failure)
	LastResult       testResult      // Last test result
	TriedOperators   int             // How many operators were tried
	AllOperatorsFail bool            // True if all operators failed
	WindowClosed     bool            // True if call window closed during test, node should be retried later
}

// OperatorResultCallback is called when an intermediate operator attempt completes
// (success or failure) before trying the next operator. This allows recording
// each operator's result to CSV, databases, and the NodelistDB API.
type OperatorResultCallback func(result testResult, operatorName, operatorPrefix string)

// operatorCacheStore is the slice of OperatorCache the failover sequence uses,
// separated out so tests can substitute an in-memory implementation.
type operatorCacheStore interface {
	Get(phone string) (*CachedOperator, bool)
	Set(phone string, op OperatorConfig) error
	Delete(phone string) error
}

// callRunner places a single test call. dialPhone carries the operator prefix;
// originalPhone is the bare number used for CDR lookups.
type callRunner func(ctx context.Context, testNum int, dialPhone, originalPhone string, onRetryAttempt RetryAttemptCallback, nodeAvailability *timeavail.NodeAvailability) testResult

// runTestWithFailover runs the failover sequence with this worker's modem
// placing the calls and its cache remembering working operators.
func (w *ModemWorker) runTestWithFailover(
	ctx context.Context,
	job phoneJob,
	operators []OperatorConfig,
	cache *OperatorCache,
	onRetryAttempt RetryAttemptCallback,
	onOperatorResult OperatorResultCallback,
) FailoverResult {
	// Explicit nil check: assigning a nil *OperatorCache into the interface
	// would make it non-nil inside runFailoverSequence.
	var store operatorCacheStore
	if cache != nil {
		store = cache
	}
	return runFailoverSequence(ctx, job, operators, store, w.log, w.runTest, onRetryAttempt, onOperatorResult)
}

// runFailoverSequence executes tests with operator failover.
// It tries operators in order (cached first if available) and caches the
// operator that succeeds. A failure on one operator — including a busy
// destination — moves on to the next; only a closed call window stops the
// sequence early.
//
// Parameters:
//   - ctx: context for cancellation
//   - job: the phone job to process
//   - operators: full list of operators to try
//   - cache: operator cache (may be nil)
//   - log: session logger
//   - runTest: places a single test call
//   - onRetryAttempt: callback for retry tracking
//   - onOperatorResult: callback to emit intermediate operator results
//
// Returns FailoverResult with the outcome.
func runFailoverSequence(
	ctx context.Context,
	job phoneJob,
	operators []OperatorConfig,
	cache operatorCacheStore,
	log *TestLogger,
	runTest callRunner,
	onRetryAttempt RetryAttemptCallback,
	onOperatorResult OperatorResultCallback,
) FailoverResult {
	if len(operators) == 0 {
		// No operators configured - run test directly
		result := runTest(ctx, job.testNum, job.phone, job.phone, onRetryAttempt, job.nodeAvailability)
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
				log.Info("Using cached operator %q for %s", cached.OperatorName, job.phone)
				orderedOperators = ReorderOperatorsWithCached(operators, cached)
			} else {
				log.Info("Cached operator %q no longer in config, ignoring", cached.OperatorName)
			}
		}
	}

	// Pre-dial availability check before starting operator loop
	if job.nodeAvailability != nil && !job.nodeAvailability.IsCallableNow(time.Now().UTC()) {
		log.Warn("Node %s: outside call window, deferring", job.nodeAddress)
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
				log.Info("Trying operator: %s (prefix: %q)", op.Name, op.Prefix)
			} else {
				log.Info("Failover to operator: %s (prefix: %q)", op.Name, op.Prefix)
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
		lastResult = runTest(ctx, job.testNum, dialPhone, job.phone, opRetryCallback, job.nodeAvailability)

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
					log.Warn("Failed to cache operator: %v", err)
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
				log.Warn("Node %s: call window closed between operators, deferring", job.nodeAddress)
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
			log.Warn("Operator %q failed, will try next", op.Name)
		}
	}

	// All operators failed - clear cache entry if it existed
	if cache != nil {
		if err := cache.Delete(job.phone); err != nil {
			log.Warn("Failed to clear operator cache: %v", err)
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
