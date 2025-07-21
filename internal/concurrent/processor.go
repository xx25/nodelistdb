package concurrent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"nodelistdb/internal/database"
	"nodelistdb/internal/parser"
	"nodelistdb/internal/storage"
)

// Job represents a file processing job
type Job struct {
	FilePath string
	JobID    int
}

// Result represents the result of processing a job
type Result struct {
	JobID     int
	FilePath  string
	Nodes     []database.Node
	Error     error
	Duration  time.Duration
	NodesCount int
}

// Processor manages concurrent file processing
type Processor struct {
	storage    *storage.Storage
	parser     *parser.AdvancedParser
	numWorkers int
	batchSize  int
	verbose    bool
}

// New creates a new concurrent processor
func New(storage *storage.Storage, parser *parser.AdvancedParser, numWorkers int, batchSize int, verbose bool) *Processor {
	return &Processor{
		storage:    storage,
		parser:     parser,
		numWorkers: numWorkers,
		batchSize:  batchSize,
		verbose:    verbose,
	}
}

// ProcessFiles processes multiple files concurrently
func (p *Processor) ProcessFiles(ctx context.Context, filePaths []string) error {
	if len(filePaths) == 0 {
		return nil
	}

	// Create job and result channels
	jobs := make(chan Job, len(filePaths))
	results := make(chan Result, len(filePaths))

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < p.numWorkers; i++ {
		wg.Add(1)
		go p.worker(ctx, i, jobs, results, &wg)
	}

	// Send jobs
	go func() {
		defer close(jobs)
		for i, filePath := range filePaths {
			select {
			case jobs <- Job{FilePath: filePath, JobID: i}:
			case <-ctx.Done():
				return
			}
		}
	}()

	// Collect results
	go func() {
		wg.Wait()
		close(results)
	}()

	// Process results and batch insert
	return p.collectResults(ctx, results, len(filePaths))
}

// worker processes jobs from the jobs channel
func (p *Processor) worker(ctx context.Context, workerID int, jobs <-chan Job, results chan<- Result, wg *sync.WaitGroup) {
	defer wg.Done()

	if p.verbose {
		fmt.Printf("Worker %d started\n", workerID)
	}

	for {
		select {
		case job, ok := <-jobs:
			if !ok {
				if p.verbose {
					fmt.Printf("Worker %d finished\n", workerID)
				}
				return
			}
			
			// Process the job
			result := p.processJob(ctx, job)
			
			select {
			case results <- result:
			case <-ctx.Done():
				return
			}
			
		case <-ctx.Done():
			return
		}
	}
}

// processJob processes a single job
func (p *Processor) processJob(ctx context.Context, job Job) Result {
	start := time.Now()
	
	fmt.Printf("[Job %d] Processing: %s\n", job.JobID, job.FilePath)

	// Parse the file
	nodes, err := p.parser.ParseFile(job.FilePath)
	duration := time.Since(start)

	if err != nil {
		fmt.Printf("[Job %d] ERROR: %v\n", job.JobID, err)
		return Result{
			JobID:      job.JobID,
			FilePath:   job.FilePath,
			Error:      err,
			Duration:   duration,
			NodesCount: 0,
		}
	}

	if len(nodes) == 0 {
		fmt.Printf("[Job %d] No nodes found in file\n", job.JobID)
	} else {
		fmt.Printf("[Job %d] ✓ Parsed %d nodes\n", job.JobID, len(nodes))
	}

	return Result{
		JobID:      job.JobID,
		FilePath:   job.FilePath,
		Nodes:      nodes,
		Duration:   duration,
		NodesCount: len(nodes),
	}
}

// collectResults collects all results and performs batch inserts
func (p *Processor) collectResults(ctx context.Context, results <-chan Result, expectedResults int) error {
	var totalNodes int
	var totalErrors int
	var batch []database.Node
	
	successfulJobs := 0
	
	for result := range results {
		if result.Error != nil {
			totalErrors++
			fmt.Printf("ERROR processing %s: %v\n", result.FilePath, result.Error)
			continue
		}
		
		successfulJobs++
		totalNodes += result.NodesCount
		
		// Add nodes to batch
		batch = append(batch, result.Nodes...)
		
		// Insert batch when it reaches batch size or this is the last successful result
		if len(batch) >= p.batchSize || (successfulJobs == expectedResults-totalErrors && len(batch) > 0) {
			if err := p.insertBatch(ctx, batch); err != nil {
				return fmt.Errorf("failed to insert batch: %w", err)
			}
			batch = nil // Reset batch
		}
	}
	
	// Insert any remaining batch
	if len(batch) > 0 {
		if err := p.insertBatch(ctx, batch); err != nil {
			return fmt.Errorf("failed to insert final batch: %w", err)
		}
	}
	
	fmt.Printf("Concurrent processing complete: %d jobs processed, %d nodes imported, %d errors\n", 
		successfulJobs, totalNodes, totalErrors)
	
	if totalErrors > 0 {
		return fmt.Errorf("%d jobs failed during processing", totalErrors)
	}
	
	return nil
}

// insertBatch inserts a batch of nodes
func (p *Processor) insertBatch(ctx context.Context, batch []database.Node) error {
	if p.verbose {
		fmt.Printf("Inserting batch of %d nodes...\n", len(batch))
	}
	
	start := time.Now()
	err := p.storage.InsertNodes(batch)
	duration := time.Since(start)
	
	if err != nil {
		return err
	}
	
	if p.verbose {
		fmt.Printf("✓ Batch inserted in %v\n", duration)
	}
	
	return nil
}