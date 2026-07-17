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
	c, err := noop.New(driver.Spec{}).Connect(context.Background())
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
	var rec TxRecorder

	called := 0
	err := Transaction(ctx, conn, classifyRetry, driver.ReadCommitted, RetryOpts{MaxAttempts: 3}, &rec,
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
	if rec.Committed != 1 || rec.RolledBack != 0 || rec.Retried != 0 {
		t.Errorf("rec = %+v, want Committed=1", rec)
	}
}

// TestTransactionRollbackTerminal runs fn returning a non-retryable error and
// checks the helper rolls back and surfaces it without replaying.
func TestTransactionRollbackTerminal(t *testing.T) {
	ctx := context.Background()
	conn := noopConn(t)
	var rec TxRecorder
	errSentinel := errors.New("boom")

	called := 0
	err := Transaction(ctx, conn, classifyRetry, driver.ReadCommitted, RetryOpts{MaxAttempts: 3}, &rec,
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
	if rec.Committed != 0 || rec.RolledBack != 1 {
		t.Errorf("rec = %+v, want RolledBack=1", rec)
	}
}

// TestTransactionRetriesUntilSuccess returns errRetryable twice then nil: the
// helper replays the whole fn until it succeeds, counting retries.
func TestTransactionRetriesUntilSuccess(t *testing.T) {
	ctx := context.Background()
	conn := noopConn(t)
	var rec TxRecorder

	called := 0
	err := Transaction(ctx, conn, classifyRetry, driver.ReadCommitted,
		RetryOpts{MaxAttempts: 5, Backoff: time.Microsecond}, &rec,
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
	if rec.Committed != 1 || rec.RolledBack != 2 || rec.Retried != 2 {
		t.Errorf("rec = %+v, want Committed=1 RolledBack=2 Retried=2", rec)
	}
}

// TestTransactionRetriesExhausted returns errRetryable every attempt and checks
// the helper gives up after MaxAttempts, surfacing the final error.
func TestTransactionRetriesExhausted(t *testing.T) {
	ctx := context.Background()
	conn := noopConn(t)
	var rec TxRecorder

	called := 0
	err := Transaction(ctx, conn, classifyRetry, driver.ReadCommitted,
		RetryOpts{MaxAttempts: 3, Backoff: time.Microsecond}, &rec,
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
	if rec.Committed != 0 || rec.RolledBack != 3 || rec.Retried != 2 {
		t.Errorf("rec = %+v, want RolledBack=3 Retried=2", rec)
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

// TestTransactionRecorderOptional confirms a nil recorder pointer is accepted
// (the recording path is skipped silently) — the contract an author uses to opt
// out of whole-tx latency recording for raw uninstrumented timing.
func TestTransactionRecorderOptional(t *testing.T) {
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

// TestRetryReplays is the plain-query (non-tx) path: Retry calls fn again when
// classify maps the error to Retry, up to MaxAttempts, then succeeds.
func TestRetryReplays(t *testing.T) {
	ctx := context.Background()
	called := 0
	err := Retry(ctx, classifyRetry, RetryOpts{MaxAttempts: 4, Backoff: time.Microsecond},
		func() error {
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
}

// TestRetryExhausted surfaces the final error once attempts run out.
func TestRetryExhausted(t *testing.T) {
	ctx := context.Background()
	called := 0
	err := Retry(ctx, classifyRetry, RetryOpts{MaxAttempts: 2, Backoff: time.Microsecond},
		func() error {
			called++
			return errRetryable
		})
	if !errors.Is(err, errRetryable) {
		t.Fatalf("err = %v, want errRetryable", err)
	}
	if called != 2 {
		t.Errorf("fn called %d times, want 2", called)
	}
}

// TestRetryDisabledByDefault confirms a zero-value RetryOpts runs fn once — the
// opt-in contract (retry is never silently enabled).
func TestRetryDisabledByDefault(t *testing.T) {
	ctx := context.Background()
	called := 0
	err := Retry(ctx, classifyRetry, RetryOpts{},
		func() error {
			called++
			return errRetryable
		})
	if !errors.Is(err, errRetryable) {
		t.Fatalf("err = %v, want errRetryable", err)
	}
	if called != 1 {
		t.Errorf("fn called %d times, want 1", called)
	}
}
