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
	"github.com/nodelistdb/internal/testing/timeavail"
)

// WorkerResult holds the result of a single test from a worker.
type WorkerResult struct {
	WorkerID       int
	WorkerName     string
	Phone          string // Original phone number (without operator prefix)
	OperatorName   string // Operator friendly name
	OperatorPrefix string // Dial prefix used
	NodeAddress    string // FidoNet address from API (e.g., "2:5020/100")
	NodeSystemName string // BBS name from API
	NodeLocation   string // Location from API
	NodeSysop      string // Sysop name from API
	TestNum        int
	Result         testResult
	Timestamp      time.Time
	WindowClosed   bool // true = call window closed, node should be retried later
	IsIntermediate bool // true = retry/intermediate operator result, not a final per-job result
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
	id                 int
	name               string
	modem              *modem.Modem
	config             ModemInstanceConfig
	emsiConfig         EMSIConfig
	logConfig          LoggingConfig
	interDelay         time.Duration
	retryCount         int
	retryDelay         time.Duration
	cdrLookupDelay     time.Duration
	cdrService         *CDRService
	asteriskCDRService *AsteriskCDRService
	operatorCache      *OperatorCache // Cache for operator failover
	coordinator        *PhoneCoordinator
	phoneQueue         <-chan phoneJob
	results            chan<- WorkerResult
	log                *TestLogger
	wg                 *sync.WaitGroup
}

// phoneJob represents a phone number to dial with its test number and operator info.
type phoneJob struct {
	phone     string
	operators []OperatorConfig // Full list of operators for failover (empty = direct dial)
	testNum   int
	// Legacy single-operator fields (used when operators is empty)
	operatorName   string
	operatorPrefix string
	// Node info from API
	nodeAddress    string // FidoNet address, e.g., "2:5020/100" (empty if not from API)
	nodeSystemName string // BBS name (empty if not from API)
	nodeLocation   string // Location (empty if not from API)
	nodeSysop      string // Sysop name (empty if not from API)
	// Time availability for pre-dial checks (nil = always callable)
	nodeAvailability *timeavail.NodeAvailability
}

// newModemWorker creates a new modem worker.
func newModemWorker(
	id int,
	name string,
	config ModemInstanceConfig,
	emsiConfig EMSIConfig,
	logConfig LoggingConfig,
	interDelay time.Duration,
	retryCount int,
	retryDelay time.Duration,
	cdrLookupDelay time.Duration,
	cdrService *CDRService,
	asteriskCDRService *AsteriskCDRService,
	operatorCache *OperatorCache,
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
		id:                 id,
		name:               name,
		modem:              m,
		config:             config,
		emsiConfig:         emsiConfig,
		logConfig:          logConfig,
		interDelay:         interDelay,
		retryCount:         retryCount,
		retryDelay:         retryDelay,
		cdrLookupDelay:     cdrLookupDelay,
		cdrService:         cdrService,
		asteriskCDRService: asteriskCDRService,
		operatorCache:      operatorCache,
		coordinator:        coordinator,
		phoneQueue:         phoneQueue,
		results:            results,
		log:                workerLog,
		wg:                 wg,
	}, nil
}

