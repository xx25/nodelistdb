package concurrent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/nodelistdb/internal/database"
	"github.com/nodelistdb/internal/parser"
)

// StorageInterface defines the interface for storage operations
type StorageInterface interface {
	InsertNodes([]database.Node) error
	IsNodelistProcessed(time.Time) (bool, error)
	FindConflictingNode(int, int, int, time.Time) (bool, error)
}

// MultiProcessor manages concurrent file processing with generic storage interface
type MultiProcessor struct {
	storage    StorageInterface
	parser     *parser.Parser
	numWorkers int
	batchSize  int
	verbose    bool
	quiet      bool
}

// NewMultiProcessor creates a new concurrent processor with generic storage interface
func NewMultiProcessor(storage StorageInterface, parser *parser.Parser, numWorkers int, batchSize int, verbose bool, quiet bool) *MultiProcessor {
	return &MultiProcessor{
		storage:    storage,
		parser:     parser,
		numWorkers: numWorkers,
		batchSize:  batchSize,
		verbose:    verbose,
		quiet:      quiet,
	}
}

// ProcessFiles processes multiple files concurrently
func (p *MultiProcessor) ProcessFiles(ctx context.Context, files []string) error {
	if len(files) == 0 {
		return nil
	}

	startTime := time.Now()
	jobs := make(chan Job, len(files))
	results := make(chan Result, len(files))

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < p.numWorkers; i++ {
		wg.Add(1)
		go p.worker(ctx, jobs, results, &wg)
	}

	// Send jobs
	go func() {
		defer close(jobs)
		for i, filePath := range files {
			select {
			case jobs <- Job{FilePath: filePath, JobID: i + 1}:
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

	// Process results
	totalNodes := 0
	processedFiles := 0
	var errors []error

	for result := range results {
		if result.Error != nil {
			if !p.quiet {
				fmt.Printf("  ERROR processing %s: %v\n", result.FilePath, result.Error)
			}
			errors = append(errors, result.Error)
			continue
		}

		totalNodes += result.NodesCount
		processedFiles++

		// Calculate ETA
		elapsed := time.Since(startTime)
		var etaStr string
		if processedFiles > 0 {
			avgTimePerFile := elapsed / time.Duration(processedFiles)
			remaining := time.Duration(len(files)-processedFiles) * avgTimePerFile
			etaStr = fmt.Sprintf(" (ETA: %v)", remaining.Round(time.Second))
		}

		if p.verbose {
			fmt.Printf("  ✓ [%d/%d] %s: %d nodes in %v%s\n", 
				processedFiles, len(files), result.FilePath, result.NodesCount, result.Duration, etaStr)
		} else if !p.quiet {
			fmt.Printf("  ✓ [%d/%d] %s: %d nodes%s\n", 
				processedFiles, len(files), result.FilePath, result.NodesCount, etaStr)
		}
	}

	// Summary
	if !p.quiet {
		duration := time.Since(startTime)
		fmt.Printf("\nConcurrent processing completed!\n")
		fmt.Printf("Files processed: %d/%d\n", processedFiles, len(files))
		fmt.Printf("Total nodes imported: %d\n", totalNodes)
		if totalNodes > 0 {
			fmt.Printf("Average: %.2f nodes/second\n", float64(totalNodes)/duration.Seconds())
		}
		fmt.Printf("Processing time: %v\n", duration)
	}

	if len(errors) > 0 {
		return fmt.Errorf("processing failed for %d files", len(errors))
	}

	return nil
}

// worker processes jobs from the jobs channel
func (p *MultiProcessor) worker(ctx context.Context, jobs <-chan Job, results chan<- Result, wg *sync.WaitGroup) {
	defer wg.Done()

	for {
		select {
		case job, ok := <-jobs:
			if !ok {
				return
			}

			result := p.processFile(ctx, job)
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

// processFile processes a single file
func (p *MultiProcessor) processFile(ctx context.Context, job Job) Result {
	startTime := time.Now()
	result := Result{
		JobID:    job.JobID,
		FilePath: job.FilePath,
		Duration: 0,
	}

	// Parse file
	parseResult, err := p.parser.ParseFileWithCRC(job.FilePath)
	if err != nil {
		result.Error = fmt.Errorf("parse failed: %w", err)
		result.Duration = time.Since(startTime)
		return result
	}

	nodes := parseResult.Nodes
	if len(nodes) == 0 {
		result.Duration = time.Since(startTime)
		return result
	}

	// Check if already processed
	if len(nodes) > 0 && nodes[0].NodelistDate.Year() > 1900 {
		nodelistDate := nodes[0].NodelistDate
		isProcessed, err := p.storage.IsNodelistProcessed(nodelistDate)
		if err != nil {
			result.Error = fmt.Errorf("failed to check if processed: %w", err)
			result.Duration = time.Since(startTime)
			return result
		}
		if isProcessed {
			if p.verbose {
				fmt.Printf("  [%d] ALREADY IMPORTED: %s (date: %s)\n", 
					job.JobID, job.FilePath, nodelistDate.Format("2006-01-02"))
			}
			result.Duration = time.Since(startTime)
			return result
		}
	}

	// Process nodes in batches
	totalInserted := 0
	for i := 0; i < len(nodes); i += p.batchSize {
		end := i + p.batchSize
		if end > len(nodes) {
			end = len(nodes)
		}

		batch := nodes[i:end]
		if err := p.storage.InsertNodes(batch); err != nil {
			result.Error = fmt.Errorf("insert batch failed: %w", err)
			result.Duration = time.Since(startTime)
			return result
		}
		totalInserted += len(batch)

		// Check for cancellation
		select {
		case <-ctx.Done():
			result.Error = ctx.Err()
			result.Duration = time.Since(startTime)
			return result
		default:
		}
	}

	result.NodesCount = totalInserted
	result.Duration = time.Since(startTime)
	return result
}