// Package main tests the operator failover logic. The tests drive the real
// runFailoverSequence with a fake call runner and an in-memory cache, so the
// production code path is exercised without hardware.
package main

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/nodelistdb/internal/testing/timeavail"
)

// discardLogger returns a TestLogger that swallows all output.
func discardLogger() *TestLogger {
	log := NewTestLogger(LoggingConfig{})
	log.SetOutput(io.Discard)
	return log
}

// mockCall records a single call placed through the mock runner.
type mockCall struct {
	TestNum       int
	DialPhone     string
	OriginalPhone string
}

// mockCallRunner satisfies the callRunner signature and returns predefined
// results in sequence.
type mockCallRunner struct {
	mu            sync.Mutex
	results       []testResult // Results to return in order
	callIndex     int          // Current position in results
	callLog       []mockCall   // Log of all calls made
	defaultResult testResult   // Result to return if results exhausted

	// simulateRetries invokes the per-call retry callback this many times
	// before returning, letting tests observe operator attribution.
	simulateRetries int
	// onCall, if set, runs at the start of each call (e.g. to cancel a context).
	onCall func(callNum int)
}

// newMockCallRunner creates a mock runner with predefined results.
func newMockCallRunner(results ...testResult) *mockCallRunner {
	return &mockCallRunner{
		results: results,
		defaultResult: testResult{
			success: false,
			message: "default failure",
		},
	}
}

// run is the callRunner implementation.
func (m *mockCallRunner) run(_ context.Context, testNum int, dialPhone, originalPhone string, onRetryAttempt RetryAttemptCallback, _ *timeavail.NodeAvailability) testResult {
	m.mu.Lock()
	callNum := len(m.callLog) + 1
	m.callLog = append(m.callLog, mockCall{
		TestNum:       testNum,
		DialPhone:     dialPhone,
		OriginalPhone: originalPhone,
	})
	result := m.defaultResult
	if m.callIndex < len(m.results) {
		result = m.results[m.callIndex]
		m.callIndex++
	}
	retries := m.simulateRetries
	onCall := m.onCall
	m.mu.Unlock()

	if onCall != nil {
		onCall(callNum)
	}
	// The real runTest reports each retry with empty operator fields; the
	// failover wrapper is expected to fill them in.
	for i := 1; i <= retries; i++ {
		if onRetryAttempt != nil {
			onRetryAttempt(i, time.Second, "BUSY (modem)", "", "")
		}
	}
	return result
}

// getCalls returns all recorded test calls.
func (m *mockCallRunner) getCalls() []mockCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]mockCall{}, m.callLog...)
}

// mockOperatorCache provides an in-memory operatorCacheStore for testing.
type mockOperatorCache struct {
	mu          sync.Mutex
	entries     map[string]*CachedOperator
	setCalls    []mockCacheSetCall
	deleteCalls []string
}

type mockCacheSetCall struct {
	Phone    string
	Operator OperatorConfig
}

func newMockOperatorCache() *mockOperatorCache {
	return &mockOperatorCache{
		entries: make(map[string]*CachedOperator),
	}
}

func (m *mockOperatorCache) Get(phone string) (*CachedOperator, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cached, ok := m.entries[phone]
	return cached, ok
}

func (m *mockOperatorCache) Set(phone string, op OperatorConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.setCalls = append(m.setCalls, mockCacheSetCall{Phone: phone, Operator: op})
	m.entries[phone] = &CachedOperator{
		OperatorName:   op.Name,
		OperatorPrefix: op.Prefix,
		LastSuccess:    time.Now(),
	}
	return nil
}

func (m *mockOperatorCache) Delete(phone string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deleteCalls = append(m.deleteCalls, phone)
	delete(m.entries, phone)
	return nil
}

func (m *mockOperatorCache) preload(phone string, opName, opPrefix string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries[phone] = &CachedOperator{
		OperatorName:   opName,
		OperatorPrefix: opPrefix,
		LastSuccess:    time.Now(),
	}
}

