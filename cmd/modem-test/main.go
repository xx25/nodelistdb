// modem-test is a CLI tool for testing modem communication.
// It can test AT commands, dial phone numbers, and perform EMSI handshakes.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/nodelistdb/internal/modemd/modem"
	"github.com/nodelistdb/internal/testing/protocols/emsi"
)

// CLI flags
var (
	configPath  = flag.String("config", "", "Path to YAML config file")
	phone       = flag.String("phone", "", "Phone number to dial (required for batch mode)")
	count       = flag.Int("count", -1, "Number of test calls (0=infinite, overrides config)")
	device      = flag.String("device", "", "Serial device (overrides config)")
	debug       = flag.Bool("debug", false, "Enable debug output (overrides config)")
	csvFile     = flag.String("csv", "", "Output results to CSV file")
	logFile     = flag.String("log", "", "Output log to file (in addition to stderr)")
	interactive = flag.Bool("interactive", false, "Interactive AT command mode")
	batch       = flag.Bool("batch", false, "Run batch test mode")
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Modem test tool for FidoNet node connectivity testing.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  # Run batch test with config file\n")
		fmt.Fprintf(os.Stderr, "  %s -config modem-test.yaml -phone 917 -batch\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  # Test multiple phones in circular order\n")
		fmt.Fprintf(os.Stderr, "  %s -phone 917,918,919 -count 9 -batch\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  # Test phone range (901 through 910)\n")
		fmt.Fprintf(os.Stderr, "  %s -phone 901-910 -count 20 -batch\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  # Mix ranges and individual numbers\n")
		fmt.Fprintf(os.Stderr, "  %s -phone 901-903,917,920-922 -batch\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  # Interactive AT command mode\n")
		fmt.Fprintf(os.Stderr, "  %s -interactive\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  # Override device\n")
		fmt.Fprintf(os.Stderr, "  %s -config modem.yaml -phone 917 -device /dev/ttyUSB0 -batch\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  # Output results to CSV file\n")
		fmt.Fprintf(os.Stderr, "  %s -phone 917 -count 10 -csv results.csv -batch\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  # Save log to file (also outputs to stderr)\n")
		fmt.Fprintf(os.Stderr, "  %s -phone 917 -log session.log -batch\n", os.Args[0])
	}

	flag.Parse()

	// Load or create config
	var cfg *Config
	var err error
	configFile := "(defaults)"

	if *configPath != "" {
		cfg, err = LoadConfig(*configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: Failed to load config: %v\n", err)
			os.Exit(1)
		}
		configFile = *configPath
	} else {
		cfg = DefaultConfig()
	}

	// Apply CLI overrides
	cfg.ApplyCLIOverrides(*device, *phone, *count, *debug, *csvFile)

	// Create logger
	log := NewTestLogger(cfg.Logging)

	// Set up log file if specified
	if *logFile != "" {
		f, err := os.OpenFile(*logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: Failed to open log file: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()
		// Write to both stderr and file
		log.SetOutput(io.MultiWriter(os.Stderr, f))
		fmt.Fprintf(os.Stderr, "Logging to: %s\n", *logFile)
	}

	// Initialize CDR service for VoIP quality metrics (optional)
	cdrService, err := NewCDRService(cfg.CDR)
	if err != nil {
		log.Warn("CDR service unavailable: %v", err)
		cdrService = &CDRService{} // Use disabled service
	} else if cdrService.IsEnabled() {
		log.Info("CDR service enabled for VoIP quality metrics")
		defer cdrService.Close()
	}

	// Validate required parameters for batch mode
	phones := cfg.GetPhones()
	if *batch && len(phones) == 0 {
		fmt.Fprintf(os.Stderr, "ERROR: Phone number is required for batch mode. Use -phone flag or set in config.\n")
		fmt.Fprintf(os.Stderr, "       Multiple phones: -phone 917,918,919\n")
		flag.Usage()
		os.Exit(1)
	}

	// Check for multi-modem mode
	if cfg.IsMultiModem() && (*batch || len(phones) > 0) {
		log.Info("Multi-modem mode detected with %d modem(s)", len(cfg.GetModemConfigs()))
		runBatchModeMulti(cfg, log, configFile, cdrService)
		return
	}

	// Single modem mode - original flow
	// Create modem configuration
	modemCfg := modem.Config{
		Device:           cfg.Modem.Device,
		BaudRate:         cfg.Modem.BaudRate,
		InitString:       getFirstInitCommand(cfg.Modem.InitCommands),
		InitCommands:     cfg.Modem.InitCommands,
		DialPrefix:       cfg.Modem.DialPrefix,
		HangupMethod:     cfg.Modem.HangupMethod,
		Debug:            cfg.Logging.Debug,
		DialTimeout:      cfg.Modem.DialTimeout.Duration(),
		CarrierTimeout:   cfg.Modem.CarrierTimeout.Duration(),
		ATCommandTimeout: cfg.Modem.ATCommandTimeout.Duration(),
		ReadTimeout:      cfg.Modem.ReadTimeout.Duration(),
	}

	// Set line stats command from post-disconnect commands
	if len(cfg.Modem.PostDisconnectCommands) > 0 {
		modemCfg.LineStatsCommand = cfg.Modem.PostDisconnectCommands[0]
	}

	// Create modem
	m, err := modem.New(modemCfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: Failed to create modem: %v\n", err)
		os.Exit(1)
	}

	// Setup signal handler for cleanup
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Fprintln(os.Stderr, "\nReceived interrupt, cleaning up...")
		if m.InDataMode() {
			fmt.Fprintln(os.Stderr, "Hanging up...")
			if err := m.Hangup(); err != nil {
				fmt.Fprintf(os.Stderr, "Hangup error: %v\n", err)
			}
		}
		m.Close()
		os.Exit(0)
	}()

	// Open modem
	log.Init("Opening %s...", cfg.Modem.Device)
	if err := m.Open(); err != nil {
		log.Fail("Failed to open modem: %v", err)
		os.Exit(1)
	}
	defer m.Close()

	log.OK("Modem opened and initialized successfully")

	// Route to appropriate mode
	if *interactive {
		runInteractiveMode(m, log)
	} else if *batch || len(phones) > 0 {
		runBatchMode(m, cfg, log, configFile, cdrService)
	} else {
		// Default: show modem info
		runInfoMode(m, log)
	}
}

