package services

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/idna"

	"github.com/nodelistdb/internal/emailflags"
)

// MXHost and EmailDomainResult are defined in emailflags so that the storage
// layer, which persists them, does not have to import this package.
type (
	MXHost            = emailflags.MXHost
	EmailDomainResult = emailflags.DomainResult
)

// EmailDomainResolver checks whether the mail domains published in nodelist
// email flags can still receive mail.
//
// Everything it does is plain DNS. Connecting to port 25 would say more, but
// the production host cannot: outbound 25 is blocked by the cloud provider.
// SMTP-level probing (RCPT TO callback verification) is also unreliable
// against catch-all domains and greylisting, and is indistinguishable from
// spammer address harvesting, so it is deliberately not implemented here.
type EmailDomainResolver struct {
	resolver *net.Resolver
	timeout  time.Duration

	// concurrency bounds simultaneous lookups. The population is tiny (a few
	// dozen domains), so this exists to be polite to the local resolver
	// rather than to survive rate limiting.
	sem chan struct{}

	mu    sync.RWMutex
	cache map[string]*emailDomainCacheEntry
	ttl   time.Duration
}

type emailDomainCacheEntry struct {
	result    EmailDomainResult
	expiresAt time.Time
}

// EmailDomainResolverConfig configures the resolver.
type EmailDomainResolverConfig struct {
	// Timeout bounds a single DNS lookup. Defaults to 5s.
	Timeout time.Duration
	// Concurrency bounds simultaneous domain checks. Defaults to 4.
	Concurrency int
	// CacheTTL is how long a stable verdict is reused in memory. Defaults
	// to 6h. Transient failures are cached for a tenth of this.
	CacheTTL time.Duration
}

// NewEmailDomainResolver builds a resolver with sensible defaults.
func NewEmailDomainResolver(cfg EmailDomainResolverConfig) *EmailDomainResolver {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Second
	}
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 4
	}
	if cfg.CacheTTL <= 0 {
		cfg.CacheTTL = 6 * time.Hour
	}

	return &EmailDomainResolver{
		resolver: net.DefaultResolver,
		timeout:  cfg.Timeout,
		sem:      make(chan struct{}, cfg.Concurrency),
		cache:    make(map[string]*emailDomainCacheEntry),
		ttl:      cfg.CacheTTL,
	}
}

// CheckAddress verifies the domain of a single email address.
func (r *EmailDomainResolver) CheckAddress(ctx context.Context, address string) EmailDomainResult {
	domain := emailflags.MailDomain(address)
	if domain == "" {
		return EmailDomainResult{
			Domain:    domain,
			Status:    emailflags.DomainStatusInvalid,
			Detail:    "not a usable email address",
			CheckTime: time.Now(),
		}
	}
	return r.CheckDomain(ctx, domain)
}

// CheckDomain runs the DNS ladder for one mail domain.
func (r *EmailDomainResolver) CheckDomain(ctx context.Context, domain string) EmailDomainResult {
	domain = strings.ToLower(strings.TrimSpace(strings.TrimSuffix(domain, ".")))
	if domain == "" {
		return EmailDomainResult{
			Status:    emailflags.DomainStatusInvalid,
			Detail:    "empty domain",
			CheckTime: time.Now(),
		}
	}

	if cached, ok := r.cached(domain); ok {
		return cached
	}

	select {
	case r.sem <- struct{}{}:
		defer func() { <-r.sem }()
	case <-ctx.Done():
		return EmailDomainResult{
			Domain:    domain,
			Status:    emailflags.DomainStatusError,
			Detail:    "cancelled before lookup",
			Error:     ctx.Err().Error(),
			CheckTime: time.Now(),
		}
	}

	result := r.check(ctx, domain)
	r.store(domain, result)
	return result
}

