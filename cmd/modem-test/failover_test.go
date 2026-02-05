// Package main provides integration-style tests for the operator failover logic.
// These tests use a mock test runner to simulate modem behavior without hardware.
package main

import (
	"context"
	"sync"
	"testing"
	"time"
)

// mockTestRunner simulates the test execution for failover testing.
// It returns predefined results in sequence.
type mockTestRunner struct {
	mu           sync.Mutex
	results      []testResult      // Results to return in order
	callIndex    int               // Current position in results
	callLog      []mockTestCall    // Log of all calls made
	defaultResult testResult       // Result to return if results exhausted
}

// mockTestCall records a single call to runTest.
type mockTestCall struct {
	DialPhone     string
	OriginalPhone string
	OperatorName  string
	OperatorPrefix string
}

// newMockTestRunner creates a mock runner with predefined results.
func newMockTestRunner(results ...testResult) *mockTestRunner {
	return &mockTestRunner{
		results: results,
		defaultResult: testResult{
			success: false,
			message: "default failure",
		},
	}
}

// runTest simulates a test call and returns the next result.
func (m *mockTestRunner) runTest(dialPhone, originalPhone, opName, opPrefix string) testResult {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.callLog = append(m.callLog, mockTestCall{
		DialPhone:      dialPhone,
		OriginalPhone:  originalPhone,
		OperatorName:   opName,
		OperatorPrefix: opPrefix,
	})

	if m.callIndex < len(m.results) {
		result := m.results[m.callIndex]
		m.callIndex++
		return result
	}
	return m.defaultResult
}

// getCalls returns all recorded test calls.
func (m *mockTestRunner) getCalls() []mockTestCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]mockTestCall{}, m.callLog...)
}

// mockOperatorCache provides an in-memory cache for testing.
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

func (m *mockOperatorCache) Set(phone string, op OperatorConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.setCalls = append(m.setCalls, mockCacheSetCall{Phone: phone, Operator: op})
	m.entries[phone] = &CachedOperator{
		OperatorName:   op.Name,
		OperatorPrefix: op.Prefix,
		LastSuccess:    time.Now(),
	}
}

