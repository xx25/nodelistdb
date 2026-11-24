package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

type flakyModem struct {
	openFailures int
	opens        int
}

func (f *flakyModem) Reset() error      { return errors.New("modem not open") }
func (f *flakyModem) Close() error      { return nil }
func (f *flakyModem) IsUSBDevice() bool { return false }
func (f *flakyModem) USBReset() error   { return nil }
func (f *flakyModem) Open() error {
	f.opens++
	if f.opens <= f.openFailures {
		return errors.New("init command \"ATZ\" failed")
	}
	return nil
}

func TestRecoverModemRetriesUntilOpen(t *testing.T) {
	old := modemRecoverInterval
	modemRecoverInterval = time.Millisecond
	defer func() { modemRecoverInterval = old }()

	m := &flakyModem{openFailures: 2}
	if !recoverModem(context.Background(), m, func(string, ...interface{}) {}) {
		t.Fatal("expected recovery")
	}
	if m.opens != 3 {
		t.Fatalf("opens = %d, want 3", m.opens)
	}
}

func TestRecoverModemStopsOnCancel(t *testing.T) {
	old := modemRecoverInterval
	modemRecoverInterval = time.Hour
	defer func() { modemRecoverInterval = old }()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	m := &flakyModem{openFailures: 1 << 30}
	if recoverModem(ctx, m, func(string, ...interface{}) {}) {
		t.Fatal("expected no recovery after cancel")
	}
}
