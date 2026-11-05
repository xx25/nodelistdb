package ratelimit

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nodelistdb/internal/logging"
)

const (
	// gateChunk is the most the gate hands to the socket between two waits.
	gateChunk = 16 * 1024

	// gateQueueWait is how long a download may wait for a slot. It is long
	// enough to ride out the file ahead of it and short enough that a caller
	// behind a sweep gets an answer rather than a hung page.
	gateQueueWait = 10 * time.Second

	// gateWriteWindow bounds one write to the client. It replaces the server's
	// WriteTimeout here, which is one deadline for the whole response and would
	// cut a throttled tarball off at a length the throttle itself caused.
	gateWriteWindow = 60 * time.Second

	gateRetryAfter = "30"
)

// GateConfig is the budget of one DownloadGate.
type GateConfig struct {
	Name          string   // for the log
	Prefixes      []string // paths the gate applies to
	BytesPerSec   int64    // shared by every download through the gate
	MaxConcurrent int
}

// DownloadGate holds downloads to a total byte rate and a concurrency cap.
//
// Rate limiting counts requests, so a sweep that stays under the request limit
// can still take the whole archive; on a listener where any number of callers
// look like one, the only limit that means anything is a total. The gate sits
// outside the compressor so what it meters is what leaves the process.
type DownloadGate struct {
	cfg      GateConfig
	slots    chan struct{}
	pace     *bytePacer
	rejected atomic.Uint64
	logged   *Limiter

	// queueWait and writeWindow are the constants above; tests shorten them.
	queueWait   time.Duration
	writeWindow time.Duration
}

// NewDownloadGate builds a gate. The caller validates the limits.
func NewDownloadGate(cfg GateConfig) *DownloadGate {
	return &DownloadGate{
		cfg:         cfg,
		slots:       make(chan struct{}, cfg.MaxConcurrent),
		pace:        &bytePacer{rate: float64(cfg.BytesPerSec)},
		logged:      NewLimiter(1, time.Hour),
		queueWait:   gateQueueWait,
		writeWindow: gateWriteWindow,
	}
}

// Rejected reports how many downloads were turned away for want of a slot.
func (g *DownloadGate) Rejected() uint64 { return g.rejected.Load() }

// Wrap returns next guarded by the gate.
func (g *DownloadGate) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !g.applies(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		rc := http.NewResponseController(w)

		// The server's WriteTimeout has been running since the request was
		// read, and a deadline that has already passed cannot be extended, so
		// it has to come off before the wait for a slot rather than after.
		// The last line of defence is the deferred one: the server flushes its
		// buffer after the handler returns, and with no deadline a client that
		// stopped reading would pin that flush forever.
		if err := setDeadline(rc, time.Time{}); err != nil {
			return
		}
		defer func() { _ = setDeadline(rc, time.Now().Add(g.writeWindow)) }()

		timer := time.NewTimer(g.queueWait)
		defer timer.Stop()
		select {
		case g.slots <- struct{}{}:
			defer func() { <-g.slots }()
		case <-timer.C:
			g.reject(w, r, rc)
			return
		case <-r.Context().Done():
			return
		}

		next.ServeHTTP(&pacedWriter{ResponseWriter: w, rc: rc, r: r, gate: g}, r)
	})
}