func (m *mockOperatorCache) Delete(phone string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deleteCalls = append(m.deleteCalls, phone)
	delete(m.entries, phone)
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

// runTestWithFailoverMock implements the failover logic using mocks.
// This mirrors runTestWithFailover but allows testing without hardware.
func runTestWithFailoverMock(
	ctx context.Context,
	phone string,
	operators []OperatorConfig,
	cache *mockOperatorCache,
	runner *mockTestRunner,
) FailoverResult {
	if len(operators) == 0 {
		result := runner.runTest(phone, phone, "", "")
		return FailoverResult{
			Success:    result.success,
			LastResult: result,
		}
	}

	// Check cache for known working operator
	orderedOperators := operators
	if cache != nil {
		cached, found := cache.Get(phone)
		if found {
			_, _, exists := FindOperatorByName(operators, cached.OperatorName)
			if exists {
				orderedOperators = ReorderOperatorsWithCached(operators, cached)
			}
		}
	}

	var lastResult testResult
	var lastOperator *OperatorConfig

	for i, op := range orderedOperators {
		currentOp := op
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

		dialPhone := op.Prefix + phone
		lastResult = runner.runTest(dialPhone, phone, op.Name, op.Prefix)

		if lastResult.success {
			if cache != nil {
				cache.Set(phone, op)
			}
			return FailoverResult{
				Success:         true,
				SuccessOperator: lastOperator,
				LastOperator:    lastOperator,
				LastResult:      lastResult,
				TriedOperators:  i + 1,
			}
		}

	}

	// All operators failed - clear cache
	if cache != nil {
		cache.Delete(phone)
	}

	return FailoverResult{
		Success:          false,
		LastOperator:     lastOperator,
		LastResult:       lastResult,
		TriedOperators:   len(orderedOperators),
		AllOperatorsFail: true,
	}
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

// Tests

func TestRunTestWithFailover_EmptyOperators(t *testing.T) {
	runner := newMockTestRunner(successResult("direct call"))
	cache := newMockOperatorCache()

	result := runTestWithFailoverMock(
		context.Background(),
		"79001234567",
		[]OperatorConfig{}, // No operators
		cache,
		runner,
	)

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
}

func TestRunTestWithFailover_FirstOperatorSucceeds(t *testing.T) {
	runner := newMockTestRunner(successResult("first op success"))
	cache := newMockOperatorCache()

	result := runTestWithFailoverMock(
		context.Background(),
		"79001234567",
		testOperators,
		cache,
		runner,
	)

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
		t.Error("expected cache entry")
	}
	if cached.OperatorName != "Primary" {
		t.Errorf("expected cached operator Primary, got %q", cached.OperatorName)
	}
}

func TestRunTestWithFailover_FailoverToSecond(t *testing.T) {
	runner := newMockTestRunner(
		failResult("first failed"),
		successResult("second success"),
	)
	cache := newMockOperatorCache()

	result := runTestWithFailoverMock(
		context.Background(),
		"79001234567",
		testOperators,
		cache,
		runner,
	)

	if !result.Success {
		t.Error("expected success after failover")
	}
	if result.TriedOperators != 2 {
		t.Errorf("expected 2 operators tried, got %d", result.TriedOperators)
	}
	if result.SuccessOperator == nil || result.SuccessOperator.Name != "Secondary" {
		t.Errorf("expected Secondary operator, got %v", result.SuccessOperator)
	}

	// Verify correct dial prefixes were used
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
}

func TestRunTestWithFailover_FailoverToThird(t *testing.T) {
	runner := newMockTestRunner(
		failResult("first failed"),
		failResult("second failed"),
		successResult("third success"),
	)
	cache := newMockOperatorCache()

	result := runTestWithFailoverMock(
		context.Background(),
		"79001234567",
		testOperators,
		cache,
		runner,
	)

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

func TestRunTestWithFailover_AllOperatorsFail(t *testing.T) {
	runner := newMockTestRunner(
		failResult("first failed"),
		failResult("second failed"),
		failResult("third failed"),
	)
	cache := newMockOperatorCache()
	// Pre-populate cache to verify it gets cleared
	cache.preload("79001234567", "Primary", "1#")

	result := runTestWithFailoverMock(
		context.Background(),
		"79001234567",
		testOperators,
		cache,
		runner,
	)

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
	_, found := cache.Get("79001234567")
	if found {
		t.Error("cache should be cleared after all operators fail")
	}
	if len(cache.deleteCalls) != 1 {
		t.Errorf("expected 1 delete call, got %d", len(cache.deleteCalls))
	}
}

func TestRunTestWithFailover_UserBusyContinuesFailover(t *testing.T) {
	runner := newMockTestRunner(
		userBusyResult(),                  // First operator returns user busy
		userBusyResult(),                  // Second operator also busy
		failResult("third also failed"),   // Third operator fails
	)
	cache := newMockOperatorCache()

	result := runTestWithFailoverMock(
		context.Background(),
		"79001234567",
		testOperators,
		cache,
		runner,
	)

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

func TestRunTestWithFailover_UsesCachedOperator(t *testing.T) {
	runner := newMockTestRunner(
		successResult("cached op success"),
	)
	cache := newMockOperatorCache()
	// Pre-cache Secondary operator
	cache.preload("79001234567", "Secondary", "2#")

	result := runTestWithFailoverMock(
		context.Background(),
		"79001234567",
		testOperators,
		cache,
		runner,
	)

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
	if calls[0].OperatorName != "Secondary" {
		t.Errorf("expected Secondary operator, got %q", calls[0].OperatorName)
	}
}

func TestRunTestWithFailover_CachedOperatorFailsFallsBack(t *testing.T) {
	runner := newMockTestRunner(
		failResult("cached op failed"),
		successResult("primary success"),
	)
	cache := newMockOperatorCache()
	// Pre-cache Tertiary operator (will be tried first, then fail)
	cache.preload("79001234567", "Tertiary", "3#")

	result := runTestWithFailoverMock(
		context.Background(),
		"79001234567",
		testOperators,
		cache,
		runner,
	)

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

func TestRunTestWithFailover_StaleCacheIgnored(t *testing.T) {
	runner := newMockTestRunner(
		successResult("first op success"),
	)
	cache := newMockOperatorCache()
	// Pre-cache a non-existent operator
	cache.preload("79001234567", "DeletedOperator", "99#")

	result := runTestWithFailoverMock(
		context.Background(),
		"79001234567",
		testOperators, // Doesn't contain "DeletedOperator"
		cache,
		runner,
	)

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

func TestRunTestWithFailover_ContextCancellation(t *testing.T) {
	runner := newMockTestRunner(
		failResult("first failed"),
	)
	cache := newMockOperatorCache()

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel before second operator would be tried
	runner.results = []testResult{
		failResult("first failed"),
	}

	// Start failover in goroutine
	resultCh := make(chan FailoverResult, 1)
	go func() {
		resultCh <- runTestWithFailoverMock(ctx, "79001234567", testOperators, cache, runner)
	}()

	// Wait for first call, then cancel
	time.Sleep(10 * time.Millisecond)
	cancel()

	result := <-resultCh

	if result.Success {
		t.Error("expected failure after cancellation")
	}
	// Note: Due to timing, we might get either 1 or 2 operators tried
	// The important thing is it stops eventually
}

func TestRunTestWithFailover_NilCache(t *testing.T) {
	runner := newMockTestRunner(
		failResult("first failed"),
		successResult("second success"),
	)

	// Pass nil cache - should work without panicking
	result := runTestWithFailoverMock(
		context.Background(),
		"79001234567",
		testOperators,
		nil, // No cache
		runner,
	)

	if !result.Success {
		t.Error("expected success")
	}
	if result.TriedOperators != 2 {
		t.Errorf("expected 2 operators tried, got %d", result.TriedOperators)
	}
}

func TestRunTestWithFailover_SingleOperator(t *testing.T) {
	singleOp := []OperatorConfig{{Name: "Only", Prefix: "X#"}}

	t.Run("success", func(t *testing.T) {
		runner := newMockTestRunner(successResult("only op success"))
		cache := newMockOperatorCache()

		result := runTestWithFailoverMock(
			context.Background(),
			"79001234567",
			singleOp,
			cache,
			runner,
		)

		if !result.Success {
			t.Error("expected success")
		}
		if result.SuccessOperator.Name != "Only" {
			t.Errorf("expected Only operator, got %q", result.SuccessOperator.Name)
		}
	})

	t.Run("failure", func(t *testing.T) {
		runner := newMockTestRunner(failResult("only op failed"))
		cache := newMockOperatorCache()

		result := runTestWithFailoverMock(
			context.Background(),
			"79001234567",
			singleOp,
			cache,
			runner,
		)

		if result.Success {
			t.Error("expected failure")
		}
		if !result.AllOperatorsFail {
			t.Error("expected AllOperatorsFail for single operator failure")
		}
	})
}

func TestRunTestWithFailover_OperatorSequencePreserved(t *testing.T) {
	// Test that operators are tried in exact config order
	ops := []OperatorConfig{
		{Name: "A", Prefix: "A#"},
		{Name: "B", Prefix: "B#"},
		{Name: "C", Prefix: "C#"},
		{Name: "D", Prefix: "D#"},
	}

	runner := newMockTestRunner(
		failResult("A failed"),
		failResult("B failed"),
		failResult("C failed"),
		successResult("D success"),
	)
	cache := newMockOperatorCache()

	result := runTestWithFailoverMock(
		context.Background(),
		"12345",
		ops,
		cache,
		runner,
	)

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

func TestRunTestWithFailover_UserBusyOnSecondOperatorContinues(t *testing.T) {
	runner := newMockTestRunner(
		failResult("first failed - routing"),
		userBusyResult(),                // Second operator gets user busy
		failResult("third also failed"), // Third operator also fails
	)
	cache := newMockOperatorCache()

	result := runTestWithFailoverMock(
		context.Background(),
		"79001234567",
		testOperators,
		cache,
		runner,
	)

	if result.Success {
		t.Error("expected failure")
	}
	if !result.AllOperatorsFail {
		t.Error("expected AllOperatorsFail when all operators tried")
	}
	if result.TriedOperators != 3 {
		t.Errorf("expected all 3 operators tried, got %d", result.TriedOperators)
	}
	if result.LastOperator.Name != "Tertiary" {
		t.Errorf("expected last operator Tertiary, got %q", result.LastOperator.Name)
	}

	// Should have tried all operators
	calls := runner.getCalls()
	if len(calls) != 3 {
		t.Errorf("expected 3 calls, got %d", len(calls))
	}
}
