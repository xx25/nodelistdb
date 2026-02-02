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
	prefix       = flag.String("prefix", "", "Phone prefix to fetch PSTN nodes from API (e.g., \"+7\")")
	cmOnly       = flag.Bool("cm-only", false, "Only test CM (24/7) nodes (prefix mode only)")
	perOperator  = flag.Int("per-operator", -1, "Calls per operator per phone (mutually exclusive with -count)")
	retryCount    = flag.Int("retry", 20, "Number of retries for failed calls (0=disabled)")
	retryDelayStr = flag.String("retry-delay", "5s", "Delay between retry attempts")
	interDelayStr = flag.String("delay", "", "Delay between consecutive tests (default: config inter_delay or 5s)")
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
		fmt.Fprintf(os.Stderr, "  # 2 calls per operator per phone (with operators in config)\n")
		fmt.Fprintf(os.Stderr, "  %s -config modem-test.yaml -per-operator 2 -phone 917 -batch\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  # Save log to file (also outputs to stderr)\n")
		fmt.Fprintf(os.Stderr, "  %s -phone 917 -log session.log -batch\n", os.Args[0])
	}

	flag.Parse()

	// Validate mutually exclusive flags
	if *count >= 0 && *perOperator >= 0 {
		fmt.Fprintf(os.Stderr, "ERROR: -count and -per-operator are mutually exclusive\n")
		flag.Usage()
		os.Exit(1)
	}
	if *perOperator == 0 {
		fmt.Fprintf(os.Stderr, "ERROR: -per-operator must be positive\n")
		os.Exit(1)
	}

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

	// Apply CLI overrides (config-file fields only)
	cfg.ApplyCLIOverrides(*device, *phone, *debug, *csvFile)

	// Set CLI-only test parameters
	if *count >= 0 {
		cfg.Test.Count = *count
	}
	if *perOperator > 0 {
		cfg.Test.CallsPerOperator = *perOperator
	}
	cfg.Test.RetryCount = *retryCount
	retryDelay, err := time.ParseDuration(*retryDelayStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: invalid -retry-delay value %q: %v\n", *retryDelayStr, err)
		os.Exit(1)
	}
	cfg.Test.RetryDelay = Duration(retryDelay)
	if *interDelayStr != "" {
		interDelay, err := time.ParseDuration(*interDelayStr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: invalid -delay value %q: %v\n", *interDelayStr, err)
			os.Exit(1)
		}
		cfg.Test.InterDelay = Duration(interDelay)
	}

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
		// Write to both stderr (with colors) and file (without colors)
		log.SetOutput(io.MultiWriter(os.Stderr, NewStripANSIWriter(f)))
		fmt.Fprintf(os.Stderr, "Logging to: %s\n", *logFile)
	}

	// Initialize CDR service for VoIP quality metrics (optional)
	cdrService, err := NewCDRService(cfg.CDR)
	if err != nil {
		log.Warn("CDR service unavailable: %v", err)
		cdrService = &CDRService{} // Use disabled service
	} else if cdrService.IsEnabled() {
		log.Info("AudioCodes CDR service enabled for VoIP quality metrics")
		defer cdrService.Close()
	}

	// Initialize Asterisk CDR service for call routing info (optional)
	asteriskCDRService, err := NewAsteriskCDRService(cfg.AsteriskCDR)
	if err != nil {
		log.Warn("Asterisk CDR service unavailable: %v", err)
		asteriskCDRService = &AsteriskCDRService{} // Use disabled service
	} else if asteriskCDRService.IsEnabled() {
		log.Info("Asterisk CDR service enabled for call routing info")
		defer asteriskCDRService.Close()
	}

	// Initialize PostgreSQL results writer (optional)
	pgWriter, err := NewPostgresResultsWriter(cfg.PostgresResults)
	if err != nil {
		log.Warn("PostgreSQL results writer unavailable: %v", err)
		pgWriter = &PostgresResultsWriter{} // Use disabled writer
	} else if pgWriter.IsEnabled() {
		log.Info("PostgreSQL results writer enabled: table=%s", cfg.PostgresResults.TableName)
		defer pgWriter.Close()
	}

	// Initialize MySQL results writer (optional)
	mysqlWriter, err := NewMySQLResultsWriter(cfg.MySQLResults)
	if err != nil {
		log.Warn("MySQL results writer unavailable: %v", err)
		mysqlWriter = &MySQLResultsWriter{} // Use disabled writer
	} else if mysqlWriter.IsEnabled() {
		log.Info("MySQL results writer enabled: table=%s", cfg.MySQLResults.TableName)
		defer mysqlWriter.Close()
	}

	// Prefix mode: fetch PSTN nodes from API and replace phone list
	var nodeLookup map[string]*NodeTarget
	var filteredNodes []NodeTarget

	pfx := *prefix
	if pfx == "" {
		pfx = cfg.Test.Prefix
	}
	if *cmOnly && pfx == "" {
		fmt.Fprintf(os.Stderr, "WARNING: -cm-only has no effect without -prefix\n")
	}
	if pfx != "" {
		if cfg.NodelistDB.URL == "" {
			fmt.Fprintf(os.Stderr, "ERROR: nodelistdb.url is required when using -prefix\n")
			os.Exit(1)
		}
		if *phone != "" {
			fmt.Fprintf(os.Stderr, "WARNING: -phone is ignored when -prefix is set\n")
		}

		log.Info("Fetching PSTN nodes from %s...", cfg.NodelistDB.URL)
		allNodes, err := FetchPSTNNodes(cfg.NodelistDB.URL, 30*time.Second)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: Failed to fetch PSTN nodes: %v\n", err)
			os.Exit(1)
		}
		log.Info("Fetched %d PSTN nodes total", len(allNodes))

		if len(allNodes) >= 10000 {
			log.Warn("Fetched exactly %d nodes — results may be truncated by API limit", len(allNodes))
		}

		filtered := FilterByPrefix(allNodes, pfx)
		log.Info("Filtered to %d nodes matching prefix %q", len(filtered), pfx)

		if *cmOnly {
			var cmNodes []NodeTarget
			for _, n := range filtered {
				if n.IsCM {
					cmNodes = append(cmNodes, n)
				}
			}
			log.Info("CM-only filter: %d of %d nodes are CM (24/7)", len(cmNodes), len(filtered))
			filtered = cmNodes
		}

		if len(filtered) == 0 {
			fmt.Fprintf(os.Stderr, "ERROR: No PSTN nodes found matching prefix %q\n", pfx)
			os.Exit(1)
		}

		// Strip leading + from phone numbers for modem dialing.
		// Prefix matching is already done, and ATDT doesn't understand +.
		for i := range filtered {
			filtered[i].Phone = strings.TrimPrefix(filtered[i].Phone, "+")
		}

		// Replace phone list with API-sourced phones
		cfg.Test.Phones = UniquePhones(filtered)
		cfg.Test.Phone = ""

		// Default to 1 call per operator if nothing explicitly set
		if cfg.Test.CallsPerOperator <= 0 && cfg.Test.Count <= 0 {
			cfg.Test.CallsPerOperator = 1
		}

		nodeLookup = BuildNodeLookupByPhone(filtered)
		filteredNodes = filtered

		// Auto-enable batch mode
		*batch = true
	}

	// Validate: need count or per-operator (unless prefix mode set defaults)
	if *batch && cfg.Test.Count <= 0 && cfg.Test.CallsPerOperator <= 0 && pfx == "" {
		fmt.Fprintf(os.Stderr, "ERROR: specify -count N (total calls) or -per-operator N (calls per operator per phone)\n")
		os.Exit(1)
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
		runBatchModeMulti(cfg, log, configFile, cdrService, asteriskCDRService, pgWriter, mysqlWriter, nodeLookup, filteredNodes)
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
		DebugWriter:      log.GetOutput(),
		DialTimeout:      cfg.Modem.DialTimeout.Duration(),
		CarrierTimeout:   cfg.Modem.CarrierTimeout.Duration(),
		ATCommandTimeout: cfg.Modem.ATCommandTimeout.Duration(),
		ReadTimeout:      cfg.Modem.ReadTimeout.Duration(),
		// DTR hangup timing
		DTRHoldTime:      cfg.Modem.DTRHoldTime.Duration(),
		DTRWaitInterval:  cfg.Modem.DTRWaitInterval.Duration(),
		DTRMaxWaitTime:   cfg.Modem.DTRMaxWaitTime.Duration(),
		DTRStabilizeTime: cfg.Modem.DTRStabilizeTime.Duration(),
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
		runBatchMode(m, cfg, log, configFile, cdrService, asteriskCDRService, pgWriter, mysqlWriter, filteredNodes)
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

func runBatchMode(m *modem.Modem, cfg *Config, log *TestLogger, configFile string, cdrService *CDRService, asteriskCDRService *AsteriskCDRService, pgWriter *PostgresResultsWriter, mysqlWriter *MySQLResultsWriter, filteredNodes []NodeTarget) {
	phones := cfg.GetPhones()
	operators := cfg.GetOperators()
	testCount := cfg.GetTotalTestCount()
	infinite := testCount <= 0 && !cfg.IsPerOperatorMode() // Per-operator mode is never infinite
	interDelay := cfg.Test.InterDelay.Duration()
	perOperatorMode := cfg.IsPerOperatorMode()
	callsPerOperator := cfg.Test.CallsPerOperator

	// Setup context for cancellation (e.g., Ctrl-C during BUSY retry)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	cancelled := false
	go func() {
		<-sigChan
		fmt.Fprintln(os.Stderr, "\nReceived interrupt, stopping...")
		cancelled = true
		cancel()
	}()

	// If no operators configured, use a single "no operator" entry for simpler loop logic
	if len(operators) == 0 {
		operators = []OperatorConfig{{Name: "", Prefix: ""}}
	}

	// Print session header
	if infinite {
		log.PrintHeader(configFile, cfg.Modem.Device, phones, -1) // -1 signals infinite
	} else {
		log.PrintHeader(configFile, cfg.Modem.Device, phones, testCount)
	}

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

	// Per-operator statistics
	operatorStats := make(map[string]*OperatorStats)
	for _, op := range operators {
		operatorStats[op.Name] = &OperatorStats{Name: op.Name, Prefix: op.Prefix}
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

	// Prefix mode: use ScheduleNodes for time-aware job ordering
	var schedChan <-chan phoneJob
	if len(filteredNodes) > 0 {
		schedChan = ScheduleNodes(ctx, filteredNodes, operators, cfg.Test.CallsPerOperator, log)
	}

	for i := 1; ; i++ {
		// Check for cancellation
		if cancelled {
			log.Info("Test loop cancelled")
			break
		}

		var currentPhone string
		var currentOperator OperatorConfig

		if schedChan != nil {
			// Prefix/schedule mode: consume pre-scheduled, time-aware jobs
			job, ok := <-schedChan
			if !ok {
				// All scheduled nodes processed
				break
			}
			currentPhone = job.phone
			currentOperator = OperatorConfig{Name: job.operatorName, Prefix: job.operatorPrefix}

			log.PrintTestHeader(i, testCount)

			// Log node info
			if job.nodeAddress != "" {
				log.Info("Node: %s %s (sysop: %s)", job.nodeAddress, job.nodeSystemName, job.nodeSysop)
			}
		} else if perOperatorMode {
			// Per-operator mode: find next combo that hasn't reached its limit
			found := false
			for _, phone := range phones {
				for _, op := range operators {
					key := comboKey{phone: phone, operator: op.Name}
					if callCounts[key] < callsPerOperator {
						currentPhone = phone
						currentOperator = op
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
				break
			}
			callCounts[comboKey{phone: currentPhone, operator: currentOperator.Name}]++

			if infinite {
				log.PrintTestHeader(i, 0)
			} else {
				log.PrintTestHeader(i, testCount)
			}
		} else {
			// Legacy mode: round-robin through all combinations
			if !infinite && i > testCount {
				break
			}
			comboIndex := (i - 1) % totalCombinations
			phoneIndex := comboIndex / len(operators)
			operatorIndex := comboIndex % len(operators)
			currentPhone = phones[phoneIndex]
			currentOperator = operators[operatorIndex]

			if infinite {
				log.PrintTestHeader(i, 0)
			} else {
				log.PrintTestHeader(i, testCount)
			}
		}

		// Log operator info if configured
		if currentOperator.Name != "" {
			log.Info("Operator: %s (prefix: %q)", currentOperator.Name, currentOperator.Prefix)
		}

		// Dial with operator prefix prepended
		dialPhone := currentOperator.Prefix + currentPhone
		result := runSingleTest(ctx, m, cfg, log, cdrService, asteriskCDRService, i, dialPhone, currentPhone)
		results = append(results, result.message)

		// Lookup CDR data for VoIP quality metrics (AudioCodes)
		// Note: Use original phone (without operator prefix) for CDR lookup
		if cdrService != nil && cdrService.IsEnabled() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			cdrData, err := cdrService.LookupByPhone(ctx, currentPhone, time.Now())
			cancel()
			if err != nil {
				log.Warn("AudioCodes CDR lookup failed for %s: %v", currentPhone, err)
			} else if cdrData != nil {
				result.cdrData = cdrData
				log.Info("CDR: MOS=%.1f jitter=%dms delay=%dms loss=%d codec=%s term=%s",
					float64(cdrData.LocalMOSCQ)/10.0, cdrData.RTPJitter,
					cdrData.RTPDelay, cdrData.PacketLoss, cdrData.Codec, cdrData.TermReason)
			} else {
				log.Warn("AudioCodes CDR not found for phone %s", currentPhone)
			}
		}

		// Lookup Asterisk CDR for call routing info
		if asteriskCDRService != nil && asteriskCDRService.IsEnabled() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			asteriskCDR, err := asteriskCDRService.LookupByPhone(ctx, currentPhone, time.Now())
			cancel()
			if err != nil {
				log.Warn("Asterisk CDR lookup failed for %s: %v", currentPhone, err)
			} else if asteriskCDR != nil {
				result.asteriskCDR = asteriskCDR
				log.Info("Asterisk: disposition=%s peer=%s duration=%ds cause=%s src=%s early_media=%t",
					asteriskCDR.Disposition, asteriskCDR.Peer, asteriskCDR.Duration,
					asteriskCDR.HangupCauseString(), asteriskCDR.HangupSource, asteriskCDR.EarlyMedia)
			} else {
				log.Warn("Asterisk CDR not found for phone %s", currentPhone)
			}
		}

		// Write to CSV and databases if enabled
		var rec *TestRecord
		if csvWriter != nil || (pgWriter != nil && pgWriter.IsEnabled()) || (mysqlWriter != nil && mysqlWriter.IsEnabled()) {
			rec = RecordFromTestResult(
				i,
				currentPhone, // Store original phone without operator prefix
				currentOperator.Name,
				currentOperator.Prefix,
				result.success,
				result.dialTime,
				result.connectSpeed,
				result.connectString,
				result.emsiTime,
				result.emsiError,
				result.emsiInfo,
				result.lineStats,
				result.cdrData,
				result.asteriskCDR,
			)
		}
		if csvWriter != nil && rec != nil {
			if err := csvWriter.WriteRecord(rec); err != nil {
				log.Error("Failed to write CSV record: %v", err)
			}
		}

		// Write to PostgreSQL if enabled
		if pgWriter != nil && pgWriter.IsEnabled() && rec != nil {
			if err := pgWriter.WriteRecord(rec); err != nil {
				log.Error("Failed to write PostgreSQL record: %v", err)
			}
		}

		// Write to MySQL if enabled
		if mysqlWriter != nil && mysqlWriter.IsEnabled() && rec != nil {
			if err := mysqlWriter.WriteRecord(rec); err != nil {
				log.Error("Failed to write MySQL record: %v", err)
			}
		}

		// Update per-phone stats
		stats := phoneStats[currentPhone]
		stats.Total++

		// Update per-operator stats
		opStats := operatorStats[currentOperator.Name]
		opStats.Total++

		if result.success {
			success++
			stats.Success++
			opStats.Success++
			totalDialTime += result.dialTime
			totalEmsiTime += result.emsiTime
			stats.TotalDialTime += result.dialTime
			stats.TotalEmsiTime += result.emsiTime
			opStats.TotalDialTime += result.dialTime
			opStats.TotalEmsiTime += result.emsiTime
		} else {
			failed++
			stats.Failed++
			opStats.Failed++
		}

		if infinite || i < testCount {
			log.Info("Waiting %v before next test...", interDelay)
			select {
			case <-time.After(interDelay):
			case <-ctx.Done():
				log.Info("Inter-test delay cancelled")
			}
		}
	}

	// Calculate averages
	var avgDialTime, avgEmsiTime time.Duration
	if success > 0 {
		avgDialTime = totalDialTime / time.Duration(success)
		avgEmsiTime = totalEmsiTime / time.Duration(success)
	}

	// Print summary with per-phone and per-operator stats
	log.PrintSummaryWithStats(testCount, success, failed, time.Since(sessionStart), avgDialTime, avgEmsiTime, results, phoneStats, operatorStats)
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
	cdrData       *CDRData         // VoIP quality metrics from AudioCodes CDR
	asteriskCDR   *AsteriskCDRData // Call routing info from Asterisk CDR
}

func runSingleTest(ctx context.Context, m *modem.Modem, cfg *Config, log *TestLogger, cdrService *CDRService, asteriskCDRService *AsteriskCDRService, testNum int, phoneNumber string, originalPhone string) testResult {
	// Determine retry settings
	retryCount := cfg.GetRetryCount()
	retryDelay := cfg.GetRetryDelay()
	cdrLookupDelay := cfg.GetCDRLookupDelay()

	var result *modem.DialResult
	var err error
	retryAttempts := 0
	var lastRetryReason string

	// Dial with retry on BUSY or CDR-detected failures
	for {
		// Check for cancellation before retry wait
		if retryAttempts > 0 {
			log.Info("Retry %d/%d (%s), waiting %v...", retryAttempts, retryCount, lastRetryReason, retryDelay)
			select {
			case <-time.After(retryDelay):
			case <-ctx.Done():
				log.Info("Cancelled during retry wait")
				return testResult{
					success: false,
					message: fmt.Sprintf("Test %d [%s]: CANCELLED", testNum, phoneNumber),
				}
			}
		}

		// Check for cancellation before dialing
		select {
		case <-ctx.Done():
			log.Info("Cancelled before dial")
			return testResult{
				success: false,
				message: fmt.Sprintf("Test %d [%s]: CANCELLED", testNum, phoneNumber),
			}
		default:
		}

		log.Dial("%s -> ATDT%s", phoneNumber, phoneNumber)
		result, err = m.DialNumber(phoneNumber)
		callTime := time.Now()

		if err != nil {
			msg := fmt.Sprintf("Test %d [%s]: DIAL ERROR - %v", testNum, phoneNumber, err)
			log.Fail("%s", msg)

			// Try to recover - first with software reset
			log.Info("Attempting modem reset...")
			if resetErr := m.Reset(); resetErr != nil {
				log.Warn("Software reset failed: %v", resetErr)
				// Try USB reset as last resort
				if m.IsUSBDevice() {
					log.Info("Attempting USB reset...")
					if usbErr := m.USBReset(); usbErr != nil {
						log.Error("USB reset failed: %v", usbErr)
					} else {
						log.OK("USB reset successful")
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

		// Check 2: CDR-based retry (only if not already retrying for modem BUSY)
		// Wait for CDR to be written, then check disposition
		if !shouldRetry && asteriskCDRService != nil && asteriskCDRService.IsEnabled() {
			// Wait for CDR to be written
			log.Info("Waiting %v for CDR to be written...", cdrLookupDelay)
			select {
			case <-time.After(cdrLookupDelay):
			case <-ctx.Done():
				log.Info("Cancelled during CDR lookup delay")
				// Don't fail, just skip CDR check
			}

			lookupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			asteriskCDR, lookupErr := asteriskCDRService.LookupByPhone(lookupCtx, originalPhone, callTime)
			cancel()

			if lookupErr != nil {
				log.Warn("Asterisk CDR lookup failed for %s: %v (not retrying)", originalPhone, lookupErr)
			} else if asteriskCDR != nil {
				if reason := asteriskCDR.RetryReason(); reason != "" {
					shouldRetry = true
					retryReason = reason
					log.Info("Asterisk CDR indicates retry: %s", reason)
				}
				// Log CDR info for diagnostics
				log.Info("Asterisk CDR: disposition=%s peer=%s duration=%ds billsec=%d cause=%s src=%s early_media=%t",
					asteriskCDR.Disposition, asteriskCDR.Peer, asteriskCDR.Duration, asteriskCDR.BillSec,
					asteriskCDR.HangupCauseString(), asteriskCDR.HangupSource, asteriskCDR.EarlyMedia)
			} else {
				log.Warn("Asterisk CDR not found for %s (not retrying)", originalPhone)
			}
		}

		// Should we retry?
		if shouldRetry && retryCount > 0 && retryAttempts < retryCount {
			log.Fail("DIAL FAILED - %s (%.1fs)", retryReason, result.DialTime.Seconds())
			retryAttempts++
			lastRetryReason = retryReason

			// IMPORTANT: If modem is in data mode (connected), hang up before retrying
			// This can happen when CDR says NO ANSWER but modem got CONNECT
			if m.InDataMode() {
				log.Info("Modem in data mode, hanging up before retry...")
				if err := m.Hangup(); err != nil {
					log.Warn("Hangup before retry failed: %v, resetting...", err)
					_ = m.Reset()
				}
			}

			// Wait for CDR to be written before diagnostic lookups
			log.Info("Waiting %v for CDR to be written...", cdrLookupDelay)
			select {
			case <-time.After(cdrLookupDelay):
			case <-ctx.Done():
				// Cancelled, skip CDR lookups
				continue
			}

			// AudioCodes CDR lookup for additional diagnostics
			if cdrService != nil && cdrService.IsEnabled() {
				lookupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				cdrData, lookupErr := cdrService.LookupByPhone(lookupCtx, originalPhone, callTime)
				cancel()
				if lookupErr != nil {
					log.Warn("AudioCodes CDR lookup failed for %s: %v", originalPhone, lookupErr)
				} else if cdrData != nil {
					log.Info("AudioCodes CDR: term=%s codec=%s MOS=%.1f jitter=%dms",
						cdrData.TermReason, cdrData.Codec,
						float64(cdrData.LocalMOSCQ)/10.0, cdrData.RTPJitter)
				} else {
					log.Warn("AudioCodes CDR not found for %s", originalPhone)
				}
			}

			// Asterisk CDR lookup for call routing diagnostics
			if asteriskCDRService != nil && asteriskCDRService.IsEnabled() {
				lookupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				asteriskCDR, lookupErr := asteriskCDRService.LookupByPhone(lookupCtx, originalPhone, callTime)
				cancel()
				if lookupErr != nil {
					log.Warn("Asterisk CDR lookup failed for %s: %v", originalPhone, lookupErr)
				} else if asteriskCDR != nil {
					log.Info("Asterisk CDR: disposition=%s peer=%s duration=%ds billsec=%d cause=%s src=%s early_media=%t",
						asteriskCDR.Disposition, asteriskCDR.Peer, asteriskCDR.Duration, asteriskCDR.BillSec,
						asteriskCDR.HangupCauseString(), asteriskCDR.HangupSource, asteriskCDR.EarlyMedia)
				} else {
					log.Warn("Asterisk CDR not found for %s", originalPhone)
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
		msg := fmt.Sprintf("Test %d [%s]: DIAL FAILED - %s (%.1fs)%s", testNum, phoneNumber, result.Error, result.DialTime.Seconds(), retryInfo)
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
	if cfg.EMSI.InitialStrategy != "" {
		emsiCfg.InitialStrategy = cfg.EMSI.InitialStrategy
	}
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
		// Verify hangup actually worked - modem should not be in data mode
		if m.InDataMode() {
			log.Warn("Modem still in data mode after hangup, resetting...")
			_ = m.Reset()
		} else {
			// Log RS232 status after hangup
			if status, err := m.GetStatus(); err == nil {
				log.RS232(status.DCD, status.DSR, status.CTS, status.RI)
			}
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