func (g *DownloadGate) applies(path string) bool {
	for _, p := range g.cfg.Prefixes {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

func (g *DownloadGate) reject(w http.ResponseWriter, r *http.Request, rc *http.ResponseController) {
	g.rejected.Add(1)
	if noisy, _ := g.logged.Allow(g.cfg.Name, logSampleRate); noisy {
		logging.Warn("download gate full",
			slog.String("listener", g.cfg.Name),
			slog.String("path", r.URL.Path),
			slog.Uint64("rejected_total", g.rejected.Load()))
	}
	if setDeadline(rc, time.Now().Add(g.writeWindow)) != nil {
		return
	}
	w.Header().Set("Retry-After", gateRetryAfter)
	// Per-listener state, so it must not be cached on the way out.
	w.Header().Set("Cache-Control", "no-store")
	http.Error(w, "Too many downloads through this network right now; please retry shortly.", http.StatusServiceUnavailable)
}

// setDeadline treats a writer without deadlines as nothing to do, which is
// every writer in a test recorder and none in production.
func setDeadline(rc *http.ResponseController, t time.Time) error {
	if err := rc.SetWriteDeadline(t); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return err
	}
	return nil
}

// pacedWriter sends a response in chunks, each at the time the pacer gives it.
//
// It deliberately does not implement io.ReaderFrom: that is how http.ServeContent
// would reach sendfile and skip Write altogether.
type pacedWriter struct {
	http.ResponseWriter
	rc   *http.ResponseController
	r    *http.Request
	gate *DownloadGate
}

// Unwrap lets http.ResponseController reach the connection underneath.
func (p *pacedWriter) Unwrap() http.ResponseWriter { return p.ResponseWriter }

// Write sends b. The deadline is armed only around the socket write and is
// clear during the wait, so time spent in the queue for bytes is never billed
// to the client and an expired deadline never has to be revived.
func (p *pacedWriter) Write(b []byte) (int, error) {
	sent := 0
	for len(b) > 0 {
		n := min(len(b), gateChunk)
		if err := p.wait(n); err != nil {
			return sent, err
		}
		m, err := p.bounded(func() (int, error) { return p.ResponseWriter.Write(b[:n]) })
		sent += m
		if err != nil {
			return sent, err
		}
		b = b[n:]
	}
	return sent, nil
}

// Flush implements http.Flusher under the same deadline as Write.
func (p *pacedWriter) Flush() {
	_, _ = p.bounded(func() (int, error) { return 0, p.rc.Flush() })
}

func (p *pacedWriter) bounded(write func() (int, error)) (int, error) {
	if err := setDeadline(p.rc, time.Now().Add(p.gate.writeWindow)); err != nil {
		return 0, err
	}
	n, err := write()
	if err != nil && !errors.Is(err, http.ErrNotSupported) {
		return n, err
	}
	return n, setDeadline(p.rc, time.Time{})
}

func (p *pacedWriter) wait(n int) error {
	delay := p.gate.pace.reserve(n, time.Now())
	if delay <= 0 {
		return p.r.Context().Err()
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-p.r.Context().Done():
		// These bytes will never be sent, so their place in the schedule goes
		// back. Left booked, callers that connect and hang up would slow
		// everyone behind them for bytes nobody received.
		p.gate.pace.refund(n)
		return p.r.Context().Err()
	}
}

// bytePacer spaces chunks out at a fixed rate, first come first served. With at
// most MaxConcurrent writers and one chunk each, the queue stays that short.
// It has no burst on purpose: a burst is exactly what a sweep would spend.
type bytePacer struct {
	mu   sync.Mutex
	rate float64 // bytes per second
	next time.Time
}

// reserve books n bytes and returns how long to wait before sending them.
func (b *bytePacer) reserve(n int, now time.Time) time.Duration {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.next.Before(now) {
		b.next = now
	}
	wait := b.next.Sub(now)
	b.next = b.next.Add(b.cost(n))
	return wait
}

// refund returns a reservation that will not be used. The schedule is one
// timeline rather than a queue, so this shortens it for whoever books next while
// writers already waiting keep their times: cancelling a reservation in the
// middle lets the next booking land on top of a later one. That is a brief
// overlap of one chunk per cancellation. The long-run rate holds, because only
// bytes that were never sent are given back, and a queue that reschedules
// pending writers is not worth it to smooth out 16 KiB.
func (b *bytePacer) refund(n int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.next = b.next.Add(-b.cost(n))
}

func (b *bytePacer) cost(n int) time.Duration {
	return time.Duration(float64(n) / b.rate * float64(time.Second))
}
