// Package main provides worker pool management for multi-modem testing.
package main

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/nodelistdb/internal/modemd/modem"
	"github.com/nodelistdb/internal/testing/protocols/emsi"
)

// WorkerResult holds the result of a single test from a worker.
type WorkerResult struct {
	WorkerID       int
	WorkerName     string
	Phone          string // Original phone number (without operator prefix)
	OperatorName   string // Operator friendly name
	OperatorPrefix string // Dial prefix used
	TestNum        int
	Result         testResult
	Timestamp      time.Time
}

// WorkerStats holds per-worker statistics.
type WorkerStats struct {
	Total         int
	Success       int
	Failed        int
	TotalDialTime time.Duration
	TotalEmsiTime time.Duration
}

// ModemWorker manages a single modem's test execution goroutine.
type ModemWorker struct {
	id          int
	name        string
	modem       *modem.Modem
	config      ModemInstanceConfig
	emsiConfig  EMSIConfig
	logConfig   LoggingConfig
	interDelay  time.Duration
	coordinator *PhoneCoordinator
	phoneQueue  <-chan phoneJob
	results     chan<- WorkerResult
	log         *TestLogger
	wg          *sync.WaitGroup
}

// phoneJob represents a phone number to dial with its test number and operator info.
type phoneJob struct {
	phone          string
	operatorName   string
	operatorPrefix string
	testNum        int
}

// newModemWorker creates a new modem worker.
func newModemWorker(
	id int,
	name string,
	config ModemInstanceConfig,
	emsiConfig EMSIConfig,
	logConfig LoggingConfig,
	interDelay time.Duration,
	logOutput io.Writer,
	coordinator *PhoneCoordinator,
	phoneQueue <-chan phoneJob,
	results chan<- WorkerResult,
	wg *sync.WaitGroup,
) (*ModemWorker, error) {
	// Create modem configuration
	modemCfg := modem.Config{
		Device:           config.Device,
		BaudRate:         config.BaudRate,
		InitString:       getFirstInitCommand(config.InitCommands),
		InitCommands:     config.InitCommands,
		DialPrefix:       config.DialPrefix,
		HangupMethod:     config.HangupMethod,
		Debug:            logConfig.Debug,
		DebugWriter:      logOutput,
		Name:             name,
		DialTimeout:      config.DialTimeout.Duration(),
		CarrierTimeout:   config.CarrierTimeout.Duration(),
		ATCommandTimeout: config.ATCommandTimeout.Duration(),
		ReadTimeout:      config.ReadTimeout.Duration(),
		// DTR hangup timing
		DTRHoldTime:      config.DTRHoldTime.Duration(),
		DTRWaitInterval:  config.DTRWaitInterval.Duration(),
		DTRMaxWaitTime:   config.DTRMaxWaitTime.Duration(),
		DTRStabilizeTime: config.DTRStabilizeTime.Duration(),
	}

	// Set line stats command
	if len(config.PostDisconnectCommands) > 0 {
		modemCfg.LineStatsCommand = config.PostDisconnectCommands[0]
	}

	// Create modem instance
	m, err := modem.New(modemCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create modem %s: %w", name, err)
	}

	workerLog := NewTestLogger(logConfig)
	if logOutput != nil {
		workerLog.SetOutput(logOutput)
	}
	workerLog.SetPrefix(name)

	return &ModemWorker{
		id:          id,
		name:        name,
		modem:       m,
		config:      config,
		emsiConfig:  emsiConfig,
		logConfig:   logConfig,
		interDelay:  interDelay,
		coordinator: coordinator,
		phoneQueue:  phoneQueue,
		results:     results,
		log:         workerLog,
		wg:          wg,
	}, nil
}

