package modem

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/nodelistdb/internal/config"
	"github.com/nodelistdb/internal/logging"
	"github.com/nodelistdb/internal/storage"
)

// ModemAssigner handles node-to-daemon assignment based on prefix configuration
type ModemAssigner struct {
	config       *config.ModemAPIConfig
	queueStorage *storage.ModemQueueOperations
	callerStatus *storage.ModemCallerOperations
}

// NewModemAssigner creates a new ModemAssigner instance
func NewModemAssigner(
	cfg *config.ModemAPIConfig,
	queueStorage *storage.ModemQueueOperations,
	callerStatus *storage.ModemCallerOperations,
) *ModemAssigner {
	return &ModemAssigner{
		config:       cfg,
		queueStorage: queueStorage,
		callerStatus: callerStatus,
	}
}

// AssignModemTestNode assigns a node to the appropriate daemon based on prefix matching
func (a *ModemAssigner) AssignModemTestNode(ctx context.Context, entry *storage.ModemQueueEntry) error {
	// Check if node already exists in queue
	exists, err := a.queueStorage.NodeExistsInQueue(ctx, entry.Zone, entry.Net, entry.Node, entry.ConflictSequence)
	if err != nil {
		return fmt.Errorf("failed to check queue: %w", err)
	}
	if exists {
		return nil // Already queued
	}

	// Get callers from config sorted by priority
	callers := a.getSortedCallers()

	phoneNorm := NormalizePhone(entry.Phone)
	if phoneNorm == "" {
		return fmt.Errorf("invalid phone number: %s", entry.Phone)
	}

	for _, caller := range callers {
		if matchesPrefix(caller, phoneNorm) {
			// Check if daemon is active
			if !a.isDaemonActive(ctx, caller.CallerID) {
				continue // Skip inactive daemons
			}

			entry.AssignedTo = caller.CallerID
			entry.PhoneNormalized = phoneNorm
			return a.queueStorage.InsertQueueEntry(ctx, entry)
		}
	}

	return fmt.Errorf("no daemon available for phone prefix: %s", ExtractCountryCode(phoneNorm))
}

// ReassignNode updates an existing queue entry to a new daemon
func (a *ModemAssigner) ReassignNode(ctx context.Context, entry *storage.ModemQueueEntry) error {
	phoneNorm := NormalizePhone(entry.Phone)
	if phoneNorm == "" {
		return fmt.Errorf("invalid phone number: %s", entry.Phone)
	}

	callers := a.getSortedCallers()

	for _, caller := range callers {
		if matchesPrefix(caller, phoneNorm) {
			if !a.isDaemonActive(ctx, caller.CallerID) {
				continue
			}

			return a.queueStorage.UpdateNodeAssignment(ctx,
				entry.Zone, entry.Net, entry.Node, entry.ConflictSequence,
				caller.CallerID)
		}
	}

	return fmt.Errorf("no daemon available for phone prefix: %s", ExtractCountryCode(phoneNorm))
}

// ReassignOrphanedNodes reassigns nodes from offline daemons to active ones
func (a *ModemAssigner) ReassignOrphanedNodes(ctx context.Context) error {
	for _, caller := range a.config.Callers {
		// Check if daemon is offline
		active, err := a.callerStatus.IsCallerActive(ctx, caller.CallerID, a.config.OfflineThreshold)
		if err != nil {
			logging.Warn("failed to check caller status",
				"caller_id", caller.CallerID, "error", err)
			continue
		}

		if active {
			continue // Daemon is online
		}

		// Get nodes from offline daemon
		nodes, err := a.queueStorage.GetNodesForOfflineCaller(ctx, caller.CallerID)
		if err != nil {
			logging.Warn("failed to get nodes for offline caller",
				"caller_id", caller.CallerID, "error", err)
			continue
		}

		for _, node := range nodes {
			// Reset status to pending
			if err := a.queueStorage.ResetNodeStatus(ctx, node.Zone, node.Net, node.Node, node.ConflictSequence); err != nil {
				logging.Warn("failed to reset node status",
					"node", node.Address(), "error", err)
				continue
			}

			// Reassign to another daemon
			if err := a.ReassignNode(ctx, &node); err != nil {
				logging.Warn("failed to reassign node",
					"node", node.Address(), "error", err)
			}
		}

		// Update caller status to inactive
		if err := a.callerStatus.SetCallerStatus(ctx, caller.CallerID, storage.ModemCallerStatusInactive); err != nil {
			logging.Warn("failed to update caller status",
				"caller_id", caller.CallerID, "error", err)
		}
	}

	return nil
}

