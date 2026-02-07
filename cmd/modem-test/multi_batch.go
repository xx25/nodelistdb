// Package main provides multi-modem batch testing orchestration.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// runBatchModeMulti orchestrates batch testing with multiple modems.
func runBatchModeMulti(cfg *Config, log *TestLogger, configFile string, cdrService *CDRService, asteriskCDRService *AsteriskCDRService, operatorCache *OperatorCache, pgWriter *PostgresResultsWriter, mysqlWriter *MySQLResultsWriter, sqliteWriter *SQLiteResultsWriter, nodelistDBWriter *NodelistDBWriter, nodeLookup map[string]*NodeTarget, filteredNodes []NodeTarget) {
	phones := cfg.GetPhones()
	operators := cfg.GetOperators()
	pause := cfg.GetPause()
	retryCount := cfg.GetRetryCount()

	// Test count: with operator failover, it's 1 job per node (operators tried in sequence)
	// For filtered nodes, it's the number of nodes; otherwise the number of phones
	testCount := len(phones)
	if len(filteredNodes) > 0 {
		testCount = len(filteredNodes)
	}
	infinite := false // In multi-modem mode, run through all nodes once

	// Get modem configurations
	modemConfigs := cfg.GetModemConfigs()

	// Create modem pool
	cdrDelay := cfg.GetCDRDelay()
	pool, err := NewModemPool(modemConfigs, cfg.EMSI, cfg.Logging, pause, retryCount, pause, cdrDelay, cdrService, asteriskCDRService, operatorCache, log.GetOutput())
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
	if len(operators) > 0 && (len(operators) > 1 || operators[0].Name != "") {
		log.Info("Operator failover enabled with %d global operator(s):", len(operators))
		for i, op := range operators {
			priority := ""
			if i == 0 {
				priority = " (primary)"
			}
			if op.Prefix == "" {
				log.Info("  %d. %s (direct dial)%s", i+1, op.Name, priority)
			} else {
				log.Info("  %d. %s (prefix: %s)%s", i+1, op.Name, op.Prefix, priority)
			}
		}
		if operatorCache != nil {
			log.Info("Operator cache enabled")
		}
	}

	// Log prefix operator overrides
	if len(cfg.Test.PrefixOperators) > 0 {
		log.Info("Per-prefix operator overrides configured:")
		for _, po := range cfg.Test.PrefixOperators {
			opNames := make([]string, len(po.Operators))
			for i, op := range po.Operators {
				opNames[i] = op.Name
			}
			log.Info("  prefix %q -> %s", po.PhonePrefix, strings.Join(opNames, ", "))
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

	// Deferred node tracking for re-scheduling when call windows close
	var deferredMu sync.Mutex
	var deferredNodes []NodeTarget

	// Completion tracking: atomic counter so collector can signal when a batch is done
	var completedCount atomic.Int64
	completionSignal := make(chan struct{}, 1)

	// Result collector goroutine
	var collectorWg sync.WaitGroup
	collectorWg.Add(1)
	go func() {
		defer collectorWg.Done()
		for result := range pool.Results() {
			statsMu.Lock()

			// Track deferred nodes for re-scheduling
			if result.WindowClosed {
				if target, ok := nodeLookup[result.Phone]; ok {
					deferredMu.Lock()
					deferredNodes = append(deferredNodes, *target)
					deferredMu.Unlock()
				}
				// Don't count deferred results as success/failure
				statsMu.Unlock()
				completedCount.Add(1)
				select {
				case completionSignal <- struct{}{}:
				default:
				}
				continue
			}

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
			if csvWriter != nil || (pgWriter != nil && pgWriter.IsEnabled()) || (mysqlWriter != nil && mysqlWriter.IsEnabled()) || (sqliteWriter != nil && sqliteWriter.IsEnabled()) || (nodelistDBWriter != nil && nodelistDBWriter.IsEnabled()) {
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

				if nodelistDBWriter != nil && nodelistDBWriter.IsEnabled() {
					if err := nodelistDBWriter.WriteRecord(rec); err != nil {
						log.Error("Failed to write NodelistDB record: %v", err)
					}
				}
			}

			statsMu.Unlock()

			completedCount.Add(1)
			select {
			case completionSignal <- struct{}{}:
			default:
			}
		}
	}()

	// Start workers
	pool.Start()

	// Calculate submission pacing
	numWorkers := pool.WorkerCount()
	submissionDelay := time.Duration(0)
	if numWorkers > 0 && pause > 0 {
		// Divide pause by number of workers for pacing
		submissionDelay = pause / time.Duration(numWorkers)
		// Minimum 50ms to prevent overwhelming the queue
		if submissionDelay < 50*time.Millisecond {
			submissionDelay = 50 * time.Millisecond
		}
	}

	submitted := 0
	cancelled := false

	if len(filteredNodes) > 0 {
		// Prefix mode: use ScheduleNodes for time-aware job ordering.
		// ScheduleNodes sorts callable nodes first, waits for call windows,
		// and emits one job per node with full operator list for failover.
		jobs := ScheduleNodes(ctx, filteredNodes, cfg.GetOperatorsForPhone, log)
		for job := range jobs {
			submitted++
			job.testNum = submitted
			if !pool.SubmitJob(ctx, job) {
				cancelled = true
				break
			}
			// Pace submissions to maintain overall test rate
			if submissionDelay > 0 {
				select {
				case <-time.After(submissionDelay):
				case <-ctx.Done():
					cancelled = true
				}
			}
			if cancelled {
				break
			}
		}

		// Wait for all submitted jobs to complete before checking deferred
		if !cancelled {
			waitForCompletion(ctx, submitted, &completedCount, completionSignal)
		}

		// Re-schedule deferred nodes (call windows that closed during processing)
		for !cancelled {
			deferredMu.Lock()
			pending := deferredNodes
			deferredNodes = nil
			deferredMu.Unlock()

			if len(pending) == 0 {
				break
			}

			log.Info("%d node(s) deferred due to closed call windows, re-scheduling...", len(pending))
			reJobs := ScheduleNodes(ctx, pending, cfg.GetOperatorsForPhone, log)
			for job := range reJobs {
				submitted++
				job.testNum = submitted
				if !pool.SubmitJob(ctx, job) {
					cancelled = true
					break
				}
				if submissionDelay > 0 {
					select {
					case <-time.After(submissionDelay):
					case <-ctx.Done():
						cancelled = true
					}
				}
				if cancelled {
					break
				}
			}

			if !cancelled {
				waitForCompletion(ctx, submitted, &completedCount, completionSignal)
			}
		}
	} else {
		// Legacy submission loop: one job per phone with full operator list for failover
		for i := 0; infinite || submitted < testCount; i++ {
			select {
			case <-ctx.Done():
				cancelled = true
			default:
			}
			if cancelled {
				break
			}

			phoneIndex := i % len(phones)
			phone := phones[phoneIndex]

			submitted++

			job := phoneJob{
				phone:     phone,
				operators: cfg.GetOperatorsForPhone(phone),
				testNum:   submitted,
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
				break
			}

			// Pace submissions to maintain overall test rate
			if submissionDelay > 0 && (infinite || submitted < testCount) {
				select {
				case <-time.After(submissionDelay):
				case <-ctx.Done():
					cancelled = true
				}
			}
		}
	}

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

// waitForCompletion blocks until the number of completed results reaches the target
// or the context is cancelled.
func waitForCompletion(ctx context.Context, target int, completed *atomic.Int64, signal <-chan struct{}) {
	for {
		if completed.Load() >= int64(target) {
			return
		}
		select {
		case <-signal:
			// Check again
		case <-ctx.Done():
			return
		}
	}
}
