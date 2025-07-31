package version

import (
	"os/exec"
	"strings"
)

// Version will be set during build time via ldflags, fallback to Git
var Version = "dev"

// BuildTime will be set during build time via ldflags
var BuildTime = "unknown"

// GitCommit will be set during build time via ldflags
var GitCommit = "unknown"

// GetVersionInfo returns formatted version information
func GetVersionInfo() string {
	// If version is still "dev", try to get it from Git
	if Version == "dev" {
		if gitVersion := getGitVersion(); gitVersion != "" {
			return gitVersion
		}
	}
	return Version
}

// GetFullVersionInfo returns detailed version information
func GetFullVersionInfo() string {
	version := GetVersionInfo()
	if BuildTime != "unknown" && GitCommit != "unknown" {
		return version + " (built " + BuildTime + ", commit " + GitCommit + ")"
	}
	if GitCommit != "unknown" {
		return version + " (commit " + GitCommit + ")"
	}
	return version
}

// getGitVersion attempts to get version from Git tags
func getGitVersion() string {
	// Try to get the latest git tag
	cmd := exec.Command("git", "describe", "--tags", "--abbrev=0")
	if output, err := cmd.Output(); err == nil {
		version := strings.TrimSpace(string(output))
		if version != "" {
			// Remove 'v' prefix if present since templates add it
			return strings.TrimPrefix(version, "v")
		}
	}

	// Fallback: try to get current commit hash
	cmd = exec.Command("git", "rev-parse", "--short", "HEAD")
	if output, err := cmd.Output(); err == nil {
		commit := strings.TrimSpace(string(output))
		if commit != "" {
			return "dev-" + commit
		}
	}

	return "dev-unknown"
}
