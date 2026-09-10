package protocols

import (
	"context"
	"testing"
	"time"
)

func TestPaceSpacesSecondAndLaterDials(t *testing.T) {
	ctx := WithPacer(context.Background(), 150*time.Millisecond)
	start := time.Now()
	if err := Pace(ctx); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > 50*time.Millisecond {
		t.Fatalf("first dial waited %v; it should not wait", d)
	}
	if err := Pace(ctx); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d < 150*time.Millisecond {
		t.Fatalf("second dial came after %v; want at least the delay", d)
	}
}

func TestPaceWithoutPacerNeverWaits(t *testing.T) {
	start := time.Now()
	for i := 0; i < 3; i++ {
		if err := Pace(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if d := time.Since(start); d > 50*time.Millisecond {
		t.Fatalf("unpaced dials waited %v", d)
	}
}

func TestPaceZeroDelayIsNoPacer(t *testing.T) {
	ctx := WithPacer(context.Background(), 0)
	if _, ok := ctx.Value(pacerKey{}).(*Pacer); ok {
		t.Fatal("zero delay installed a pacer")
	}
}

func TestPaceHonoursCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(WithPacer(context.Background(), time.Hour))
	if err := Pace(ctx); err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	if err := Pace(ctx); err != context.Canceled {
		t.Fatalf("Pace returned %v, want context.Canceled", err)
	}
}
