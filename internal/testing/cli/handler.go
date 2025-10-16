package cli

import (
	"bufio"
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type Handler struct {
	daemon         DaemonInterface
	writer         *bufio.Writer
	formatter      *Formatter
	debugToTelnet  bool           // Whether to show debug output in telnet session
	debugCapture   *TelnetCapture // Debug output capturer
}

func NewHandler(daemon DaemonInterface, writer *bufio.Writer) *Handler {
	h := &Handler{
		daemon:        daemon,
		writer:        writer,
		formatter:     NewFormatter(writer),
		debugToTelnet: false,
	}
	// Setup debug capture
	h.debugCapture = SetupTelnetCapture(h)
	return h
}

func (h *Handler) HandleCommand(ctx context.Context, input string) error {
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return nil
	}
	
	command := strings.ToLower(parts[0])
	args := parts[1:]
	
	switch command {
	case "help":
		return h.handleHelp(args)
	case "test":
		return h.handleTestSimple(ctx, args)
	case "show":
		return h.handleShowSimple(ctx, args)
	case "status":
		return h.handleStatus()
	case "workers":
		return h.handleWorkers()
	case "pause":
		return h.handlePause()
	case "resume":
		return h.handleResume()
	case "reload":
		return h.handleReload()
	case "debug":
		return h.handleDebug(args)
	case "clear":
		return h.handleClear()
	default:
		return fmt.Errorf("unknown command: %s (type 'help' for available commands)", command)
	}
}

func (h *Handler) handleHelp(args []string) error {
	if len(args) > 0 {
		return h.showCommandHelp(args[0])
	}
	
	help := `
Available Commands:
===================

Testing Commands:
  test <address> [hostname/ip] [protocol] - Test a FidoNet node
       address:  FidoNet address (e.g., 2:5001/100)
       hostname: Optional hostname or IP address
       protocol: Optional protocol (binkp, ifcico, telnet, ftp, vmodem)

Information Commands:
  show <address>  - Show node information from database

Status Commands:
  status    - Show daemon status
  workers   - Show worker pool status

Control Commands:
  pause     - Pause all testing
  resume    - Resume testing
  reload    - Reload configuration
  debug     - Toggle debug mode (on/off/status)

Utility Commands:
  help [command] - Show help
  clear         - Clear screen
  exit/quit     - Disconnect

Examples:
  test 2:5001/100                     - Test using database info
  test 2:5001/100 f100.5001.ru        - Test with specific hostname
  test 2:5001/100 192.168.1.10 binkp  - Test BinkP on specific IP
  show 2:5001/100                     - Show node info

Note: Tests automatically try both IPv4 and IPv6 when available.
`
	h.writer.WriteString(help)
	return h.writer.Flush()
}

func (h *Handler) showCommandHelp(command string) error {
	var help string
	
	switch command {
	case "test":
		help = `
test - Run tests on FidoNet nodes

Usage:
  test <address> [hostname/ip] [protocol]

Parameters:
  address     - FidoNet address (e.g., 2:5001/100)
  hostname/ip - Optional hostname or IP address to test
  protocol    - Optional specific protocol to test:
                binkp, ifcico, telnet, ftp, vmodem
                (if not specified, all enabled protocols are tested)

Examples:
  test 2:5001/100                      - Test using database info
  test 2:5001/100 f100.5001.ru         - Test with specific hostname
  test 2:5001/100 192.168.1.10 binkp   - Test only BinkP on IP

The command will test the node using both IPv4 and IPv6 (when available)
and display detailed results including connection status, response times,
and system information.
`
	case "status":
		help = `
status - Show daemon status

Displays current daemon statistics including:
- Uptime
- Total tests completed
- Success/failure rates
- Active workers
- Queue status
`
	case "show":
		help = `
show - Show information about FidoNet nodes

Usage:
  show <address>

Example:
  show 2:5001/100

Displays detailed information about the node from the database,
including system name, sysop, location, and connectivity details.
`
	default:
		help = fmt.Sprintf("No detailed help available for '%s'\n", command)
	}
	
	h.writer.WriteString(help)
	return h.writer.Flush()
}

