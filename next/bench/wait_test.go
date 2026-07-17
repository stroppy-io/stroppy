package bench

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stroppy-io/stroppy/next/driver"
	"github.com/stroppy-io/stroppy/next/driver/noop"
)

// TestWaitReady confirms Wait returns immediately when Ping answers.
func TestWaitReady(t *testing.T) {
	ctx := context.Background()
	p := mustPinger(t)
	if err := Wait(ctx, p, 0, 0); err != nil {
		t.Fatalf("Wait ready: %v", err)
	}
}

// TestWaitTimeout confirms Wait surfaces ErrDBTimeout when Ping never answers.
func TestWaitTimeout(t *testing.T) {
	ctx := context.Background()
	p := failingPinger{}
	if err := Wait(ctx, p, 20*time.Millisecond, time.Millisecond); !errors.Is(err, ErrDBTimeout) {
		t.Fatalf("err = %v, want ErrDBTimeout", err)
	}
}

// TestWaitCanceled confirms Wait unblocks on ctx cancellation.
func TestWaitCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(5 * time.Millisecond)
		cancel()
	}()
	p := failingPinger{}
	if err := Wait(ctx, p, time.Second, 10*time.Millisecond); err == nil {
		t.Fatal("err = nil, want cancellation error")
	}
}

type failingPinger struct{}

func (failingPinger) Ping(context.Context) error { return errors.New("nope") }

func mustPinger(t *testing.T) driver.Pinger {
	t.Helper()
	var d any = noop.New(driver.Spec{})
	if p, ok := d.(driver.Pinger); ok {
		return p
	}
	t.Fatalf("noop driver is not a Pinger")
	return nil
}
