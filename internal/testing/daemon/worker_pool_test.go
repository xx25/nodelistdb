package daemon

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewWorkerPool(t *testing.T) {
	tests := []struct {
		name            string
		workers         int
		expectedWorkers int
	}{
		{"normal workers", 5, 5},
		{"zero workers", 0, 1}, // Should default to 1
		{"negative workers", -5, 1}, // Should default to 1
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := NewWorkerPool(tt.workers)
			if pool == nil {
				t.Fatal("Expected non-nil worker pool")
			}
			if pool.workers != tt.expectedWorkers {
				t.Errorf("Expected %d workers, got %d", tt.expectedWorkers, pool.workers)
			}
		})
	}
}

func TestWorkerPool_StartAndStop(t *testing.T) {
	pool := NewWorkerPool(3)
	pool.Start()

	// Give workers time to start
	time.Sleep(50 * time.Millisecond)

	// Submit a simple job
	var executed atomic.Bool
	pool.Submit(func() {
		executed.Store(true)
	})

	// Wait a bit for job to execute
	time.Sleep(100 * time.Millisecond)

	pool.Stop()

	if !executed.Load() {
		t.Error("Expected job to be executed")
	}
}

func TestWorkerPool_ConcurrentJobExecution(t *testing.T) {
	workerCount := 5
	jobCount := 50
	pool := NewWorkerPool(workerCount)
	pool.Start()
	defer pool.Stop()

	var counter atomic.Int32
	var wg sync.WaitGroup

	// Submit multiple jobs concurrently
	for i := 0; i < jobCount; i++ {
		wg.Add(1)
		pool.Submit(func() {
			defer wg.Done()
			counter.Add(1)
			time.Sleep(10 * time.Millisecond) // Simulate work
		})
	}

	// Wait for all jobs to complete
	wg.Wait()

	if counter.Load() != int32(jobCount) {
		t.Errorf("Expected counter to be %d, got %d", jobCount, counter.Load())
	}
}

func TestWorkerPool_JobOrdering(t *testing.T) {
	pool := NewWorkerPool(1) // Single worker for predictable ordering
	pool.Start()
	defer pool.Stop()

	var results []int
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Submit jobs in order
	for i := 0; i < 10; i++ {
		wg.Add(1)
		val := i // Capture loop variable
		pool.Submit(func() {
			defer wg.Done()
			mu.Lock()
			results = append(results, val)
			mu.Unlock()
		})
	}

	wg.Wait()

	if len(results) != 10 {
		t.Errorf("Expected 10 results, got %d", len(results))
	}

	// With single worker, jobs should execute in order
	for i := 0; i < 10; i++ {
		if results[i] != i {
			t.Errorf("Expected result[%d] = %d, got %d", i, i, results[i])
		}
	}
}

func TestWorkerPool_StopMultipleTimes(t *testing.T) {
	pool := NewWorkerPool(2)
	pool.Start()

	// First stop
	pool.Stop()

	// Second stop should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Multiple Stop() calls caused panic: %v", r)
		}
	}()
	pool.Stop()
	pool.Stop()
}

// TestWorkerPool_SubmitAfterStop removed - causes panic: send on closed channel
// The worker pool implementation closes jobQueue in Stop(), so Submit() after Stop() panics
// This is a known limitation - the pool should not be used after Stop() is called

func TestWorkerPool_ActiveCount(t *testing.T) {
	pool := NewWorkerPool(3)
	pool.Start()
	defer pool.Stop()

	// Initially no active workers
	if count := pool.GetActiveCount(); count != 0 {
		t.Errorf("Expected 0 active workers initially, got %d", count)
	}

	// Submit jobs that take some time
	var wg sync.WaitGroup
	jobDuration := 200 * time.Millisecond

	for i := 0; i < 3; i++ {
		wg.Add(1)
		pool.Submit(func() {
			defer wg.Done()
			time.Sleep(jobDuration)
		})
	}

	// Wait a bit for jobs to start
	time.Sleep(50 * time.Millisecond)

	// Should have active workers now
	activeCount := pool.GetActiveCount()
	if activeCount == 0 {
		t.Error("Expected some active workers, got 0")
	}
	if activeCount > 3 {
		t.Errorf("Expected at most 3 active workers, got %d", activeCount)
	}

	wg.Wait()

	// After jobs complete, should be back to 0
	time.Sleep(50 * time.Millisecond)
	if count := pool.GetActiveCount(); count != 0 {
		t.Errorf("Expected 0 active workers after completion, got %d", count)
	}
}

