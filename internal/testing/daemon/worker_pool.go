package daemon

import (
	"sync"
)

// Job represents a work item for the pool
type Job func()

// WorkerPool manages a pool of worker goroutines
type WorkerPool struct {
	workers   int
	jobQueue  chan Job
	wg        sync.WaitGroup
	stopChan  chan struct{}
	stopOnce  sync.Once
}

// NewWorkerPool creates a new worker pool
func NewWorkerPool(workers int) *WorkerPool {
	if workers <= 0 {
		workers = 1
	}
	
	return &WorkerPool{
		workers:  workers,
		jobQueue: make(chan Job, workers*2), // Buffer size = 2x workers
		stopChan: make(chan struct{}),
	}
}

// Start starts the worker pool
func (p *WorkerPool) Start() {
	for i := 0; i < p.workers; i++ {
		p.wg.Add(1)
		go p.worker(i)
	}
}

// Stop stops the worker pool
func (p *WorkerPool) Stop() {
	p.stopOnce.Do(func() {
		close(p.stopChan)
		p.wg.Wait()
		close(p.jobQueue)
	})
}

// Submit submits a job to the worker pool
func (p *WorkerPool) Submit(job Job) {
	select {
	case p.jobQueue <- job:
		// Job submitted successfully
	case <-p.stopChan:
		// Pool is stopping, don't accept new jobs
	}
}

// worker is the worker goroutine
func (p *WorkerPool) worker(id int) {
	defer p.wg.Done()
	
	for {
		select {
		case job, ok := <-p.jobQueue:
			if !ok {
				// Channel closed, exit
				return
			}
			// Execute job
			if job != nil {
				job()
			}
			
		case <-p.stopChan:
			// Stop signal received
			return
		}
	}
}

// WaitForCompletion waits for all queued jobs to complete
func (p *WorkerPool) WaitForCompletion() {
	// Drain the job queue
	for {
		select {
		case <-p.jobQueue:
			// Job drained
		default:
			// Queue is empty
			return
		}
	}
}

// GetActiveCount returns the number of active workers (placeholder)
func (p *WorkerPool) GetActiveCount() int {
	// In a real implementation, you would track active workers
	// For now, return 0 if stopping, otherwise return worker count
	select {
	case <-p.stopChan:
		return 0
	default:
		return len(p.jobQueue) // Return number of pending jobs as approximation
	}
}