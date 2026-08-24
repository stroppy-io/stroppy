package sqldriver

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/pkg/driver"
)

// errPinger always reports the database is not ready, so WaitForDB loops until
// ctx is canceled — mirroring a readiness wait behind a signal cancellation.
type errPinger struct{}

func (errPinger) Ping(context.Context) error { return errors.New("not ready") }

func TestWaitForDBContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := WaitForDB(ctx, zap.NewNop(), errPinger{}, 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("WaitForDB() error = %v, want context.Canceled", err)
	}
}

// blockingQueryConn blocks its QueryContext call until ctx is canceled, standing
// in for an in-flight query that a SIGINT/SIGTERM must interrupt.
type blockingQueryConn struct {
	released chan struct{}
}

func (c *blockingQueryConn) QueryContext(ctx context.Context, _ string, _ ...any) (any, error) {
	<-ctx.Done()
	close(c.released)

	return nil, ctx.Err()
}

func TestRunQueryCancelsBlockedQuery(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	conn := &blockingQueryConn{released: make(chan struct{})}

	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	_, err := RunQuery(
		ctx,
		conn,
		func(any) driver.Rows { return nil },
		testDialect{},
		zap.NewNop(),
		"SELECT 1",
		nil,
		0,
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("RunQuery() error = %v, want context.Canceled", err)
	}

	select {
	case <-conn.released:
	case <-time.After(5 * time.Second):
		t.Fatal("blocked query did not observe cancellation")
	}
}