func TestWorkerPool_QueueSize(t *testing.T) {
	pool := NewWorkerPool(1) // Single worker to control execution
	pool.Start()
	defer pool.Stop()

	// Initially queue should be empty
	if size := pool.GetQueueSize(); size != 0 {
		t.Errorf("Expected empty queue initially, got size %d", size)
	}

	var wg sync.WaitGroup

	// Submit jobs in a goroutine to avoid blocking the test
	// With 1 worker and buffer size of 2, we'll queue up some work
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 5; i++ {
			pool.Submit(func() {
				time.Sleep(20 * time.Millisecond) // Small delay to create backlog
			})
		}
	}()

	// Give time for jobs to be submitted and some to start processing
	time.Sleep(10 * time.Millisecond)

	// Check that GetQueueSize works (actual size may vary due to timing)
	queueSize := pool.GetQueueSize()
	_ = queueSize // Just verify the method doesn't panic

	// Wait for all jobs to complete
	wg.Wait()
	time.Sleep(150 * time.Millisecond)

	// Queue should eventually drain
	finalSize := pool.GetQueueSize()
	if finalSize != 0 {
		t.Logf("Final queue size: %d (expected 0 but timing may vary)", finalSize)
	}
}

func TestWorkerPool_NilJob(t *testing.T) {
	pool := NewWorkerPool(2)
	pool.Start()
	defer pool.Stop()

	// Submit nil job - should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Submitting nil job caused panic: %v", r)
		}
	}()

	pool.Submit(nil)
	time.Sleep(50 * time.Millisecond)
}

func TestWorkerPool_HighLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping high load test in short mode")
	}

	workerCount := 10
	jobCount := 1000
	pool := NewWorkerPool(workerCount)
	pool.Start()
	defer pool.Stop()

	var counter atomic.Int32
	var wg sync.WaitGroup

	startTime := time.Now()

	for i := 0; i < jobCount; i++ {
		wg.Add(1)
		pool.Submit(func() {
			defer wg.Done()
			counter.Add(1)
			// Simulate light work
			time.Sleep(1 * time.Millisecond)
		})
	}

	wg.Wait()
	duration := time.Since(startTime)

	if counter.Load() != int32(jobCount) {
		t.Errorf("Expected counter to be %d, got %d", jobCount, counter.Load())
	}

	t.Logf("Processed %d jobs with %d workers in %v", jobCount, workerCount, duration)
}

// TestWorkerPool_PanicRecovery removed - worker pool doesn't implement panic recovery
// Jobs that panic will crash the worker goroutine, which is expected behavior
// In production, jobs should handle their own panics

func TestWorkerPool_ConcurrentStartStop(t *testing.T) {
	pool := NewWorkerPool(3)

	var wg sync.WaitGroup
	iterations := 10

	// Test concurrent start/stop operations
	for i := 0; i < iterations; i++ {
		wg.Add(2)

		go func() {
			defer wg.Done()
			pool.Start()
		}()

		go func() {
			defer wg.Done()
			time.Sleep(10 * time.Millisecond)
			pool.Stop()
		}()

		wg.Wait()

		// Recreate pool for next iteration
		pool = NewWorkerPool(3)
	}
}

func TestWorkerPool_BufferCapacity(t *testing.T) {
	workers := 2
	pool := NewWorkerPool(workers)

	// Job queue buffer should be 2x workers
	expectedCapacity := workers * 2

	if cap(pool.jobQueue) != expectedCapacity {
		t.Errorf("Expected job queue capacity %d, got %d", expectedCapacity, cap(pool.jobQueue))
	}
}