func (r *EmailDomainResolver) check(ctx context.Context, domain string) EmailDomainResult {
	result := EmailDomainResult{Domain: domain, CheckTime: time.Now()}

	// A non-ASCII domain must be punycoded before it is queried; the Go
	// resolver rejects non-ASCII names outright, which would otherwise be
	// misreported as a dead domain.
	ascii, err := idna.Lookup.ToASCII(domain)
	if err != nil {
		result.Status = emailflags.DomainStatusInvalid
		// The reason goes in Detail, not Error: this is a settled verdict, and
		// a non-empty Error is what marks a stored verdict as stale.
		result.Detail = "not a valid domain name: " + err.Error()
		return result
	}
	if ascii != domain {
		result.ASCIIDomain = ascii
	}

	lookupCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	records, err := r.resolver.LookupMX(lookupCtx, fqdn(ascii))
	if err != nil {
		var dnsErr *net.DNSError
		if errors.As(err, &dnsErr) && !dnsErr.IsNotFound {
			// SERVFAIL, timeout, or a resolver problem. This says nothing
			// about the domain, so it must never overwrite a good verdict.
			result.Status = emailflags.DomainStatusError
			result.Detail = "DNS lookup failed"
			result.Error = err.Error()
			return result
		}

		// IsNotFound covers both NXDOMAIN and "domain exists but has no MX".
		// Go collapses the two, so an address lookup decides between them:
		// if the name resolves, mail routes there implicitly (RFC 5321
		// section 5.1); if it does not, there is nowhere to deliver.
		return r.checkImplicitMX(ctx, result, ascii)
	}

	// RFC 7505: a single MX of preference 0 pointing at the root is an
	// explicit declaration that the domain accepts no mail.
	if isNullMX(records) {
		result.Status = emailflags.DomainStatusNullMX
		result.Detail = "domain publishes a null MX (RFC 7505) and accepts no mail"
		return result
	}

	hosts := make([]MXHost, 0, len(records))
	for _, rec := range records {
		host := strings.TrimSuffix(rec.Host, ".")
		if host == "" || host == "." {
			continue
		}
		hosts = append(hosts, MXHost{Preference: rec.Pref, Host: host})
	}
	if len(hosts) == 0 {
		return r.checkImplicitMX(ctx, result, ascii)
	}

	sort.SliceStable(hosts, func(i, j int) bool { return hosts[i].Preference < hosts[j].Preference })

	// Every MX is resolved, not just the primary: a dead primary with a live
	// backup still routes mail, and reporting only the primary would call a
	// working domain dead.
	resolved := 0
	var placeholder []string
	// unknown holds hosts whose lookup failed transiently. Such a host is not
	// evidence of anything, but it is also not a reason to abandon the whole
	// domain: a dead backup MX is common (owlserver.de publishes two on
	// dynamic-DNS providers that no longer exist), and giving up at the first
	// one would discard a primary that already resolved and report a perfectly
	// deliverable domain as unchecked.
	var unknown []string
	for i := range hosts {
		count, err := r.resolveHost(ctx, hosts[i].Host)
		if err != nil {
			var dnsErr *net.DNSError
			if errors.As(err, &dnsErr) && !dnsErr.IsNotFound {
				unknown = append(unknown, hosts[i].Host)
				if result.Error == "" {
					result.Error = err.Error()
				}
			}
		}
		hosts[i].Addresses = count
		hosts[i].Resolved = count > 0
		if count > 0 {
			resolved++
		}
		if strings.HasSuffix(hosts[i].Host, ".invalid") {
			placeholder = append(placeholder, hosts[i].Host)
		}
	}
	result.MXHosts = hosts

	// One working exchanger is enough to deliver, so the verdict follows what
	// resolved. Only when nothing resolved does an unknown host matter: then
	// the lookups genuinely failed to establish anything and the domain must
	// stay retryable rather than be recorded as dead.
	switch {
	case resolved > 0:
		if resolved == len(hosts) {
			result.Status = emailflags.DomainStatusOK
			result.Detail = fmt.Sprintf("%d MX host(s), all resolving", len(hosts))
		} else {
			result.Status = emailflags.DomainStatusDegraded
			result.Detail = fmt.Sprintf("%d of %d MX host(s) resolve", resolved, len(hosts))
			if len(unknown) > 0 {
				result.Detail += fmt.Sprintf("; %s did not answer", strings.Join(unknown, ", "))
			}
		}
		// The domain is deliverable, so a failed backup lookup is detail, not
		// a reason to mark the stored verdict stale.
		result.Error = ""
	case len(unknown) > 0:
		result.Status = emailflags.DomainStatusError
		result.Detail = fmt.Sprintf("no MX host resolved and %d of %d did not answer", len(unknown), len(hosts))
	default:
		result.Status = emailflags.DomainStatusMXUnresolvable
		result.Detail = fmt.Sprintf("all %d MX host(s) fail to resolve", len(hosts))
	}

	// A .invalid target is the placeholder some hosted-mail providers leave
	// behind when a domain was added but never verified. Mail to it bounces.
	if len(placeholder) > 0 {
		result.Detail += fmt.Sprintf("; placeholder target %s suggests an unverified hosted-mail domain", strings.Join(placeholder, ", "))
	}

	return result
}

