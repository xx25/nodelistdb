package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/nodelistdb/internal/concurrent"
	"github.com/nodelistdb/internal/config"
	"github.com/nodelistdb/internal/database"
	"github.com/nodelistdb/internal/parser"
	"github.com/nodelistdb/internal/storage"
	"github.com/nodelistdb/internal/version"
)

func main() {
	// Command line flags
	var (
		configPath         = flag.String("config", "config.yaml", "Path to configuration file")
		dbPath             = flag.String("db", "", "Path to database file (overrides config)")
		dbType             = flag.String("dbtype", "", "Database type: 'duckdb' or 'clickhouse' (overrides config)")
		enableDual         = flag.Bool("dual", false, "Enable dual database mode (both DuckDB and ClickHouse)")
		clickhouseHost     = flag.String("ch-host", "localhost", "ClickHouse host (for dual mode)")
		clickhousePort     = flag.Int("ch-port", 9000, "ClickHouse port (for dual mode)")
		clickhouseDatabase = flag.String("ch-db", "nodelist", "ClickHouse database name (for dual mode)")
		clickhouseUser     = flag.String("ch-user", "default", "ClickHouse username (for dual mode)")
		clickhousePassword = flag.String("ch-password", "", "ClickHouse password (for dual mode)")
		path               = flag.String("path", "", "Path to nodelist file or directory (required)")
		recursive          = flag.Bool("recursive", false, "Scan directories recursively")
		verbose            = flag.Bool("verbose", false, "Verbose output")
		quiet              = flag.Bool("quiet", false, "Quiet mode - only print errors (useful for cron)")
		batchSize          = flag.Int("batch", 1000, "Batch size for bulk inserts")
		workers            = flag.Int("workers", 4, "Number of concurrent workers")
		enableConcurrent   = flag.Bool("concurrent", false, "Enable concurrent processing")
		createFTSIndexes   = flag.Bool("create-fts", true, "Create Full-Text Search indexes after import")
		rebuildFTSOnly     = flag.Bool("rebuild-fts", false, "Only rebuild FTS indexes (no data import)")
		showVersion        = flag.Bool("version", false, "Show version information")
	)
	flag.Parse()

	// Handle version flag
	if *showVersion {
		fmt.Printf("NodelistDB Parser %s\n", version.GetFullVersionInfo())
		os.Exit(0)
	}

	if *path == "" && !*rebuildFTSOnly {
		fmt.Fprintf(os.Stderr, "Error: -path is required (unless using -rebuild-fts)\n")
		flag.Usage()
		os.Exit(1)
	}

	// Load configuration
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Handle dual database mode
	if *enableDual {
		duckdbPath := "./nodelist.duckdb"
		if *dbPath != "" {
			duckdbPath = *dbPath
		}
		cfg = config.CreateDualDatabaseConfig(duckdbPath, *clickhouseHost, *clickhousePort, *clickhouseDatabase)
		// Override ClickHouse credentials if provided
		if cfg.Database.Databases != nil && len(cfg.Database.Databases) > 1 {
			cfg.Database.Databases[1].ClickHouse.Username = *clickhouseUser
			cfg.Database.Databases[1].ClickHouse.Password = *clickhousePassword
		}
	} else {
		// Override database type if specified via command line
		if *dbType != "" {
			switch *dbType {
			case "duckdb":
				cfg.Database.Type = config.DatabaseTypeDuckDB
			case "clickhouse":
				cfg.Database.Type = config.DatabaseTypeClickHouse
			default:
				log.Fatalf("Invalid database type: %s (must be 'duckdb' or 'clickhouse')", *dbType)
			}
		}
	}

	// Override database path if specified via command line (handle both single and multi-database configs)
	if *dbPath != "" {
		if cfg.IsMultiDatabase() {
			// Update DuckDB path in multi-database config
			databases := cfg.GetAllDatabases()
			for i, db := range databases {
				if db.Type == config.DatabaseTypeDuckDB && db.DuckDB != nil {
					cfg.Database.Databases[i].DuckDB.Path = *dbPath
					break
				}
			}
		} else if cfg.Database.Type == config.DatabaseTypeDuckDB && cfg.Database.DuckDB != nil {
			cfg.Database.DuckDB.Path = *dbPath
		}
	}


	if !*quiet {
		if cfg.IsMultiDatabase() {
			fmt.Println("FidoNet Nodelist Parser (Multi-Database Mode)")
			fmt.Println("============================================")
			databases := cfg.GetAllDatabases()
			for i, db := range databases {
				switch db.Type {
				case config.DatabaseTypeDuckDB:
					fmt.Printf("Database %d (%s): %s (DuckDB)\n", i+1, db.Name, db.DuckDB.Path)
				case config.DatabaseTypeClickHouse:
					fmt.Printf("Database %d (%s): %s:%d/%s (ClickHouse)\n", i+1, db.Name,
						db.ClickHouse.Host, db.ClickHouse.Port, db.ClickHouse.Database)
				}
			}
		} else {
			fmt.Printf("FidoNet Nodelist Parser (%s)\n", cfg.Database.Type)
			fmt.Println("================================")
			switch cfg.Database.Type {
			case config.DatabaseTypeDuckDB:
				fmt.Printf("Database: %s (DuckDB)\n", cfg.Database.DuckDB.Path)
			case config.DatabaseTypeClickHouse:
				fmt.Printf("Database: %s:%d/%s (ClickHouse)\n",
					cfg.Database.ClickHouse.Host, cfg.Database.ClickHouse.Port, cfg.Database.ClickHouse.Database)
			}
		}
		fmt.Printf("Path: %s\n", *path)
		fmt.Printf("Batch size: %d\n", *batchSize)
		fmt.Printf("Workers: %d\n", *workers)
		fmt.Printf("Concurrent: %t\n", *enableConcurrent)
		fmt.Printf("Verbose: %t\n", *verbose)
		fmt.Println()
	}

	// Initialize database(s) based on configuration
	var storageLayer interface {
		InsertNodes([]database.Node) error
		IsNodelistProcessed(time.Time) (bool, error)
		FindConflictingNode(int, int, int, time.Time) (bool, error)
		Close() error
	}

	if cfg.IsMultiDatabase() {
		// Multi-database mode
		if *verbose {
			log.Println("Initializing multiple databases...")
		}

		databases := make(map[string]database.DatabaseInterface)
		dbNames := cfg.GetAllDatabases()

		for _, dbConfig := range dbNames {
			var db database.DatabaseInterface

			switch dbConfig.Type {
			case config.DatabaseTypeDuckDB:
				db, err = database.NewWithPerfSettings(
					dbConfig.DuckDB.Path,
					dbConfig.DuckDB.MemoryLimit,
					dbConfig.DuckDB.TempDirectory,
					dbConfig.DuckDB.Threads,
					dbConfig.DuckDB.ReadOnly,
					dbConfig.DuckDB.BulkMode,
					dbConfig.DuckDB.CheckpointThreshold,
					dbConfig.DuckDB.WALAutoCheckpoint,
				)
			case config.DatabaseTypeClickHouse:
				chConfig := &database.ClickHouseConfig{
					Host:         dbConfig.ClickHouse.Host,
					Port:         dbConfig.ClickHouse.Port,
					Database:     dbConfig.ClickHouse.Database,
					Username:     dbConfig.ClickHouse.Username,
					Password:     dbConfig.ClickHouse.Password,
					UseSSL:       dbConfig.ClickHouse.UseSSL,
					MaxOpenConns: dbConfig.ClickHouse.MaxOpenConns,
					MaxIdleConns: dbConfig.ClickHouse.MaxIdleConns,
				}

				// Parse timeout strings
				if dbConfig.ClickHouse.DialTimeout != "" {
					if chConfig.DialTimeout, err = time.ParseDuration(dbConfig.ClickHouse.DialTimeout); err != nil {
						log.Fatalf("Invalid dial timeout for %s: %v", dbConfig.Name, err)
					}
				} else {
					chConfig.DialTimeout = 30 * time.Second
				}

				if dbConfig.ClickHouse.ReadTimeout != "" {
					if chConfig.ReadTimeout, err = time.ParseDuration(dbConfig.ClickHouse.ReadTimeout); err != nil {
						log.Fatalf("Invalid read timeout for %s: %v", dbConfig.Name, err)
					}
				} else {
					chConfig.ReadTimeout = 5 * time.Minute
				}

				if dbConfig.ClickHouse.WriteTimeout != "" {
					if chConfig.WriteTimeout, err = time.ParseDuration(dbConfig.ClickHouse.WriteTimeout); err != nil {
						log.Fatalf("Invalid write timeout for %s: %v", dbConfig.Name, err)
					}
				} else {
					chConfig.WriteTimeout = 1 * time.Minute
				}

				chConfig.Compression = dbConfig.ClickHouse.Compression
				db, err = database.NewClickHouse(chConfig)
			default:
				log.Fatalf("Unsupported database type for %s: %s", dbConfig.Name, dbConfig.Type)
			}

			if err != nil {
				log.Fatalf("Failed to initialize %s database: %v", dbConfig.Name, err)
			}

			// Get database version
			if version, err := db.GetVersion(); err == nil && !*quiet {
				fmt.Printf("%s (%s) version: %s\n", dbConfig.Name, dbConfig.Type, version)
			}

			// Create schema
			if err := db.CreateSchema(); err != nil {
				log.Fatalf("Failed to create schema for %s: %v", dbConfig.Name, err)
			}

			databases[dbConfig.Name] = db
		}

		if *verbose {
			log.Println("All databases initialized successfully")
		}

		// Initialize multi-storage layer
		multiStorage, err := storage.NewMultiStorage(databases)
		if err != nil {
			log.Fatalf("Failed to initialize multi-storage: %v", err)
		}
		storageLayer = multiStorage
		defer multiStorage.Close()

	} else {
		// Single database mode (legacy)
		if *verbose {
			log.Printf("Initializing %s database...", cfg.Database.Type)
		}

		var db database.DatabaseInterface
		switch cfg.Database.Type {
		case config.DatabaseTypeDuckDB:
			db, err = database.NewWithPerfSettings(
				cfg.Database.DuckDB.Path,
				cfg.Database.DuckDB.MemoryLimit,
				cfg.Database.DuckDB.TempDirectory,
				cfg.Database.DuckDB.Threads,
				cfg.Database.DuckDB.ReadOnly,
				cfg.Database.DuckDB.BulkMode,
				cfg.Database.DuckDB.CheckpointThreshold,
				cfg.Database.DuckDB.WALAutoCheckpoint,
			)
		case config.DatabaseTypeClickHouse:
			chConfig := &database.ClickHouseConfig{
				Host:         cfg.Database.ClickHouse.Host,
				Port:         cfg.Database.ClickHouse.Port,
				Database:     cfg.Database.ClickHouse.Database,
				Username:     cfg.Database.ClickHouse.Username,
				Password:     cfg.Database.ClickHouse.Password,
				UseSSL:       cfg.Database.ClickHouse.UseSSL,
				MaxOpenConns: cfg.Database.ClickHouse.MaxOpenConns,
				MaxIdleConns: cfg.Database.ClickHouse.MaxIdleConns,
			}

			// Parse timeout strings
			if cfg.Database.ClickHouse.DialTimeout != "" {
				if chConfig.DialTimeout, err = time.ParseDuration(cfg.Database.ClickHouse.DialTimeout); err != nil {
					log.Fatalf("Invalid dial timeout: %v", err)
				}
			} else {
				chConfig.DialTimeout = 30 * time.Second
			}

			if cfg.Database.ClickHouse.ReadTimeout != "" {
				if chConfig.ReadTimeout, err = time.ParseDuration(cfg.Database.ClickHouse.ReadTimeout); err != nil {
					log.Fatalf("Invalid read timeout: %v", err)
				}
			} else {
				chConfig.ReadTimeout = 5 * time.Minute
			}

			if cfg.Database.ClickHouse.WriteTimeout != "" {
				if chConfig.WriteTimeout, err = time.ParseDuration(cfg.Database.ClickHouse.WriteTimeout); err != nil {
					log.Fatalf("Invalid write timeout: %v", err)
				}
			} else {
				chConfig.WriteTimeout = 1 * time.Minute
			}

			chConfig.Compression = cfg.Database.ClickHouse.Compression
			db, err = database.NewClickHouse(chConfig)
		default:
			log.Fatalf("Unsupported database type: %s", cfg.Database.Type)
		}

		if err != nil {
			log.Fatalf("Failed to initialize database: %v", err)
		}
		defer db.Close()

		// Get database version
		if version, err := db.GetVersion(); err == nil && !*quiet {
			fmt.Printf("%s version: %s\n", cfg.Database.Type, version)
		}

		// Create schema
		if err := db.CreateSchema(); err != nil {
			log.Fatalf("Failed to create schema: %v", err)
		}

		if *verbose {
			log.Println("Database initialized successfully")
		}

		// Initialize single storage layer
		singleStorage, err := storage.New(db)
		if err != nil {
			log.Fatalf("Failed to initialize storage: %v", err)
		}
		storageLayer = singleStorage
		defer singleStorage.Close()
	}


	// Handle FTS rebuild mode
	if *rebuildFTSOnly {
		if !*quiet {
			fmt.Println("Rebuilding Full-Text Search indexes...")
		}

		if cfg.IsMultiDatabase() {
			// Handle FTS rebuild for multiple databases
			databases := cfg.GetAllDatabases()
			for _, dbConfig := range databases {
				if !*quiet {
					fmt.Printf("Rebuilding FTS indexes for %s...\n", dbConfig.Name)
				}
				
				var db database.DatabaseInterface
				switch dbConfig.Type {
				case config.DatabaseTypeDuckDB:
					db, err = database.NewWithPerfSettings(
						dbConfig.DuckDB.Path,
						dbConfig.DuckDB.MemoryLimit,
						dbConfig.DuckDB.TempDirectory,
						dbConfig.DuckDB.Threads,
						dbConfig.DuckDB.ReadOnly,
						dbConfig.DuckDB.BulkMode,
						dbConfig.DuckDB.CheckpointThreshold,
						dbConfig.DuckDB.WALAutoCheckpoint,
					)
				case config.DatabaseTypeClickHouse:
					chConfig, configErr := dbConfig.ClickHouse.ToClickHouseDatabaseConfig()
					if configErr != nil {
						log.Fatalf("Invalid ClickHouse config for %s: %v", dbConfig.Name, configErr)
					}
					db, err = database.NewClickHouse(chConfig)
				}
				
				if err != nil {
					log.Fatalf("Failed to connect to %s for FTS rebuild: %v", dbConfig.Name, err)
				}
				
				// Drop existing FTS indexes
				if err := db.DropFTSIndexes(); err != nil {
					log.Printf("Warning: Could not drop existing FTS indexes for %s: %v", dbConfig.Name, err)
				}

				// Create new FTS indexes
				if err := db.CreateFTSIndexes(); err != nil {
					log.Fatalf("Failed to create FTS indexes for %s: %v", dbConfig.Name, err)
				}
				
				db.Close()
			}
		} else {
			// Handle FTS rebuild for single database (already initialized above)
			var db database.DatabaseInterface
			switch cfg.Database.Type {
			case config.DatabaseTypeDuckDB:
				db, err = database.NewWithPerfSettings(
					cfg.Database.DuckDB.Path,
					cfg.Database.DuckDB.MemoryLimit,
					cfg.Database.DuckDB.TempDirectory,
					cfg.Database.DuckDB.Threads,
					cfg.Database.DuckDB.ReadOnly,
					cfg.Database.DuckDB.BulkMode,
					cfg.Database.DuckDB.CheckpointThreshold,
					cfg.Database.DuckDB.WALAutoCheckpoint,
				)
			case config.DatabaseTypeClickHouse:
				chConfig, configErr := cfg.Database.ClickHouse.ToClickHouseDatabaseConfig()
				if configErr != nil {
					log.Fatalf("Invalid ClickHouse config: %v", configErr)
				}
				db, err = database.NewClickHouse(chConfig)
			}
			
			if err != nil {
				log.Fatalf("Failed to connect to database for FTS rebuild: %v", err)
			}
			defer db.Close()
			
			// Drop existing FTS indexes
			if err := db.DropFTSIndexes(); err != nil {
				log.Printf("Warning: Could not drop existing FTS indexes: %v", err)
			}

			// Create new FTS indexes
			if err := db.CreateFTSIndexes(); err != nil {
				log.Fatalf("Failed to create FTS indexes: %v", err)
			}
		}

		if !*quiet {
			fmt.Println("FTS indexes rebuilt successfully!")
		}
		return
	}

	// Initialize advanced parser
	nodelistParser := parser.NewAdvanced(*verbose)

	// Find nodelist files
	files, err := findNodelistFiles(*path, *recursive)
	if err != nil {
		log.Fatalf("Failed to find nodelist files: %v", err)
	}

	if len(files) == 0 {
		if !*quiet {
			fmt.Printf("No nodelist files found in: %s\n", *path)
		}
		return
	}

	if !*quiet {
		fmt.Printf("Found %d nodelist files to process\n", len(files))
	}
	if *verbose {
		for i, file := range files {
			fmt.Printf("  %d: %s\n", i+1, filepath.Base(file))
		}
	}
	if !*quiet {
		fmt.Println()
	}

	// Process files
	startTime := time.Now()

	ctx := context.Background()

	if *enableConcurrent && len(files) > 1 {
		// Use concurrent processing
		if !*quiet {
			fmt.Printf("Using concurrent processing with %d workers\n", *workers)
		}
		processor := concurrent.NewMultiProcessor(storageLayer, nodelistParser, *workers, *batchSize, *verbose, *quiet)

		err := processor.ProcessFiles(ctx, files)
		if err != nil {
			log.Fatalf("Concurrent processing failed: %v", err)
		}
	} else {
		// Use sequential processing
		if !*quiet {
			fmt.Println("Using sequential processing")
		}
		totalNodes := 0
		filesProcessed := 0

		for i, filePath := range files {
			// Calculate ETA
			elapsed := time.Since(startTime)
			var etaStr string
			if i > 0 {
				avgTimePerFile := elapsed / time.Duration(i)
				remaining := time.Duration(len(files)-i) * avgTimePerFile
				etaStr = fmt.Sprintf(" (ETA: %v)", remaining.Round(time.Second))
			}

			if !*quiet {
				fmt.Printf("[%d/%d] Processing: %s%s\n", i+1, len(files), filePath, etaStr)
			}

			// Parse file
			parseResult, err := nodelistParser.ParseFileWithCRC(filePath)
			if err != nil {
				fmt.Printf("  ERROR: %v\n", err)
				continue
			}

			nodes := parseResult.Nodes
			if len(nodes) == 0 {
				if !*quiet {
					fmt.Println("  No nodes found in file")
				}
				continue
			}

			// Check if nodelist already processed based on date
			if len(nodes) > 0 && nodes[0].NodelistDate.Year() > 1900 {
				nodelistDate := nodes[0].NodelistDate
				if *verbose {
					fmt.Printf("  Checking if nodelist already processed: date=%s (year %d, day %d)\n",
						nodelistDate.Format("2006-01-02"), nodelistDate.Year(), nodes[0].DayNumber)
				}
				isProcessed, err := storageLayer.IsNodelistProcessed(nodelistDate)
				if err != nil {
					fmt.Printf("  ERROR checking if nodelist processed: %v\n", err)
					continue
				}
				if *verbose {
					fmt.Printf("  Nodelist processed check result: %t\n", isProcessed)
				}
				if isProcessed {
					if *verbose {
						fmt.Printf("  ALREADY IMPORTED: Nodelist for %s (year %d, day %d) was previously processed\n",
							nodelistDate.Format("2006-01-02"), nodelistDate.Year(), nodes[0].DayNumber)
						fmt.Printf("    This prevents duplicate imports of the same nodelist date\n")
					} else if !*quiet {
						fmt.Println("  Nodelist already processed, skipping")
					}
					filesProcessed++
					continue
				}
			}

			// Process nodes in batches, but only from current file
			for i := 0; i < len(nodes); i += *batchSize {
				end := i + *batchSize
				if end > len(nodes) {
					end = len(nodes)
				}

				batch := nodes[i:end]
				if err := insertBatch(storageLayer, batch, *verbose, *quiet); err != nil {
					fmt.Printf("  ERROR inserting batch: %v\n", err)
					break // Skip remaining batches from this file
				}
				totalNodes += len(batch)
			}

			filesProcessed++
			if !*quiet {
				fmt.Printf("  ✓ Parsed %d nodes\n", len(nodes))
			}
		}

		// Summary for sequential processing
		if !*quiet {
			fmt.Printf("Files processed: %d/%d\n", filesProcessed, len(files))
			fmt.Printf("Total nodes imported: %d\n", totalNodes)
			if totalNodes > 0 {
				duration := time.Since(startTime)
				fmt.Printf("Average: %.2f nodes/second\n", float64(totalNodes)/duration.Seconds())
			}
		}
	}

	// Create FTS indexes for better search performance (after data loading)
	if *createFTSIndexes {
		if !*quiet {
			fmt.Println("\nCreating Full-Text Search indexes...")
		}

		if cfg.IsMultiDatabase() {
			// Create FTS indexes for multiple databases
			if multiStorage, ok := storageLayer.(*storage.MultiStorage); ok {
				for _, dbName := range multiStorage.GetStorageNames() {
					if singleStorage, exists := multiStorage.GetStorage(dbName); exists {
						if !*quiet {
							fmt.Printf("Creating FTS indexes for %s...\n", dbName)
						}
						// Get the underlying database from the storage
						// We'll need to add a method to access the database interface
						if db := singleStorage.GetDatabase(); db != nil {
							if err := db.CreateFTSIndexes(); err != nil {
								if !*quiet {
									fmt.Printf("Warning: Could not create FTS indexes for %s: %v\n", dbName, err)
								}
							} else if !*quiet {
								fmt.Printf("FTS indexes created successfully for %s!\n", dbName)
							}
						}
					}
				}
			}
		} else {
			// Single database mode - need to get the database interface
			if singleStorage, ok := storageLayer.(*storage.Storage); ok {
				if db := singleStorage.GetDatabase(); db != nil {
					if err := db.CreateFTSIndexes(); err != nil {
						if !*quiet {
							fmt.Printf("Warning: Could not create FTS indexes: %v\n", err)
							fmt.Println("Text search will use slower ILIKE queries")
						}
					} else if !*quiet {
						fmt.Println("FTS indexes created successfully!")
					}
				}
			}
		}
	}

	// Overall summary
	if !*quiet {
		duration := time.Since(startTime)
		fmt.Println()
		fmt.Println("Processing completed!")
		fmt.Printf("Processing time: %v\n", duration)
	}
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

	// Remove .gz extension for pattern matching
	if strings.HasSuffix(filename, ".gz") {
		filename = strings.TrimSuffix(filename, ".gz")
	}

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

// parseConflictKey extracts zone, net, node, date from duplicate key error
func parseConflictKey(errorMsg string) (int, int, int, string, bool) {
	// Parse error like: duplicate key "2, 28, 2, 1988-09-09, 0"
	// Use regex to extract the duplicate key more reliably
	pattern := `duplicate key "([^"]+)"`
	re := regexp.MustCompile(pattern)
	matches := re.FindStringSubmatch(errorMsg)

	if len(matches) < 2 {
		return 0, 0, 0, "", false
	}

	keyStr := matches[1]
	parts := strings.Split(keyStr, ", ")
	if len(parts) < 4 {
		return 0, 0, 0, "", false
	}

	zone, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	net, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	node, err3 := strconv.Atoi(strings.TrimSpace(parts[2]))
	date := strings.TrimSpace(parts[3])

	if err1 != nil || err2 != nil || err3 != nil {
		return 0, 0, 0, "", false
	}

	return zone, net, node, date, true
}

// insertBatch inserts a batch of nodes into storage
func insertBatch(storageLayer interface {
	InsertNodes([]database.Node) error
	FindConflictingNode(int, int, int, time.Time) (bool, error)
}, batch []database.Node, verbose bool, quiet bool) error {
	if verbose {
		fmt.Printf("  Inserting batch of %d nodes...\n", len(batch))
	}

	start := time.Now()
	if err := storageLayer.InsertNodes(batch); err != nil {
		// Check if this is a primary key constraint error
		if strings.Contains(err.Error(), "PRIMARY KEY or UNIQUE constraint violated") && strings.Contains(err.Error(), "duplicate key") {
			fmt.Printf("  DATABASE CONFLICT DETECTED: %v\n", err)

			// Parse the conflicting key from the error message
			zone, net, node, dateStr, parsed := parseConflictKey(err.Error())
			if parsed {
				// Parse the date
				conflictDate, parseErr := time.Parse("2006-01-02", dateStr)
				if parseErr == nil {
					conflictExists, checkErr := storageLayer.FindConflictingNode(zone, net, node, conflictDate)
					if checkErr == nil && conflictExists {
						fmt.Printf("    CONFLICT ANALYSIS for node %d:%d/%d on %s:\n",
							zone, net, node, dateStr)
						fmt.Printf("      Node already exists in database for this date\n")
						fmt.Printf("      DIAGNOSIS: Multiple nodelist files contain the same node for the same date\n")
						fmt.Printf("        - This could be a corrected version, regional copy, or duplicate source\n")
						fmt.Printf("        - This prevents duplicate imports of the same nodelist\n")
					} else if checkErr != nil {
						fmt.Printf("    Error finding conflicting file: %v\n", checkErr)
					} else {
						fmt.Printf("    DIAGNOSIS: Intra-file duplicate detected\n")
						fmt.Printf("      The same node appears multiple times within the current nodelist file\n")
						fmt.Printf("      This is preserved as historical data with conflict tracking\n")
					}
				} else {
					fmt.Printf("    Error parsing conflict date '%s': %v\n", dateStr, parseErr)
				}
			} else {
				fmt.Printf("    Could not parse duplicate key from error message\n")
				fmt.Printf("    Raw error: %s\n", err.Error())

				// Debug: show what we're trying to parse
				if strings.Contains(err.Error(), "duplicate key") {
					start := strings.Index(err.Error(), "duplicate key \"")
					if start != -1 {
						start += 14
						end := strings.Index(err.Error()[start:], "\"")
						if end != -1 {
							keyStr := err.Error()[start : start+end]
							fmt.Printf("    Debug: extracted key string: '%s'\n", keyStr)
							parts := strings.Split(keyStr, ", ")
							fmt.Printf("    Debug: split into %d parts: %v\n", len(parts), parts)
						}
					}
				}
			}
		}
		return err
	}

	if verbose {
		fmt.Printf("  ✓ Batch inserted in %v\n", time.Since(start))
	}

	return nil
}
