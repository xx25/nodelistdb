// Package main provides multi-modem batch testing orchestration.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// runBatchModeMulti orchestrates batch testing with multiple modems.
func runBatchModeMulti(cfg *Config, log *TestLogger, configFile string, cdrService *CDRService, asteriskCDRService *AsteriskCDRService, pgWriter *PostgresResultsWriter) {
	phones := cfg.GetPhones()
	testCount := cfg.Test.Count
	infinite := testCount <= 0
	interDelay := cfg.Test.InterDelay.Duration()

	// Get modem configurations
	modemConfigs := cfg.GetModemConfigs()

	// Create modem pool
	pool, err := NewModemPool(modemConfigs, cfg.EMSI, cfg.Logging, log.GetOutput())
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

			if result.Result.success {
				totalSuccess++
				ps.Success++
				ms.Success++
				totalDialTime += result.Result.dialTime
				totalEmsiTime += result.Result.emsiTime
				ps.TotalDialTime += result.Result.dialTime
				ps.TotalEmsiTime += result.Result.emsiTime
				ms.TotalDialTime += result.Result.dialTime
				ms.TotalEmsiTime += result.Result.emsiTime
			} else {
				totalFailed++
				ps.Failed++
				ms.Failed++
			}

			results = append(results, result.Result.message)

			// Lookup CDR data for VoIP quality metrics (AudioCodes)
			var cdrData *CDRData
			if cdrService != nil && cdrService.IsEnabled() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				var err error
				cdrData, err = cdrService.LookupByPhone(ctx, result.Phone, result.Timestamp)
				cancel()
				if err != nil {
					log.Debug("AudioCodes CDR lookup failed for %s: %v", result.Phone, err)
				} else if cdrData != nil {
					log.Info("[%s] CDR: MOS=%.1f jitter=%dms delay=%dms loss=%d term=%s",
						result.WorkerName, float64(cdrData.LocalMOSCQ)/10.0,
						cdrData.RTPJitter, cdrData.RTPDelay, cdrData.PacketLoss, cdrData.TermReason)
				}
			}

			// Lookup Asterisk CDR for call routing info
			var asteriskCDR *AsteriskCDRData
			if asteriskCDRService != nil && asteriskCDRService.IsEnabled() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				var err error
				asteriskCDR, err = asteriskCDRService.LookupByPhone(ctx, result.Phone, result.Timestamp)
				cancel()
				if err != nil {
					log.Debug("Asterisk CDR lookup failed for %s: %v", result.Phone, err)
				} else if asteriskCDR != nil {
					log.Info("[%s] Asterisk: disposition=%s peer=%s duration=%ds",
						result.WorkerName, asteriskCDR.Disposition, asteriskCDR.Peer, asteriskCDR.Duration)
				}
			}

			// Write CSV and PostgreSQL
			if csvWriter != nil || (pgWriter != nil && pgWriter.IsEnabled()) {
				rec := RecordFromTestResult(
					result.TestNum,
					result.Phone,
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

	submitted := 0
	for i := 0; infinite || submitted < testCount; i++ {
		select {
		case <-ctx.Done():
			goto cleanup
		default:
		}

		phoneIndex := i % len(phones)
		phone := phones[phoneIndex]
		submitted++

		if !pool.SubmitPhone(ctx, phone, submitted) {
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

cleanup:
	pool.Stop()
	collectorWg.Wait()

	// Calculate averages
	var avgDialTime, avgEmsiTime time.Duration
	if totalSuccess > 0 {
		avgDialTime = totalDialTime / time.Duration(totalSuccess)
		avgEmsiTime = totalEmsiTime / time.Duration(totalSuccess)
	}

	// Print summary
	log.PrintMultiModemSummary(
		submitted,
		totalSuccess,
		totalFailed,
		time.Since(sessionStart),
		avgDialTime,
		avgEmsiTime,
		results,
		phoneStats,
		modemStats,
	)
}
