package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/nodelistdb/internal/testing/daemon"
)

var (
	version = "1.0.0"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	var (
		configFile = flag.String("config", "config.yaml", "Configuration file path")
		debugMode  = flag.Bool("debug", false, "Enable debug mode")
		once       = flag.Bool("once", false, "Run single test cycle and exit")
		dryRun     = flag.Bool("dry-run", false, "Test without storing results")
		cliOnly    = flag.Bool("cli-only", false, "Disable automatic testing, only test via CLI commands")
		showVer    = flag.Bool("version", false, "Show version and exit")
		testNode   = flag.String("test-node", "", "Test specific node (format: address or host:port) and exit")
		testProto  = flag.String("test-proto", "ifcico", "Protocol to test (binkp, ifcico, telnet)")
		testLimit  = flag.String("test-limit", "", "Limit testing to specific node(s) during cycles (e.g., '2:5001/100')")
	)

	flag.Parse()

	if *showVer {
		fmt.Printf("NodeTest Daemon v%s (%s) built %s\n", version, commit, date)
		os.Exit(0)
	}

	// Load configuration
	cfg, err := daemon.LoadConfig(*configFile)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Override with command line flags
	if *debugMode {
		cfg.Logging.Level = "debug"
	}
	cfg.Daemon.RunOnce = *once
	cfg.Daemon.DryRun = *dryRun
	cfg.Daemon.CLIOnly = *cliOnly
	cfg.Daemon.TestLimit = *testLimit

	// Initialize daemon with version info
	cfg.Version = fmt.Sprintf("v%s (%s) built %s", version, commit, date)
	d, err := daemon.New(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize daemon: %v", err)
	}

	// If test-node is specified, run single test and exit
	if *testNode != "" {
		ctx := context.Background()
		if err := d.TestSingleNode(ctx, *testNode, *testProto); err != nil {
			log.Fatalf("Test failed: %v", err)
		}
		log.Println("Test completed")
		os.Exit(0)
	}

	// Setup signal handling
	ctx, cancel := context.WithCancel(context.Background())
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		log.Printf("Received signal %v, shutting down...", sig)
		cancel()
	}()

	// Run daemon
	if err := d.Run(ctx); err != nil {
		log.Fatalf("Daemon error: %v", err)
	}

	log.Println("Daemon stopped")
}