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
	daemon    DaemonInterface
	writer    *bufio.Writer
	formatter *Formatter
}

func NewHandler(daemon DaemonInterface, writer *bufio.Writer) *Handler {
	return &Handler{
		daemon:    daemon,
		writer:    writer,
		formatter: NewFormatter(writer),
	}
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
		return h.handleTest(ctx, args)
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
  test node <zone> <net> <node> [hostname] - Test specific node
  test address <address> [hostname]        - Test by FidoNet address (e.g., 2:5001/100)

Status Commands:
  status    - Show daemon status
  workers   - Show worker pool status

Control Commands:
  pause     - Pause all testing
  resume    - Resume testing
  reload    - Reload configuration

Utility Commands:
  help [command] - Show help
  clear         - Clear screen
  exit/quit     - Disconnect

Type 'help <command>' for detailed help on a specific command.
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
  test node <zone> <net> <node> [hostname] [options]
  test address <address> [hostname] [options]

Examples:
  test node 2 5001 100 f100.5001.ru
  test address 2:5001/100 f100.5001.ru

The command will test the specified node using available protocols
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
	default:
		help = fmt.Sprintf("No detailed help available for '%s'\n", command)
	}
	
	h.writer.WriteString(help)
	return h.writer.Flush()
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