// Run is the main worker loop. It processes phones from the queue until the context is cancelled.
func (w *ModemWorker) Run(ctx context.Context) {
	defer w.wg.Done()

	// Open modem
	w.log.Init("Opening %s...", w.config.Device)
	if err := w.modem.Open(); err != nil {
		w.log.Fail("Failed to open modem: %v", err)
		return
	}
	defer w.modem.Close()

	w.log.OK("Modem opened and initialized")

	for {
		select {
		case <-ctx.Done():
			w.log.Info("Shutting down")
			return

		case job, ok := <-w.phoneQueue:
			if !ok {
				// Queue closed
				w.log.Info("Queue closed, shutting down")
				return
			}

			// Acquire phone lock (blocks if phone is in use)
			w.log.Info("Waiting for phone %s...", job.phone)
			if !w.coordinator.AcquirePhone(ctx, job.phone, w.name) {
				// Context cancelled while waiting
				continue
			}

			// Log operator info if configured
			if job.operatorName != "" {
				w.log.Info("Operator: %s (prefix: %q)", job.operatorName, job.operatorPrefix)
			}

			// Run the test with operator prefix prepended
			dialPhone := job.operatorPrefix + job.phone
			result := w.runTest(job.testNum, dialPhone)

			// Release phone lock
			w.coordinator.ReleasePhone(job.phone)

			// Send result
			select {
			case w.results <- WorkerResult{
				WorkerID:       w.id,
				WorkerName:     w.name,
				Phone:          job.phone, // Original phone without prefix
				OperatorName:   job.operatorName,
				OperatorPrefix: job.operatorPrefix,
				TestNum:        job.testNum,
				Result:         result,
				Timestamp:      time.Now(),
			}:
			case <-ctx.Done():
				return
			}

			// Apply inter-test delay before picking up next job
			if w.interDelay > 0 {
				w.log.Info("Waiting %v before next test...", w.interDelay)
				select {
				case <-time.After(w.interDelay):
				case <-ctx.Done():
					return
				}
			}
		}
	}
}

