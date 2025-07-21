package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"nodelistdb/internal/concurrent"
	"nodelistdb/internal/database"
	"nodelistdb/internal/parser"
	"nodelistdb/internal/storage"
)

func main() {
	// Command line flags
	var (
		dbPath     = flag.String("db", "./nodelist.duckdb", "Path to DuckDB database file")
		path       = flag.String("path", "", "Path to nodelist file or directory (required)")
		recursive  = flag.Bool("recursive", false, "Scan directories recursively")
		verbose    = flag.Bool("verbose", false, "Verbose output")
		batchSize  = flag.Int("batch", 1000, "Batch size for bulk inserts")
		workers    = flag.Int("workers", 4, "Number of concurrent workers")
		enableConcurrent = flag.Bool("concurrent", false, "Enable concurrent processing")
	)
	flag.Parse()

	if *path == "" {
		fmt.Fprintf(os.Stderr, "Error: -path is required\n")
		flag.Usage()
		os.Exit(1)
	}

	fmt.Println("FidoNet Nodelist Parser (DuckDB)")
	fmt.Println("================================")
	fmt.Printf("Database: %s\n", *dbPath)
	fmt.Printf("Path: %s\n", *path)
	fmt.Printf("Batch size: %d\n", *batchSize)
	fmt.Printf("Workers: %d\n", *workers)
	fmt.Printf("Concurrent: %t\n", *enableConcurrent)
	fmt.Printf("Verbose: %t\n", *verbose)
	fmt.Println()

	// Initialize database
	if *verbose {
		log.Println("Initializing DuckDB database...")
	}

	db, err := database.New(*dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	// Get DuckDB version
	if version, err := db.GetVersion(); err == nil {
		fmt.Printf("DuckDB version: %s\n", version)
	}

	// Run migrations
	if err := db.Migrate(); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	if *verbose {
		log.Println("Database initialized successfully")
	}

	// Initialize storage layer
	storage, err := storage.New(db)
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}
	defer storage.Close()

	// Initialize advanced parser
	nodelistParser := parser.NewAdvanced(*verbose)

	// Find nodelist files
	files, err := findNodelistFiles(*path, *recursive)
	if err != nil {
		log.Fatalf("Failed to find nodelist files: %v", err)
	}

	if len(files) == 0 {
		fmt.Printf("No nodelist files found in: %s\n", *path)
		return
	}

	fmt.Printf("Found %d nodelist files to process\n", len(files))
	if *verbose {
		for i, file := range files {
			fmt.Printf("  %d: %s\n", i+1, filepath.Base(file))
		}
	}
	fmt.Println()

	// Process files
	startTime := time.Now()
	
	ctx := context.Background()
	
	if *enableConcurrent && len(files) > 1 {
		// Use concurrent processing
		fmt.Printf("Using concurrent processing with %d workers\n", *workers)
		processor := concurrent.New(storage, nodelistParser, *workers, *batchSize, *verbose)
		
		err := processor.ProcessFiles(ctx, files)
		if err != nil {
			log.Fatalf("Concurrent processing failed: %v", err)
		}
	} else {
		// Use sequential processing  
		fmt.Println("Using sequential processing")
		totalNodes := 0
		filesProcessed := 0

		for i, filePath := range files {
			fmt.Printf("[%d/%d] Processing: %s\n", i+1, len(files), filepath.Base(filePath))

			// Parse file
			nodes, err := nodelistParser.ParseFile(filePath)
			if err != nil {
				fmt.Printf("  ERROR: %v\n", err)
				continue
			}

			if len(nodes) == 0 {
				fmt.Println("  No nodes found in file")
				continue
			}

			// Process nodes in batches, but only from current file
			for i := 0; i < len(nodes); i += *batchSize {
				end := i + *batchSize
				if end > len(nodes) {
					end = len(nodes)
				}
				
				batch := nodes[i:end]
				if err := insertBatch(storage, batch, *verbose); err != nil {
					fmt.Printf("  ERROR inserting batch: %v\n", err)
					break // Skip remaining batches from this file
				}
				totalNodes += len(batch)
			}

			filesProcessed++
			fmt.Printf("  ✓ Parsed %d nodes\n", len(nodes))
		}
		
		// Summary for sequential processing
		fmt.Printf("Files processed: %d/%d\n", filesProcessed, len(files))
		fmt.Printf("Total nodes imported: %d\n", totalNodes)
		if totalNodes > 0 {
			duration := time.Since(startTime)
			fmt.Printf("Average: %.2f nodes/second\n", float64(totalNodes)/duration.Seconds())
		}
	}

	// Overall summary
	duration := time.Since(startTime)
	fmt.Println()
	fmt.Println("Processing completed!")
	fmt.Printf("Processing time: %v\n", duration)
}

// findNodelistFiles finds all nodelist files in the specified path
func findNodelistFiles(path string, recursive bool) ([]string, error) {
	var files []string

	// Check if path is a file or directory
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("path not found: %s", path)
	}

	if !info.IsDir() {
		// Single file
		if isNodelistFile(path) {
			files = append(files, path)
		}
		return files, nil
	}

	// Directory - walk through files
	walkFunc := func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories unless recursive
		if info.IsDir() {
			if !recursive && filePath != path {
				return filepath.SkipDir
			}
			return nil
		}

		// Check if it's a nodelist file
		if isNodelistFile(filePath) {
			files = append(files, filePath)
		}

		return nil
	}

	if err := filepath.Walk(path, walkFunc); err != nil {
		return nil, fmt.Errorf("error walking directory: %w", err)
	}

	return files, nil
}

// isNodelistFile checks if a file is a nodelist file based on naming patterns
func isNodelistFile(filePath string) bool {
	filename := strings.ToLower(filepath.Base(filePath))
	
	// Common nodelist filename patterns
	patterns := []string{
		"nodelist",
		"nodelist.",
	}

	for _, pattern := range patterns {
		if strings.HasPrefix(filename, pattern) {
			return true
		}
	}

	return false
}

// insertBatch inserts a batch of nodes into storage
func insertBatch(storage *storage.Storage, batch []database.Node, verbose bool) error {
	if verbose {
		fmt.Printf("  Inserting batch of %d nodes...\n", len(batch))
	}

	start := time.Now()
	if err := storage.InsertNodes(batch); err != nil {
		return err
	}

	if verbose {
		fmt.Printf("  ✓ Batch inserted in %v\n", time.Since(start))
	}

	return nil
}