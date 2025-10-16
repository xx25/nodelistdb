package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"
)

// TelnetServer provides a simple telnet interface for CLI commands
type TelnetServer struct {
	daemon   DaemonInterface
	config   TelnetConfig
	listener net.Listener
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

// TelnetConfig holds configuration for the telnet server
type TelnetConfig struct {
	Host    string
	Port    int
	Prompt  string
	Welcome string
	Timeout time.Duration
}

// NewTelnetServer creates a new telnet server
func NewTelnetServer(daemon DaemonInterface, config TelnetConfig) *TelnetServer {
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
	if config.Welcome == "" {
		config.Welcome = "NodelistDB Test Daemon CLI\nType 'help' for available commands.\n"
	}
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Minute
	}

	return &TelnetServer{
		daemon: daemon,
		config: config,
	}
}

// Start starts the telnet server
func (s *TelnetServer) Start(ctx context.Context) error {
	s.ctx, s.cancel = context.WithCancel(ctx)

	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}
	s.listener = listener

	fmt.Printf("Telnet server listening on %s\n", addr)

	s.wg.Add(1)
	go s.acceptLoop()

	return nil
}

// Stop stops the telnet server
func (s *TelnetServer) Stop() error {
	s.cancel()
	if s.listener != nil {
		s.listener.Close()
	}
	s.wg.Wait()
	return nil
}

// acceptLoop accepts incoming connections
func (s *TelnetServer) acceptLoop() {
	defer s.wg.Done()

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.ctx.Done():
				return
			default:
				if ne, ok := err.(net.Error); ok && ne.Temporary() {
					time.Sleep(100 * time.Millisecond)
					continue
				}
				fmt.Printf("Accept error: %v\n", err)
				return
			}
		}

		s.wg.Add(1)
		go s.handleConnection(conn)
	}
}

// handleConnection handles individual client connections
func (s *TelnetServer) handleConnection(conn net.Conn) {
	defer s.wg.Done()
	defer conn.Close()

	// Set initial timeout
	_ = conn.SetDeadline(time.Now().Add(s.config.Timeout))

	// Send welcome message
	_, _ = fmt.Fprint(conn, s.config.Welcome)
	_, _ = fmt.Fprint(conn, s.config.Prompt)

	// Create handler
	writer := bufio.NewWriter(conn)
	handler := NewHandler(s.daemon, writer)

	// Read commands
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		// Reset timeout on each command
		_ = conn.SetDeadline(time.Now().Add(s.config.Timeout))

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			_, _ = fmt.Fprint(conn, s.config.Prompt)
			continue
		}

		// Handle exit commands
		if line == "exit" || line == "quit" {
			_, _ = fmt.Fprintln(conn, "Goodbye!")
			return
		}

		// Handle the command
		if err := handler.HandleCommand(s.ctx, line); err != nil {
			if err == io.EOF {
				return
			}
			_, _ = fmt.Fprintf(conn, "Error: %v\n", err)
		}

		// Flush output
		_ = writer.Flush()

		// Send prompt for next command
		_, _ = fmt.Fprint(conn, s.config.Prompt)
	}

	if err := scanner.Err(); err != nil {
		select {
		case <-s.ctx.Done():
			// Context cancelled, normal shutdown
		default:
			// Unexpected error
			fmt.Printf("Scanner error: %v\n", err)
		}
	}
}