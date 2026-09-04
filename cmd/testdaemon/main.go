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
	versionpkg "github.com/nodelistdb/internal/version"
)

var (
	version = versionpkg.GetVersionInfo()
	commit  = versionpkg.GitCommit
	date    = versionpkg.BuildTime
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
		testProto  = flag.String("test-proto", "ifcico", "Protocol to test (binkp, ifcico, telnet, vmodem)")
		testLimit  = flag.String("test-limit", "", "Limit testing to specific node(s) during cycles (e.g., '2:5001/100')")
		vmpCall    = flag.Bool("vmp-call", false, "Place real VMP calls on IVM ports: rings the remote sysop's mailer, and needs this host reachable inbound on the callback port")
		vmpPort    = flag.Int("vmp-port", 0, "Callback port to advertise for -vmp-call (0 = config, or 14592, which is what a real VMODEM asks for first)")
		pingNode   = flag.String("ping-node", "", "Send one FTS-4010 netmail PING to this node (zone:net/node[@domain]) via fidomail now and exit; needs services.pingtrace configured")
		pingDirect = flag.Bool("ping-direct", false, "With -ping-node: send it with the Direct attribute (dialed straight from the nodelist) instead of routed")
		pingPoll   = flag.Bool("ping-poll", false, "Run one PING/TRACE pass (read replies, refresh dispatch state, expire, send due pings) and exit")
		logFile    = flag.String("log-file", "", "Write this run's log here instead of the configured file, and mirror it to the console. A hand-run cannot share the service's log: that file belongs to the service user, and losing the rotation race silently drops every line.")
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
	if *logFile != "" {
		cfg.Logging.File = *logFile
		cfg.Logging.Console = true
	}
	cfg.Daemon.RunOnce = *once
	cfg.Daemon.DryRun = *dryRun
	cfg.Daemon.CLIOnly = *cliOnly
	cfg.Daemon.TestLimit = *testLimit
	if *vmpCall {
		// A one-off run may call without the config saying so — placing calls
		// every cycle rings the same sysops repeatedly, which is a decision for
		// the config file, not a side effect of testing by hand.
		cfg.Protocols.VModem.DataChannel.Enabled = true
		if *vmpPort > 0 {
			cfg.Protocols.VModem.DataChannel.PreferredPort = *vmpPort
			cfg.Protocols.VModem.DataChannel.PortMin = 0
			cfg.Protocols.VModem.DataChannel.PortMax = 0
		}
	}

	// Initialize daemon with version info
	cfg.Version = fmt.Sprintf("v%s (%s) built %s", version, commit, date)
	d, err := daemon.New(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize daemon: %v", err)
	}

	// If test-node is specified, run single test and exit
	if *testNode != "" {
		ctx := context.Background()
		testErr := d.TestSingleNode(ctx, *testNode, *testProto)
		// Storage batches inserts and flushes on a size or time trigger, so a
		// single test's result only reaches ClickHouse if the daemon is closed
		// properly. Exiting straight from here discarded it silently.
		if err := d.Close(); err != nil {
			log.Printf("Warning: shutdown failed, the result may not have been stored: %v", err)
		}
		if testErr != nil {
			log.Fatalf("Test failed: %v", testErr)
		}
		log.Println("Test completed")
		os.Exit(0)
	}

	if *pingNode != "" || *pingPoll {
		ctx := context.Background()
		var runErr error
		if *pingNode != "" {
			p, err := d.PingNodeNow(ctx, *pingNode, *pingDirect)
			runErr = err
			if err == nil {
				log.Printf("PING to %s queued: msgid=%q fidomail_id=%d via %s (%s) status=%s %s",
					p.Address, p.MSGID, p.FidomailMessageID, p.FirstHop, p.RouteSource, p.Status, p.Error)
			}
		} else {
			runErr = d.PingTracePass(ctx)
		}
		if err := d.Close(); err != nil {
			log.Printf("Warning: shutdown failed: %v", err)
		}
		if runErr != nil {
			log.Fatalf("PING/TRACE failed: %v", runErr)
		}
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
