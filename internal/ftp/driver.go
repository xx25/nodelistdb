package ftp

import (
	"crypto/tls"
	"errors"

	ftpserver "github.com/fclairamb/ftpserverlib"
	"github.com/spf13/afero"
)

// Driver implements the ftpserver.MainDriver interface for nodelist serving
type Driver struct {
	rootFs   afero.Fs // Root filesystem (mount-based)
	settings *ftpserver.Settings
}

// NewDriver creates a new FTP driver
func NewDriver(rootFs afero.Fs, settings *ftpserver.Settings) (*Driver, error) {
	if rootFs == nil {
		return nil, errors.New("root filesystem cannot be nil")
	}

	return &Driver{
		rootFs:   rootFs,
		settings: settings,
	}, nil
}

// GetSettings returns the FTP server settings
func (d *Driver) GetSettings() (*ftpserver.Settings, error) {
	return d.settings, nil
}

// ClientConnected is called when a client connects
func (d *Driver) ClientConnected(cc ftpserver.ClientContext) (string, error) {
	return "Welcome to FidoNet NodelistDB FTP Server - Anonymous access only", nil
}

// ClientDisconnected is called when a client disconnects
func (d *Driver) ClientDisconnected(cc ftpserver.ClientContext) {
	// Nothing to cleanup
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

// GetTLSConfig returns nil as we don't support TLS (anonymous read-only)
func (d *Driver) GetTLSConfig() (*tls.Config, error) {
	return nil, nil // No TLS support for now
}
