package sqldriver

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/pkg/driver"
)

// fakeQuery surfaces the per-statement context RunQuery derives, so tests can
// prove the deadline fires without a real database.
type fakeQuery struct {
	fn func(ctx context.Context) error
}

func (f *fakeQuery) QueryContext(ctx context.Context, _ string, _ ...any) (int, error) {
	return 0, f.fn(ctx)
}

func noopWrap(int) driver.Rows { return nil }

func TestStatementTimeoutDisabled(t *testing.T) {
	t.Parallel()

	ctx, cancel := StatementTimeout(context.Background(), 0)
	defer cancel()

	if ctx != context.Background() {
		t.Fatal("zero timeout should return the parent context unchanged")
	}
}

func TestRunQueryAppliesPerStatementDeadline(t *testing.T) {
	t.Parallel()

	parent := context.Background()

	f := &fakeQuery{fn: func(ctx context.Context) error {
		<-ctx.Done()

		return ctx.Err()
	}}

	start := time.Now()

	_, err := RunQuery(parent, f, noopWrap, testDialect{}, zap.NewNop(), "SELECT 1", nil, 30*time.Millisecond)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded (timeout)", err)
	}

	if errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v was classified as canceled, want timeout", err)
	}

	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("deadline did not fire promptly: took %v", elapsed)
	}

	if parent.Err() != nil {
		t.Fatalf("parent context mutated: %v", parent.Err())
	}
}

func TestRunQueryConnectionReusableAfterTimeout(t *testing.T) {
	t.Parallel()

	calls := 0
	f := &fakeQuery{fn: func(ctx context.Context) error {
		calls++
		if calls == 1 {
			<-ctx.Done()

			return ctx.Err()
		}

		return nil
	}}

	_, err := RunQuery(
		context.Background(), f, noopWrap, testDialect{}, zap.NewNop(), "SELECT 1", nil, 20*time.Millisecond,
	)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("first query err = %v, want DeadlineExceeded", err)
	}

	// The same fake connection serves the next statement with a fresh,
	// disabled deadline, proving the timer does not stick to the connection.
	res, err := RunQuery(context.Background(), f, noopWrap, testDialect{}, zap.NewNop(), "SELECT 2", nil, 0)
	if err != nil {
		t.Fatalf("second query err = %v, want nil", err)
	}

	if res == nil {
		t.Fatal("second query result = nil")
	}
}
