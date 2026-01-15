package storage

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/nodelistdb/internal/database"
	"github.com/nodelistdb/internal/testing/timeavail"
)

// ModemQueueOperations handles modem test queue database operations
type ModemQueueOperations struct {
	db database.DatabaseInterface
	mu sync.RWMutex
}

// NewModemQueueOperations creates a new ModemQueueOperations instance
func NewModemQueueOperations(db database.DatabaseInterface) *ModemQueueOperations {
	return &ModemQueueOperations{
		db: db,
	}
}

// GetAssignedNodes retrieves pending nodes assigned to a specific daemon
func (m *ModemQueueOperations) GetAssignedNodes(ctx context.Context, callerID string, limit int, onlyCallable bool) ([]ModemQueueNode, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	query := `
		SELECT zone, net, node, conflict_sequence, phone, phone_normalized,
			   modem_flags, flags, is_cm, time_flags, assigned_to, assigned_at,
			   priority, retry_count, next_attempt_after, status,
			   in_progress_since, last_tested_at, last_error, created_at, updated_at
		FROM modem_test_queue
		WHERE assigned_to = ?
		  AND status = 'pending'
		  AND next_attempt_after <= now()
		  AND NOT (has(flags, 'ICM') AND NOT is_cm)
		ORDER BY priority DESC, next_attempt_after ASC
		LIMIT ?
	`

	rows, err := m.db.Conn().QueryContext(ctx, query, callerID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query assigned nodes: %w", err)
	}
	defer rows.Close()

	var nodes []ModemQueueNode
	now := time.Now().UTC()

	for rows.Next() {
		var entry ModemQueueEntry
		err := rows.Scan(
			&entry.Zone, &entry.Net, &entry.Node, &entry.ConflictSequence,
			&entry.Phone, &entry.PhoneNormalized,
			&entry.ModemFlags, &entry.Flags, &entry.IsCM, &entry.TimeFlags,
			&entry.AssignedTo, &entry.AssignedAt,
			&entry.Priority, &entry.RetryCount, &entry.NextAttemptAfter,
			&entry.Status, &entry.InProgressSince, &entry.LastTestedAt,
			&entry.LastError, &entry.CreatedAt, &entry.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan queue entry: %w", err)
		}

		node := ModemQueueNode{ModemQueueEntry: entry}

		// Compute time availability
		node.IsCallableNow = isCallableNow(entry.IsCM, entry.TimeFlags, entry.Zone, now)
		if !node.IsCallableNow {
			node.NextCallWindow = getNextCallWindow(entry.TimeFlags, entry.Zone, now)
		}

		// Filter non-callable if requested
		if onlyCallable && !node.IsCallableNow {
			continue
		}

		nodes = append(nodes, node)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return nodes, nil
}

// GetPendingCount returns the count of pending nodes for a daemon
func (m *ModemQueueOperations) GetPendingCount(ctx context.Context, callerID string) (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	query := `SELECT count() FROM modem_test_queue WHERE assigned_to = ? AND status = 'pending'`

	var count uint64
	err := m.db.Conn().QueryRowContext(ctx, query, callerID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count pending nodes: %w", err)
	}

	return int(count), nil
}

// MarkNodesInProgress marks nodes as in_progress
// Returns the number of nodes that were eligible to be marked (owned by caller and in pending status).
// ClickHouse ALTER TABLE UPDATE is async and doesn't return affected row count,
// so we first count eligible nodes, then perform mutations.
func (m *ModemQueueOperations) MarkNodesInProgress(ctx context.Context, callerID string, nodes []NodeIdentifier) (int, error) {
	if len(nodes) == 0 {
		return 0, nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()

	// First, count nodes that CAN be marked (pending + owned by caller)
	eligibleCount := 0
	for _, node := range nodes {
		var count uint64
		err := m.db.Conn().QueryRowContext(ctx, `
			SELECT count() FROM modem_test_queue
			WHERE zone = ? AND net = ? AND node = ? AND conflict_sequence = ?
			  AND assigned_to = ? AND status = 'pending'
		`, node.Zone, node.Net, node.Node, node.ConflictSequence, callerID).Scan(&count)
		if err != nil {
			return 0, fmt.Errorf("failed to check node eligibility: %w", err)
		}
		if count > 0 {
			eligibleCount++
		}
	}

	// Then perform the mutations
	for _, node := range nodes {
		query := `
			ALTER TABLE modem_test_queue
			UPDATE status = 'in_progress', in_progress_since = ?, updated_at = ?
			WHERE zone = ? AND net = ? AND node = ? AND conflict_sequence = ?
			  AND assigned_to = ? AND status = 'pending'
		`
		_, err := m.db.Conn().ExecContext(ctx, query, now, now,
			node.Zone, node.Net, node.Node, node.ConflictSequence, callerID)
		if err != nil {
			return eligibleCount, fmt.Errorf("failed to mark node in_progress: %w", err)
		}
	}

	return eligibleCount, nil
}

// MarkNodeCompleted marks a single node as completed
// Only updates if the node is assigned to the caller and is in_progress status
func (m *ModemQueueOperations) MarkNodeCompleted(ctx context.Context, callerID string, node NodeIdentifier) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	query := `
		ALTER TABLE modem_test_queue
		UPDATE status = 'completed', in_progress_since = toDateTime(0),
			   last_tested_at = ?, updated_at = ?
		WHERE zone = ? AND net = ? AND node = ? AND conflict_sequence = ?
		  AND assigned_to = ? AND status = 'in_progress'
	`
	_, err := m.db.Conn().ExecContext(ctx, query, now, now,
		node.Zone, node.Net, node.Node, node.ConflictSequence, callerID)
	return err
}

// MarkNodeFailed marks a single node as failed with error message
// Only updates if the node is assigned to the caller and is in_progress status
func (m *ModemQueueOperations) MarkNodeFailed(ctx context.Context, callerID string, node NodeIdentifier, errMsg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	query := `
		ALTER TABLE modem_test_queue
		UPDATE status = 'failed', in_progress_since = toDateTime(0),
			   last_tested_at = ?, last_error = ?, retry_count = retry_count + 1,
			   next_attempt_after = now() + INTERVAL 5 MINUTE, updated_at = ?
		WHERE zone = ? AND net = ? AND node = ? AND conflict_sequence = ?
		  AND assigned_to = ? AND status = 'in_progress'
	`
	_, err := m.db.Conn().ExecContext(ctx, query, now, errMsg, now,
		node.Zone, node.Net, node.Node, node.ConflictSequence, callerID)
	return err
}

// VerifyNodeOwnership checks if a node is assigned to the caller and in in_progress status
// This is used to verify ownership before storing results (ClickHouse mutations don't return affected row counts)
func (m *ModemQueueOperations) VerifyNodeOwnership(ctx context.Context, callerID string, node NodeIdentifier) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	query := `
		SELECT count() FROM modem_test_queue
		WHERE zone = ? AND net = ? AND node = ? AND conflict_sequence = ?
		  AND assigned_to = ?
		  AND status = 'in_progress'
	`

	var count uint64
	err := m.db.Conn().QueryRowContext(ctx, query,
		node.Zone, node.Net, node.Node, node.ConflictSequence, callerID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to verify node ownership: %w", err)
	}

	return count > 0, nil
}

