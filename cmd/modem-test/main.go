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
	"github.com/nodelistdb/internal/testing/timeavail"
	"github.com/nodelistdb/internal/version"
)

// CLI flags
var (
	configPath  = flag.String("config", "", "Path to YAML config file")
	phone       = flag.String("phone", "", "Phone number(s) to dial (comma-separated or ranges like 901-910)")
	device      = flag.String("device", "", "Serial device (overrides config)")
	debug       = flag.Bool("debug", false, "Enable debug output (overrides config)")
	csvFile     = flag.String("csv", "", "Output results to CSV file")
	logFile     = flag.String("log", "", "Output log to file (in addition to stderr)")
	interactive = flag.Bool("interactive", false, "Interactive AT command mode")
	batch       = flag.Bool("batch", false, "") // Hidden flag for backward compatibility
	prefix      = flag.String("prefix", "", "Phone prefix to fetch PSTN nodes from API (e.g., \"+7\")")
	cmOnly      = flag.Bool("cm-only", false, "Only test CM (24/7) nodes (prefix mode only)")
	retryCount  = flag.Int("retry", 3, "Number of retries for BUSY/NO ANSWER (default: 3)")
	pauseStr    = flag.String("pause", "", "Pause between calls, retries, and CDR lookups (default: 60s)")
	nodeAddr    = flag.String("node", "", "Single node address to test (format: zone:net/node, e.g., 2:5001/100)")
	allNodes    = flag.Bool("all", false, "Test all PSTN nodes (use with -except to exclude prefixes)")
	exceptPfx   = flag.String("except", "", "Comma-separated prefixes to exclude from -all mode (e.g., \"+7,+1\")")
	forceRetest = flag.Bool("force", false, "Force re-testing of recently successful nodes")
	skipDays    = flag.Int("skip-days", 7, "Skip nodes tested successfully within N days (0 to disable)")
	showVersion = flag.Bool("version", false, "Show version information and exit")
	markDead    = flag.String("mark-dead", "", "Mark a node's PSTN number as dead (format: zone:net/node)")
	unmarkDead  = flag.String("unmark-dead", "", "Unmark a node's PSTN number as dead (format: zone:net/node)")
	deadReason  = flag.String("reason", "", "Reason for marking a node as PSTN dead")
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Modem test tool for FidoNet node connectivity testing.\n\n")
		fmt.Fprintf(os.Stderr, "Test Modes (mutually exclusive):\n")
		fmt.Fprintf(os.Stderr, "  -node      Test a specific FidoNet node by address\n")
		fmt.Fprintf(os.Stderr, "  -phone     Test arbitrary phone number(s)\n")
		fmt.Fprintf(os.Stderr, "  -prefix    Test all nodes matching a phone prefix\n")
		fmt.Fprintf(os.Stderr, "  -all       Test ALL PSTN nodes (with optional exceptions)\n")
		fmt.Fprintf(os.Stderr, "\nOptions:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  # Test a specific node by address\n")
		fmt.Fprintf(os.Stderr, "  %s -node 2:5001/100\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  # Test a node with phone override\n")
		fmt.Fprintf(os.Stderr, "  %s -node 2:5001/100 -phone 79001234567\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  # Test multiple phones\n")
		fmt.Fprintf(os.Stderr, "  %s -phone 917,918,919\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  # Test phone range (901 through 910)\n")
		fmt.Fprintf(os.Stderr, "  %s -phone 901-910\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  # Test all Russian nodes (prefix +7), CM only\n")
		fmt.Fprintf(os.Stderr, "  %s -prefix +7 -cm-only\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  # Test all nodes EXCEPT Russian and US\n")
		fmt.Fprintf(os.Stderr, "  %s -all -except +7,+1\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  # Interactive AT command mode\n")
		fmt.Fprintf(os.Stderr, "  %s -interactive\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  # Custom pause between calls (default: 60s)\n")
		fmt.Fprintf(os.Stderr, "  %s -prefix +7 -pause 30s\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  # Force re-test all nodes (ignore recent success)\n")
		fmt.Fprintf(os.Stderr, "  %s -prefix +7 -force\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  # Custom skip window (skip nodes tested in last 14 days)\n")
		fmt.Fprintf(os.Stderr, "  %s -prefix +7 -skip-days 14\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  # Mark a node's phone as dead (requires nodelistdb config)\n")
		fmt.Fprintf(os.Stderr, "  %s -mark-dead 2:5001/100 -reason \"number disconnected\"\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  # Unmark a previously dead node\n")
		fmt.Fprintf(os.Stderr, "  %s -unmark-dead 2:5001/100\n", os.Args[0])
	}

	flag.Parse()

	// Handle version flag
	if *showVersion {
		fmt.Printf("modem-test %s\n", version.GetFullVersionInfo())
		os.Exit(0)
	}

	// Handle mark-dead / unmark-dead (no modem needed, exits immediately)
	if *markDead != "" || *unmarkDead != "" {
		handlePSTNDeadCommand(*markDead, *unmarkDead, *deadReason, *configPath)
		return
	}

	// Validate mutually exclusive mode flags
	modeCount := 0
	if *interactive {
		modeCount++
	}
	if *nodeAddr != "" {
		modeCount++
	}
	if *phone != "" && *nodeAddr == "" { // phone alone = phone mode; phone with node = override
		modeCount++
	}
	if *prefix != "" {
		modeCount++
	}
	if *allNodes {
		modeCount++
	}
	if modeCount > 1 {
		fmt.Fprintf(os.Stderr, "ERROR: only one of -interactive, -node, -phone, -prefix, -all can be specified\n")
		flag.Usage()
		os.Exit(1)
	}

	// Validate -node with -phone (single phone only for override)
	if *nodeAddr != "" && *phone != "" && strings.Contains(*phone, ",") {
		fmt.Fprintf(os.Stderr, "ERROR: -phone override with -node must be a single number, not a list\n")
		os.Exit(1)
	}

	// Validate -except only with -all
	if *exceptPfx != "" && !*allNodes {
		fmt.Fprintf(os.Stderr, "ERROR: -except can only be used with -all\n")
		os.Exit(1)
	}

	// Load or create config
	var cfg *Config
	var err error
	configFile := "(defaults)"

	if *configPath != "" {
		// Explicit config path specified
		cfg, err = LoadConfig(*configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: Failed to load config: %v\n", err)
			os.Exit(1)
		}
		configFile = *configPath
	} else if discovered := DiscoverConfigFile(); discovered != "" {
		// Auto-discovered config file
		cfg, err = LoadConfig(discovered)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: Failed to load config %s: %v\n", discovered, err)
			os.Exit(1)
		}
		configFile = discovered
	} else {
		cfg = DefaultConfig()
	}

	// Apply CLI overrides (config-file fields only)
	cfg.ApplyCLIOverrides(*device, *phone, *debug, *csvFile)

	// Set CLI-only test parameters
	cfg.Test.RetryCount = *retryCount
	if *pauseStr != "" {
		pauseDur, err := time.ParseDuration(*pauseStr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: invalid -pause value %q: %v\n", *pauseStr, err)
			os.Exit(1)
		}
		cfg.Test.Pause = Duration(pauseDur)
	}

	// Acquire PID file to prevent multiple instances
	pidPath := cfg.PidFile
	if pidPath == "" {
		pidPath = defaultPidFile
	}
	pidCleanup, err := AcquirePidFile(pidPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
	defer pidCleanup()

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
		fmt.Fprintf(log.GetOutput(), "Logging to: %s\n", *logFile)
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

	// Initialize SQLite results writer (optional)
	sqliteWriter, err := NewSQLiteResultsWriter(cfg.SQLiteResults)
	if err != nil {
		log.Warn("SQLite results writer unavailable: %v", err)
		sqliteWriter = &SQLiteResultsWriter{} // Use disabled writer
	} else if sqliteWriter.IsEnabled() {
		log.Info("SQLite results writer enabled: %s table=%s", cfg.SQLiteResults.Path, cfg.SQLiteResults.TableName)
		defer sqliteWriter.Close()
	}

	// Initialize NodelistDB results writer (optional)
	nodelistDBWriter, err := NewNodelistDBWriter(cfg.NodelistDB)
	if err != nil {
		log.Warn("NodelistDB results writer unavailable: %v", err)
		nodelistDBWriter = &NodelistDBWriter{} // Use disabled writer
	} else if nodelistDBWriter.IsEnabled() {
		log.Info("NodelistDB results writer enabled: %s", cfg.NodelistDB.URL)
		defer nodelistDBWriter.Close()
	}

	// Variables for node testing modes
	var nodeLookup map[string]*NodeTarget
	var filteredNodes []NodeTarget

	// Determine effective prefix (CLI or config)
	pfx := *prefix
	if pfx == "" {
		pfx = cfg.Test.Prefix
	}

	// Validate -cm-only usage
	if *cmOnly && pfx == "" && !*allNodes {
		fmt.Fprintf(log.GetOutput(), "WARNING: -cm-only has no effect without -prefix or -all\n")
	}

	// Handle different test modes
	if *nodeAddr != "" {
		// Mode 1: Single node test
		if cfg.NodelistDB.URL == "" {
			fmt.Fprintf(log.GetOutput(), "ERROR: nodelistdb.url is required for -node mode\n")
			os.Exit(1)
		}
		zone, net, node, err := ParseNodeAddress(*nodeAddr)
		if err != nil {
			fmt.Fprintf(log.GetOutput(), "ERROR: Invalid node address %q: %v\n", *nodeAddr, err)
			os.Exit(1)
		}
		log.Info("Fetching node %d:%d/%d from %s...", zone, net, node, cfg.NodelistDB.URL)
		target, err := FetchNodeByAddress(cfg.NodelistDB.URL, zone, net, node, 30*time.Second)
		if err != nil {
			fmt.Fprintf(log.GetOutput(), "ERROR: Failed to fetch node: %v\n", err)
			os.Exit(1)
		}
		// Phone override takes precedence
		if *phone != "" {
			target.Phone = *phone
		}
		if target.Phone == "" || target.Phone == "-Unpublished-" {
			fmt.Fprintf(log.GetOutput(), "ERROR: Node %s has no phone number. Use -phone to specify one.\n", *nodeAddr)
			os.Exit(1)
		}
		// Strip leading + from phone for modem dialing
		target.Phone = strings.TrimPrefix(target.Phone, "+")
		filteredNodes = []NodeTarget{*target}
		nodeLookup = BuildNodeLookupByPhone(filteredNodes)
		cfg.Test.Phones = []string{target.Phone}
		*batch = true

	} else if pfx != "" {
		// Mode 3: Prefix test
		if cfg.NodelistDB.URL == "" {
			fmt.Fprintf(log.GetOutput(), "ERROR: nodelistdb.url is required when using -prefix\n")
			os.Exit(1)
		}
		log.Info("Fetching PSTN nodes from %s...", cfg.NodelistDB.URL)
		allNodesList, totalCount, err := FetchPSTNNodesWithCount(cfg.NodelistDB.URL, 30*time.Second, log)
		if err != nil {
			fmt.Fprintf(log.GetOutput(), "ERROR: Failed to fetch PSTN nodes: %v\n", err)
			os.Exit(1)
		}
		log.Info("Fetched %d PSTN nodes total", len(allNodesList))
		if len(allNodesList) < totalCount {
			log.Warn("Result truncated: fetched %d of %d nodes", len(allNodesList), totalCount)
		}
		filtered := FilterByPrefix(allNodesList, pfx)
		log.Info("Filtered to %d nodes matching prefix %q", len(filtered), pfx)
		if *cmOnly {
			filtered = filterCMOnly(filtered, log)
		}
		if len(filtered) == 0 {
			fmt.Fprintf(log.GetOutput(), "ERROR: No PSTN nodes found matching prefix %q\n", pfx)
			os.Exit(1)
		}
		stripPhonePlus(filtered)
		cfg.Test.Phones = UniquePhones(filtered)
		nodeLookup = BuildNodeLookupByPhone(filtered)
		filteredNodes = filtered
		*batch = true

	} else if *allNodes {
		// Mode 4: All nodes with exceptions
		if cfg.NodelistDB.URL == "" {
			fmt.Fprintf(log.GetOutput(), "ERROR: nodelistdb.url is required for -all mode\n")
			os.Exit(1)
		}
		log.Info("Fetching all PSTN nodes from %s...", cfg.NodelistDB.URL)
		allNodesList, totalCount, err := FetchPSTNNodesWithCount(cfg.NodelistDB.URL, 30*time.Second, log)
		if err != nil {
			fmt.Fprintf(log.GetOutput(), "ERROR: Failed to fetch PSTN nodes: %v\n", err)
			os.Exit(1)
		}
		log.Info("Fetched %d PSTN nodes total", len(allNodesList))
		if len(allNodesList) < totalCount {
			log.Warn("Result truncated: fetched %d of %d nodes", len(allNodesList), totalCount)
		}
		filtered := allNodesList
		if *exceptPfx != "" {
			exceptPrefixes := strings.Split(*exceptPfx, ",")
			filtered = FilterExceptPrefixes(filtered, exceptPrefixes)
			log.Info("After excluding prefixes %v: %d nodes", exceptPrefixes, len(filtered))
		}
		if *cmOnly {
			filtered = filterCMOnly(filtered, log)
		}
		if len(filtered) == 0 {
			fmt.Fprintf(log.GetOutput(), "ERROR: No PSTN nodes found after applying filters\n")
			os.Exit(1)
		}
		stripPhonePlus(filtered)
		cfg.Test.Phones = UniquePhones(filtered)
		nodeLookup = BuildNodeLookupByPhone(filtered)
		filteredNodes = filtered
		*batch = true
	}
	// Mode 2: Phone list mode - phones already set from config/CLI

	// Clamp -skip-days to valid range
	if *skipDays > 90 {
		*skipDays = 90
	}

	// Skip recently-tested nodes (unless -force or -skip-days 0)
	beforeCount := len(cfg.GetPhones())
	if !*forceRetest && *skipDays > 0 && beforeCount > 0 && cfg.NodelistDB.URL != "" {
		recentPhones, err := FetchRecentSuccessPhones(cfg.NodelistDB.URL, *skipDays, 30*time.Second)
		if err != nil {
			log.Warn("Could not fetch recent test results, testing all nodes: %v", err)
		} else if len(recentPhones) > 0 {
			if len(filteredNodes) > 0 {
				// Modes 1/3/4: filter node list, then rebuild phones and lookup
				var kept []NodeTarget
				for _, n := range filteredNodes {
					if !recentPhones[strings.TrimPrefix(n.Phone, "+")] {
						kept = append(kept, n)
					}
				}
				filteredNodes = kept
				cfg.Test.Phones = UniquePhones(filteredNodes)
				nodeLookup = BuildNodeLookupByPhone(filteredNodes)
			} else {
				// Mode 2: filter phone list directly
				var kept []string
				for _, p := range cfg.GetPhones() {
					if !recentPhones[strings.TrimPrefix(p, "+")] {
						kept = append(kept, p)
					}
				}
				cfg.Test.Phones = kept
			}
			skipped := beforeCount - len(cfg.GetPhones())
			if skipped > 0 {
				log.Info("Skipped %d phone(s) tested successfully in last %d day(s) (use -force to override)", skipped, *skipDays)
			}
			if len(cfg.GetPhones()) == 0 {
				log.Info("All nodes were recently tested, nothing to do")
				return
			}
		}
	}

	// Update phones after mode handling
	phones := cfg.GetPhones()

	// Initialize operator cache for failover mode (multi-operator scenarios)
	var operatorCache *OperatorCache
	if cfg.HasMultipleOperators() && cfg.Test.OperatorCache.Enabled {
		var err error
		operatorCache, err = NewOperatorCache(cfg.Test.OperatorCache, log)
		if err != nil {
			log.Warn("Operator cache unavailable: %v", err)
			// Continue without cache - failover will still work, just without caching
		} else if operatorCache != nil {
			defer operatorCache.Close()
		}
	}

	// Check for multi-modem mode
	if cfg.IsMultiModem() && (*batch || len(phones) > 0) {
		log.Info("Multi-modem mode detected with %d modem(s)", len(cfg.GetModemConfigs()))
		runBatchModeMulti(cfg, log, configFile, cdrService, asteriskCDRService, operatorCache, pgWriter, mysqlWriter, sqliteWriter, nodelistDBWriter, nodeLookup, filteredNodes)
		return
	}

	// Single modem mode - create modem configuration
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
		fmt.Fprintf(log.GetOutput(), "ERROR: Failed to create modem: %v\n", err)
		os.Exit(1)
	}

	// Setup signal handler for cleanup
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Fprintln(log.GetOutput(), "\nReceived interrupt, cleaning up...")
		if m.InDataMode() {
			fmt.Fprintln(log.GetOutput(), "Hanging up...")
			if err := m.Hangup(); err != nil {
				fmt.Fprintf(log.GetOutput(), "Hangup error: %v\n", err)
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
		runBatchMode(m, cfg, log, configFile, cdrService, asteriskCDRService, pgWriter, mysqlWriter, sqliteWriter, nodelistDBWriter, nodeLookup, filteredNodes)
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

// filterCMOnly filters nodes to only include CM (24/7) nodes
func filterCMOnly(nodes []NodeTarget, log *TestLogger) []NodeTarget {
	var cmNodes []NodeTarget
	for _, n := range nodes {
		if n.IsCM {
			cmNodes = append(cmNodes, n)
		}
	}
	log.Info("CM-only filter: %d of %d nodes are CM (24/7)", len(cmNodes), len(nodes))
	return cmNodes
}

// stripPhonePlus removes leading + from all phone numbers for modem dialing
func stripPhonePlus(nodes []NodeTarget) {
	for i := range nodes {
		nodes[i].Phone = strings.TrimPrefix(nodes[i].Phone, "+")
	}
}

func runInfoMode(m *modem.Modem, log *TestLogger) {
	out := log.GetOutput()
	fmt.Fprintln(out, "\n--- Modem Info ---")
	info, err := m.GetInfo()
	if err != nil {
		log.Fail("Failed to get modem info: %v", err)
	} else {
		if info.Manufacturer != "" {
			fmt.Fprintf(out, "Manufacturer: %s\n", info.Manufacturer)
		}
		if info.Model != "" {
			fmt.Fprintf(out, "Model: %s\n", info.Model)
		}
		if info.Firmware != "" {
			fmt.Fprintf(out, "Firmware: %s\n", info.Firmware)
		}
		fmt.Fprintf(out, "Raw response:\n%s\n", info.RawResponse)
	}

	// Get modem status
	status, err := m.GetStatus()
	if err != nil {
		log.Fail("Failed to get modem status: %v", err)
	} else {
		fmt.Fprintln(out, "\n--- Modem Status ---")
		fmt.Fprintf(out, "DCD (Carrier): %v\n", status.DCD)
		fmt.Fprintf(out, "DSR (Ready):   %v\n", status.DSR)
		fmt.Fprintf(out, "CTS (Clear):   %v\n", status.CTS)
		fmt.Fprintf(out, "RI (Ring):     %v\n", status.RI)
	}

	fmt.Fprintln(out, "\nUse -interactive for AT command mode or -batch -phone <number> for batch testing.")
}

func runInteractiveMode(m *modem.Modem, log *TestLogger) {
	out := log.GetOutput()
	fmt.Fprintln(out, "\n=== Interactive AT Command Mode ===")
	fmt.Fprintln(out, "Enter AT commands (type 'quit' to exit)")

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
			fmt.Fprintf(out, "%s\n", response)
		}
	}
}