func getFirstInitCommand(cmds []string) string {
	if len(cmds) > 0 {
		return cmds[0]
	}
	return "ATZ"
}

func runInfoMode(m *modem.Modem, log *TestLogger) {
	fmt.Fprintln(os.Stderr, "\n--- Modem Info ---")
	info, err := m.GetInfo()
	if err != nil {
		log.Fail("Failed to get modem info: %v", err)
	} else {
		if info.Manufacturer != "" {
			fmt.Fprintf(os.Stderr, "Manufacturer: %s\n", info.Manufacturer)
		}
		if info.Model != "" {
			fmt.Fprintf(os.Stderr, "Model: %s\n", info.Model)
		}
		if info.Firmware != "" {
			fmt.Fprintf(os.Stderr, "Firmware: %s\n", info.Firmware)
		}
		fmt.Fprintf(os.Stderr, "Raw response:\n%s\n", info.RawResponse)
	}

	// Get modem status
	status, err := m.GetStatus()
	if err != nil {
		log.Fail("Failed to get modem status: %v", err)
	} else {
		fmt.Fprintln(os.Stderr, "\n--- Modem Status ---")
		fmt.Fprintf(os.Stderr, "DCD (Carrier): %v\n", status.DCD)
		fmt.Fprintf(os.Stderr, "DSR (Ready):   %v\n", status.DSR)
		fmt.Fprintf(os.Stderr, "CTS (Clear):   %v\n", status.CTS)
		fmt.Fprintf(os.Stderr, "RI (Ring):     %v\n", status.RI)
	}

	fmt.Fprintln(os.Stderr, "\nUse -interactive for AT command mode or -batch -phone <number> for batch testing.")
}

func runInteractiveMode(m *modem.Modem, log *TestLogger) {
	fmt.Fprintln(os.Stderr, "\n=== Interactive AT Command Mode ===")
	fmt.Fprintln(os.Stderr, "Enter AT commands (type 'quit' to exit)")

	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Fprint(os.Stderr, "> ")
		input, err := reader.ReadString('\n')
		if err != nil {
			break
		}

		cmd := strings.TrimSpace(input)
		if cmd == "" {
			continue
		}
		if strings.ToLower(cmd) == "quit" || strings.ToLower(cmd) == "exit" {
			break
		}

		log.TXString(cmd + "\r")
		response, err := m.SendAT(cmd, 10*time.Second)
		if err != nil {
			log.Fail("Error: %v", err)
		} else {
			log.RXString(response)
			fmt.Fprintf(os.Stderr, "%s\n", response)
		}
	}
}

