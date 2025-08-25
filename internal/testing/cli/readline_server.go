package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"

	"github.com/chzyer/readline"
)

// ReadlineServer provides an enhanced CLI interface with history and line editing
type ReadlineServer struct {
	daemon     DaemonInterface
	config     ReadlineConfig
	listener   net.Listener
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	handler    *Handler
}

// ReadlineConfig holds configuration for the enhanced CLI server
type ReadlineConfig struct {
	Host         string
	Port         int
	Prompt       string
	HistoryLimit int  // In-memory history limit
	Welcome      string
}

// NewReadlineServer creates a new enhanced CLI server
func NewReadlineServer(daemon DaemonInterface, config ReadlineConfig) *ReadlineServer {
	// Set defaults
	if config.Host == "" {
		config.Host = "127.0.0.1"
	}
	if config.Port == 0 {
		config.Port = 2323
	}
	if config.Prompt == "" {
		config.Prompt = "testdaemon> "
	}
	if config.HistoryLimit == 0 {
		config.HistoryLimit = 100  // Keep last 100 commands in memory
	}
	if config.Welcome == "" {
		config.Welcome = "NodelistDB Test Daemon Enhanced CLI\nType 'help' for available commands.\n"
	}

	return &ReadlineServer{
		daemon: daemon,
		config: config,
	}
}

// Start starts the enhanced CLI server
func (s *ReadlineServer) Start(ctx context.Context) error {
	s.ctx, s.cancel = context.WithCancel(ctx)

	// Create readline configuration
	cfg := &readline.Config{
		Prompt:          s.config.Prompt,
		HistoryLimit:    s.config.HistoryLimit,  // In-memory history only
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",
		AutoComplete:    s.createCompleter(),
	}

	// Start listening for remote connections
	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)
	
	fmt.Printf("Enhanced CLI server listening on %s\n", addr)
	
	// Use readline's ListenRemote for network support (this is blocking)
	return readline.ListenRemote("tcp", addr, cfg, s.handleConnection)
}

// handleConnection handles individual client connections
func (s *ReadlineServer) handleConnection(rl *readline.Instance) {
	defer rl.Close()
	
	// Send welcome message
	rl.Write([]byte(s.config.Welcome))
	
	// Create a bufio.Writer wrapper for the handler
	rlWriter := &readlineWriter{rl: rl}
	bufWriter := bufio.NewWriter(rlWriter)
	handler := NewHandler(s.daemon, bufWriter)

	for {
		select {
		case <-s.ctx.Done():
			return
		default:
			line, err := rl.Readline()
			if err == readline.ErrInterrupt {
				if len(line) == 0 {
					rl.Write([]byte("Use 'exit' or 'quit' to disconnect\n"))
					continue
				}
			} else if err == io.EOF {
				return
			} else if err != nil {
				return
			}

			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			if line == "exit" || line == "quit" {
				rl.Write([]byte("Goodbye!\n"))
				return
			}

			// Handle the command
			if err := handler.HandleCommand(s.ctx, line); err != nil {
				if err == io.EOF {
					return
				}
				rl.Write([]byte(fmt.Sprintf("Error: %v\n", err)))
			}
			// Flush the buffer to ensure output is sent
			bufWriter.Flush()
		}
	}
}

// createCompleter creates auto-completion configuration
func (s *ReadlineServer) createCompleter() *readline.PrefixCompleter {
	return readline.NewPrefixCompleter(
		readline.PcItem("help",
			readline.PcItem("test"),
			readline.PcItem("status"),
			readline.PcItem("workers"),
			readline.PcItem("pause"),
			readline.PcItem("resume"),
			readline.PcItem("reload"),
			readline.PcItem("debug"),
			readline.PcItem("info"),
		),
		readline.PcItem("test",
			readline.PcItem("node"),
			readline.PcItem("ifcico"),
			readline.PcItem("binkp"),
			readline.PcItem("telnet"),
		),
		readline.PcItem("status"),
		readline.PcItem("workers"),
		readline.PcItem("pause"),
		readline.PcItem("resume"),
		readline.PcItem("reload"),
		readline.PcItem("debug",
			readline.PcItem("on"),
			readline.PcItem("off"),
			readline.PcItem("status"),
		),
		readline.PcItem("info"),
		readline.PcItem("clear"),
		readline.PcItem("history"),
		readline.PcItem("exit"),
		readline.PcItem("quit"),
	)
}

// Stop stops the enhanced CLI server
func (s *ReadlineServer) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
}

// readlineWriter wraps readline.Instance to implement io.Writer
type readlineWriter struct {
	rl *readline.Instance
}

func (w *readlineWriter) Write(p []byte) (n int, err error) {
	// Convert to string and ensure proper line endings
	str := string(p)
	str = strings.ReplaceAll(str, "\n", "\r\n")
	_, err = w.rl.Write([]byte(str))
	return len(p), err
}