func runBatchMode(m *modem.Modem, cfg *Config, log *TestLogger, configFile string, cdrService *CDRService, asteriskCDRService *AsteriskCDRService, pgWriter *PostgresResultsWriter, mysqlWriter *MySQLResultsWriter, sqliteWriter *SQLiteResultsWriter, nodelistDBWriter *NodelistDBWriter, nodeLookup map[string]*NodeTarget, filteredNodes []NodeTarget) {
	phones := cfg.GetPhones()
	testCount := len(phones)
	pause := cfg.GetPause()

	// Setup context for cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	cancelled := false
	go func() {
		<-sigChan
		fmt.Fprintln(log.GetOutput(), "\nReceived interrupt, stopping...")
		cancelled = true
		cancel()
	}()

	// Print session header
	log.PrintHeader(configFile, cfg.Modem.Device, phones, testCount)

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

	// Deferred node tracking for re-scheduling when call windows close
	var deferredNodes []NodeTarget

	// Use ScheduleNodes for time-aware job ordering (handles CM, availability windows)
	var schedChan <-chan phoneJob
	if len(filteredNodes) > 0 {
		schedChan = ScheduleNodes(ctx, filteredNodes, cfg.GetOperatorsForPhone, log)
	}

	for i := 1; ; i++ {
		// Check for cancellation
		if cancelled {
			log.Info("Test loop cancelled")
			break
		}

		var currentPhone string
		var currentNodeAddress, currentNodeSystemName, currentNodeLocation, currentNodeSysop string
		var currentAvail *timeavail.NodeAvailability

		if schedChan != nil {
			// Schedule mode: consume pre-scheduled, time-aware jobs
			job, ok := <-schedChan
			if !ok {
				// All scheduled nodes processed
				break
			}
			currentPhone = job.phone
			currentNodeAddress = job.nodeAddress
			currentNodeSystemName = job.nodeSystemName
			currentNodeLocation = job.nodeLocation
			currentNodeSysop = job.nodeSysop
			currentAvail = job.nodeAvailability

			log.PrintTestHeader(i, testCount)

			// Log node info
			if job.nodeAddress != "" {
				log.Info("Node: %s %s (sysop: %s)", job.nodeAddress, job.nodeSystemName, job.nodeSysop)
			}
		} else {
			// Simple phone list mode
			if i > len(phones) {
				break
			}
			currentPhone = phones[i-1]
			log.PrintTestHeader(i, testCount)

			// Look up node info from nodeLookup if available
			if nodeLookup != nil {
				if target, ok := nodeLookup[currentPhone]; ok {
					currentNodeAddress = target.Address()
					currentNodeSystemName = strings.ReplaceAll(target.SystemName, "_", " ")
					currentNodeLocation = strings.ReplaceAll(target.Location, "_", " ")
					currentNodeSysop = strings.ReplaceAll(target.SysopName, "_", " ")
					log.Info("Node: %s %s (sysop: %s)", currentNodeAddress, currentNodeSystemName, currentNodeSysop)
				}
			}
		}

		// Dial phone directly (no operator prefix)
		result := runSingleTest(ctx, m, cfg, log, cdrService, asteriskCDRService, i, currentPhone, currentPhone, currentAvail)

		// Handle deferred nodes (call window closed)
		if result.windowClosed {
			if target, ok := nodeLookup[currentPhone]; ok {
				deferredNodes = append(deferredNodes, *target)
			}
			continue // Don't count, don't write records, don't pause
		}

		results = append(results, result.message)

		// Lookup CDR data for VoIP quality metrics (AudioCodes)
		if cdrService != nil && cdrService.IsEnabled() {
			cdrCtx, cdrCancel := context.WithTimeout(context.Background(), 5*time.Second)
			cdrData, err := cdrService.LookupByPhone(cdrCtx, currentPhone, time.Now())
			cdrCancel()
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
			astCtx, astCancel := context.WithTimeout(context.Background(), 5*time.Second)
			asteriskCDR, err := asteriskCDRService.LookupByPhone(astCtx, currentPhone, time.Now())
			astCancel()
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
		if csvWriter != nil || (pgWriter != nil && pgWriter.IsEnabled()) || (mysqlWriter != nil && mysqlWriter.IsEnabled()) || (sqliteWriter != nil && sqliteWriter.IsEnabled()) || (nodelistDBWriter != nil && nodelistDBWriter.IsEnabled()) {
			rec = RecordFromTestResult(
				i,
				currentPhone,
				"", // No operator name
				"", // No operator prefix
				currentNodeAddress,
				currentNodeSystemName,
				currentNodeLocation,
				currentNodeSysop,
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

		// Write to SQLite if enabled
		if sqliteWriter != nil && sqliteWriter.IsEnabled() && rec != nil {
			if err := sqliteWriter.WriteRecord(rec); err != nil {
				log.Error("Failed to write SQLite record: %v", err)
			}
		}

		// Write to NodelistDB if enabled
		if nodelistDBWriter != nil && nodelistDBWriter.IsEnabled() && rec != nil {
			if err := nodelistDBWriter.WriteRecord(rec); err != nil {
				log.Error("Failed to write NodelistDB record: %v", err)
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

		// Wait pause between tests (except after last one)
		if i < testCount {
			log.Info("Waiting %v before next test...", pause)
			select {
			case <-time.After(pause):
			case <-ctx.Done():
				log.Info("Pause cancelled")
			}
		}
	}

	// Re-schedule deferred nodes (call windows that closed during processing)
	for !cancelled && len(deferredNodes) > 0 {
		pending := deferredNodes
		deferredNodes = nil

		log.Info("%d node(s) deferred due to closed call windows, re-scheduling...", len(pending))
		schedChan = ScheduleNodes(ctx, pending, cfg.GetOperatorsForPhone, log)

		for i := 1; ; i++ {
			if cancelled {
				break
			}

			job, ok := <-schedChan
			if !ok {
				break
			}

			currentPhone := job.phone
			log.PrintTestHeader(i, len(pending))
			if job.nodeAddress != "" {
				log.Info("Node: %s %s (sysop: %s)", job.nodeAddress, job.nodeSystemName, job.nodeSysop)
			}

			result := runSingleTest(ctx, m, cfg, log, cdrService, asteriskCDRService, i, currentPhone, currentPhone, job.nodeAvailability)

			if result.windowClosed {
				if target, ok := nodeLookup[currentPhone]; ok {
					deferredNodes = append(deferredNodes, *target)
				}
				continue
			}

			results = append(results, result.message)

			if result.success {
				success++
				if stats, ok := phoneStats[currentPhone]; ok {
					stats.Success++
					stats.TotalDialTime += result.dialTime
					stats.TotalEmsiTime += result.emsiTime
				}
				totalDialTime += result.dialTime
				totalEmsiTime += result.emsiTime
			} else {
				failed++
				if stats, ok := phoneStats[currentPhone]; ok {
					stats.Failed++
				}
			}

			if pause > 0 {
				select {
				case <-time.After(pause):
				case <-ctx.Done():
					cancelled = true
				}
			}
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
	cdrData       *CDRData         // VoIP quality metrics from AudioCodes CDR
	asteriskCDR   *AsteriskCDRData // Call routing info from Asterisk CDR
	windowClosed  bool             // true = stopped because call window closed, should retry later
}

func runSingleTest(ctx context.Context, m *modem.Modem, cfg *Config, log *TestLogger, cdrService *CDRService, asteriskCDRService *AsteriskCDRService, testNum int, phoneNumber string, originalPhone string, nodeAvailability *timeavail.NodeAvailability) testResult {
	// Determine retry settings
	retryCount := cfg.GetRetryCount()
	pause := cfg.GetPause()
	retryDelay := pause
	cdrLookupDelay := cfg.GetCDRDelay()

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

		// Pre-dial availability check: stop if call window closed during retries
		if nodeAvailability != nil && !nodeAvailability.IsCallableNow(time.Now().UTC()) {
			log.Warn("Call window closed during retries for %s, deferring", phoneNumber)
			return testResult{
				windowClosed: true,
				message:      fmt.Sprintf("Test %d [%s]: DEFERRED - call window closed", testNum, phoneNumber),
			}
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

		// Check 2: CDR-based retry for failed dials only (not connected calls).
		// For successful connections, CDR is looked up after the call ends (post-EMSI).
		if !shouldRetry && !result.Success && asteriskCDRService != nil && asteriskCDRService.IsEnabled() {
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
	session.SetDebug(cfg.Logging.Debug)
	if cfg.Logging.Debug {
		session.SetDebugFunc(func(format string, args ...interface{}) {
			log.EMSI(format, args...)
		})
	}

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
