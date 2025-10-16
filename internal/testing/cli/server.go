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

type DaemonInterface interface {
	TestNode(ctx context.Context, zone, net, node uint16, hostname string, options TestOptions) (*TestResult, error)
	GetStatus() DaemonStatus
	GetWorkerStatus() WorkerStatus
	GetNodeInfo(ctx context.Context, zone, net, node uint16) (*NodeInfo, error)
	Pause() error
	Resume() error
	ReloadConfig() error
	SetDebugMode(enabled bool) error
	GetDebugMode() bool
}

type CLIServer struct {
	daemon     DaemonInterface
	host       string
	port       int
	maxClients int
	timeout    time.Duration
	prompt     string
	welcome    string
	
	mu       sync.RWMutex
	clients  map[net.Conn]*Client
	listener net.Listener
	ctx      context.Context
	cancel   context.CancelFunc
}

type Client struct {
	conn     net.Conn
	reader   *bufio.Reader
	writer   *bufio.Writer
	lastSeen time.Time
}

type Config struct {
	Host       string
	Port       int
	MaxClients int
	Timeout    time.Duration
	Prompt     string
	Welcome    string
}

func NewServer(daemon DaemonInterface, config Config) *CLIServer {
	if config.Host == "" {
		config.Host = "127.0.0.1"
	}
	if config.Port == 0 {
		config.Port = 2323
	}
	if config.MaxClients == 0 {
		config.MaxClients = 5
	}
	if config.Timeout == 0 {
		config.Timeout = 5 * time.Minute
	}
	if config.Prompt == "" {
		config.Prompt = "> "
	}
	if config.Welcome == "" {
		config.Welcome = "NodelistDB Test Daemon CLI v1.0.0\nType 'help' for available commands.\n"
	}
	
	return &CLIServer{
		daemon:     daemon,
		host:       config.Host,
		port:       config.Port,
		maxClients: config.MaxClients,
		timeout:    config.Timeout,
		prompt:     config.Prompt,
		welcome:    config.Welcome,
		clients:    make(map[net.Conn]*Client),
	}
}

func (s *CLIServer) Start(ctx context.Context) error {
	s.ctx, s.cancel = context.WithCancel(ctx)
	
	addr := fmt.Sprintf("%s:%d", s.host, s.port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to start CLI server on %s: %w", addr, err)
	}
	s.listener = listener
	
	fmt.Printf("CLI server listening on %s\n", addr)
	
	go s.acceptLoop()
	go s.cleanupLoop()
	
	<-s.ctx.Done()
	return s.shutdown()
}

func (s *CLIServer) acceptLoop() {
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
			conn, err := s.listener.Accept()
			if err != nil {
				if !strings.Contains(err.Error(), "closed") {
					fmt.Printf("Accept error: %v\n", err)
				}
				continue
			}
			
			s.mu.Lock()
			if len(s.clients) >= s.maxClients {
				s.mu.Unlock()
				_, _ = conn.Write([]byte("Server full, try again later\r\n"))
				conn.Close()
				continue
			}
			
			client := &Client{
				conn:     conn,
				reader:   bufio.NewReader(conn),
				writer:   bufio.NewWriter(conn),
				lastSeen: time.Now(),
			}
			s.clients[conn] = client
			s.mu.Unlock()
			
			go s.handleClient(client)
		}
	}
}

func (s *CLIServer) handleClient(client *Client) {
	defer func() {
		s.mu.Lock()
		delete(s.clients, client.conn)
		s.mu.Unlock()
		client.conn.Close()
	}()
	
	_, _ = client.writer.WriteString(s.welcome)
	_, _ = client.writer.WriteString("\r\n")
	_, _ = client.writer.WriteString(s.prompt)
	client.writer.Flush()
	
	handler := NewHandler(s.daemon, client.writer)
	
	for {
		_ = client.conn.SetReadDeadline(time.Now().Add(s.timeout))
		
		line, err := client.reader.ReadString('\n')
		if err != nil {
			if err != io.EOF && !strings.Contains(err.Error(), "timeout") {
				fmt.Printf("Read error: %v\n", err)
			}
			return
		}
		
		client.lastSeen = time.Now()
		
		line = strings.TrimSpace(line)
		if line == "" {
			client.writer.WriteString(s.prompt)
			client.writer.Flush()
			continue
		}
		
		if line == "exit" || line == "quit" {
			client.writer.WriteString("Goodbye!\r\n")
			client.writer.Flush()
			return
		}
		
		if err := handler.HandleCommand(s.ctx, line); err != nil {
			if err == io.EOF {
				return
			}
			client.writer.WriteString(fmt.Sprintf("Error: %v\r\n", err))
			client.writer.Flush()
		}
		
		client.writer.WriteString(s.prompt)
		client.writer.Flush()
	}
}

func (s *CLIServer) cleanupLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.mu.Lock()
			now := time.Now()
			for conn, client := range s.clients {
				if now.Sub(client.lastSeen) > s.timeout {
					client.writer.WriteString("\r\nSession timeout\r\n")
					client.writer.Flush()
					conn.Close()
					delete(s.clients, conn)
				}
			}
			s.mu.Unlock()
		}
	}
}

func (s *CLIServer) shutdown() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	for conn, client := range s.clients {
		client.writer.WriteString("\r\nServer shutting down\r\n")
		client.writer.Flush()
		conn.Close()
	}
	
	if s.listener != nil {
		s.listener.Close()
	}
	
	return nil
}

func (s *CLIServer) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
}