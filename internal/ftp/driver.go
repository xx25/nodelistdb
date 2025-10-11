package ftp

import (
	"crypto/tls"
	"errors"
	"os"
	"path/filepath"
	"strings"

	ftpserver "github.com/fclairamb/ftpserverlib"
	"github.com/spf13/afero"
)

// Driver implements the ftpserver.MainDriver interface for nodelist serving
type Driver struct {
	baseDir  string // Base directory for nodelist files
	settings *ftpserver.Settings
}

// NewDriver creates a new FTP driver
func NewDriver(baseDir string, settings *ftpserver.Settings) (*Driver, error) {
	// Verify base directory exists
	if _, err := os.Stat(baseDir); err != nil {
		return nil, err
	}

	return &Driver{
		baseDir:  baseDir,
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

	// Create a chrooted filesystem that:
	// 1. Is read-only (using ReadOnlyFs)
	// 2. Is restricted to baseDir (using BasePathFs)
	baseFs := afero.NewOsFs()
	basePath := afero.NewBasePathFs(baseFs, d.baseDir)
	readOnlyFs := afero.NewReadOnlyFs(basePath)

	return readOnlyFs, nil
}

// GetTLSConfig returns nil as we don't support TLS (anonymous read-only)
func (d *Driver) GetTLSConfig() (*tls.Config, error) {
	return nil, nil // No TLS support for now
}

// ChrootedFs is a wrapper around afero.Fs that prevents directory traversal
type ChrootedFs struct {
	afero.Fs
	baseDir string
}

// resolvePath ensures the path is within baseDir
func (c *ChrootedFs) resolvePath(path string) (string, error) {
	// Clean the path
	cleanPath := filepath.Clean(path)

	// Build absolute path
	absPath := filepath.Join(c.baseDir, cleanPath)

	// Ensure the resolved path is within baseDir (prevent directory traversal)
	if !strings.HasPrefix(absPath, c.baseDir) {
		return "", errors.New("access denied: path outside base directory")
	}

	return absPath, nil
}