// runTest executes a single test call.
func (w *ModemWorker) runTest(testNum int, phoneNumber string) testResult {
	m := w.modem
	cfg := &w.config

	// Dial
	w.log.Dial("%s -> ATDT%s", phoneNumber, phoneNumber)
	result, err := m.DialNumber(phoneNumber)
	if err != nil {
		msg := fmt.Sprintf("Test %d [%s] %s: DIAL ERROR - %v", testNum, w.name, phoneNumber, err)
		w.log.Fail("DIAL ERROR - %v", err)

		// Try to recover
		w.log.Info("Attempting modem reset...")
		_ = m.Reset()

		return testResult{
			success: false,
			message: msg,
		}
	}

	if !result.Success {
		msg := fmt.Sprintf("Test %d [%s] %s: DIAL FAILED - %s (%.1fs)", testNum, w.name, phoneNumber, result.Error, result.DialTime.Seconds())
		w.log.Fail("DIAL FAILED - %s (%.1fs)", result.Error, result.DialTime.Seconds())
		return testResult{
			success: false,
			message: msg,
		}
	}

	// Log connection
	if result.ConnectString != "" {
		w.log.OK("%s (%.1fs)", result.ConnectString, result.DialTime.Seconds())
	} else {
		w.log.OK("Connected at %d bps (%.1fs)", result.ConnectSpeed, result.DialTime.Seconds())
	}

	// Log RS232 status
	if w.logConfig.ShowRS232 {
		if status, err := m.GetStatus(); err == nil {
			w.log.RS232(status.DCD, status.DSR, status.CTS, status.RI)
		}
	}

	// EMSI handshake
	w.log.EMSI("Starting handshake...")
	conn := m.GetConn()
	emsiCfg := emsi.DefaultConfig()
	emsiCfg.Protocols = w.emsiConfig.Protocols
	session := emsi.NewSessionWithInfoAndConfig(
		conn,
		w.emsiConfig.OurAddress,
		w.emsiConfig.SystemName,
		w.emsiConfig.Sysop,
		w.emsiConfig.Location,
		emsiCfg,
	)
	session.SetTimeout(w.emsiConfig.Timeout.Duration())

	emsiStart := time.Now()
	emsiErr := session.Handshake()
	emsiTime := time.Since(emsiStart)

	var testRes testResult
	testRes.dialTime = result.DialTime
	testRes.connectSpeed = result.ConnectSpeed
	testRes.connectString = result.ConnectString

	completionReason := session.GetCompletionReason()
	timing := session.GetHandshakeTiming()

	if emsiErr != nil {
		msg := fmt.Sprintf("Test %d [%s] %s: CONNECT %d, EMSI FAILED - %v", testNum, w.name, phoneNumber, result.ConnectSpeed, emsiErr)
		w.log.Fail("EMSI handshake failed: %v (%.1fs) [%s]", emsiErr, emsiTime.Seconds(), completionReason)
		testRes.success = false
		testRes.message = msg
		testRes.emsiError = emsiErr
	} else {
		info := session.GetRemoteInfo()
		sysName := ""
		if info != nil {
			sysName = info.SystemName
			testRes.emsiInfo = &EMSIDetails{
				SystemName:    info.SystemName,
				Location:      info.Location,
				Sysop:         info.Sysop,
				Addresses:     info.Addresses,
				MailerName:    info.MailerName,
				MailerVersion: info.MailerVersion,
				Protocols:     info.Protocols,
				Capabilities:  info.Capabilities,
			}
			w.log.PrintEMSIDetails(testRes.emsiInfo)
		}
		msg := fmt.Sprintf("Test %d [%s] %s: OK - CONNECT %d, EMSI %.1fs, %s", testNum, w.name, phoneNumber, result.ConnectSpeed, emsiTime.Seconds(), sysName)
		w.log.OK("EMSI complete (%.1fs) [%s] init=%.1fs dat=%.1fs",
			emsiTime.Seconds(), completionReason,
			timing.InitialPhase.Seconds(), timing.DATExchange.Seconds())
		testRes.success = true
		testRes.message = msg
		testRes.emsiTime = emsiTime
	}

	// Hangup
	w.log.Hangup("Disconnecting...")
	if err := m.Hangup(); err != nil {
		w.log.Fail("Hangup error: %v, resetting...", err)
		_ = m.Reset()
	} else if w.logConfig.ShowRS232 {
		if status, err := m.GetStatus(); err == nil {
			w.log.RS232(status.DCD, status.DSR, status.CTS, status.RI)
		}
	}

	// Get line stats if configured
	canGetStats := !m.InDataMode() && len(cfg.PostDisconnectCommands) > 0
	if canGetStats {
		if status, err := m.GetStatus(); err == nil && status.DCD {
			w.log.Warn("Skipping stats collection - modem still connected (DCD high)")
			canGetStats = false
		}
	}

	if canGetStats {
		m.DrainPendingResponse(2 * time.Second)

		if delay := cfg.PostDisconnectDelay.Duration(); delay > 0 {
			time.Sleep(delay)
		}

		for _, cmd := range cfg.PostDisconnectCommands {
			var response string
			var err error

			// Use pagination-aware method if configured (e.g., for MT5634ZBA's ATI11)
			if cfg.StatsPagination {
				response, err = m.SendATWithPagination(cmd, cfg.ATCommandTimeout.Duration())
			} else {
				response, err = m.SendAT(cmd, cfg.ATCommandTimeout.Duration())
			}

			if err == nil {
				w.log.PrintLineStatsWithProfile(response, cfg.StatsProfile)
				if testRes.lineStats == nil && cfg.StatsProfile != "" && cfg.StatsProfile != "raw" {
					testRes.lineStats = ParseStats(response, cfg.StatsProfile)
				}
			}
		}
	}

	return testRes
}