func runBatchMode(m *modem.Modem, cfg *Config, log *TestLogger, configFile string, cdrService *CDRService) {
	phones := cfg.GetPhones()
	testCount := cfg.Test.Count
	infinite := testCount <= 0
	interDelay := cfg.Test.InterDelay.Duration()

	// Print session header
	if infinite {
		log.PrintHeader(configFile, cfg.Modem.Device, phones, -1) // -1 signals infinite
	} else {
		log.PrintHeader(configFile, cfg.Modem.Device, phones, testCount)
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

	// Stats
	var success, failed int
	var totalDialTime, totalEmsiTime time.Duration
	results := make([]string, 0, testCount)
	sessionStart := time.Now()

	// Per-phone statistics
	phoneStats := make(map[string]*PhoneStats)
	for _, phone := range phones {
		phoneStats[phone] = &PhoneStats{Phone: phone}
	}

	for i := 1; infinite || i <= testCount; i++ {
		// Select phone in circular order
		phoneIndex := (i - 1) % len(phones)
		currentPhone := phones[phoneIndex]

		if infinite {
			log.PrintTestHeader(i, 0) // 0 signals infinite
		} else {
			log.PrintTestHeader(i, testCount)
		}

		result := runSingleTest(m, cfg, log, i, currentPhone)
		results = append(results, result.message)

		// Lookup CDR data for VoIP quality metrics
		if cdrService != nil && cdrService.IsEnabled() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			cdrData, err := cdrService.LookupByPhone(ctx, currentPhone, time.Now())
			cancel()
			if err != nil {
				log.Debug("CDR lookup failed: %v", err)
			} else if cdrData != nil {
				result.cdrData = cdrData
				log.Info("CDR: MOS=%.1f jitter=%dms delay=%dms loss=%d codec=%s",
					float64(cdrData.LocalMOSCQ)/10.0, cdrData.RTPJitter,
					cdrData.RTPDelay, cdrData.PacketLoss, cdrData.Codec)
			}
		}

		// Write to CSV if enabled
		if csvWriter != nil {
			rec := RecordFromTestResult(
				i,
				currentPhone,
				result.success,
				result.dialTime,
				result.connectSpeed,
				result.connectString,
				result.emsiTime,
				result.emsiError,
				result.emsiInfo,
				result.lineStats,
				result.cdrData,
			)
			if err := csvWriter.WriteRecord(rec); err != nil {
				log.Error("Failed to write CSV record: %v", err)
			}
		}

		// Update per-phone stats
		stats := phoneStats[currentPhone]
		stats.Total++

		if result.success {
			success++
			stats.Success++
			totalDialTime += result.dialTime
			totalEmsiTime += result.emsiTime
			stats.TotalDialTime += result.dialTime
			stats.TotalEmsiTime += result.emsiTime
		} else {
			failed++
			stats.Failed++
		}

		if infinite || i < testCount {
			log.Info("Waiting %v before next test...", interDelay)
			time.Sleep(interDelay)
		}
	}

	// Calculate averages
	var avgDialTime, avgEmsiTime time.Duration
	if success > 0 {
		avgDialTime = totalDialTime / time.Duration(success)
		avgEmsiTime = totalEmsiTime / time.Duration(success)
	}

	// Print summary with per-phone stats
	log.PrintSummaryWithPhoneStats(testCount, success, failed, time.Since(sessionStart), avgDialTime, avgEmsiTime, results, phoneStats)
}

type testResult struct {
	success       bool
	message       string
	dialTime      time.Duration
	emsiTime      time.Duration
	connectSpeed  int
	connectString string
	emsiError     error
	emsiInfo      *EMSIDetails
	lineStats     *LineStats
	cdrData       *CDRData // VoIP quality metrics from AudioCodes CDR
}