// handleTestSimple handles the simplified test command syntax
// test <address> [hostname/ip] [protocol]
func (h *Handler) handleTestSimple(ctx context.Context, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: test <address> [hostname/ip] [protocol]")
	}
	
	// Parse FidoNet address
	address := args[0]
	zone, net, node, err := parseAddress(address)
	if err != nil {
		return fmt.Errorf("invalid address: %v", err)
	}
	
	// Parse optional arguments
	hostname := ""
	protocol := ""
	
	if len(args) > 1 {
		hostname = args[1]
	}
	
	if len(args) > 2 {
		protocol = strings.ToLower(args[2])
	}
	
	h.formatter.WriteHeader(fmt.Sprintf("Testing node %s", address))
	
	if hostname != "" {
		h.formatter.WriteInfo(fmt.Sprintf("Target: %s", hostname))
	}
	if protocol != "" {
		h.formatter.WriteInfo(fmt.Sprintf("Protocol: %s", protocol))
	}
	h.formatter.WriteInfo("Testing both IPv4 and IPv6 when available")
	
	h.formatter.WriteInfo("Starting tests...")
	h.writer.Flush()
	
	// Determine which protocols to test
	protocols := []string{}
	if protocol != "" && protocol != "all" {
		protocols = []string{protocol}
	} else {
		// Use all enabled protocols
		protocols = []string{"binkp", "ifcico", "telnet", "ftp", "vmodem"}
	}
	
	options := TestOptions{
		Protocols: protocols,
		Timeout:   10 * time.Second,
		Verbose:   true,
	}
	
	testCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	
	result, err := h.daemon.TestNode(testCtx, zone, net, node, hostname, options)
	if err != nil {
		h.formatter.WriteError(fmt.Sprintf("Test failed: %v", err))
		return h.writer.Flush()
	}
	
	h.formatter.FormatTestResult(result)
	return h.writer.Flush()
}

// handleShowSimple handles the simplified show command syntax
// show <address>
func (h *Handler) handleShowSimple(ctx context.Context, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: show <address>")
	}
	
	// Parse FidoNet address
	address := args[0]
	zone, net, node, err := parseAddress(address)
	if err != nil {
		return fmt.Errorf("invalid address: %v", err)
	}
	
	// Get node information from daemon
	nodeInfo, err := h.daemon.GetNodeInfo(ctx, zone, net, node)
	if err != nil {
		h.formatter.WriteError(fmt.Sprintf("Failed to get node info: %v", err))
		return h.writer.Flush()
	}
	
	// Format and display node information
	h.formatter.FormatNodeInfo(nodeInfo)
	return h.writer.Flush()
}

// parseAddress parses a FidoNet address string (e.g., "2:5001/100")
func parseAddress(address string) (uint16, uint16, uint16, error) {
	parts := strings.Split(address, ":")
	if len(parts) != 2 {
		return 0, 0, 0, fmt.Errorf("expected format zone:net/node")
	}
	
	zonePart := parts[0]
	netNodePart := parts[1]
	
	netNodeParts := strings.Split(netNodePart, "/")
	if len(netNodeParts) != 2 {
		return 0, 0, 0, fmt.Errorf("expected format zone:net/node")
	}
	
	zone, err := strconv.ParseUint(zonePart, 10, 16)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid zone")
	}
	
	net, err := strconv.ParseUint(netNodeParts[0], 10, 16)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid net")
	}
	
	node, err := strconv.ParseUint(netNodeParts[1], 10, 16)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid node")
	}
	
	return uint16(zone), uint16(net), uint16(node), nil
}

func (h *Handler) handleTest(ctx context.Context, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: test node <zone> <net> <node> [hostname] OR test address <address> [hostname]")
	}
	
	subcommand := args[0]
	
	switch subcommand {
	case "node":
		return h.handleTestNode(ctx, args[1:])
	case "address":
		return h.handleTestAddress(ctx, args[1:])
	default:
		return fmt.Errorf("unknown test subcommand: %s", subcommand)
	}
}

