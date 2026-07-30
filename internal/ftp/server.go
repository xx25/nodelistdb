package ftp

import (
	"fmt"
	"log"
	"time"

	ftpserver "github.com/fclairamb/ftpserverlib"
	"github.com/spf13/afero"
)

// Server represents an FTP server instance
type Server struct {
	driver     *Driver
	ftpServer  *ftpserver.FtpServer
	host       string
	port       int
	maxClients int
}

// MountConfig represents a virtual path mount
type MountConfig struct {
	VirtualPath string
	RealPath    string
}

// Config holds FTP server configuration
type Config struct {
	Enabled              bool
	Host                 string
	Port                 int
	Mounts               []MountConfig // Virtual path mounts
	MaxConnections       int
	PassivePortMin       int
	PassivePortMax       int
	IdleTimeout          time.Duration
	PublicHost           string // For passive mode (external IP/hostname)
	DisableActiveIPCheck bool   // Disable IP matching for active mode (PORT/EPRT)
}

// New creates a new FTP server
func New(cfg *Config) (*Server, error) {
	if !cfg.Enabled {
		return nil, nil
	}

	// Prepare FTP settings.
	//
	// DeflateCompressionLevel is deliberately left at 0, which disables MODE Z.
	// Do not raise it: ftpserverlib compresses with compress/flate, so it emits
	// a bare DEFLATE stream (RFC 1951) where every real client expects a zlib
	// stream (RFC 1950). lftp negotiates MODE Z whenever FEAT advertises it and
	// then fails every listing and transfer with
	//     zlib inflate error: incorrect header check
	// which is what ftpserverlib v0.29.0 did to this server: it forced the
	// level to 5 when unset, so MODE Z was always on and always broken.
	// v0.30.0 made it opt-in, and 0 now means "do not advertise MODE Z at all",
	// so clients stay in stream mode. Verified on the wire: the bytes are valid
	// raw deflate (inflates with wbits=-15) and invalid zlib (wbits=15).
	settings := &ftpserver.Settings{
		ListenAddr:              fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		PublicHost:              cfg.PublicHost,
		IdleTimeout:             int(cfg.IdleTimeout.Seconds()),
		DisableMLSD:             false,
		DisableMLST:             false,
		DisableMFMT:             false,
		ActiveTransferPortNon20: true, // Allow active mode without root (don't require port 20)
	}

	// Configure IP matching for active mode connections
	if cfg.DisableActiveIPCheck {
		settings.ActiveConnectionsCheck = ftpserver.IPMatchDisabled
	} else {
		settings.ActiveConnectionsCheck = ftpserver.IPMatchRequired
	}

	// Configure passive mode port range if specified
	if cfg.PassivePortMin > 0 && cfg.PassivePortMax > 0 {
		settings.PassiveTransferPortRange = &ftpserver.PortRange{
			Start: cfg.PassivePortMin,
			End:   cfg.PassivePortMax,
		}
	}

	// Create mount filesystem from configured mounts
	mounts := make([]Mount, 0, len(cfg.Mounts))
	for _, mountCfg := range cfg.Mounts {
		// Create a read-only, chrooted filesystem for each mount
		baseFs := afero.NewOsFs()
		basePath := afero.NewBasePathFs(baseFs, mountCfg.RealPath)
		readOnlyFs := afero.NewReadOnlyFs(basePath)

		mounts = append(mounts, Mount{
			VirtualPath: mountCfg.VirtualPath,
			Fs:          readOnlyFs,
		})
	}

	mountFs := NewMountFs(mounts)

	// Create driver with connection limit
	driver, err := NewDriver(mountFs, settings, cfg.MaxConnections)
	if err != nil {
		return nil, fmt.Errorf("failed to create FTP driver: %w", err)
	}

	// Create FTP server
	ftpServer := ftpserver.NewFtpServer(driver)

	server := &Server{
		driver:     driver,
		ftpServer:  ftpServer,
		host:       cfg.Host,
		port:       cfg.Port,
		maxClients: cfg.MaxConnections,
	}

	return server, nil
}

// Start starts the FTP server
func (s *Server) Start() error {
	if s == nil {
		return nil // FTP disabled
	}

	log.Printf("Starting FTP server on %s:%d", s.host, s.port)
	log.Printf("FTP server: anonymous-only, read-only access")
	log.Printf("FTP server: max connections = %d", s.maxClients)

	// Start listening (blocking)
	if err := s.ftpServer.ListenAndServe(); err != nil {
		return fmt.Errorf("failed to start FTP server: %w", err)
	}

	log.Printf("FTP server started successfully")
	return nil
}

// Stop stops the FTP server gracefully
func (s *Server) Stop() error {
	if s == nil {
		return nil // FTP disabled
	}

	log.Println("Stopping FTP server...")

	if err := s.ftpServer.Stop(); err != nil {
		return fmt.Errorf("failed to stop FTP server: %w", err)
	}

	log.Println("FTP server stopped")
	return nil
}

// GetStats returns server statistics
func (s *Server) GetStats() map[string]interface{} {
	if s == nil {
		return map[string]interface{}{
			"enabled": false,
		}
	}

	return map[string]interface{}{
		"enabled":            true,
		"host":               s.host,
		"port":               s.port,
		"max_connections":    s.maxClients,
		"active_connections": s.driver.ActiveConnections(),
	}
}
