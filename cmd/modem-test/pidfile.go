package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

const defaultPidFile = "~/.modem-test/modem-test.pid"

// resolvePidFilePath expands ~ and returns an absolute path.
func resolvePidFilePath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			path = filepath.Join(home, path[2:])
		}
	}
	return path
}

// AcquirePidFile checks that no other instance is running and writes our PID.
// Returns a cleanup function to remove the PID file on exit.
func AcquirePidFile(path string) (cleanup func(), err error) {
	path = resolvePidFilePath(path)

	// Ensure parent directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("cannot create pid file directory %s: %w", dir, err)
	}

	// Check existing PID file
	data, err := os.ReadFile(path)
	if err == nil {
		pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err == nil && pid > 0 {
			// Check if process is still running
			proc, err := os.FindProcess(pid)
			if err == nil {
				// Signal 0 checks existence without actually sending a signal
				if err := proc.Signal(syscall.Signal(0)); err == nil {
					return nil, fmt.Errorf("another modem-test is already running (pid %d, pidfile %s)", pid, path)
				}
			}
		}
		// Stale PID file — process not running, we'll overwrite it
	}

	// Write our PID
	if err := os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())+"\n"), 0644); err != nil {
		return nil, fmt.Errorf("cannot write pid file %s: %w", path, err)
	}

	cleanup = func() {
		os.Remove(path)
	}
	return cleanup, nil
}
