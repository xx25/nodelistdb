package storage

import (
	"fmt"
	"sync"
	"time"

	"github.com/nodelistdb/internal/database"
)

// MultiStorage manages multiple database connections for concurrent writes
type MultiStorage struct {
	storages map[string]*Storage
	mu       sync.RWMutex
}

// NewMultiStorage creates a new MultiStorage instance with multiple databases
func NewMultiStorage(databases map[string]database.DatabaseInterface) (*MultiStorage, error) {
	storages := make(map[string]*Storage)
	
	for name, db := range databases {
		storage, err := New(db)
		if err != nil {
			return nil, fmt.Errorf("failed to create storage for %s: %w", name, err)
		}
		storages[name] = storage
	}
	
	return &MultiStorage{
		storages: storages,
	}, nil
}

// InsertNodes inserts nodes into all configured databases
func (ms *MultiStorage) InsertNodes(nodes []database.Node) error {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	
	// Use a channel to collect errors from concurrent inserts
	errCh := make(chan error, len(ms.storages))
	var wg sync.WaitGroup
	
	for name, storage := range ms.storages {
		wg.Add(1)
		go func(dbName string, s *Storage) {
			defer wg.Done()
			if err := s.InsertNodes(nodes); err != nil {
				errCh <- fmt.Errorf("database %s: %w", dbName, err)
			}
		}(name, storage)
	}
	
	wg.Wait()
	close(errCh)
	
	// Collect any errors
	var errors []string
	for err := range errCh {
		errors = append(errors, err.Error())
	}
	
	if len(errors) > 0 {
		return fmt.Errorf("insert errors: %s", errors[0]) // Return first error for now
	}
	
	return nil
}

// IsNodelistProcessed checks if a nodelist was already processed in any database
func (ms *MultiStorage) IsNodelistProcessed(nodelistDate time.Time) (bool, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	
	// Check in the first available database (they should be in sync)
	for _, storage := range ms.storages {
		return storage.IsNodelistProcessed(nodelistDate)
	}
	
	return false, fmt.Errorf("no databases configured")
}

// FindConflictingNode finds conflicting nodes in any database
func (ms *MultiStorage) FindConflictingNode(zone, net, node int, nodelistDate time.Time) (bool, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	
	// Check in the first available database
	for _, storage := range ms.storages {
		return storage.FindConflictingNode(zone, net, node, nodelistDate)
	}
	
	return false, fmt.Errorf("no databases configured")
}


// Close closes all database connections
func (ms *MultiStorage) Close() error {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	
	var errors []string
	for name, storage := range ms.storages {
		if err := storage.Close(); err != nil {
			errors = append(errors, fmt.Sprintf("database %s: %v", name, err))
		}
	}
	
	if len(errors) > 0 {
		return fmt.Errorf("close errors: %s", errors[0])
	}
	
	return nil
}

// GetStorageNames returns the names of all configured storages
func (ms *MultiStorage) GetStorageNames() []string {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	
	names := make([]string, 0, len(ms.storages))
	for name := range ms.storages {
		names = append(names, name)
	}
	return names
}

// GetStorage returns a specific storage by name
func (ms *MultiStorage) GetStorage(name string) (*Storage, bool) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	
	storage, exists := ms.storages[name]
	return storage, exists
}