func (h *Handler) handleTestNode(ctx context.Context, args []string) error {
	if len(args) < 3 {
		return fmt.Errorf("usage: test node <zone> <net> <node> [hostname[:port]]")
	}
	
	zone, err := strconv.ParseUint(args[0], 10, 16)
	if err != nil {
		return fmt.Errorf("invalid zone: %s", args[0])
	}
	
	net, err := strconv.ParseUint(args[1], 10, 16)
	if err != nil {
		return fmt.Errorf("invalid net: %s", args[1])
	}
	
	node, err := strconv.ParseUint(args[2], 10, 16)
	if err != nil {
		return fmt.Errorf("invalid node: %s", args[2])
	}
	
	hostname := ""
	port := 0  // 0 means use default port
	if len(args) > 3 {
		hostname = args[3]
		// Parse hostname:port if port is specified
		if colonIdx := strings.LastIndex(hostname, ":"); colonIdx != -1 {
			portStr := hostname[colonIdx+1:]
			hostname = hostname[:colonIdx]
			if p, err := strconv.Atoi(portStr); err == nil && p > 0 && p < 65536 {
				port = p
			}
		}
	}
	
	address := fmt.Sprintf("%d:%d/%d", zone, net, node)
	h.formatter.WriteHeader(fmt.Sprintf("Testing node %s", address))
	
	if hostname != "" {
		if port > 0 {
			h.formatter.WriteInfo(fmt.Sprintf("Target: %s:%d", hostname, port))
		} else {
			h.formatter.WriteInfo(fmt.Sprintf("Target hostname: %s", hostname))
		}
	}
	
	h.formatter.WriteInfo("Starting tests...")
	h.writer.Flush()
	
	options := TestOptions{
		Protocols: []string{"binkp", "ifcico", "telnet"},
		Timeout:   10 * time.Second,
		Verbose:   true,
		Port:      port,  // Pass custom port if specified
	}
	
	testCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	
	result, err := h.daemon.TestNode(testCtx, uint16(zone), uint16(net), uint16(node), hostname, options)
	if err != nil {
		h.formatter.WriteError(fmt.Sprintf("Test failed: %v", err))
		return h.writer.Flush()
	}
	
	h.formatter.FormatTestResult(result)
	return h.writer.Flush()
}

func (h *Handler) handleTestAddress(ctx context.Context, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: test address <address> [hostname[:port]]")
	}
	
	address := args[0]
	parts := strings.Split(address, ":")
	if len(parts) != 2 {
		return fmt.Errorf("invalid address format, expected zone:net/node")
	}
	
	zonePart := parts[0]
	netNodePart := parts[1]
	
	netNodeParts := strings.Split(netNodePart, "/")
	if len(netNodeParts) != 2 {
		return fmt.Errorf("invalid address format, expected zone:net/node")
	}
	
	zone, err := strconv.ParseUint(zonePart, 10, 16)
	if err != nil {
		return fmt.Errorf("invalid zone in address")
	}
	
	net, err := strconv.ParseUint(netNodeParts[0], 10, 16)
	if err != nil {
		return fmt.Errorf("invalid net in address")
	}
	
	node, err := strconv.ParseUint(netNodeParts[1], 10, 16)
	if err != nil {
		return fmt.Errorf("invalid node in address")
	}
	
	hostname := ""
	if len(args) > 1 {
		hostname = args[1]
	}
	
	testArgs := []string{
		fmt.Sprintf("%d", zone),
		fmt.Sprintf("%d", net),
		fmt.Sprintf("%d", node),
	}
	if hostname != "" {
		testArgs = append(testArgs, hostname)
	}
	
	return h.handleTestNode(ctx, testArgs)
}

func (h *Handler) handleStatus() error {
	status := h.daemon.GetStatus()
	
	h.formatter.WriteHeader("Daemon Status")
	h.formatter.WriteKeyValue("Uptime", formatDuration(status.Uptime))
	h.formatter.WriteKeyValue("Tests Completed", fmt.Sprintf("%d", status.TestsCompleted))
	h.formatter.WriteKeyValue("Success Rate", fmt.Sprintf("%.1f%%", status.SuccessRate))
	h.formatter.WriteKeyValue("Active Workers", fmt.Sprintf("%d", status.ActiveWorkers))
	h.formatter.WriteKeyValue("Queue Size", fmt.Sprintf("%d", status.QueueSize))
	h.formatter.WriteKeyValue("Status", status.Status)
	
	if status.NextCycle.After(time.Now()) {
		h.formatter.WriteKeyValue("Next Cycle", fmt.Sprintf("in %s", time.Until(status.NextCycle).Round(time.Second)))
	}
	
	return h.writer.Flush()
}

func (h *Handler) handleWorkers() error {
	status := h.daemon.GetWorkerStatus()
	
	h.formatter.WriteHeader("Worker Pool Status")
	h.formatter.WriteKeyValue("Total Workers", fmt.Sprintf("%d", status.TotalWorkers))
	h.formatter.WriteKeyValue("Active", fmt.Sprintf("%d", status.Active))
	h.formatter.WriteKeyValue("Idle", fmt.Sprintf("%d", status.Idle))
	h.formatter.WriteKeyValue("Queue Length", fmt.Sprintf("%d", status.QueueLength))
	
	if len(status.CurrentTasks) > 0 {
		h.formatter.WriteInfo("\nCurrent Tasks:")
		for i, task := range status.CurrentTasks {
			h.formatter.WriteInfo(fmt.Sprintf("  %d. %s (started %s ago)", 
				i+1, task.Node, time.Since(task.StartTime).Round(time.Second)))
		}
	}
	
	return h.writer.Flush()
}