// runFailover invokes the real failover sequence with test doubles.
func runFailover(ctx context.Context, phone string, operators []OperatorConfig, cache operatorCacheStore, runner *mockCallRunner) FailoverResult {
	return runFailoverSequence(ctx, phoneJob{phone: phone, testNum: 1}, operators, cache, discardLogger(), runner.run, nil, nil)
}

// Test fixtures
var testOperators = []OperatorConfig{
	{Name: "Primary", Prefix: "1#"},
	{Name: "Secondary", Prefix: "2#"},
	{Name: "Tertiary", Prefix: "3#"},
}

func successResult(msg string) testResult {
	return testResult{success: true, message: msg}
}

func failResult(msg string) testResult {
	return testResult{success: false, message: msg}
}

func userBusyResult() testResult {
	return testResult{
		success: false,
		message: "user busy",
		asteriskCDR: &AsteriskCDRData{
			HangupCause: 17, // Q.931 User Busy
			Disposition: "BUSY",
		},
	}
}

// neverCallable returns a NodeAvailability whose only window is empty,
// so IsCallableNow is false at any wall-clock time.
func neverCallable() *timeavail.NodeAvailability {
	instant := time.Date(2026, 1, 1, 3, 0, 0, 0, time.UTC)
	return &timeavail.NodeAvailability{
		Windows: []timeavail.TimeWindow{{StartUTC: instant, EndUTC: instant}},
	}
}

// Tests

func TestRunFailoverSequence_EmptyOperators(t *testing.T) {
	runner := newMockCallRunner(successResult("direct call"))
	cache := newMockOperatorCache()

	result := runFailover(context.Background(), "79001234567", []OperatorConfig{}, cache, runner)

	if !result.Success {
		t.Error("expected success for direct call")
	}

	calls := runner.getCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].DialPhone != "79001234567" {
		t.Errorf("expected direct dial, got %q", calls[0].DialPhone)
	}
	if calls[0].OriginalPhone != "79001234567" {
		t.Errorf("expected original phone passthrough, got %q", calls[0].OriginalPhone)
	}
}

func TestRunFailoverSequence_FirstOperatorSucceeds(t *testing.T) {
	runner := newMockCallRunner(successResult("first op success"))
	cache := newMockOperatorCache()

	result := runFailover(context.Background(), "79001234567", testOperators, cache, runner)

	if !result.Success {
		t.Error("expected success")
	}
	if result.TriedOperators != 1 {
		t.Errorf("expected 1 operator tried, got %d", result.TriedOperators)
	}
	if result.SuccessOperator == nil || result.SuccessOperator.Name != "Primary" {
		t.Errorf("expected Primary operator, got %v", result.SuccessOperator)
	}

	// Verify cache was updated
	cached, found := cache.Get("79001234567")
	if !found {
		t.Fatal("expected cache entry")
	}
	if cached.OperatorName != "Primary" {
		t.Errorf("expected cached operator Primary, got %q", cached.OperatorName)
	}
}

func TestRunFailoverSequence_FailoverToSecond(t *testing.T) {
	runner := newMockCallRunner(
		failResult("first failed"),
		successResult("second success"),
	)
	cache := newMockOperatorCache()

	result := runFailover(context.Background(), "79001234567", testOperators, cache, runner)

	if !result.Success {
		t.Error("expected success after failover")
	}
	if result.TriedOperators != 2 {
		t.Errorf("expected 2 operators tried, got %d", result.TriedOperators)
	}
	if result.SuccessOperator == nil || result.SuccessOperator.Name != "Secondary" {
		t.Errorf("expected Secondary operator, got %v", result.SuccessOperator)
	}

	// Verify correct dial prefixes were used, and that the bare number is
	// always passed for CDR matching.
	calls := runner.getCalls()
	if len(calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(calls))
	}
	if calls[0].DialPhone != "1#79001234567" {
		t.Errorf("first call dial = %q, want 1#79001234567", calls[0].DialPhone)
	}
	if calls[1].DialPhone != "2#79001234567" {
		t.Errorf("second call dial = %q, want 2#79001234567", calls[1].DialPhone)
	}
	for i, c := range calls {
		if c.OriginalPhone != "79001234567" {
			t.Errorf("call %d original phone = %q, want bare number", i, c.OriginalPhone)
		}
	}
}

