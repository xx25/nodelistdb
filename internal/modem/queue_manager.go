package modem

import (
	"context"
	"sync"
	"time"

	"github.com/nodelistdb/internal/config"
	"github.com/nodelistdb/internal/logging"
)

// QueueManager handles background queue management tasks
type QueueManager struct {
	assigner  *ModemAssigner
	populator *QueuePopulator
	config    *config.ModemAPIConfig
	stop      chan struct{}
	wg        sync.WaitGroup
	mu        sync.Mutex
	running   bool
}

// NewQueueManager creates a new QueueManager instance
func NewQueueManager(
	assigner *ModemAssigner,
	populator *QueuePopulator,
	cfg *config.ModemAPIConfig,
) *QueueManager {
	return &QueueManager{
		assigner:  assigner,
		populator: populator,
		config:    cfg,
		stop:      make(chan struct{}),
	}
}

// Start begins the background queue management tasks
func (qm *QueueManager) Start(ctx context.Context) {
	qm.mu.Lock()
	if qm.running {
		qm.mu.Unlock()
		return
	}
	qm.running = true
	qm.mu.Unlock()

	logging.Info("starting modem queue manager",
		"orphan_check_interval", qm.config.OrphanCheckInterval,
		"stale_threshold", qm.config.StaleInProgressThreshold)

	// Start background goroutines
	qm.wg.Add(3)
	go qm.runOrphanCheck(ctx)
	go qm.runStaleReclaim(ctx)
	go qm.runQueuePopulation(ctx)
}

// Stop gracefully stops all background tasks
func (qm *QueueManager) Stop() {
	qm.mu.Lock()
	if !qm.running {
		qm.mu.Unlock()
		return
	}
	qm.running = false
	qm.mu.Unlock()

	logging.Info("stopping modem queue manager")
	close(qm.stop)
	qm.wg.Wait()
	logging.Info("modem queue manager stopped")
}

// runOrphanCheck periodically reassigns nodes from offline daemons
func (qm *QueueManager) runOrphanCheck(ctx context.Context) {
	defer qm.wg.Done()

	ticker := time.NewTicker(qm.config.OrphanCheckInterval)
	defer ticker.Stop()

	// Run initial check
	qm.doOrphanCheck(ctx)

	for {
		select {
		case <-qm.stop:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			qm.doOrphanCheck(ctx)
		}
	}
}

func (qm *QueueManager) doOrphanCheck(ctx context.Context) {
	if err := qm.assigner.ReassignOrphanedNodes(ctx); err != nil {
		logging.Error("orphan check failed", "error", err)
	}
	if err := qm.assigner.RecoverOrphanedNodes(ctx); err != nil {
		logging.Error("recover orphaned nodes failed", "error", err)
	}
}

// runStaleReclaim periodically reclaims nodes stuck in in_progress status
func (qm *QueueManager) runStaleReclaim(ctx context.Context) {
	defer qm.wg.Done()

	// Check more frequently than the stale threshold
	interval := qm.config.StaleInProgressThreshold / 2
	if interval < time.Minute {
		interval = time.Minute
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-qm.stop:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := qm.assigner.ReclaimStaleNodes(ctx); err != nil {
				logging.Error("stale reclaim failed", "error", err)
			}
		}
	}
}

// runQueuePopulation periodically adds new nodes to the queue
func (qm *QueueManager) runQueuePopulation(ctx context.Context) {
	defer qm.wg.Done()

	// Population interval - run less frequently
	interval := 15 * time.Minute
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Run initial population
	qm.doQueuePopulation(ctx)

	for {
		select {
		case <-qm.stop:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			qm.doQueuePopulation(ctx)
		}
	}
}

func (qm *QueueManager) doQueuePopulation(ctx context.Context) {
	if qm.populator == nil {
		return
	}

	added, err := qm.populator.PopulateFromNodelist(ctx)
	if err != nil {
		logging.Error("queue population failed", "error", err)
		return
	}

	if added > 0 {
		logging.Info("populated modem test queue", "nodes_added", added)
	}
}

// ForceOrphanCheck triggers an immediate orphan check
func (qm *QueueManager) ForceOrphanCheck(ctx context.Context) error {
	qm.doOrphanCheck(ctx)
	return nil
}

// ForcePopulation triggers an immediate queue population
func (qm *QueueManager) ForcePopulation(ctx context.Context) (int, error) {
	if qm.populator == nil {
		return 0, nil
	}
	return qm.populator.PopulateFromNodelist(ctx)
}
