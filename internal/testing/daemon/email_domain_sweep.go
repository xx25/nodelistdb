package daemon

import (
	"context"
	"sync"
	"time"

	"github.com/nodelistdb/internal/emailflags"
	"github.com/nodelistdb/internal/logging"
	"github.com/nodelistdb/internal/testing/services"
	"github.com/nodelistdb/internal/testing/storage"
)

// emailDomainStore is the slice of storage the sweep needs.
type emailDomainStore interface {
	GetEmailDomainsToCheck(ctx context.Context, staleAfter time.Duration) ([]string, error)
	GetEmailDomainCheck(ctx context.Context, domain string) (*storage.StoredEmailDomainCheck, error)
	StoreEmailDomainCheck(ctx context.Context, result emailflags.DomainResult, previous *storage.StoredEmailDomainCheck) error
}

// EmailDomainSweeper periodically verifies the mail domains published in
// nodelist email flags.
//
// It is deliberately much simpler than the WHOIS worker. That one carries a
// queue, a one-request-per-second limiter and singleflight because WHOIS
// registries rate-limit and blacklist. This talks to a recursive DNS resolver
// about a few dozen domains, so a bounded periodic sweep is enough. What it
// does keep from the WHOIS design is the rule that a transient failure must
// never overwrite a good stored verdict.
type EmailDomainSweeper struct {
	resolver *services.EmailDomainResolver
	store    emailDomainStore

	interval   time.Duration
	staleAfter time.Duration

	// concurrency bounds how many domains are in flight at once.
	concurrency int

	stopOnce sync.Once
	done     chan struct{}
}

// NewEmailDomainSweeper builds a sweeper from config.
func NewEmailDomainSweeper(cfg EmailVerifyConfig, store emailDomainStore) *EmailDomainSweeper {
	if cfg.Interval <= 0 {
		cfg.Interval = 24 * time.Hour
	}
	if cfg.StaleAfter <= 0 {
		cfg.StaleAfter = 7 * 24 * time.Hour
	}
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 4
	}

	return &EmailDomainSweeper{
		resolver: services.NewEmailDomainResolver(services.EmailDomainResolverConfig{
			Timeout:     cfg.Timeout,
			Concurrency: cfg.Concurrency,
			// The persistent table is the real cache; keep the in-memory one
			// short so a sweep started soon after a config reload still works.
			CacheTTL: time.Hour,
		}),
		store:       store,
		interval:    cfg.Interval,
		staleAfter:  cfg.StaleAfter,
		concurrency: cfg.Concurrency,
		done:        make(chan struct{}),
	}
}

// Start runs a sweep immediately and then on every interval, until the context
// is cancelled or Stop is called.
func (s *EmailDomainSweeper) Start(ctx context.Context) {
	go func() {
		// The first sweep is deferred briefly so daemon startup is not
		// competing with it for the resolver.
		select {
		case <-time.After(30 * time.Second):
		case <-ctx.Done():
			return
		case <-s.done:
			return
		}

		if err := s.Sweep(ctx); err != nil {
			logging.Errorf("Email domain sweep failed: %v", err)
		}

		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-s.done:
				return
			case <-ticker.C:
				if err := s.Sweep(ctx); err != nil {
					logging.Errorf("Email domain sweep failed: %v", err)
				}
			}
		}
	}()
}

// Stop ends the sweep loop.
func (s *EmailDomainSweeper) Stop() {
	s.stopOnce.Do(func() { close(s.done) })
}

// Sweep checks every published mail domain whose stored verdict is missing or
// stale, and records the results.
func (s *EmailDomainSweeper) Sweep(ctx context.Context) error {
	domains, err := s.store.GetEmailDomainsToCheck(ctx, s.staleAfter)
	if err != nil {
		return err
	}
	if len(domains) == 0 {
		logging.Debug("Email domain sweep: nothing due for checking")
		return nil
	}

	logging.Infof("Email domain sweep: checking %d mail domain(s)", len(domains))

	var (
		wg        sync.WaitGroup
		sem       = make(chan struct{}, s.concurrency)
		mu        sync.Mutex
		routable  int
		dead      int
		transient int
	)

	for _, domain := range domains {
		select {
		case <-ctx.Done():
			wg.Wait()
			return ctx.Err()
		default:
		}

		wg.Add(1)
		go func(domain string) {
			defer wg.Done()

			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}

			// The previous row is needed so a transient failure can carry the
			// last established verdict forward instead of erasing it.
			previous, readErr := s.store.GetEmailDomainCheck(ctx, domain)
			if readErr != nil {
				logging.Debugf("Email domain sweep: could not read previous verdict for %s: %v", domain, readErr)
				previous = nil
			}

			result := s.resolver.CheckDomain(ctx, domain)

			if !result.Stable() && readErr != nil {
				// Both the lookup and the read of what was there before
				// failed. Writing now would replace a possibly-good stored
				// verdict with this hiccup, and nothing here knows what that
				// verdict was. Leave the row alone and retry next sweep.
				logging.Debugf("Email domain sweep: skipping write for %s, both the lookup and the previous-verdict read failed", domain)
				s.resolver.InvalidateCache(domain)
				mu.Lock()
				transient++
				mu.Unlock()
				return
			}

			if err := s.store.StoreEmailDomainCheck(ctx, result, previous); err != nil {
				logging.Errorf("Email domain sweep: failed to store verdict for %s: %v", domain, err)
				// Drop the memoised verdict so the next sweep really re-resolves
				// rather than replaying a success that was never persisted.
				s.resolver.InvalidateCache(domain)
				return
			}

			mu.Lock()
			switch {
			case !result.Stable():
				transient++
			case result.Routable():
				routable++
			default:
				dead++
			}
			mu.Unlock()
		}(domain)
	}

	wg.Wait()

	logging.Infof("Email domain sweep complete: %d routable, %d unreachable, %d transient failures",
		routable, dead, transient)
	return nil
}