func TestRunFailoverSequence_FailoverToThird(t *testing.T) {
	runner := newMockCallRunner(
		failResult("first failed"),
		failResult("second failed"),
		successResult("third success"),
	)
	cache := newMockOperatorCache()

	result := runFailover(context.Background(), "79001234567", testOperators, cache, runner)

	if !result.Success {
		t.Error("expected success after two failovers")
	}
	if result.TriedOperators != 3 {
		t.Errorf("expected 3 operators tried, got %d", result.TriedOperators)
	}
	if result.SuccessOperator.Name != "Tertiary" {
		t.Errorf("expected Tertiary operator, got %q", result.SuccessOperator.Name)
	}

	// Cache should have Tertiary
	cached, _ := cache.Get("79001234567")
	if cached.OperatorName != "Tertiary" {
		t.Errorf("cache should have Tertiary, got %q", cached.OperatorName)
	}
}

func TestRunFailoverSequence_AllOperatorsFail(t *testing.T) {
	runner := newMockCallRunner(
		failResult("first failed"),
		failResult("second failed"),
		failResult("third failed"),
	)
	cache := newMockOperatorCache()
	// Pre-populate cache to verify it gets cleared
	cache.preload("79001234567", "Primary", "1#")

	result := runFailover(context.Background(), "79001234567", testOperators, cache, runner)

	if result.Success {
		t.Error("expected failure")
	}
	if !result.AllOperatorsFail {
		t.Error("expected AllOperatorsFail=true")
	}
	if result.TriedOperators != 3 {
		t.Errorf("expected 3 operators tried, got %d", result.TriedOperators)
	}
	if result.LastOperator.Name != "Tertiary" {
		t.Errorf("expected last operator Tertiary, got %q", result.LastOperator.Name)
	}

	// Cache should be cleared
	if _, found := cache.Get("79001234567"); found {
		t.Error("cache should be cleared after all operators fail")
	}
	if len(cache.deleteCalls) != 1 {
		t.Errorf("expected 1 delete call, got %d", len(cache.deleteCalls))
	}
}

func TestRunFailoverSequence_UserBusyContinuesFailover(t *testing.T) {
	runner := newMockCallRunner(
		userBusyResult(),                // First operator returns user busy
		userBusyResult(),                // Second operator also busy
		failResult("third also failed"), // Third operator fails
	)
	cache := newMockOperatorCache()

	result := runFailover(context.Background(), "79001234567", testOperators, cache, runner)

	if result.Success {
		t.Error("expected failure")
	}
	if !result.AllOperatorsFail {
		t.Error("expected AllOperatorsFail when all operators tried")
	}
	if result.TriedOperators != 3 {
		t.Errorf("expected all 3 operators tried even with busy, got %d", result.TriedOperators)
	}

	// Should have tried all operators
	calls := runner.getCalls()
	if len(calls) != 3 {
		t.Errorf("expected 3 calls (all operators), got %d", len(calls))
	}
}

func TestRunFailoverSequence_UsesCachedOperator(t *testing.T) {
	runner := newMockCallRunner(
		successResult("cached op success"),
	)
	cache := newMockOperatorCache()
	// Pre-cache Secondary operator
	cache.preload("79001234567", "Secondary", "2#")

	result := runFailover(context.Background(), "79001234567", testOperators, cache, runner)

	if !result.Success {
		t.Error("expected success")
	}

	// Should have tried Secondary first (from cache)
	calls := runner.getCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].DialPhone != "2#79001234567" {
		t.Errorf("expected cached operator (2#), got %q", calls[0].DialPhone)
	}
	if result.SuccessOperator == nil || result.SuccessOperator.Name != "Secondary" {
		t.Errorf("expected Secondary operator, got %v", result.SuccessOperator)
	}
}

