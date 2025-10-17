package concurrent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/nodelistdb/internal/database"
	"github.com/nodelistdb/internal/parser"
	"github.com/nodelistdb/internal/storage"
)

// Job represents a file processing job
type Job struct {
	FilePath string
	JobID    int
}

// Result represents the result of processing a job
type Result struct {
	JobID        int
	FilePath     string
	Nodes        []database.Node
	Error        error
	Duration     time.Duration
	NodesCount   int
	NodelistDate time.Time // Date of the nodelist for flag statistics updates
}

// Processor manages concurrent file processing
type Processor struct {
	storage    *storage.Storage
	parser     *parser.Parser
	numWorkers int
	batchSize  int
	verbose    bool
	quiet      bool
}

// New creates a new concurrent processor
func New(storage *storage.Storage, parser *parser.Parser, numWorkers int, batchSize int, verbose bool, quiet bool) *Processor {
	return &Processor{
		storage:    storage,
		parser:     parser,
		numWorkers: numWorkers,
		batchSize:  batchSize,
		verbose:    verbose,
		quiet:      quiet,
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

	// Create a dedicated parser instance for this worker to avoid shared state issues
	workerParser := parser.NewAdvanced(p.verbose)

	for {
		select {
		case job, ok := <-jobs:
			if !ok {
				if p.verbose {
					fmt.Printf("Worker %d finished\n", workerID)
				}
				return
			}

			// Process the job with the worker's dedicated parser
			result := p.processJobWithParser(ctx, job, workerParser)

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

// processJobWithParser processes a single job using the provided parser instance
func (p *Processor) processJobWithParser(ctx context.Context, job Job, workerParser *parser.Parser) Result {
	start := time.Now()

	if !p.quiet {
		fmt.Printf("[Job %d] Processing: %s\n", job.JobID, job.FilePath)
	}

	// Parse the file with the worker's dedicated parser
	parseResult, err := workerParser.ParseFileWithCRC(job.FilePath)
	duration := time.Since(start)

	if err != nil {
		fmt.Printf("[Job %d] ERROR: %v\n", job.JobID, err)
		return Result{
			JobID:        job.JobID,
			FilePath:     job.FilePath,
			Error:        err,
			Duration:     duration,
			NodesCount:   0,
			NodelistDate: time.Time{},
		}
	}

	nodes := parseResult.Nodes
	if len(nodes) == 0 {
		if !p.quiet {
			fmt.Printf("[Job %d] No nodes found in file\n", job.JobID)
		}
	} else if !p.quiet {
		fmt.Printf("[Job %d] ✓ Parsed %d nodes\n", job.JobID, len(nodes))
	}

	return Result{
		JobID:        job.JobID,
		FilePath:     job.FilePath,
		Nodes:        nodes,
		Duration:     duration,
		NodesCount:   len(nodes),
		NodelistDate: parseResult.NodelistDate,
	}
}

// collectResults collects all results and performs batch inserts
func (p *Processor) collectResults(ctx context.Context, results <-chan Result, expectedResults int) error {
	var totalNodes int
	var totalErrors int
	var batch []database.Node
	var totalInserted int
	var batchCount int

	successfulJobs := 0
	startTime := time.Now()
	var avgInsertionTime time.Duration
	var insertionCount int

	// Track unique nodelist dates for flag statistics updates
	uniqueDates := make(map[time.Time]bool)

	if !p.quiet {
		fmt.Printf("\n=== DATABASE INSERTION PHASE ===\n")
		fmt.Printf("Collecting results from %d jobs and performing batch inserts...\n", expectedResults)
	}

	for result := range results {
		if result.Error != nil {
			totalErrors++
			fmt.Printf("ERROR processing %s: %v\n", result.FilePath, result.Error)
			continue
		}

		successfulJobs++
		totalNodes += result.NodesCount

		// Track nodelist date for flag statistics updates
		if !result.NodelistDate.IsZero() {
			uniqueDates[result.NodelistDate] = true
		}

		// Add nodes to batch
		batch = append(batch, result.Nodes...)

		// Progress update every 10 jobs
		if successfulJobs%10 == 0 || successfulJobs == expectedResults-totalErrors {
			elapsed := time.Since(startTime)
			progress := float64(successfulJobs) / float64(expectedResults-totalErrors) * 100
			if progress > 0 && !p.quiet {
				// Calculate ETA based on both collection and insertion rates
				var eta string
				if insertionCount > 0 && avgInsertionTime > 0 {
					// Estimate remaining nodes and batches
					nodesPerJob := float64(totalNodes) / float64(successfulJobs)
					remainingJobs := expectedResults - totalErrors - successfulJobs
					estimatedRemainingNodes := int(nodesPerJob * float64(remainingJobs))
					totalExpectedNodes := totalNodes + estimatedRemainingNodes
					remainingNodesToInsert := totalExpectedNodes - totalInserted
					estimatedBatchesRemaining := (remainingNodesToInsert + p.batchSize - 1) / p.batchSize

					// Calculate ETA based on average insertion time
					estimatedRemainingTime := time.Duration(estimatedBatchesRemaining) * avgInsertionTime
					eta = estimatedRemainingTime.Round(time.Second).String()
				} else {
					// Fallback to collection-based ETA
					estimatedTotal := elapsed * time.Duration(float64(expectedResults-totalErrors)/float64(successfulJobs))
					remaining := estimatedTotal - elapsed
					eta = remaining.Round(time.Second).String()
				}

				fmt.Printf("Collection progress: %d/%d jobs (%.1f%%) - %d nodes collected, %d inserted - ETA: %v\n",
					successfulJobs, expectedResults-totalErrors, progress, totalNodes, totalInserted, eta)
			}
		}

		// Insert batch when it reaches batch size or this is the last successful result
		if len(batch) >= p.batchSize || (successfulJobs == expectedResults-totalErrors && len(batch) > 0) {
			batchCount++

			insertStart := time.Now()
			if err := p.insertBatchWithProgress(ctx, batch, batchCount, totalInserted, totalNodes); err != nil {
				return fmt.Errorf("failed to insert batch: %w", err)
			}
			insertDuration := time.Since(insertStart)

			// Update average insertion time
			insertionCount++
			// Running average
			avgInsertionTime = (avgInsertionTime*time.Duration(insertionCount-1) + insertDuration) / time.Duration(insertionCount)

			totalInserted += len(batch)
			batch = nil // Reset batch
		}
	}

	// Insert any remaining batch
	if len(batch) > 0 {
		batchCount++

		insertStart := time.Now()
		if err := p.insertBatchWithProgress(ctx, batch, batchCount, totalInserted, totalNodes); err != nil {
			return fmt.Errorf("failed to insert final batch: %w", err)
		}
		_ = time.Since(insertStart) // Track duration but no longer used for ETA after loop completes

		totalInserted += len(batch)
	}

	if !p.quiet {
		fmt.Printf("\n✓ Database insertion complete: %d batches, %d nodes inserted in %v\n",
			batchCount, totalInserted, time.Since(startTime).Round(time.Second))
		fmt.Printf("Concurrent processing complete: %d jobs processed, %d nodes imported, %d errors\n",
			successfulJobs, totalNodes, totalErrors)
	}

	// Update flag statistics for all unique nodelist dates
	if len(uniqueDates) > 0 {
		if !p.quiet {
			fmt.Printf("\nUpdating flag analytics for %d unique nodelist dates...\n", len(uniqueDates))
		}
		updateStart := time.Now()
		updatedCount := 0
		for date := range uniqueDates {
			if p.verbose {
				fmt.Printf("  Updating flag statistics for %s...\n", date.Format("2006-01-02"))
			}
			if err := p.storage.UpdateFlagStatistics(date); err != nil {
				fmt.Printf("  Warning: Failed to update flag statistics for %s: %v\n", date.Format("2006-01-02"), err)
				// Non-fatal error - continue with other dates
			} else {
				updatedCount++
				if p.verbose {
					fmt.Printf("  ✓ Flag statistics updated for %s\n", date.Format("2006-01-02"))
				}
			}
		}
		if !p.quiet {
			fmt.Printf("✓ Flag analytics updated for %d/%d dates in %v\n",
				updatedCount, len(uniqueDates), time.Since(updateStart).Round(time.Millisecond))
		}
	}

	if totalErrors > 0 {
		return fmt.Errorf("%d jobs failed during processing", totalErrors)
	}

	return nil
}

// insertBatchWithProgress inserts a batch of nodes with detailed progress reporting
func (p *Processor) insertBatchWithProgress(ctx context.Context, batch []database.Node, batchNum, totalInserted, totalNodes int) error {
	if !p.quiet {
		fmt.Printf("Inserting batch %d: %d nodes...\n", batchNum, len(batch))
	}

	start := time.Now()
	err := p.storage.NodeOps().InsertNodes(batch)
	duration := time.Since(start)

	if err != nil {
		fmt.Printf("✗ Batch %d failed: %v\n", batchNum, err)
		return err
	}

	if !p.quiet {
		rate := float64(len(batch)) / duration.Seconds()
		fmt.Printf("✓ Batch %d inserted in %v (%.0f nodes/sec)\n", batchNum, duration.Round(time.Millisecond), rate)
	}

	return nil
}
