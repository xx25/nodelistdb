// Package main provides phone coordination for multi-modem testing.
package main

import (
	"context"
	"sync"
	"time"
)

// PhoneCoordinator manages phone number locking across multiple modems.
// It ensures that no two modems attempt to call the same phone number simultaneously.
type PhoneCoordinator struct {
	mu       sync.Mutex
	inUse    map[string]string    // phone -> modem name currently using it
	lastUsed map[string]time.Time // phone -> last time it was released
	cond     *sync.Cond           // for waiters
}

// NewPhoneCoordinator creates a new phone coordinator.
func NewPhoneCoordinator() *PhoneCoordinator {
	c := &PhoneCoordinator{
		inUse:    make(map[string]string),
		lastUsed: make(map[string]time.Time),
	}
	c.cond = sync.NewCond(&c.mu)
	return c
}

// AcquirePhone attempts to lock a phone number for a modem.
// Blocks if the phone is currently in use by another modem.
// Returns true if acquired successfully, false if context was cancelled.
func (c *PhoneCoordinator) AcquirePhone(ctx context.Context, phone, modemName string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Wait until phone is available or context is cancelled
	for {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return false
		default:
		}

		// Check if phone is available
		if _, busy := c.inUse[phone]; !busy {
			// Phone is available - acquire it
			c.inUse[phone] = modemName
			return true
		}

		// Phone is busy - wait for release signal with timeout
		// Release lock, wait, then reacquire
		c.waitWithTimeout(100 * time.Millisecond)
	}
}

// waitWithTimeout waits on the condition variable with a timeout.
// This allows periodic checking of context cancellation.
// Must be called with mutex held. Releases mutex during wait.
func (c *PhoneCoordinator) waitWithTimeout(timeout time.Duration) {
	// Start a timer that will broadcast to wake us up
	timer := time.AfterFunc(timeout, func() {
		c.cond.Broadcast()
	})
	// Wait releases the lock, blocks until signaled, then reacquires lock
	c.cond.Wait()
	// Stop the timer - if we were woken by ReleasePhone, the timer is still pending
	timer.Stop()
}

// ReleasePhone releases a phone number lock.
func (c *PhoneCoordinator) ReleasePhone(phone string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.inUse, phone)
	c.lastUsed[phone] = time.Now()
	c.cond.Broadcast() // Wake up any waiting workers
}

// GetStatus returns the current phone usage status.
// Useful for logging and debugging.
func (c *PhoneCoordinator) GetStatus() map[string]string {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Return a copy to avoid race conditions
	result := make(map[string]string, len(c.inUse))
	for phone, modem := range c.inUse {
		result[phone] = modem
	}
	return result
}

// InUseCount returns the number of phones currently in use.
func (c *PhoneCoordinator) InUseCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.inUse)
}

// IsPhoneInUse checks if a specific phone is currently in use.
func (c *PhoneCoordinator) IsPhoneInUse(phone string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, inUse := c.inUse[phone]
	return inUse
}

// GetPhoneUser returns the modem name currently using a phone, or empty string if not in use.
func (c *PhoneCoordinator) GetPhoneUser(phone string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.inUse[phone]
}