func TestRunFailoverSequence_CachedOperatorFailsFallsBack(t *testing.T) {
	runner := newMockCallRunner(
		failResult("cached op failed"),
		successResult("primary success"),
	)
	cache := newMockOperatorCache()
	// Pre-cache Tertiary operator (will be tried first, then fail)
	cache.preload("79001234567", "Tertiary", "3#")

	result := runFailover(context.Background(), "79001234567", testOperators, cache, runner)

	if !result.Success {
		t.Error("expected success after fallback")
	}
	if result.TriedOperators != 2 {
		t.Errorf("expected 2 operators tried, got %d", result.TriedOperators)
	}

	calls := runner.getCalls()
	if len(calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(calls))
	}
	// First call should be cached operator (Tertiary)
	if calls[0].DialPhone != "3#79001234567" {
		t.Errorf("first call should use cached (3#), got %q", calls[0].DialPhone)
	}
	// Second call should be Primary (next in reordered list)
	if calls[1].DialPhone != "1#79001234567" {
		t.Errorf("second call should use Primary (1#), got %q", calls[1].DialPhone)
	}

	// Cache should now have Primary
	cached, _ := cache.Get("79001234567")
	if cached.OperatorName != "Primary" {
		t.Errorf("cache should be updated to Primary, got %q", cached.OperatorName)
	}
}

func TestRunFailoverSequence_StaleCacheIgnored(t *testing.T) {
	runner := newMockCallRunner(
		successResult("first op success"),
	)
	cache := newMockOperatorCache()
	// Pre-cache a non-existent operator
	cache.preload("79001234567", "DeletedOperator", "99#")

	result := runFailover(context.Background(), "79001234567", testOperators, cache, runner)

	if !result.Success {
		t.Error("expected success")
	}

	// Should have used Primary (first in config) since cache was stale
	calls := runner.getCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].DialPhone != "1#79001234567" {
		t.Errorf("expected Primary (1#) for stale cache, got %q", calls[0].DialPhone)
	}
}

func TestRunFailoverSequence_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	runner := newMockCallRunner(
		failResult("first failed"),
	)
	// Cancel during the first call: the sequence must stop before dialing
	// the second operator.
	runner.onCall = func(callNum int) {
		if callNum == 1 {
			cancel()
		}
	}
	cache := newMockOperatorCache()

	result := runFailover(ctx, "79001234567", testOperators, cache, runner)

	if result.Success {
		t.Error("expected failure after cancellation")
	}
	if result.TriedOperators != 1 {
		t.Errorf("expected 1 operator tried before cancellation, got %d", result.TriedOperators)
	}
	if len(runner.getCalls()) != 1 {
		t.Errorf("expected no further calls after cancellation, got %d", len(runner.getCalls()))
	}
}

func TestRunFailoverSequence_CancelledBeforeStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runner := newMockCallRunner()
	cache := newMockOperatorCache()

	result := runFailover(ctx, "79001234567", testOperators, cache, runner)

	if result.Success {
		t.Error("expected failure for pre-cancelled context")
	}
	if result.TriedOperators != 0 {
		t.Errorf("expected 0 operators tried, got %d", result.TriedOperators)
	}
	if len(runner.getCalls()) != 0 {
		t.Errorf("expected no calls, got %d", len(runner.getCalls()))
	}
}

func TestRunFailoverSequence_NilCache(t *testing.T) {
	runner := newMockCallRunner(
		failResult("first failed"),
		successResult("second success"),
	)

	// Pass nil cache - should work without panicking
	result := runFailover(context.Background(), "79001234567", testOperators, nil, runner)

	if !result.Success {
		t.Error("expected success")
	}
	if result.TriedOperators != 2 {
		t.Errorf("expected 2 operators tried, got %d", result.TriedOperators)
	}
}

