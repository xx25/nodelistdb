package storage

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/nodelistdb/internal/database"
)

// ModemCallerOperations handles modem caller status database operations
type ModemCallerOperations struct {
	db database.DatabaseInterface
	mu sync.RWMutex
}

// NewModemCallerOperations creates a new ModemCallerOperations instance
func NewModemCallerOperations(db database.DatabaseInterface) *ModemCallerOperations {
	return &ModemCallerOperations{
		db: db,
	}
}

// GetCallerStatus retrieves the status for a specific modem daemon
func (m *ModemCallerOperations) GetCallerStatus(ctx context.Context, callerID string) (*ModemCallerStatus, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	query := `
		SELECT caller_id, last_heartbeat, status, modems_available, modems_in_use,
			   tests_completed, tests_failed, last_test_time, updated_at
		FROM modem_caller_status
		WHERE caller_id = ?
		ORDER BY updated_at DESC
		LIMIT 1
	`

	var status ModemCallerStatus
	err := m.db.Conn().QueryRowContext(ctx, query, callerID).Scan(
		&status.CallerID, &status.LastHeartbeat, &status.Status,
		&status.ModemsAvailable, &status.ModemsInUse,
		&status.TestsCompleted, &status.TestsFailed,
		&status.LastTestTime, &status.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get caller status: %w", err)
	}

	return &status, nil
}

// UpdateHeartbeat updates the heartbeat and stats for a modem daemon.
// Uses INSERT instead of ALTER TABLE UPDATE because ReplacingMergeTree(updated_at)
// deduplicates by keeping the row with the highest updated_at for each caller_id.
func (m *ModemCallerOperations) UpdateHeartbeat(ctx context.Context, callerID string, stats HeartbeatStats) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()

	query := `
		INSERT INTO modem_caller_status (
			caller_id, last_heartbeat, status, modems_available, modems_in_use,
			tests_completed, tests_failed, last_test_time, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := m.db.Conn().ExecContext(ctx, query,
		callerID, now, stats.Status, stats.ModemsAvailable, stats.ModemsInUse,
		stats.TestsCompleted, stats.TestsFailed, stats.LastTestTime, now)
	if err != nil {
		return fmt.Errorf("failed to upsert caller status: %w", err)
	}

	return nil
}

// SetCallerStatus updates just the status field for a modem daemon.
// Reads current row then INSERTs with updated status (ReplacingMergeTree pattern).
func (m *ModemCallerOperations) SetCallerStatus(ctx context.Context, callerID string, newStatus string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()

	// Read current values so we preserve them in the new row
	readQuery := `
		SELECT last_heartbeat, modems_available, modems_in_use,
			   tests_completed, tests_failed, last_test_time
		FROM modem_caller_status
		WHERE caller_id = ?
		ORDER BY updated_at DESC
		LIMIT 1
	`
	var lastHeartbeat time.Time
	var modemsAvailable, modemsInUse uint8
	var testsCompleted, testsFailed uint32
	var lastTestTime time.Time

	err := m.db.Conn().QueryRowContext(ctx, readQuery, callerID).Scan(
		&lastHeartbeat, &modemsAvailable, &modemsInUse,
		&testsCompleted, &testsFailed, &lastTestTime,
	)
	if err == sql.ErrNoRows {
		// Caller doesn't exist yet, insert with defaults
		lastHeartbeat = now
	} else if err != nil {
		return fmt.Errorf("failed to read caller status: %w", err)
	}

	query := `
		INSERT INTO modem_caller_status (
			caller_id, last_heartbeat, status, modems_available, modems_in_use,
			tests_completed, tests_failed, last_test_time, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err = m.db.Conn().ExecContext(ctx, query,
		callerID, lastHeartbeat, newStatus, modemsAvailable, modemsInUse,
		testsCompleted, testsFailed, lastTestTime, now)
	if err != nil {
		return fmt.Errorf("failed to set caller status: %w", err)
	}

	return nil
}

// GetAllCallerStatuses retrieves status for all modem daemons
func (m *ModemCallerOperations) GetAllCallerStatuses(ctx context.Context) ([]ModemCallerStatus, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	query := `
		SELECT caller_id, last_heartbeat, status, modems_available, modems_in_use,
			   tests_completed, tests_failed, last_test_time, updated_at
		FROM modem_caller_status
		ORDER BY caller_id, updated_at DESC
		LIMIT 1 BY caller_id
	`

	rows, err := m.db.Conn().QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query caller statuses: %w", err)
	}
	defer rows.Close()

	var statuses []ModemCallerStatus
	for rows.Next() {
		var status ModemCallerStatus
		err := rows.Scan(
			&status.CallerID, &status.LastHeartbeat, &status.Status,
			&status.ModemsAvailable, &status.ModemsInUse,
			&status.TestsCompleted, &status.TestsFailed,
			&status.LastTestTime, &status.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan caller status: %w", err)
		}
		statuses = append(statuses, status)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return statuses, nil
}

// IsCallerActive checks if a daemon is considered active based on heartbeat
func (m *ModemCallerOperations) IsCallerActive(ctx context.Context, callerID string, threshold time.Duration) (bool, error) {
	status, err := m.GetCallerStatus(ctx, callerID)
	if err != nil {
		return false, err
	}
	if status == nil {
		return false, nil
	}

	return time.Since(status.LastHeartbeat) < threshold, nil
}

// DeleteCallerStatus removes a caller status entry (for cleanup)
func (m *ModemCallerOperations) DeleteCallerStatus(ctx context.Context, callerID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	query := `ALTER TABLE modem_caller_status DELETE WHERE caller_id = ?`
	_, err := m.db.Conn().ExecContext(ctx, query, callerID)
	if err != nil {
		return fmt.Errorf("failed to delete caller status: %w", err)
	}

	return nil
}
