package bench

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stroppy-io/stroppy/next/driver"
	"github.com/stroppy-io/stroppy/next/driver/noop"
)

// classifyRetry treats errRetryable as Retry and everything else as Continue —
// the predicate a real driver's Classify provides, narrowed to the test's
// sentinel.
var errRetryable = errors.New("retryable")

func classifyRetry(err error) driver.Action {
	if err == nil {
		return driver.Continue
	}
	if errors.Is(err, errRetryable) {
		return driver.Retry
	}
	return driver.Continue
}

// noopConn returns a pinned noop connection for the helper tests.
func noopConn(t *testing.T) driver.Conn {
	t.Helper()
	c, err := noop.New().Connect(context.Background())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = c.Close(context.Background()) })
	return c
}

// TestTransactionCommit runs fn returning nil and checks the helper commits and
// returns nil, recording the committed outcome.
func TestTransactionCommit(t *testing.T) {
	ctx := context.Background()
	conn := noopConn(t)
	var stats TxStats

	called := 0
	err := Transaction(ctx, conn, classifyRetry, driver.ReadCommitted, RetryOpts{MaxAttempts: 3}, &stats,
		func(tx driver.Tx) error {
			called++
			return nil
		})
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if called != 1 {
		t.Errorf("fn called %d times, want 1", called)
	}
	if stats.Committed != 1 || stats.RolledBack != 0 || stats.Retried != 0 {
		t.Errorf("stats = %+v, want Committed=1", stats)
	}
}

// TestTransactionRollbackTerminal runs fn returning a non-retryable error and
// checks the helper rolls back and surfaces it without replaying.
func TestTransactionRollbackTerminal(t *testing.T) {
	ctx := context.Background()
	conn := noopConn(t)
	var stats TxStats
	errSentinel := errors.New("boom")

	called := 0
	err := Transaction(ctx, conn, classifyRetry, driver.ReadCommitted, RetryOpts{MaxAttempts: 3}, &stats,
		func(tx driver.Tx) error {
			called++
			return errSentinel
		})
	if !errors.Is(err, errSentinel) {
		t.Fatalf("err = %v, want errSentinel", err)
	}
	if called != 1 {
		t.Errorf("fn called %d times, want 1 (no retry on Continue)", called)
	}
	if stats.Committed != 0 || stats.RolledBack != 1 {
		t.Errorf("stats = %+v, want RolledBack=1", stats)
	}
}

// TestTransactionRetriesUntilSuccess returns errRetryable twice then nil: the
// helper replays the whole fn until it succeeds, counting retries.
func TestTransactionRetriesUntilSuccess(t *testing.T) {
	ctx := context.Background()
	conn := noopConn(t)
	var stats TxStats

	called := 0
	err := Transaction(ctx, conn, classifyRetry, driver.ReadCommitted,
		RetryOpts{MaxAttempts: 5, Backoff: time.Microsecond}, &stats,
		func(tx driver.Tx) error {
			called++
			if called < 3 {
				return errRetryable
			}
			return nil
		})
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if called != 3 {
		t.Errorf("fn called %d times, want 3", called)
	}
	if stats.Committed != 1 || stats.RolledBack != 2 || stats.Retried != 2 {
		t.Errorf("stats = %+v, want Committed=1 RolledBack=2 Retried=2", stats)
	}
}

// TestTransactionRetriesExhausted returns errRetryable every attempt and checks
// the helper gives up after MaxAttempts, surfacing the final error.
func TestTransactionRetriesExhausted(t *testing.T) {
	ctx := context.Background()
	conn := noopConn(t)
	var stats TxStats

	called := 0
	err := Transaction(ctx, conn, classifyRetry, driver.ReadCommitted,
		RetryOpts{MaxAttempts: 3, Backoff: time.Microsecond}, &stats,
		func(tx driver.Tx) error {
			called++
			return errRetryable
		})
	if !errors.Is(err, errRetryable) {
		t.Fatalf("err = %v, want errRetryable", err)
	}
	if called != 3 {
		t.Errorf("fn called %d times, want 3 (MaxAttempts)", called)
	}
	if stats.Committed != 0 || stats.RolledBack != 3 || stats.Retried != 2 {
		t.Errorf("stats = %+v, want RolledBack=3 Retried=2", stats)
	}
}

// TestTransactionNoClassifyNoRetry confirms a nil classifier disables retry —
// the fn runs exactly once even when it returns a value the caller might have
// considered retryable.
func TestTransactionNoClassifyNoRetry(t *testing.T) {
	ctx := context.Background()
	conn := noopConn(t)

	called := 0
	err := Transaction(ctx, conn, nil, driver.ReadCommitted,
		RetryOpts{MaxAttempts: 5}, nil,
		func(tx driver.Tx) error {
			called++
			return errRetryable
		})
	if !errors.Is(err, errRetryable) {
		t.Fatalf("err = %v, want errRetryable", err)
	}
	if called != 1 {
		t.Errorf("fn called %d times, want 1 (nil classify disables retry)", called)
	}
}

// TestTransactionZeroOptsRunsOnce confirms a zero RetryOpts runs the fn exactly
// once with no replay — the conservative default.
func TestTransactionZeroOptsRunsOnce(t *testing.T) {
	ctx := context.Background()
	conn := noopConn(t)

	called := 0
	_ = Transaction(ctx, conn, classifyRetry, driver.ReadCommitted, RetryOpts{}, nil,
		func(tx driver.Tx) error {
			called++
			return errRetryable
		})
	if called != 1 {
		t.Errorf("fn called %d times, want 1 (zero RetryOpts)", called)
	}
}

// TestTransactionStatsOptional confirms a nil stats pointer is accepted (the
// recording path is skipped silently). Mirrors how tpcc passes nil and keeps its
// own per-VU counts.
func TestTransactionStatsOptional(t *testing.T) {
	ctx := context.Background()
	conn := noopConn(t)

	called := 0
	err := Transaction(ctx, conn, classifyRetry, driver.ReadCommitted, RetryOpts{}, nil,
		func(tx driver.Tx) error {
			called++
			return nil
		})
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if called != 1 {
		t.Errorf("fn called %d times, want 1", called)
	}
}
