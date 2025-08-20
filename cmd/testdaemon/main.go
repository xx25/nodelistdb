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

	// Initialize daemon
	d, err := daemon.New(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize daemon: %v", err)
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