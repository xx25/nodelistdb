package daemon

import (
	"context"

	"github.com/nodelistdb/internal/testing/logging"
)

// checkAndRefreshNodelist checks if the nodelist has been updated and refreshes if needed
func (d *Daemon) checkAndRefreshNodelist(ctx context.Context) bool {
	// Get current nodelist date from database
	currentDate, err := d.storage.GetLatestNodelistDate(ctx)
	if err != nil {
		logging.Errorf("Failed to check nodelist date: %v", err)
		return false
	}

	// Check if it's different from our last known date
	d.nodelistMu.RLock()
	lastDate := d.lastNodelistDate
	d.nodelistMu.RUnlock()

	if currentDate.After(lastDate) {
		// New nodelist detected!
		logging.Infof("New nodelist detected: %s (was %s)",
			currentDate.Format("2006-01-02"),
			lastDate.Format("2006-01-02"))

		// Refresh the scheduler with new nodes
		// This will automatically schedule immediate retests for nodes with changed internet config
		if err := d.scheduler.RefreshNodes(ctx); err != nil {
			logging.Errorf("Failed to refresh nodes: %v", err)
			return false
		}

		// Update our last known date
		d.nodelistMu.Lock()
		d.lastNodelistDate = currentDate
		d.nodelistMu.Unlock()

		logging.Infof("Nodelist refresh complete. Nodes with changed internet config will be retested immediately.")

		return true
	}

	return false
}