// ModemPool manages multiple modem workers.
type ModemPool struct {
	workers     []*ModemWorker
	coordinator *PhoneCoordinator
	phoneQueue  chan phoneJob
	results     chan WorkerResult
	wg          sync.WaitGroup
	ctx         context.Context
	cancel      context.CancelFunc
	log         *TestLogger
	interDelay  time.Duration
}

// NewModemPool creates a pool of modem workers from the given configurations.
func NewModemPool(
	configs []ModemInstanceConfig,
	emsiCfg EMSIConfig,
	logCfg LoggingConfig,
	interDelay time.Duration,
	logOutput io.Writer,
) (*ModemPool, error) {
	ctx, cancel := context.WithCancel(context.Background())

	p := &ModemPool{
		coordinator: NewPhoneCoordinator(),
		phoneQueue:  make(chan phoneJob, 100),
		results:     make(chan WorkerResult, len(configs)*2),
		ctx:         ctx,
		cancel:      cancel,
		log:         NewTestLogger(logCfg),
		interDelay:  interDelay,
	}

	// Create workers for each modem
	for i, cfg := range configs {
		if !cfg.IsEnabled() {
			continue
		}

		worker, err := newModemWorker(
			i,
			cfg.Name,
			cfg,
			emsiCfg,
			logCfg,
			interDelay,
			logOutput,
			p.coordinator,
			p.phoneQueue,
			p.results,
			&p.wg,
		)
		if err != nil {
			// Log error but continue with other modems
			p.log.Error("Failed to create worker for %s: %v", cfg.Name, err)
			continue
		}

		p.workers = append(p.workers, worker)
	}

	if len(p.workers) == 0 {
		cancel()
		return nil, fmt.Errorf("no modems could be initialized")
	}

	return p, nil
}

// Start launches all worker goroutines.
func (p *ModemPool) Start() {
	for _, w := range p.workers {
		p.wg.Add(1)
		go w.Run(p.ctx)
	}
}

// Stop gracefully stops all workers and waits for them to finish.
// It first closes the phone queue to signal workers to finish current work,
// then waits for all workers to complete before closing results.
func (p *ModemPool) Stop() {
	// Close the queue first - workers will finish processing remaining items
	close(p.phoneQueue)
	// Wait for all workers to finish their current work
	p.wg.Wait()
	// Now cancel context (cleanup) and close results
	p.cancel()
	close(p.results)
}

// SubmitPhone adds a phone to the queue for any available worker.
// Returns false if context is cancelled or pool is stopped.
func (p *ModemPool) SubmitPhone(ctx context.Context, phone string, testNum int) bool {
	return p.SubmitPhoneWithOperator(ctx, phone, "", "", testNum)
}

// SubmitPhoneWithOperator adds a phone with operator info to the queue.
// Returns false if context is cancelled or pool is stopped.
func (p *ModemPool) SubmitPhoneWithOperator(ctx context.Context, phone, operatorName, operatorPrefix string, testNum int) bool {
	select {
	case p.phoneQueue <- phoneJob{phone: phone, operatorName: operatorName, operatorPrefix: operatorPrefix, testNum: testNum}:
		return true
	case <-ctx.Done():
		return false
	case <-p.ctx.Done():
		return false
	}
}

// Cancel cancels the pool context, signaling workers to stop after current work.
func (p *ModemPool) Cancel() {
	p.cancel()
}

// Results returns the results channel for reading worker results.
func (p *ModemPool) Results() <-chan WorkerResult {
	return p.results
}

// WorkerCount returns the number of active workers.
func (p *ModemPool) WorkerCount() int {
	return len(p.workers)
}

// WorkerNames returns the names of all workers.
func (p *ModemPool) WorkerNames() []string {
	names := make([]string, len(p.workers))
	for i, w := range p.workers {
		names[i] = w.name
	}
	return names
}

// GetCoordinator returns the phone coordinator for external status checks.
func (p *ModemPool) GetCoordinator() *PhoneCoordinator {
	return p.coordinator
}
