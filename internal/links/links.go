// Package links provides hot-reloadable FidoNet links configuration
package links

import (
	"os"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/nodelistdb/internal/logging"
)

// Link represents a single external link
type Link struct {
	Title       string `yaml:"title"`
	URL         string `yaml:"url"`
	Description string `yaml:"description"`
}

// Category represents a group of related links
type Category struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Links       []Link `yaml:"links"`
}

// Config represents the links configuration file
type Config struct {
	Categories []Category `yaml:"categories"`
}

// Loader provides hot-reloadable links configuration
type Loader struct {
	mu           sync.RWMutex
	config       *Config
	filePath     string
	lastModified time.Time
	checkTicker  *time.Ticker
	stopChan     chan struct{}
}

// NewLoader creates a new links loader that watches the config file for changes
func NewLoader(filePath string) *Loader {
	l := &Loader{
		filePath: filePath,
		config:   &Config{},
		stopChan: make(chan struct{}),
	}

	// Initial load
	if err := l.reload(); err != nil {
		logging.Warn("Failed to load links config", "path", filePath, "error", err)
	}

	// Start background file watcher
	l.checkTicker = time.NewTicker(10 * time.Second)
	go l.watchFile()

	return l
}

// GetConfig returns the current links configuration
func (l *Loader) GetConfig() *Config {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.config
}

// Stop stops the background file watcher
func (l *Loader) Stop() {
	close(l.stopChan)
	if l.checkTicker != nil {
		l.checkTicker.Stop()
	}
}

// reload loads the configuration from the file
func (l *Loader) reload() error {
	data, err := os.ReadFile(l.filePath)
	if err != nil {
		return err
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return err
	}

	l.mu.Lock()
	l.config = &config
	l.mu.Unlock()

	logging.Info("Links configuration loaded", "path", l.filePath, "categories", len(config.Categories))
	return nil
}

// watchFile periodically checks if the config file has been modified
func (l *Loader) watchFile() {
	for {
		select {
		case <-l.stopChan:
			return
		case <-l.checkTicker.C:
			l.checkAndReload()
		}
	}
}

// checkAndReload checks if the file was modified and reloads if necessary
func (l *Loader) checkAndReload() {
	stat, err := os.Stat(l.filePath)
	if err != nil {
		// File might not exist, that's OK
		return
	}

	modTime := stat.ModTime()
	l.mu.RLock()
	lastMod := l.lastModified
	l.mu.RUnlock()

	if modTime.After(lastMod) {
		if err := l.reload(); err != nil {
			logging.Warn("Failed to reload links config", "path", l.filePath, "error", err)
			return
		}

		l.mu.Lock()
		l.lastModified = modTime
		l.mu.Unlock()

		logging.Info("Links configuration reloaded", "path", l.filePath)
	}
}
