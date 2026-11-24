package main

import (
	"context"
	"time"

	"github.com/xx25/fidomail/pkg/modem"
)

// modemRecoverInterval is the pause between attempts to bring a modem back.
var modemRecoverInterval = 30 * time.Second

// recoverableModem is the part of a modem needed to bring it back into service.
type recoverableModem interface {
	Reset() error
	Close() error
	Open() error
	IsUSBDevice() bool
	USBReset() error
}

var _ recoverableModem = (*modem.Modem)(nil)

// recoverModem brings a modem back after a local fault (port closed, no reply
// to AT commands). A dial error is such a fault, not a result for the node
// being called, so the caller dials again once this returns true. It tries a
// reset, then closing and reopening the port (with a USB reset for USB
// modems), and keeps trying until the modem
// answers or ctx is cancelled (false).
func recoverModem(ctx context.Context, m recoverableModem, info func(format string, args ...interface{})) bool {
	for attempt := 1; ; attempt++ {
		if err := m.Reset(); err == nil {
			info("Modem recovered by reset (attempt %d)", attempt)
			return true
		}
		_ = m.Close()
		err := m.Open()
		if err != nil && m.IsUSBDevice() {
			if usbErr := m.USBReset(); usbErr == nil {
				info("USB reset done, reopening the port")
				_ = m.Close()
				err = m.Open()
			}
		}
		if err == nil {
			info("Modem recovered by reopening the port (attempt %d)", attempt)
			return true
		}
		info("Modem still unusable (attempt %d): %v; next try in %s", attempt, err, modemRecoverInterval)
		select {
		case <-ctx.Done():
			return false
		case <-time.After(modemRecoverInterval):
		}
	}
}
