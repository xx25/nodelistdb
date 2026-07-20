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

func (c *whoisReadCache) GetWithContext(ctx context.Context, domain string) (*models.WhoisResult, error) {
	return c.storage.GetRecentWhoisResult(ctx, domain, 24*time.Hour)
}

// WhoisWorker processes WHOIS lookups in a background goroutine,
// decoupled from the test pipeline to avoid blocking test workers.
type WhoisWorker struct {
	resolver *services.WhoisResolver
	storage  storage.Storage
	queue    chan string
	seen     sync.Map // domain → time.Time (last successful lookup), for dedup with TTL
	seenTTL  time.Duration
	stopped  atomic.Bool
	stopOnce sync.Once
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
// Safe to call multiple times (idempotent).
func (w *WhoisWorker) Stop() {
	w.stopOnce.Do(func() {
		w.stopped.Store(true)
		close(w.queue)
	})
	w.wg.Wait()
}

// Enqueue adds a domain to the WHOIS lookup queue.
// Non-blocking: if queue is full, stopped, or domain was recently looked up, it's silently skipped.
// Safe to call concurrently with Stop() — uses recover to handle send-on-closed-channel.
func (w *WhoisWorker) Enqueue(domain string) {
	if domain == "" || w.stopped.Load() {
		return
	}

	// Dedup with TTL: skip if successfully looked up within seenTTL
	if val, loaded := w.seen.Load(domain); loaded {
		if seenAt, ok := val.(time.Time); ok && time.Since(seenAt) < w.seenTTL {
			return
		}
	}

	// Non-blocking send with recover to handle race between Enqueue and Stop/close.
	// The atomic check above catches most cases; recover handles the narrow window.
	defer func() {
		_ = recover() // Channel was closed between stopped check and send — safe to ignore
	}()

	select {
	case w.queue <- domain:
		// domain enqueued; seen will be marked after successful lookup in processQueue
	default:
		logging.Debugf("WHOIS queue full, skipping domain %s", domain)
	}
}

// processQueue reads domains from the queue and performs WHOIS lookups
func (w *WhoisWorker) processQueue(ctx context.Context) {
	defer w.wg.Done()

	// Periodic cleanup of stale seen entries to prevent unbounded growth
	cleanupTicker := time.NewTicker(1 * time.Hour)
	defer cleanupTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-cleanupTicker.C:
			w.cleanupSeen()

		case domain, ok := <-w.queue:
			if !ok {
				return // queue closed
			}

			result := w.resolver.Resolve(ctx, domain)

			shouldPersist, shouldMarkSeen := classifyWhoisResult(result)

			if result.Cached {
				// Already resolved recently (persistent or in-memory cache) — don't
				// re-store, but honor the same seen rules so stable results keep
				// suppressing retries and cached transient errors stay retryable.
				if shouldMarkSeen {
					w.seen.Store(domain, time.Now())
				}
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

			if shouldPersist {
				if err := w.storage.StoreWhoisResult(ctx, result); err != nil {
					logging.Errorf("Failed to store WHOIS result for %s: %v", domain, err)
					// The write failed — allow a retry rather than marking seen, and
					// drop the resolver's in-memory cache entry so the retry actually
					// re-resolves and re-persists instead of returning the cached hit.
					shouldMarkSeen = false
					w.resolver.InvalidateCache(domain)
				}
			}

			if shouldMarkSeen {
				w.seen.Store(domain, time.Now())
			}
		}
	}
}

// classifyWhoisResult decides whether a WHOIS/RDAP result should be persisted to
// ClickHouse and/or marked "seen" (suppressing retries for seenTTL). The persist
// gate protects the ReplacingMergeTree cache from overwriting good rows with junk:
//   - success WITH usable data (registrar, expiry, or status) → persist + seen
//   - success WITHOUT usable data (empty stub, e.g. DENIC)     → seen only
//   - "domain not found" (stable)                             → persist + seen
//   - "no whois server" (RDAP-only TLD, no data)              → seen only
//   - transient errors                                        → neither (retry)
func classifyWhoisResult(result *models.WhoisResult) (persist, markSeen bool) {
	switch result.Error {
	case "":
		return result.HasUsableData(), true
	case "domain not found":
		return true, true
	case models.WhoisNoServerError:
		return false, true
	default:
		return false, false // transient — retry next cycle
	}
}

// cleanupSeen removes entries older than seenTTL from the seen map
func (w *WhoisWorker) cleanupSeen() {
	now := time.Now()
	w.seen.Range(func(key, value any) bool {
		if seenAt, ok := value.(time.Time); ok {
			if now.Sub(seenAt) >= w.seenTTL {
				w.seen.Delete(key)
			}
		}
		return true
	})
}