func runSingleTest(m *modem.Modem, cfg *Config, log *TestLogger, testNum int, phoneNumber string) testResult {
	// Dial
	log.Dial("%s -> ATDT%s", phoneNumber, phoneNumber)
	result, err := m.DialNumber(phoneNumber)
	if err != nil {
		msg := fmt.Sprintf("Test %d [%s]: DIAL ERROR - %v", testNum, phoneNumber, err)
		log.Fail("%s", msg)

		// Try to recover
		log.Info("Attempting modem reset...")
		_ = m.Reset()

		return testResult{
			success: false,
			message: msg,
		}
	}

	if !result.Success {
		msg := fmt.Sprintf("Test %d [%s]: DIAL FAILED - %s (%.1fs)", testNum, phoneNumber, result.Error, result.DialTime.Seconds())
		log.Fail("%s", msg)
		return testResult{
			success: false,
			message: msg,
		}
	}

	// Log connection with full CONNECT string and parsed speeds
	if result.ConnectString != "" {
		log.OK("%s (%.1fs)", result.ConnectString, result.DialTime.Seconds())
	} else {
		log.OK("Connected at %d bps (%.1fs)", result.ConnectSpeed, result.DialTime.Seconds())
	}
	// Show parsed line speed if TX/RX available
	if result.ConnectSpeedTX > 0 || result.ConnectSpeedRX > 0 {
		log.Info("Line speed: %d bps (TX:%d RX:%d)", result.ConnectSpeed, result.ConnectSpeedTX, result.ConnectSpeedRX)
	}

	// Log RS232 status after connect
	if status, err := m.GetStatus(); err == nil {
		log.RS232(status.DCD, status.DSR, status.CTS, status.RI)
	}

	// EMSI handshake
	log.EMSI("Starting handshake...")
	conn := m.GetConn()
	emsiCfg := emsi.DefaultConfig()
	emsiCfg.Protocols = cfg.EMSI.Protocols
	session := emsi.NewSessionWithInfoAndConfig(
		conn,
		cfg.EMSI.OurAddress,
		cfg.EMSI.SystemName,
		cfg.EMSI.Sysop,
		cfg.EMSI.Location,
		emsiCfg,
	)
	session.SetTimeout(cfg.EMSI.Timeout.Duration())

	emsiStart := time.Now()
	emsiErr := session.Handshake()
	emsiTime := time.Since(emsiStart)

	var testRes testResult
	testRes.dialTime = result.DialTime
	testRes.connectSpeed = result.ConnectSpeed
	testRes.connectString = result.ConnectString

	// Get completion info
	completionReason := session.GetCompletionReason()
	timing := session.GetHandshakeTiming()

	if emsiErr != nil {
		msg := fmt.Sprintf("Test %d [%s]: CONNECT %d, EMSI FAILED - %v", testNum, phoneNumber, result.ConnectSpeed, emsiErr)
		log.Fail("EMSI handshake failed: %v (%.1fs) [%s]", emsiErr, emsiTime.Seconds(), completionReason)
		testRes.success = false
		testRes.message = msg
		testRes.emsiError = emsiErr
	} else {
		info := session.GetRemoteInfo()
		sysName := ""
		if info != nil {
			sysName = info.SystemName
			// Store EMSI details for CSV
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
			// Print detailed EMSI info
			log.PrintEMSIDetails(testRes.emsiInfo)
		}
		msg := fmt.Sprintf("Test %d [%s]: OK - CONNECT %d, EMSI %.1fs, %s", testNum, phoneNumber, result.ConnectSpeed, emsiTime.Seconds(), sysName)
		// Show timing breakdown: initial phase vs DAT exchange
		log.OK("EMSI complete (%.1fs) [%s] init=%.1fs dat=%.1fs",
			emsiTime.Seconds(), completionReason,
			timing.InitialPhase.Seconds(), timing.DATExchange.Seconds())
		testRes.success = true
		testRes.message = msg
		testRes.emsiTime = emsiTime
	}

	// Hangup
	log.Hangup("Disconnecting...")
	if err := m.Hangup(); err != nil {
		log.Fail("Hangup error: %v, resetting...", err)
		_ = m.Reset()
	} else {
		// Log RS232 status after hangup
		if status, err := m.GetStatus(); err == nil {
			log.RS232(status.DCD, status.DSR, status.CTS, status.RI)
		}
	}

	// Get line stats if not in data mode and DCD is low
	canGetStats := !m.InDataMode() && len(cfg.Modem.PostDisconnectCommands) > 0
	if canGetStats {
		// Verify DCD is actually low (modem disconnected)
		if status, err := m.GetStatus(); err == nil && status.DCD {
			log.Warn("Skipping stats collection - modem still connected (DCD high)")
			canGetStats = false
		}
	}

	if canGetStats {
		// Drain any pending modem output (e.g., NO CARRIER) before sending commands
		// Wait up to 2 seconds, but stop early after 200ms of silence
		m.DrainPendingResponse(2 * time.Second)

		// Wait for modem to compute statistics (USR modems need ~2s after hangup)
		if delay := cfg.Modem.PostDisconnectDelay.Duration(); delay > 0 {
			time.Sleep(delay)
		}

		// Run all post-disconnect commands
		for _, cmd := range cfg.Modem.PostDisconnectCommands {
			response, err := m.SendAT(cmd, cfg.Modem.ATCommandTimeout.Duration())
			if err == nil {
				log.PrintLineStatsWithProfile(response, cfg.Modem.StatsProfile)
				// Parse and store stats for CSV (use first successful response)
				if testRes.lineStats == nil && cfg.Modem.StatsProfile != "" && cfg.Modem.StatsProfile != "raw" {
					testRes.lineStats = ParseStats(response, cfg.Modem.StatsProfile)
				}
			}
		}
	}

	return testRes
}
