package daemon

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nodelistdb/internal/testing/logging"
	"github.com/nodelistdb/internal/testing/models"
	"github.com/nodelistdb/internal/testing/services"
	"github.com/nodelistdb/internal/testing/storage"
)

// whoisReadCache adapts testdaemon storage to the PersistentWhoisCache interface (read-only).
// Lookups use a 24h freshness window — transient errors stored in ClickHouse are not
// returned since the worker only persists success/"not found" results.
type whoisReadCache struct {
	storage storage.Storage
}

func (c *whoisReadCache) Get(domain string) (*models.WhoisResult, error) {
	return c.storage.GetRecentWhoisResult(context.Background(), domain, 24*time.Hour)
}

// WhoisWorker processes WHOIS lookups in a background goroutine,
// decoupled from the test pipeline to avoid blocking test workers.
type WhoisWorker struct {
	resolver *services.WhoisResolver
	storage  storage.Storage
	queue    chan string
	seen     sync.Map // domain → time.Time (enqueue time), for dedup with TTL
	seenTTL  time.Duration
	stopped  atomic.Bool
	wg       sync.WaitGroup
}

// NewWhoisWorker creates a new WHOIS worker with a buffered queue
func NewWhoisWorker(resolver *services.WhoisResolver, store storage.Storage, queueSize int) *WhoisWorker {
	if queueSize <= 0 {
		queueSize = 1000
	}
	return &WhoisWorker{
		resolver: resolver,
		storage:  store,
		queue:    make(chan string, queueSize),
		seenTTL:  24 * time.Hour,
	}
}

// Start begins processing the WHOIS queue in a background goroutine
func (w *WhoisWorker) Start(ctx context.Context) {
	w.wg.Add(1)
	go w.processQueue(ctx)
}

// Stop signals the worker to stop and waits for it to finish.
// Must be called after all producers (test workers) have stopped.
func (w *WhoisWorker) Stop() {
	w.stopped.Store(true)
	close(w.queue)
	w.wg.Wait()
}

// Enqueue adds a domain to the WHOIS lookup queue.
// Non-blocking: if queue is full, stopped, or domain was recently enqueued, it's silently skipped.
func (w *WhoisWorker) Enqueue(domain string) {
	if domain == "" {
		return
	}

	// Guard against send on closed channel after Stop()
	if w.stopped.Load() {
		return
	}

	now := time.Now()

	// Dedup with TTL: skip if seen within seenTTL
	if val, loaded := w.seen.Load(domain); loaded {
		if seenAt, ok := val.(time.Time); ok && now.Sub(seenAt) < w.seenTTL {
			return
		}
	}

	// Non-blocking send — only mark as seen if successfully enqueued
	select {
	case w.queue <- domain:
		w.seen.Store(domain, now)
	default:
		// Queue full — domain will be retried on next encounter since we didn't mark it as seen
		logging.Debugf("WHOIS queue full, skipping domain %s", domain)
	}
}

// processQueue reads domains from the queue and performs WHOIS lookups
func (w *WhoisWorker) processQueue(ctx context.Context) {
	defer w.wg.Done()

	for domain := range w.queue {
		select {
		case <-ctx.Done():
			return
		default:
		}

		result := w.resolver.Resolve(ctx, domain)
		if result.Cached {
			logging.Debugf("WHOIS cache hit for %s", domain)
			continue
		}

		if result.Error != "" {
			logging.Debugf("WHOIS lookup for %s: %s", domain, result.Error)
		} else {
			expiryStr := "unknown"
			if result.ExpirationDate != nil {
				expiryStr = result.ExpirationDate.Format("2006-01-02")
			}
			logging.Infof("WHOIS for %s: expires %s, registrar %s", domain, expiryStr, result.Registrar)
		}

		// Only persist successful lookups and "domain not found" to ClickHouse.
		// Transient errors (network timeouts, rate limits) should not overwrite
		// previously known good data in ReplacingMergeTree.
		if result.Error == "" || result.Error == "domain not found" {
			if err := w.storage.StoreWhoisResult(ctx, result); err != nil {
				logging.Errorf("Failed to store WHOIS result for %s: %v", domain, err)
			}
		}
	}
}
