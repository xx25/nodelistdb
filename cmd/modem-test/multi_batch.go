// Package main provides multi-modem batch testing orchestration.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

// runBatchModeMulti orchestrates batch testing with multiple modems.
func runBatchModeMulti(cfg *Config, log *TestLogger, configFile string, cdrService *CDRService, asteriskCDRService *AsteriskCDRService, pgWriter *PostgresResultsWriter, mysqlWriter *MySQLResultsWriter, sqliteWriter *SQLiteResultsWriter, nodeLookup map[string]*NodeTarget, filteredNodes []NodeTarget) {
	phones := cfg.GetPhones()
	operators := cfg.GetOperators()
	testCount := cfg.GetTotalTestCount()
	infinite := testCount <= 0 && !cfg.IsPerOperatorMode() // Per-operator mode is never infinite
	interDelay := cfg.Test.InterDelay.Duration()
	perOperatorMode := cfg.IsPerOperatorMode()
	callsPerOperator := cfg.Test.CallsPerOperator
	retryCount := cfg.GetRetryCount()
	retryDelay := cfg.GetRetryDelay()
	cdrLookupDelay := cfg.GetCDRLookupDelay()

	// If no operators configured, use a single "no operator" entry for simpler loop logic
	if len(operators) == 0 {
		operators = []OperatorConfig{{Name: "", Prefix: ""}}
	}

	// Get modem configurations
	modemConfigs := cfg.GetModemConfigs()

	// Create modem pool
	pool, err := NewModemPool(modemConfigs, cfg.EMSI, cfg.Logging, interDelay, retryCount, retryDelay, cdrLookupDelay, cdrService, asteriskCDRService, log.GetOutput())
	if err != nil {
		log.Error("Failed to create modem pool: %v", err)
		os.Exit(1)
	}

	// Setup signal handler for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Fprintln(os.Stderr, "\nReceived interrupt, stopping workers...")
		cancel()
		pool.Cancel() // Also cancel the pool to unblock any waiting operations
	}()

	// Print header
	log.PrintMultiModemHeader(configFile, pool.WorkerNames(), phones, testCount)

	// Log operator configuration if multiple operators configured
	if len(operators) > 1 || (len(operators) == 1 && operators[0].Name != "") {
		log.Info("Operator rotation enabled with %d operator(s):", len(operators))
		for _, op := range operators {
			if op.Prefix == "" {
				log.Info("  - %s (direct dial)", op.Name)
			} else {
				log.Info("  - %s (prefix: %s)", op.Name, op.Prefix)
			}
		}
	}

	// Initialize CSV writer if configured
	var csvWriter *CSVWriter
	if cfg.Test.CSVFile != "" {
		var err error
		csvWriter, err = NewCSVWriter(cfg.Test.CSVFile)
		if err != nil {
			log.Error("Failed to open CSV file: %v", err)
		} else {
			defer csvWriter.Close()
			log.Info("Writing results to: %s", cfg.Test.CSVFile)
		}
	}

	// Statistics tracking
	var statsMu sync.Mutex
	phoneStats := make(map[string]*PhoneStats)
	for _, phone := range phones {
		phoneStats[phone] = &PhoneStats{Phone: phone}
	}
	modemStats := make(map[string]*WorkerStats)
	for _, name := range pool.WorkerNames() {
		modemStats[name] = &WorkerStats{}
	}
	operatorStats := make(map[string]*OperatorStats)
	for _, op := range operators {
		operatorStats[op.Name] = &OperatorStats{Name: op.Name, Prefix: op.Prefix}
	}

	var totalSuccess, totalFailed int
	var totalDialTime, totalEmsiTime time.Duration
	results := make([]string, 0)
	sessionStart := time.Now()

	// Result collector goroutine
	var collectorWg sync.WaitGroup
	collectorWg.Add(1)
	go func() {
		defer collectorWg.Done()
		for result := range pool.Results() {
			statsMu.Lock()

			// Update phone stats
			ps := phoneStats[result.Phone]
			if ps == nil {
				ps = &PhoneStats{Phone: result.Phone}
				phoneStats[result.Phone] = ps
			}
			ps.Total++

			// Update modem stats
			ms := modemStats[result.WorkerName]
			if ms == nil {
				ms = &WorkerStats{}
				modemStats[result.WorkerName] = ms
			}
			ms.Total++

			// Update operator stats
			os := operatorStats[result.OperatorName]
			if os == nil {
				os = &OperatorStats{Name: result.OperatorName, Prefix: result.OperatorPrefix}
				operatorStats[result.OperatorName] = os
			}
			os.Total++

			if result.Result.success {
				totalSuccess++
				ps.Success++
				ms.Success++
				os.Success++
				totalDialTime += result.Result.dialTime
				totalEmsiTime += result.Result.emsiTime
				ps.TotalDialTime += result.Result.dialTime
				ps.TotalEmsiTime += result.Result.emsiTime
				ms.TotalDialTime += result.Result.dialTime
				ms.TotalEmsiTime += result.Result.emsiTime
				os.TotalDialTime += result.Result.dialTime
				os.TotalEmsiTime += result.Result.emsiTime
			} else {
				totalFailed++
				ps.Failed++
				ms.Failed++
				os.Failed++
			}

			results = append(results, result.Result.message)

			// CDR lookups are now done in runTest() with proper delay
			// Use CDR data from the result if available
			cdrData := result.Result.cdrData
			asteriskCDR := result.Result.asteriskCDR

			// Write CSV and databases
			if csvWriter != nil || (pgWriter != nil && pgWriter.IsEnabled()) || (mysqlWriter != nil && mysqlWriter.IsEnabled()) || (sqliteWriter != nil && sqliteWriter.IsEnabled()) {
				rec := RecordFromTestResult(
					result.TestNum,
					result.Phone,
					result.OperatorName,
					result.OperatorPrefix,
					result.NodeAddress,
					result.NodeSystemName,
					result.NodeLocation,
					result.NodeSysop,
					result.Result.success,
					result.Result.dialTime,
					result.Result.connectSpeed,
					result.Result.connectString,
					result.Result.emsiTime,
					result.Result.emsiError,
					result.Result.emsiInfo,
					result.Result.lineStats,
					cdrData,
					asteriskCDR,
				)
				rec.ModemName = result.WorkerName

				if csvWriter != nil {
					if err := csvWriter.WriteRecord(rec); err != nil {
						log.Error("Failed to write CSV record: %v", err)
					}
				}

				if pgWriter != nil && pgWriter.IsEnabled() {
					if err := pgWriter.WriteRecord(rec); err != nil {
						log.Error("Failed to write PostgreSQL record: %v", err)
					}
				}

				if mysqlWriter != nil && mysqlWriter.IsEnabled() {
					if err := mysqlWriter.WriteRecord(rec); err != nil {
						log.Error("Failed to write MySQL record: %v", err)
					}
				}

				if sqliteWriter != nil && sqliteWriter.IsEnabled() {
					if err := sqliteWriter.WriteRecord(rec); err != nil {
						log.Error("Failed to write SQLite record: %v", err)
					}
				}
			}

			statsMu.Unlock()
		}
	}()

	// Start workers
	pool.Start()

	// Submit phones to pool
	// In multi-modem mode, we pace submissions to maintain similar overall test rate
	// as single-modem mode. With N modems, each submission happens N times faster.
	numWorkers := pool.WorkerCount()
	submissionDelay := time.Duration(0)
	if numWorkers > 0 && interDelay > 0 {
		// Divide inter_delay by number of workers for pacing
		submissionDelay = interDelay / time.Duration(numWorkers)
		// Minimum 50ms to prevent overwhelming the queue
		if submissionDelay < 50*time.Millisecond {
			submissionDelay = 50 * time.Millisecond
		}
	}

	// Track calls per phone+operator combination (for per-operator mode)
	type comboKey struct {
		phone    string
		operator string
	}
	callCounts := make(map[comboKey]int)

	// Calculate total combinations for rotation
	totalCombinations := len(phones) * len(operators)

	// Log per-operator mode info
	if perOperatorMode {
		log.Info("Per-operator mode: %d calls per operator per phone (total: %d calls)",
			callsPerOperator, testCount)
	}

	submitted := 0

	if len(filteredNodes) > 0 {
		// Prefix mode: use ScheduleNodes for time-aware job ordering.
		// ScheduleNodes sorts callable nodes first, waits for call windows,
		// and emits one job per operator per node.
		jobs := ScheduleNodes(ctx, filteredNodes, operators, cfg.Test.CallsPerOperator, log)
		for job := range jobs {
			submitted++
			job.testNum = submitted
			if !pool.SubmitJob(ctx, job) {
				goto cleanup
			}
			// Pace submissions to maintain overall test rate
			if submissionDelay > 0 {
				select {
				case <-time.After(submissionDelay):
				case <-ctx.Done():
					goto cleanup
				}
			}
		}
	} else {
		// Legacy submission loop: round-robin or per-operator mode
		for i := 0; infinite || submitted < testCount; i++ {
			select {
			case <-ctx.Done():
				goto cleanup
			default:
			}

			var phone string
			var operator OperatorConfig

			if perOperatorMode {
				// Per-operator mode: find next combo that hasn't reached its limit
				found := false
				for _, p := range phones {
					for _, op := range operators {
						key := comboKey{phone: p, operator: op.Name}
						if callCounts[key] < callsPerOperator {
							phone = p
							operator = op
							found = true
							break
						}
					}
					if found {
						break
					}
				}
				if !found {
					// All combinations have reached their limit
					log.Info("All phone+operator combinations completed")
					goto cleanup
				}
				callCounts[comboKey{phone: phone, operator: operator.Name}]++
			} else {
				// Legacy mode: round-robin through all combinations
				comboIndex := i % totalCombinations
				phoneIndex := comboIndex / len(operators)
				operatorIndex := comboIndex % len(operators)
				phone = phones[phoneIndex]
				operator = operators[operatorIndex]
			}

			submitted++

			job := phoneJob{
				phone:          phone,
				operatorName:   operator.Name,
				operatorPrefix: operator.Prefix,
				testNum:        submitted,
			}
			if nodeLookup != nil {
				if target, ok := nodeLookup[phone]; ok {
					job.nodeAddress = target.Address()
					job.nodeSystemName = strings.ReplaceAll(target.SystemName, "_", " ")
					job.nodeLocation = strings.ReplaceAll(target.Location, "_", " ")
					job.nodeSysop = strings.ReplaceAll(target.SysopName, "_", " ")
				}
			}
			if !pool.SubmitJob(ctx, job) {
				// Context cancelled
				goto cleanup
			}

			// Pace submissions to maintain overall test rate
			if submissionDelay > 0 && (infinite || submitted < testCount) {
				select {
				case <-time.After(submissionDelay):
				case <-ctx.Done():
					goto cleanup
				}
			}
		}
	}

cleanup:
	pool.Stop()
	collectorWg.Wait()

	// Calculate averages
	var avgDialTime, avgEmsiTime time.Duration
	if totalSuccess > 0 {
		avgDialTime = totalDialTime / time.Duration(totalSuccess)
		avgEmsiTime = totalEmsiTime / time.Duration(totalSuccess)
	}

	// Print summary with operator stats
	log.PrintMultiModemSummaryWithOperators(
		submitted,
		totalSuccess,
		totalFailed,
		time.Since(sessionStart),
		avgDialTime,
		avgEmsiTime,
		results,
		phoneStats,
		modemStats,
		operatorStats,
	)
}
