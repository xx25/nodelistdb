package ftp

import (
	"crypto/tls"
	"errors"
	"fmt"
	"sync/atomic"

	ftpserver "github.com/fclairamb/ftpserverlib"
	"github.com/spf13/afero"
)

// Driver implements the ftpserver.MainDriver interface for nodelist serving
type Driver struct {
	rootFs       afero.Fs // Root filesystem (mount-based)
	settings     *ftpserver.Settings
	maxClients   int32         // Maximum allowed concurrent connections (0 = unlimited)
	activeConns  atomic.Int32  // Current number of active connections
}

// NewDriver creates a new FTP driver
func NewDriver(rootFs afero.Fs, settings *ftpserver.Settings, maxClients int) (*Driver, error) {
	if rootFs == nil {
		return nil, errors.New("root filesystem cannot be nil")
	}

	return &Driver{
		rootFs:     rootFs,
		settings:   settings,
		maxClients: int32(maxClients),
	}, nil
}

// GetSettings returns the FTP server settings
func (d *Driver) GetSettings() (*ftpserver.Settings, error) {
	return d.settings, nil
}

// ErrTooManyConnections is returned when the connection limit is reached
var ErrTooManyConnections = errors.New("too many connections")

// ClientConnected is called when a client connects.
// It enforces the connection limit and tracks active connections.
func (d *Driver) ClientConnected(cc ftpserver.ClientContext) (string, error) {
	// Enforce connection limit if configured (maxClients > 0)
	if d.maxClients > 0 {
		current := d.activeConns.Add(1)
		if current > d.maxClients {
			// Over limit - decrement and reject
			d.activeConns.Add(-1)
			return "", fmt.Errorf("%w: limit is %d", ErrTooManyConnections, d.maxClients)
		}
	}

	return "Welcome to FidoNet NodelistDB FTP Server - Anonymous access only", nil
}

// ClientDisconnected is called when a client disconnects.
// It decrements the active connection counter.
func (d *Driver) ClientDisconnected(cc ftpserver.ClientContext) {
	// Only decrement if we're tracking connections
	if d.maxClients > 0 {
		d.activeConns.Add(-1)
	}
}

// AuthUser authenticates a user - we only allow anonymous
func (d *Driver) AuthUser(cc ftpserver.ClientContext, user, pass string) (ftpserver.ClientDriver, error) {
	// Only allow anonymous access
	if user != "anonymous" && user != "ftp" {
		return nil, errors.New("only anonymous access is allowed")
	}

	// Return the root filesystem (already configured with mounts and read-only)
	return d.rootFs, nil
}

// ActiveConnections returns the current number of active connections
func (d *Driver) ActiveConnections() int {
	return int(d.activeConns.Load())
}

// ErrTLSNotSupported is returned when a client requests TLS but it's not configured
var ErrTLSNotSupported = errors.New("TLS is not supported on this server")

// GetTLSConfig returns an error as we don't support TLS (anonymous read-only)
func (d *Driver) GetTLSConfig() (*tls.Config, error) {
	return nil, ErrTLSNotSupported
}
