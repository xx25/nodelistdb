package protocols

import (
	"context"
	"sync"
	"time"
)

// Pacer spaces the connections one node's test makes. A node with three
// internet protocols on two address families is dialed six times in a row,
// and every one of those is a fresh mailer session on the far side: mbcico
// forks a process per connection and holds the node's .bsy lock until that
// process has finished cleaning up, so a second session that arrives within
// a second of the first is refused with "all AKAs busy" — which we then
// recorded as a failure of the address family that happened to come second.
// (Reported by the sysop of 1:320/219, whose log showed our six connections
// inside eight seconds, 2026-09-09.)
//
// One Pacer travels with the context of a node's test; every dial in this
// package waits on it, so the spacing holds across protocols, address
// families and hostnames alike, and across a tester's own reconnects.
type Pacer struct {
	delay time.Duration

	mu     sync.Mutex
	dialed bool
}

type pacerKey struct{}

// WithPacer returns a context whose dials are spaced by delay. A zero or
// negative delay disables pacing.
func WithPacer(ctx context.Context, delay time.Duration) context.Context {
	if delay <= 0 {
		return ctx
	}
	return context.WithValue(ctx, pacerKey{}, &Pacer{delay: delay})
}

// Pace blocks for the configured delay before the second and every later
// connection made under ctx; the first dial never waits. Every tester closes
// its connection before returning and calls Pace right before its next dial,
// so the full delay always separates a close from the next connect — the
// pacer does not measure the gap, it imposes it, which is deliberately the
// conservative reading. It returns ctx.Err() if the wait was cut short.
func Pace(ctx context.Context) error {
	p, _ := ctx.Value(pacerKey{}).(*Pacer)
	if p == nil {
		return ctx.Err()
	}
	p.mu.Lock()
	first := !p.dialed
	p.dialed = true
	p.mu.Unlock()
	if first {
		return ctx.Err()
	}
	t := time.NewTimer(p.delay)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