// Run is the main worker loop. It processes phones from the queue until the context is cancelled.
func (w *ModemWorker) Run(ctx context.Context) {
	defer w.wg.Done()

	// Open modem
	w.log.Init("Opening %s...", w.config.Device)
	if err := w.modem.Open(); err != nil {
		w.log.Fail("Failed to open modem: %v", err)

		// Try USB reset as recovery
		vendor, product, usbErr := modem.GetUSBDeviceID(w.config.Device)
		if usbErr != nil {
			w.log.Error("Cannot attempt USB reset: %v", usbErr)
			return
		}

		w.log.Info("Attempting USB reset for %s:%s...", vendor, product)
		if resetErr := modem.ResetUSBDevice(vendor, product); resetErr != nil {
			w.log.Error("USB reset failed: %v", resetErr)
			return
		}

		w.log.Info("Waiting for %s to reappear...", w.config.Device)
		if waitErr := modem.WaitForDevice(w.config.Device, 10*time.Second); waitErr != nil {
			w.log.Error("Device did not reappear after USB reset: %v", waitErr)
			return
		}

		w.log.Info("Retrying modem open...")
		if retryErr := w.modem.Open(); retryErr != nil {
			w.log.Fail("Failed to open modem after USB reset: %v", retryErr)
			return
		}

		w.log.OK("Modem opened after USB reset")
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

			// Log node info if from API
			if job.nodeAddress != "" {
				w.log.Info("Node: %s %s (sysop: %s)", job.nodeAddress, job.nodeSystemName, job.nodeSysop)
			}

			// Determine effective operators and operator info for result
			var result testResult
			var resultOperatorName, resultOperatorPrefix string
			var windowClosed bool

			// Use failover mode if operators list is provided
			if len(job.operators) > 0 {
				// Callback for retry attempts - emits result for tracking
				onRetryAttempt := func(attempt int, dialTime time.Duration, reason, opName, opPrefix string) {
					retryResult := testResult{
						success:  false,
						message:  fmt.Sprintf("Test %d [%s] %s: DIAL FAILED - %s (%.1fs) [attempt %d]", job.testNum, w.name, job.phone, reason, dialTime.Seconds(), attempt),
						dialTime: dialTime,
					}

					// Emit retry result with operator attribution
					select {
					case w.results <- WorkerResult{
						WorkerID:       w.id,
						WorkerName:     w.name,
						Phone:          job.phone,
						OperatorName:   opName,
						OperatorPrefix: opPrefix,
						NodeAddress:    job.nodeAddress,
						NodeSystemName: job.nodeSystemName,
						NodeLocation:   job.nodeLocation,
						NodeSysop:      job.nodeSysop,
						TestNum:        job.testNum,
						Result:         retryResult,
						Timestamp:      time.Now(),
						IsIntermediate: true,
					}:
					case <-ctx.Done():
						return
					}
				}

				// Callback for intermediate operator results - emits full result before trying next operator
				onOperatorResult := func(opResult testResult, opName, opPrefix string) {
					select {
					case w.results <- WorkerResult{
						WorkerID:       w.id,
						WorkerName:     w.name,
						Phone:          job.phone,
						OperatorName:   opName,
						OperatorPrefix: opPrefix,
						NodeAddress:    job.nodeAddress,
						NodeSystemName: job.nodeSystemName,
						NodeLocation:   job.nodeLocation,
						NodeSysop:      job.nodeSysop,
						TestNum:        job.testNum,
						Result:         opResult,
						Timestamp:      time.Now(),
						IsIntermediate: true,
					}:
					case <-ctx.Done():
						return
					}
				}

				// Run with failover
				failoverResult := w.runTestWithFailover(ctx, job, job.operators, w.operatorCache, onRetryAttempt, onOperatorResult)
				result = failoverResult.LastResult
				windowClosed = failoverResult.WindowClosed

				// Set operator info - use SuccessOperator if succeeded, LastOperator otherwise
				// This ensures we always know which operator was used for attribution
				if failoverResult.SuccessOperator != nil {
					resultOperatorName = failoverResult.SuccessOperator.Name
					resultOperatorPrefix = failoverResult.SuccessOperator.Prefix
				} else if failoverResult.LastOperator != nil {
					resultOperatorName = failoverResult.LastOperator.Name
					resultOperatorPrefix = failoverResult.LastOperator.Prefix
				}
			} else {
				// Legacy single-operator mode
				if job.operatorName != "" {
					w.log.Info("Operator: %s (prefix: %q)", job.operatorName, job.operatorPrefix)
				}

				dialPhone := job.operatorPrefix + job.phone

				// Callback for retry attempts - emits result for tracking
				// Note: opName/opPrefix params unused here since we know the operator from job
				onRetryAttempt := func(attempt int, dialTime time.Duration, reason, _, _ string) {
					retryResult := testResult{
						success:  false,
						message:  fmt.Sprintf("Test %d [%s] %s: DIAL FAILED - %s (%.1fs) [attempt %d]", job.testNum, w.name, dialPhone, reason, dialTime.Seconds(), attempt),
						dialTime: dialTime,
					}

					// Emit retry result
					select {
					case w.results <- WorkerResult{
						WorkerID:       w.id,
						WorkerName:     w.name,
						Phone:          job.phone,
						OperatorName:   job.operatorName,
						OperatorPrefix: job.operatorPrefix,
						NodeAddress:    job.nodeAddress,
						NodeSystemName: job.nodeSystemName,
						NodeLocation:   job.nodeLocation,
						NodeSysop:      job.nodeSysop,
						TestNum:        job.testNum,
						Result:         retryResult,
						Timestamp:      time.Now(),
						IsIntermediate: true,
					}:
					case <-ctx.Done():
						return
					}
				}

				result = w.runTest(ctx, job.testNum, dialPhone, job.phone, onRetryAttempt, job.nodeAvailability)
				windowClosed = result.windowClosed
				resultOperatorName = job.operatorName
				resultOperatorPrefix = job.operatorPrefix
			}

			// Release phone lock
			w.coordinator.ReleasePhone(job.phone)

			// Send final result
			select {
			case w.results <- WorkerResult{
				WorkerID:       w.id,
				WorkerName:     w.name,
				Phone:          job.phone,
				OperatorName:   resultOperatorName,
				OperatorPrefix: resultOperatorPrefix,
				NodeAddress:    job.nodeAddress,
				NodeSystemName: job.nodeSystemName,
				NodeLocation:   job.nodeLocation,
				NodeSysop:      job.nodeSysop,
				TestNum:        job.testNum,
				Result:         result,
				Timestamp:      time.Now(),
				WindowClosed:   windowClosed,
			}:
			case <-ctx.Done():
				return
			}

			// Apply inter-test delay before picking up next job (skip if queue is empty)
			if w.interDelay > 0 && len(w.phoneQueue) > 0 {
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


// RetryAttemptCallback is called for each retry attempt before waiting.
// operatorName and operatorPrefix identify which operator was being tried.
type RetryAttemptCallback func(attempt int, dialTime time.Duration, reason, operatorName, operatorPrefix string)

// runTest executes a single test call.
func (w *ModemWorker) runTest(ctx context.Context, testNum int, phoneNumber string, originalPhone string, onRetryAttempt RetryAttemptCallback, nodeAvailability *timeavail.NodeAvailability) testResult {
	m := w.modem
	cfg := &w.config // For modem-specific settings (timeouts, post-disconnect commands, etc.)

	// Use retry settings from worker (passed from test config)
	retryCount := w.retryCount
	retryDelay := w.retryDelay
	cdrLookupDelay := w.cdrLookupDelay

	var result *modem.DialResult
	var err error
	retryAttempts := 0
	var lastRetryReason string

	// CDR data captured during dial attempts (for results)
	var lastAsteriskCDR *AsteriskCDRData
	var lastCDRData *CDRData
	var lastCallTime time.Time // Time of last dial attempt for CDR lookup

	// Dial with retry on BUSY or CDR-detected failures
	for {
		// Reset CDR data at start of each attempt to avoid stale data from previous attempts
		lastAsteriskCDR = nil
		lastCDRData = nil

		// Check for cancellation before retry wait
		if retryAttempts > 0 {
			w.log.Info("Retry %d/%d (%s), waiting %v...", retryAttempts, retryCount, lastRetryReason, retryDelay)
			select {
			case <-time.After(retryDelay):
			case <-ctx.Done():
				w.log.Info("Cancelled during retry wait")
				return testResult{
					success: false,
					message: fmt.Sprintf("Test %d [%s] %s: CANCELLED", testNum, w.name, phoneNumber),
				}
			}
		}

		// Check for cancellation before dialing
		select {
		case <-ctx.Done():
			w.log.Info("Cancelled before dial")
			return testResult{
				success: false,
				message: fmt.Sprintf("Test %d [%s] %s: CANCELLED", testNum, w.name, phoneNumber),
			}
		default:
		}

		// Pre-dial availability check: stop if call window closed during retries
		if nodeAvailability != nil && !nodeAvailability.IsCallableNow(time.Now().UTC()) {
			w.log.Warn("Call window closed during retries for %s, deferring", phoneNumber)
			return testResult{
				windowClosed: true,
				message:      fmt.Sprintf("Test %d [%s] %s: DEFERRED - call window closed", testNum, w.name, phoneNumber),
			}
		}

		w.log.Dial("%s -> ATDT%s", phoneNumber, phoneNumber)
		result, err = m.DialNumber(phoneNumber)
		lastCallTime = time.Now() // Store for CDR lookup
		callTime := lastCallTime  // Local alias for use in this iteration

		if err != nil {
			msg := fmt.Sprintf("Test %d [%s] %s: DIAL ERROR - %v", testNum, w.name, phoneNumber, err)
			w.log.Fail("DIAL ERROR - %v", err)

			// Try to recover - first with software reset
			w.log.Info("Attempting modem reset...")
			if resetErr := m.Reset(); resetErr != nil {
				w.log.Warn("Software reset failed: %v", resetErr)
				// Try USB reset as last resort
				if m.IsUSBDevice() {
					w.log.Info("Attempting USB reset...")
					if usbErr := m.USBReset(); usbErr != nil {
						w.log.Error("USB reset failed: %v", usbErr)
					} else {
						w.log.OK("USB reset successful")
					}
				}
			}

			return testResult{
				success: false,
				message: msg,
			}
		}

		// Determine if we should retry
		shouldRetry := false
		retryReason := ""

		// Check 1: Modem returned BUSY
		if !result.Success && result.Error == "BUSY" {
			shouldRetry = true
			retryReason = "BUSY (modem)"
		}

		// Check 2: CDR-based retry for failed dials only (not connected calls).
		// For successful connections, CDR is looked up after the call ends (post-EMSI).
		if !shouldRetry && !result.Success && w.asteriskCDRService != nil && w.asteriskCDRService.IsEnabled() {
			// Wait for CDR to be written
			w.log.Info("Waiting %v for CDR to be written...", cdrLookupDelay)
			select {
			case <-time.After(cdrLookupDelay):
			case <-ctx.Done():
				w.log.Info("Cancelled during CDR lookup delay")
				// Don't fail, just skip CDR check
			}

			lookupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			asteriskCDR, lookupErr := w.asteriskCDRService.LookupByPhone(lookupCtx, originalPhone, callTime)
			cancel()

			if lookupErr != nil {
				w.log.Warn("Asterisk CDR lookup failed for %s: %v (not retrying)", originalPhone, lookupErr)
			} else if asteriskCDR != nil {
				lastAsteriskCDR = asteriskCDR // Store for result
				if reason := asteriskCDR.RetryReason(); reason != "" {
					shouldRetry = true
					retryReason = reason
					w.log.Info("Asterisk CDR indicates retry: %s", reason)
				}
				// Log CDR info for diagnostics
				w.log.Info("Asterisk CDR: disposition=%s peer=%s duration=%ds billsec=%d cause=%s src=%s early_media=%t",
					asteriskCDR.Disposition, asteriskCDR.Peer, asteriskCDR.Duration, asteriskCDR.BillSec,
					asteriskCDR.HangupCauseString(), asteriskCDR.HangupSource, asteriskCDR.EarlyMedia)
			} else {
				w.log.Warn("Asterisk CDR not found for %s (not retrying)", originalPhone)
			}
		}

		// Should we retry?
		if shouldRetry && retryCount > 0 && retryAttempts < retryCount {
			w.log.Fail("DIAL FAILED - %s (%.1fs)", retryReason, result.DialTime.Seconds())
			retryAttempts++
			lastRetryReason = retryReason

			// IMPORTANT: If modem is in data mode (connected), hang up before retrying
			// This can happen when CDR says NO ANSWER but modem got CONNECT
			if m.InDataMode() {
				w.log.Info("Modem in data mode, hanging up before retry...")
				if err := m.Hangup(); err != nil {
					w.log.Warn("Hangup before retry failed: %v, resetting...", err)
					_ = m.Reset()
				}
			}

			// Call callback to emit result for this retry attempt
			// Note: runTest doesn't know about operators - the wrapper callback
			// (opRetryCallback in runTestWithFailover) fills in operator info
			if onRetryAttempt != nil {
				onRetryAttempt(retryAttempts, result.DialTime, retryReason, "", "")
			}

			// Wait for CDR to be written before diagnostic lookups
			w.log.Info("Waiting %v for CDR to be written...", cdrLookupDelay)
			select {
			case <-time.After(cdrLookupDelay):
			case <-ctx.Done():
				// Cancelled, skip CDR lookups
				continue
			}

			// AudioCodes CDR lookup for additional diagnostics
			if w.cdrService != nil && w.cdrService.IsEnabled() {
				lookupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				cdrData, lookupErr := w.cdrService.LookupByPhone(lookupCtx, originalPhone, callTime)
				cancel()
				if lookupErr != nil {
					w.log.Warn("AudioCodes CDR lookup failed for %s: %v", originalPhone, lookupErr)
				} else if cdrData != nil {
					lastCDRData = cdrData // Store for result
					w.log.Info("AudioCodes CDR: term=%s codec=%s MOS=%.1f jitter=%dms",
						cdrData.TermReason, cdrData.Codec,
						float64(cdrData.LocalMOSCQ)/10.0, cdrData.RTPJitter)
				} else {
					w.log.Warn("AudioCodes CDR not found for %s", originalPhone)
				}
			}

			// Asterisk CDR lookup for call routing diagnostics
			if w.asteriskCDRService != nil && w.asteriskCDRService.IsEnabled() {
				lookupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				asteriskCDR, lookupErr := w.asteriskCDRService.LookupByPhone(lookupCtx, originalPhone, callTime)
				cancel()
				if lookupErr != nil {
					w.log.Warn("Asterisk CDR lookup failed for %s: %v", originalPhone, lookupErr)
				} else if asteriskCDR != nil {
					lastAsteriskCDR = asteriskCDR // Store for result
					w.log.Info("Asterisk CDR: disposition=%s peer=%s duration=%ds billsec=%d cause=%s src=%s early_media=%t",
						asteriskCDR.Disposition, asteriskCDR.Peer, asteriskCDR.Duration, asteriskCDR.BillSec,
						asteriskCDR.HangupCauseString(), asteriskCDR.HangupSource, asteriskCDR.EarlyMedia)
				} else {
					w.log.Warn("Asterisk CDR not found for %s", originalPhone)
				}
			}

			continue
		}

		// Not retrying
		break
	}

	if !result.Success {
		retryInfo := ""
		if retryAttempts > 0 {
			retryInfo = fmt.Sprintf(" [after %d retries]", retryAttempts)
		}
		msg := fmt.Sprintf("Test %d [%s] %s: DIAL FAILED - %s (%.1fs)%s", testNum, w.name, phoneNumber, result.Error, result.DialTime.Seconds(), retryInfo)
		w.log.Fail("DIAL FAILED - %s (%.1fs)%s", result.Error, result.DialTime.Seconds(), retryInfo)

		// Ensure CDR lookup for failed dials (may have been skipped if shouldRetry was set)
		if lastAsteriskCDR == nil && w.asteriskCDRService != nil && w.asteriskCDRService.IsEnabled() {
			w.log.Info("Waiting %v for CDR to be written...", cdrLookupDelay)
			time.Sleep(cdrLookupDelay)
			lookupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			asteriskCDR, lookupErr := w.asteriskCDRService.LookupByPhone(lookupCtx, originalPhone, lastCallTime)
			cancel()
			if lookupErr != nil {
				w.log.Warn("Asterisk CDR lookup failed for %s: %v", originalPhone, lookupErr)
			} else if asteriskCDR != nil {
				lastAsteriskCDR = asteriskCDR
				w.log.Info("Asterisk: disposition=%s peer=%s cause=%s",
					asteriskCDR.Disposition, asteriskCDR.Peer, asteriskCDR.HangupCauseString())
			}
		}
		if lastCDRData == nil && w.cdrService != nil && w.cdrService.IsEnabled() {
			lookupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			cdrData, lookupErr := w.cdrService.LookupByPhone(lookupCtx, originalPhone, lastCallTime)
			cancel()
			if lookupErr != nil {
				w.log.Warn("AudioCodes CDR lookup failed for %s: %v", originalPhone, lookupErr)
			} else if cdrData != nil {
				lastCDRData = cdrData
			}
		}

		return testResult{
			success:     false,
			message:     msg,
			dialTime:    result.DialTime,
			cdrData:     lastCDRData,
			asteriskCDR: lastAsteriskCDR,
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
	if w.emsiConfig.InitialStrategy != "" {
		emsiCfg.InitialStrategy = w.emsiConfig.InitialStrategy
	}
	session := emsi.NewSessionWithInfoAndConfig(
		conn,
		w.emsiConfig.OurAddress,
		w.emsiConfig.SystemName,
		w.emsiConfig.Sysop,
		w.emsiConfig.Location,
		emsiCfg,
	)
	session.SetTimeout(w.emsiConfig.Timeout.Duration())
	session.SetDebug(w.logConfig.Debug)
	if w.logConfig.Debug {
		session.SetDebugFunc(func(format string, args ...interface{}) {
			w.log.EMSI(format, args...)
		})
	}

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
	} else {
		// Verify hangup actually worked - modem should not be in data mode
		if m.InDataMode() {
			w.log.Warn("Modem still in data mode after hangup, resetting...")
			_ = m.Reset()
		} else if w.logConfig.ShowRS232 {
			if status, err := m.GetStatus(); err == nil {
				w.log.RS232(status.DCD, status.DSR, status.CTS, status.RI)
			}
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

	// Final CDR lookups for the result (after call completes)
	// Wait for CDR to be written, then lookup using lastCallTime for accurate matching
	skipCDRLookup := false
	if (w.cdrService != nil && w.cdrService.IsEnabled()) || (w.asteriskCDRService != nil && w.asteriskCDRService.IsEnabled()) {
		w.log.Info("Waiting %v for final CDR to be written...", cdrLookupDelay)
		select {
		case <-time.After(cdrLookupDelay):
		case <-ctx.Done():
			w.log.Info("Cancelled during CDR wait, skipping lookups")
			skipCDRLookup = true
		}
	}

	if !skipCDRLookup {
		// AudioCodes CDR lookup for VoIP quality metrics
		if w.cdrService != nil && w.cdrService.IsEnabled() {
			lookupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			cdrData, lookupErr := w.cdrService.LookupByPhone(lookupCtx, originalPhone, lastCallTime)
			cancel()
			if lookupErr != nil {
				w.log.Warn("AudioCodes CDR lookup failed for %s: %v", originalPhone, lookupErr)
			} else if cdrData != nil {
				testRes.cdrData = cdrData
				w.log.Info("CDR: MOS=%.1f jitter=%dms delay=%dms loss=%d codec=%s term=%s",
					float64(cdrData.LocalMOSCQ)/10.0, cdrData.RTPJitter,
					cdrData.RTPDelay, cdrData.PacketLoss, cdrData.Codec, cdrData.TermReason)
			} else {
				w.log.Warn("AudioCodes CDR not found for %s", originalPhone)
			}
		}

		// Asterisk CDR lookup for call routing info
		if w.asteriskCDRService != nil && w.asteriskCDRService.IsEnabled() {
			lookupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			asteriskCDR, lookupErr := w.asteriskCDRService.LookupByPhone(lookupCtx, originalPhone, lastCallTime)
			cancel()
			if lookupErr != nil {
				w.log.Warn("Asterisk CDR lookup failed for %s: %v", originalPhone, lookupErr)
			} else if asteriskCDR != nil {
				testRes.asteriskCDR = asteriskCDR
				w.log.Info("Asterisk: disposition=%s peer=%s duration=%ds cause=%s src=%s early_media=%t",
					asteriskCDR.Disposition, asteriskCDR.Peer, asteriskCDR.Duration,
					asteriskCDR.HangupCauseString(), asteriskCDR.HangupSource, asteriskCDR.EarlyMedia)
			} else {
				w.log.Warn("Asterisk CDR not found for %s", originalPhone)
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
	retryCount int,
	retryDelay time.Duration,
	cdrLookupDelay time.Duration,
	cdrService *CDRService,
	asteriskCDRService *AsteriskCDRService,
	operatorCache *OperatorCache,
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
			retryCount,
			retryDelay,
			cdrLookupDelay,
			cdrService,
			asteriskCDRService,
			operatorCache,
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
	return p.SubmitJob(ctx, phoneJob{phone: phone, operatorName: operatorName, operatorPrefix: operatorPrefix, testNum: testNum})
}

// SubmitJob adds a phoneJob directly to the queue.
// Returns false if context is cancelled or pool is stopped.
func (p *ModemPool) SubmitJob(ctx context.Context, job phoneJob) bool {
	select {
	case p.phoneQueue <- job:
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