// checkImplicitMX decides between "no MX but the name resolves" and "nothing
// there at all".
func (r *EmailDomainResolver) checkImplicitMX(ctx context.Context, result EmailDomainResult, ascii string) EmailDomainResult {
	count, err := r.resolveHost(ctx, ascii)
	if err != nil {
		var dnsErr *net.DNSError
		if errors.As(err, &dnsErr) && !dnsErr.IsNotFound {
			result.Status = emailflags.DomainStatusError
			result.Detail = "DNS lookup failed"
			result.Error = err.Error()
			return result
		}
		result.Status = emailflags.DomainStatusNoDomain
		result.Detail = "domain does not resolve and publishes no MX"
		return result
	}

	if count > 0 {
		result.Status = emailflags.DomainStatusImplicitMX
		result.Detail = "no MX published; mail routes to the address record (RFC 5321)"
		return result
	}

	result.Status = emailflags.DomainStatusNoMX
	result.Detail = "domain has neither an MX nor an address record"
	return result
}

func (r *EmailDomainResolver) resolveHost(ctx context.Context, host string) (int, error) {
	lookupCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	addrs, err := r.resolver.LookupIPAddr(lookupCtx, fqdn(host))
	if err != nil {
		return 0, err
	}
	return len(addrs), nil
}

// fqdn roots a name so the resolver treats it as absolute.
//
// Without the trailing dot the stdlib applies the search suffixes from
// /etc/resolv.conf, so on a host with a "search" directive a genuinely dead
// public domain can resolve as "dead.example.<suffix>" and be reported
// routable. Every published mail domain is by definition absolute.
func fqdn(name string) string {
	if name == "" || strings.HasSuffix(name, ".") {
		return name
	}
	return name + "."
}

// isNullMX reports the RFC 7505 null MX pattern.
func isNullMX(records []*net.MX) bool {
	if len(records) != 1 {
		return false
	}
	host := strings.TrimSpace(records[0].Host)
	return records[0].Pref == 0 && (host == "." || host == "")
}

func (r *EmailDomainResolver) cached(domain string) (EmailDomainResult, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, ok := r.cache[domain]
	if !ok || time.Now().After(entry.expiresAt) {
		return EmailDomainResult{}, false
	}
	return entry.result, true
}

func (r *EmailDomainResolver) store(domain string, result EmailDomainResult) {
	ttl := r.ttl
	if !result.Stable() {
		// A transient failure is retried much sooner than a settled fact.
		ttl = r.ttl / 10
		if ttl < time.Minute {
			ttl = time.Minute
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.cache[domain] = &emailDomainCacheEntry{result: result, expiresAt: time.Now().Add(ttl)}
}

// InvalidateCache drops a domain's memoised verdict, so a retry after a failed
// write actually re-resolves instead of returning the cached success.
func (r *EmailDomainResolver) InvalidateCache(domain string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.cache, strings.ToLower(domain))
}