// ReclaimStaleNodes reclaims nodes stuck in in_progress status
func (a *ModemAssigner) ReclaimStaleNodes(ctx context.Context) error {
	staleNodes, err := a.queueStorage.GetStaleInProgressNodes(ctx, a.config.StaleInProgressThreshold)
	if err != nil {
		return fmt.Errorf("failed to get stale nodes: %w", err)
	}

	for _, node := range staleNodes {
		logging.Warn("reclaiming stale node",
			"node", node.Address(),
			"assigned_to", node.AssignedTo,
			"in_progress_since", node.InProgressSince)

		if err := a.queueStorage.RequeueStaleNode(ctx, node.Zone, node.Net, node.Node, node.ConflictSequence); err != nil {
			logging.Warn("failed to requeue stale node",
				"node", node.Address(), "error", err)
		}
	}

	return nil
}

// RecoverOrphanedNodes recovers nodes with empty assigned_to
func (a *ModemAssigner) RecoverOrphanedNodes(ctx context.Context) error {
	orphanedNodes, err := a.queueStorage.GetOrphanedNodes(ctx)
	if err != nil {
		return fmt.Errorf("failed to get orphaned nodes: %w", err)
	}

	for _, node := range orphanedNodes {
		logging.Info("recovering orphaned node",
			"node", node.Address(),
			"old_status", node.Status)

		// Reset status to pending
		if err := a.queueStorage.ResetNodeStatus(ctx, node.Zone, node.Net, node.Node, node.ConflictSequence); err != nil {
			logging.Warn("failed to reset orphaned node status",
				"node", node.Address(), "error", err)
			continue
		}

		// Reassign
		if err := a.ReassignNode(ctx, &node); err != nil {
			logging.Warn("failed to recover orphaned node",
				"node", node.Address(), "error", err)
		}
	}

	return nil
}

// OnDaemonHeartbeat is called when a daemon sends a heartbeat
func (a *ModemAssigner) OnDaemonHeartbeat(ctx context.Context, callerID string) error {
	// Check if daemon has any assigned nodes
	count, err := a.queueStorage.GetPendingCount(ctx, callerID)
	if err != nil {
		return fmt.Errorf("failed to get pending count: %w", err)
	}

	if count > 0 {
		return nil // Already has work
	}

	// This daemon has no work - new nodes will be assigned by queue population job
	return nil
}

// getSortedCallers returns callers sorted by priority (highest first)
func (a *ModemAssigner) getSortedCallers() []config.ModemCallerConfig {
	callers := make([]config.ModemCallerConfig, len(a.config.Callers))
	copy(callers, a.config.Callers)

	sort.Slice(callers, func(i, j int) bool {
		return callers[i].Priority > callers[j].Priority
	})

	return callers
}

// isDaemonActive checks if a daemon has a recent heartbeat
func (a *ModemAssigner) isDaemonActive(ctx context.Context, callerID string) bool {
	status, err := a.callerStatus.GetCallerStatus(ctx, callerID)
	if err != nil || status == nil {
		return false
	}
	return time.Since(status.LastHeartbeat) < a.config.OfflineThreshold
}

// matchesPrefix checks if a caller config can handle a phone number
func matchesPrefix(caller config.ModemCallerConfig, phoneNorm string) bool {
	switch caller.PrefixMode {
	case "all":
		return true

	case "include":
		for _, prefix := range caller.Prefixes {
			if HasPhonePrefix(phoneNorm, prefix) {
				return true
			}
		}
		return false

	case "exclude":
		for _, prefix := range caller.Prefixes {
			if HasPhonePrefix(phoneNorm, prefix) {
				return false
			}
		}
		return true
	}
	return false
}