func (h *Handler) handlePause() error {
	if err := h.daemon.Pause(); err != nil {
		h.formatter.WriteError(fmt.Sprintf("Failed to pause: %v", err))
	} else {
		h.formatter.WriteSuccess("Testing paused")
	}
	return h.writer.Flush()
}

func (h *Handler) handleResume() error {
	if err := h.daemon.Resume(); err != nil {
		h.formatter.WriteError(fmt.Sprintf("Failed to resume: %v", err))
	} else {
		h.formatter.WriteSuccess("Testing resumed")
	}
	return h.writer.Flush()
}

func (h *Handler) handleReload() error {
	if err := h.daemon.ReloadConfig(); err != nil {
		h.formatter.WriteError(fmt.Sprintf("Failed to reload config: %v", err))
	} else {
		h.formatter.WriteSuccess("Configuration reloaded")
	}
	return h.writer.Flush()
}

func (h *Handler) handleDebug(args []string) error {
	// If no arguments provided, toggle the current state
	if len(args) == 0 {
		currentState := h.daemon.GetDebugMode()
		newState := !currentState
		
		// Toggle telnet output along with debug mode
		h.debugToTelnet = false
		h.debugCapture.Disable()
		
		if err := h.daemon.SetDebugMode(newState); err != nil {
			h.formatter.WriteError(fmt.Sprintf("Failed to toggle debug mode: %v", err))
		} else {
			if newState {
				h.formatter.WriteSuccess("Debug mode ENABLED")
				h.formatter.WriteInfo("Debug logs are written to daemon console")
				h.formatter.WriteInfo("Use 'debug on telnet' to show debug output here")
			} else {
				h.formatter.WriteSuccess("Debug mode DISABLED")
			}
		}
		return h.writer.Flush()
	}
	
	action := strings.ToLower(args[0])
	
	switch action {
	case "on", "enable", "true":
		// Check if there's a second argument for telnet output
		if len(args) > 1 && strings.ToLower(args[1]) == "telnet" {
			h.debugToTelnet = true
			h.debugCapture.Enable()
			if err := h.daemon.SetDebugMode(true); err != nil {
				h.formatter.WriteError(fmt.Sprintf("Failed to enable debug mode: %v", err))
			} else {
				h.formatter.WriteSuccess("Debug mode ENABLED with telnet output")
				h.formatter.WriteInfo("Debug logs will be shown in this telnet session")
				h.formatter.WriteWarning("Note: Debug output may be verbose and affect readability")
			}
		} else {
			h.debugToTelnet = false
			h.debugCapture.Disable()
			if err := h.daemon.SetDebugMode(true); err != nil {
				h.formatter.WriteError(fmt.Sprintf("Failed to enable debug mode: %v", err))
			} else {
				h.formatter.WriteSuccess("Debug mode ENABLED")
				h.formatter.WriteInfo("Debug logs are written to daemon console")
				h.formatter.WriteInfo("Use 'debug on telnet' to show debug output here")
			}
		}
	case "off", "disable", "false":
		h.debugToTelnet = false
		h.debugCapture.Disable()
		if err := h.daemon.SetDebugMode(false); err != nil {
			h.formatter.WriteError(fmt.Sprintf("Failed to disable debug mode: %v", err))
		} else {
			h.formatter.WriteSuccess("Debug mode DISABLED")
		}
	case "status":
		debugEnabled := h.daemon.GetDebugMode()
		if debugEnabled {
			if h.debugToTelnet {
				h.formatter.WriteInfo("Debug mode: ENABLED (output to telnet)")
			} else {
				h.formatter.WriteInfo("Debug mode: ENABLED (output to console)")
			}
		} else {
			h.formatter.WriteInfo("Debug mode: DISABLED")
		}
	default:
		h.formatter.WriteError(fmt.Sprintf("Invalid debug command: %s", action))
		h.formatter.WriteInfo("Usage: debug [on [telnet]|off|status] or just 'debug' to toggle")
		h.formatter.WriteInfo("  debug on        - Enable debug (output to console)")
		h.formatter.WriteInfo("  debug on telnet - Enable debug (output to this session)")
		h.formatter.WriteInfo("  debug off       - Disable debug")
		h.formatter.WriteInfo("  debug status    - Show current debug status")
		h.formatter.WriteInfo("  debug           - Toggle debug on/off")
	}
	
	return h.writer.Flush()
}

func (h *Handler) handleClear() error {
	h.writer.WriteString("\033[2J\033[H")
	return h.writer.Flush()
}



func formatDuration(d time.Duration) string {
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60
	
	if hours > 0 {
		return fmt.Sprintf("%dh %dm %ds", hours, minutes, seconds)
	} else if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}