func TestRunFailoverSequence_SingleOperator(t *testing.T) {
	singleOp := []OperatorConfig{{Name: "Only", Prefix: "X#"}}

	t.Run("success", func(t *testing.T) {
		runner := newMockCallRunner(successResult("only op success"))
		cache := newMockOperatorCache()

		result := runFailover(context.Background(), "79001234567", singleOp, cache, runner)

		if !result.Success {
			t.Error("expected success")
		}
		if result.SuccessOperator.Name != "Only" {
			t.Errorf("expected Only operator, got %q", result.SuccessOperator.Name)
		}
	})

	t.Run("failure", func(t *testing.T) {
		runner := newMockCallRunner(failResult("only op failed"))
		cache := newMockOperatorCache()

		result := runFailover(context.Background(), "79001234567", singleOp, cache, runner)

		if result.Success {
			t.Error("expected failure")
		}
		if !result.AllOperatorsFail {
			t.Error("expected AllOperatorsFail for single operator failure")
		}
	})
}

func TestRunFailoverSequence_OperatorSequencePreserved(t *testing.T) {
	// Test that operators are tried in exact config order
	ops := []OperatorConfig{
		{Name: "A", Prefix: "A#"},
		{Name: "B", Prefix: "B#"},
		{Name: "C", Prefix: "C#"},
		{Name: "D", Prefix: "D#"},
	}

	runner := newMockCallRunner(
		failResult("A failed"),
		failResult("B failed"),
		failResult("C failed"),
		successResult("D success"),
	)
	cache := newMockOperatorCache()

	result := runFailover(context.Background(), "12345", ops, cache, runner)

	if !result.Success {
		t.Error("expected success")
	}
	if result.TriedOperators != 4 {
		t.Errorf("expected 4 operators tried, got %d", result.TriedOperators)
	}

	calls := runner.getCalls()
	expectedPrefixes := []string{"A#12345", "B#12345", "C#12345", "D#12345"}
	for i, expected := range expectedPrefixes {
		if calls[i].DialPhone != expected {
			t.Errorf("call %d: got %q, want %q", i, calls[i].DialPhone, expected)
		}
	}
}

func TestRunFailoverSequence_WindowClosedBeforeStart(t *testing.T) {
	runner := newMockCallRunner()
	cache := newMockOperatorCache()

	job := phoneJob{phone: "79001234567", testNum: 1, nodeAddress: "2:5001/100", nodeAvailability: neverCallable()}
	result := runFailoverSequence(context.Background(), job, testOperators, cache, discardLogger(), runner.run, nil, nil)

	if !result.WindowClosed {
		t.Error("expected WindowClosed for a node outside its call window")
	}
	if result.Success {
		t.Error("expected no success")
	}
	if len(runner.getCalls()) != 0 {
		t.Errorf("expected no calls placed, got %d", len(runner.getCalls()))
	}
}

func TestRunFailoverSequence_WindowClosedResultStopsSequence(t *testing.T) {
	// The runner reports the window closed mid-call; remaining operators
	// must not be dialed and the deferral must propagate.
	runner := newMockCallRunner(testResult{windowClosed: true, message: "deferred"})
	cache := newMockOperatorCache()

	result := runFailover(context.Background(), "79001234567", testOperators, cache, runner)

	if !result.WindowClosed {
		t.Error("expected WindowClosed to propagate from the call result")
	}
	if result.TriedOperators != 1 {
		t.Errorf("expected 1 operator tried, got %d", result.TriedOperators)
	}
	if len(runner.getCalls()) != 1 {
		t.Errorf("expected 1 call, got %d", len(runner.getCalls()))
	}
	// A deferred node is not a failure: the cache entry must survive.
	if len(cache.deleteCalls) != 0 {
		t.Errorf("expected no cache deletes on deferral, got %d", len(cache.deleteCalls))
	}
}