// VerifyNodeStatus checks if a node is assigned to the caller and has the expected status
// Used after mutations to verify they actually took effect (eliminates TOCTOU race)
func (m *ModemQueueOperations) VerifyNodeStatus(ctx context.Context, callerID string, node NodeIdentifier, expectedStatus string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	query := `
		SELECT count() FROM modem_test_queue
		WHERE zone = ? AND net = ? AND node = ? AND conflict_sequence = ?
		  AND assigned_to = ?
		  AND status = ?
	`

	var count uint64
	err := m.db.Conn().QueryRowContext(ctx, query,
		node.Zone, node.Net, node.Node, node.ConflictSequence, callerID, expectedStatus).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to verify node status: %w", err)
	}

	return count > 0, nil
}

// NodeExistsInQueue checks if a node is already in the queue
func (m *ModemQueueOperations) NodeExistsInQueue(ctx context.Context, zone, net, node uint16, cs uint8) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	query := `
		SELECT count() FROM modem_test_queue
		WHERE zone = ? AND net = ? AND node = ? AND conflict_sequence = ?
	`

	var count uint64
	err := m.db.Conn().QueryRowContext(ctx, query, zone, net, node, cs).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check queue: %w", err)
	}

	return count > 0, nil
}

// InsertQueueEntry inserts a new entry into the modem test queue
func (m *ModemQueueOperations) InsertQueueEntry(ctx context.Context, entry *ModemQueueEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	query := `
		INSERT INTO modem_test_queue (
			zone, net, node, conflict_sequence, phone, phone_normalized,
			modem_flags, flags, is_cm, time_flags,
			assigned_to, assigned_at, priority, retry_count, next_attempt_after,
			status, in_progress_since, last_tested_at, last_error, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := m.db.Conn().ExecContext(ctx, query,
		entry.Zone, entry.Net, entry.Node, entry.ConflictSequence,
		entry.Phone, entry.PhoneNormalized,
		entry.ModemFlags, entry.Flags, entry.IsCM, entry.TimeFlags,
		entry.AssignedTo, now, entry.Priority, 0, now,
		ModemQueueStatusPending, time.Time{}, time.Time{}, "", now, now)
	return err
}

// UpdateNodeAssignment updates the assigned daemon for an existing queue entry
func (m *ModemQueueOperations) UpdateNodeAssignment(ctx context.Context, zone, net, node uint16, cs uint8, callerID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	query := `
		ALTER TABLE modem_test_queue
		UPDATE assigned_to = ?, assigned_at = ?, updated_at = ?
		WHERE zone = ? AND net = ? AND node = ? AND conflict_sequence = ?
	`
	_, err := m.db.Conn().ExecContext(ctx, query, callerID, now, now, zone, net, node, cs)
	return err
}

// GetStaleInProgressNodes returns nodes stuck in in_progress status
func (m *ModemQueueOperations) GetStaleInProgressNodes(ctx context.Context, threshold time.Duration) ([]ModemQueueEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	thresholdSecs := int(threshold.Seconds())
	query := `
		SELECT zone, net, node, conflict_sequence, phone, phone_normalized,
			   modem_flags, flags, is_cm, time_flags, assigned_to, assigned_at,
			   priority, retry_count, next_attempt_after, status,
			   in_progress_since, last_tested_at, last_error, created_at, updated_at
		FROM modem_test_queue
		WHERE status = 'in_progress'
		  AND in_progress_since < now() - INTERVAL ? SECOND
	`

	rows, err := m.db.Conn().QueryContext(ctx, query, thresholdSecs)
	if err != nil {
		return nil, fmt.Errorf("failed to query stale nodes: %w", err)
	}
	defer rows.Close()

	return m.scanQueueEntries(rows)
}

// GetOrphanedNodes returns nodes with empty assigned_to
func (m *ModemQueueOperations) GetOrphanedNodes(ctx context.Context) ([]ModemQueueEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	query := `
		SELECT zone, net, node, conflict_sequence, phone, phone_normalized,
			   modem_flags, flags, is_cm, time_flags, assigned_to, assigned_at,
			   priority, retry_count, next_attempt_after, status,
			   in_progress_since, last_tested_at, last_error, created_at, updated_at
		FROM modem_test_queue
		WHERE assigned_to = ''
		  AND status IN ('pending', 'in_progress')
	`

	rows, err := m.db.Conn().QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query orphaned nodes: %w", err)
	}
	defer rows.Close()

	return m.scanQueueEntries(rows)
}

// GetNodesForOfflineCaller returns nodes assigned to an offline daemon
func (m *ModemQueueOperations) GetNodesForOfflineCaller(ctx context.Context, callerID string) ([]ModemQueueEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	query := `
		SELECT zone, net, node, conflict_sequence, phone, phone_normalized,
			   modem_flags, flags, is_cm, time_flags, assigned_to, assigned_at,
			   priority, retry_count, next_attempt_after, status,
			   in_progress_since, last_tested_at, last_error, created_at, updated_at
		FROM modem_test_queue
		WHERE assigned_to = ?
		  AND status IN ('pending', 'in_progress')
	`

	rows, err := m.db.Conn().QueryContext(ctx, query, callerID)
	if err != nil {
		return nil, fmt.Errorf("failed to query nodes for offline caller: %w", err)
	}
	defer rows.Close()

	return m.scanQueueEntries(rows)
}

// ResetNodeStatus resets a node back to pending status (internal use for orphan recovery)
func (m *ModemQueueOperations) ResetNodeStatus(ctx context.Context, zone, net, node uint16, cs uint8) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	query := `
		ALTER TABLE modem_test_queue
		UPDATE status = 'pending', in_progress_since = toDateTime(0), updated_at = ?
		WHERE zone = ? AND net = ? AND node = ? AND conflict_sequence = ?
	`
	_, err := m.db.Conn().ExecContext(ctx, query, now, zone, net, node, cs)
	return err
}

// ReleaseNode releases a node owned by the caller, clearing assignment for reassignment
// Only affects nodes assigned to the caller in pending or in_progress status
func (m *ModemQueueOperations) ReleaseNode(ctx context.Context, callerID string, zone, net, node uint16, cs uint8) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	query := `
		ALTER TABLE modem_test_queue
		UPDATE status = 'pending',
			   assigned_to = '',
			   in_progress_since = toDateTime(0),
			   next_attempt_after = now(),
			   updated_at = ?
		WHERE zone = ? AND net = ? AND node = ? AND conflict_sequence = ?
		  AND assigned_to = ?
		  AND status IN ('pending', 'in_progress')
	`
	_, err := m.db.Conn().ExecContext(ctx, query, now, zone, net, node, cs, callerID)
	return err
}

// RequeueStaleNode requeues a stale node with incremented retry count
func (m *ModemQueueOperations) RequeueStaleNode(ctx context.Context, zone, net, node uint16, cs uint8) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	query := `
		ALTER TABLE modem_test_queue
		UPDATE status = 'pending',
			   in_progress_since = toDateTime(0),
			   retry_count = retry_count + 1,
			   next_attempt_after = now() + INTERVAL 5 MINUTE,
			   last_error = 'stale: reclaimed after timeout',
			   updated_at = ?
		WHERE zone = ? AND net = ? AND node = ? AND conflict_sequence = ?
	`
	_, err := m.db.Conn().ExecContext(ctx, query, now, zone, net, node, cs)
	return err
}

// GetQueueStats returns statistics about the modem test queue
func (m *ModemQueueOperations) GetQueueStats(ctx context.Context) (*ModemQueueStats, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := &ModemQueueStats{
		ByDaemon: make(map[string]int),
	}

	// Get counts by status
	statusQuery := `
		SELECT status, count() as cnt
		FROM modem_test_queue
		GROUP BY status
	`
	rows, err := m.db.Conn().QueryContext(ctx, statusQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to query status counts: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var status string
		var count uint64
		if err := rows.Scan(&status, &count); err != nil {
			return nil, fmt.Errorf("failed to scan status count: %w", err)
		}
		switch status {
		case ModemQueueStatusPending:
			stats.PendingNodes = int(count)
		case ModemQueueStatusInProgress:
			stats.InProgressNodes = int(count)
		case ModemQueueStatusCompleted:
			stats.CompletedNodes = int(count)
		case ModemQueueStatusFailed:
			stats.FailedNodes = int(count)
		}
		stats.TotalNodes += int(count)
	}

	// Get counts by daemon
	daemonQuery := `
		SELECT assigned_to, count() as cnt
		FROM modem_test_queue
		WHERE assigned_to != ''
		GROUP BY assigned_to
	`
	rows2, err := m.db.Conn().QueryContext(ctx, daemonQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to query daemon counts: %w", err)
	}
	defer rows2.Close()

	for rows2.Next() {
		var daemon string
		var count uint64
		if err := rows2.Scan(&daemon, &count); err != nil {
			return nil, fmt.Errorf("failed to scan daemon count: %w", err)
		}
		stats.ByDaemon[daemon] = int(count)
	}

	return stats, nil
}

// scanQueueEntries scans rows into ModemQueueEntry slice
func (m *ModemQueueOperations) scanQueueEntries(rows *sql.Rows) ([]ModemQueueEntry, error) {
	var entries []ModemQueueEntry

	for rows.Next() {
		var entry ModemQueueEntry
		err := rows.Scan(
			&entry.Zone, &entry.Net, &entry.Node, &entry.ConflictSequence,
			&entry.Phone, &entry.PhoneNormalized,
			&entry.ModemFlags, &entry.Flags, &entry.IsCM, &entry.TimeFlags,
			&entry.AssignedTo, &entry.AssignedAt,
			&entry.Priority, &entry.RetryCount, &entry.NextAttemptAfter,
			&entry.Status, &entry.InProgressSince, &entry.LastTestedAt,
			&entry.LastError, &entry.CreatedAt, &entry.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan queue entry: %w", err)
		}
		entries = append(entries, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return entries, nil
}

// isCallableNow checks if a node is callable at the current time
func isCallableNow(isCM bool, timeFlags []string, zone uint16, now time.Time) bool {
	// CM nodes are always callable
	if isCM {
		return true
	}

	// If no time flags, assume node follows ZMH - caller should check zone-specific hours
	// For simplicity, default to callable (daemon can filter further)
	if len(timeFlags) == 0 {
		return true
	}

	// Parse flags and compute availability using timeavail package
	avail, err := timeavail.ParseAvailability(timeFlags, int(zone), "")
	if err != nil {
		// If parsing fails, assume callable
		return true
	}
	return avail.IsCallableNow(now)
}

// getNextCallWindow returns the next time window when the node will be callable
// Returns nil - detailed window calculation is deferred to daemon polling
func getNextCallWindow(timeFlags []string, zone uint16, now time.Time) *CallWindow {
	// The timeavail package doesn't expose NextWindow on NodeAvailability
	// For now, return nil - daemon will poll and re-check callable status
	// A future enhancement could compute exact windows from parsed T-flags
	_ = timeFlags
	_ = zone
	_ = now
	return nil
}
