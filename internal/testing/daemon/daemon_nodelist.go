package daemon

import (
	"context"

	"github.com/nodelistdb/internal/testing/logging"
)

// checkAndRefreshNodelist checks if any network's nodelist has been updated
// and refreshes if needed. Comparison uses a per-network fingerprint: with
// several networks at different cadences, a global max date would never notice
// an import whose date is older than another network's latest.
func (d *Daemon) checkAndRefreshNodelist(ctx context.Context) bool {
	currentFingerprint, err := d.storage.GetNodelistFingerprint(ctx)
	if err != nil {
		logging.Errorf("Failed to check nodelist fingerprint: %v", err)
		return false
	}

	d.nodelistMu.RLock()
	lastFingerprint := d.lastNodelistFingerprint
	d.nodelistMu.RUnlock()

	if currentFingerprint != lastFingerprint {
		// New nodelist detected in at least one network!
		logging.Infof("New nodelist detected: %s (was %s)", currentFingerprint, lastFingerprint)

		// Refresh the scheduler with new nodes
		// This will automatically schedule immediate retests for nodes with changed internet config
		if err := d.scheduler.RefreshNodes(ctx); err != nil {
			logging.Errorf("Failed to refresh nodes: %v", err)
			return false
		}

		// Update our last known fingerprint
		d.nodelistMu.Lock()
		d.lastNodelistFingerprint = currentFingerprint
		d.nodelistMu.Unlock()

		// Reseed the AKA equivalence index: the refreshed nodelists may have
		// changed hostname sets or added cross-network entries
		d.seedAkaEquivalence(ctx)

		logging.Infof("Nodelist refresh complete. Nodes with changed internet config will be retested immediately.")

		return true
	}

	return false
}