func TestRunFailoverSequence_IntermediateResultsEmitted(t *testing.T) {
	runner := newMockCallRunner(
		failResult("first failed"),
		failResult("second failed"),
		successResult("third success"),
	)
	cache := newMockOperatorCache()

	type emitted struct {
		opName   string
		opPrefix string
	}
	var intermediates []emitted
	onOperatorResult := func(result testResult, operatorName, operatorPrefix string) {
		if result.success {
			t.Errorf("intermediate emission for a successful result (%q)", operatorName)
		}
		intermediates = append(intermediates, emitted{operatorName, operatorPrefix})
	}

	job := phoneJob{phone: "79001234567", testNum: 1}
	result := runFailoverSequence(context.Background(), job, testOperators, cache, discardLogger(), runner.run, nil, onOperatorResult)

	if !result.Success {
		t.Fatal("expected success")
	}
	// Failed intermediate operators are emitted; the final (successful)
	// operator is reported via the returned FailoverResult instead.
	if len(intermediates) != 2 {
		t.Fatalf("expected 2 intermediate results, got %d", len(intermediates))
	}
	if intermediates[0] != (emitted{"Primary", "1#"}) {
		t.Errorf("first intermediate = %+v, want Primary/1#", intermediates[0])
	}
	if intermediates[1] != (emitted{"Secondary", "2#"}) {
		t.Errorf("second intermediate = %+v, want Secondary/2#", intermediates[1])
	}
}

func TestRunFailoverSequence_LastFailureNotEmittedAsIntermediate(t *testing.T) {
	runner := newMockCallRunner(
		failResult("first failed"),
		failResult("second failed"),
		failResult("third failed"),
	)
	cache := newMockOperatorCache()

	var count int
	onOperatorResult := func(testResult, string, string) { count++ }

	job := phoneJob{phone: "79001234567", testNum: 1}
	result := runFailoverSequence(context.Background(), job, testOperators, cache, discardLogger(), runner.run, nil, onOperatorResult)

	if result.Success {
		t.Fatal("expected failure")
	}
	// The last operator's failure is the final result, not an intermediate.
	if count != 2 {
		t.Errorf("expected 2 intermediate emissions for 3 failures, got %d", count)
	}
}

func TestRunFailoverSequence_RetryCallbackCarriesOperator(t *testing.T) {
	runner := newMockCallRunner(
		failResult("first failed"),
		successResult("second success"),
	)
	runner.simulateRetries = 1
	cache := newMockOperatorCache()

	type retry struct {
		reason   string
		opName   string
		opPrefix string
	}
	var retries []retry
	onRetryAttempt := func(attempt int, dialTime time.Duration, reason, operatorName, operatorPrefix string) {
		retries = append(retries, retry{reason, operatorName, operatorPrefix})
	}

	job := phoneJob{phone: "79001234567", testNum: 1}
	result := runFailoverSequence(context.Background(), job, testOperators, cache, discardLogger(), runner.run, onRetryAttempt, nil)

	if !result.Success {
		t.Fatal("expected success")
	}
	if len(retries) != 2 {
		t.Fatalf("expected 2 retry callbacks (one per operator), got %d", len(retries))
	}
	// runTest reports retries with empty operator fields; the failover wrapper
	// must stamp the current operator's identity onto each one.
	if retries[0].opName != "Primary" || retries[0].opPrefix != "1#" {
		t.Errorf("first retry attribution = %s/%s, want Primary/1#", retries[0].opName, retries[0].opPrefix)
	}
	if retries[0].reason != "[Primary] BUSY (modem)" {
		t.Errorf("first retry reason = %q, want operator-prefixed reason", retries[0].reason)
	}
	if retries[1].opName != "Secondary" || retries[1].opPrefix != "2#" {
		t.Errorf("second retry attribution = %s/%s, want Secondary/2#", retries[1].opName, retries[1].opPrefix)
	}
}
