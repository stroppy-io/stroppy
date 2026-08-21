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

type deadlineCaptureQuery struct {
	sql      string
	deadline time.Time
	hasLimit bool
}

func (q *deadlineCaptureQuery) QueryContext(
	ctx context.Context,
	sql string,
	_ ...any,
) (int, error) {
	q.sql = sql
	q.deadline, q.hasLimit = ctx.Deadline()

	return 0, nil
}

type selectiveHintDialect struct{ testDialect }

func (selectiveHintDialect) StatementTimeoutHint(sql string, _ time.Duration) string {
	if sql == "SELECT 1" {
		return "SELECT /* timeout hint */ 1"
	}

	return sql
}

func (selectiveHintDialect) StatementDeadline(timeout time.Duration) time.Duration {
	return timeout + time.Second
}

type lazyQueryRows struct {
	ctx context.Context
	err error
}

func (*lazyQueryRows) Columns() []string   { return nil }
func (*lazyQueryRows) Values() []any       { return nil }
func (*lazyQueryRows) ReadAll(int) [][]any { return nil }
func (r *lazyQueryRows) Err() error        { return r.err }
func (*lazyQueryRows) Close() error        { return nil }
func (r *lazyQueryRows) Next() bool {
	<-r.ctx.Done()
	r.err = r.ctx.Err()

	return false
}

type lazyQuery struct{}

func (lazyQuery) QueryContext(ctx context.Context, _ string, _ ...any) (*lazyQueryRows, error) {
	return &lazyQueryRows{ctx: ctx}, nil
}

type immediateRowsErrorQuery struct {
	err error
}

func (q immediateRowsErrorQuery) QueryContext(
	context.Context,
	string,
	...any,
) (*lazyQueryRows, error) {
	return &lazyQueryRows{err: q.err}, nil
}

func TestStatementTimeoutDisabled(t *testing.T) {
	t.Parallel()

	ctx, cancel := StatementTimeout(context.Background(), 0)
	defer cancel()

	if ctx != context.Background() {
		t.Fatal("zero timeout should return the parent context unchanged")
	}
}

func TestRunQueryDeadlineMatchesHintApplication(t *testing.T) {
	t.Parallel()

	const timeout = 150 * time.Millisecond

	tests := []struct {
		name         string
		sql          string
		wantSQL      string
		wantDeadline time.Duration
	}{
		{
			name:         "hinted statement gets grace",
			sql:          "SELECT 1",
			wantSQL:      "SELECT /* timeout hint */ 1",
			wantDeadline: timeout + time.Second,
		},
		{
			name:         "unhinted statement gets exact timeout",
			sql:          "DO 1",
			wantSQL:      "DO 1",
			wantDeadline: timeout,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			capture := &deadlineCaptureQuery{}
			before := time.Now()

			_, err := RunQuery(
				context.Background(),
				capture,
				noopWrap,
				selectiveHintDialect{},
				zap.NewNop(),
				tt.sql,
				nil,
				timeout,
			)
			after := time.Now()

			if err != nil {
				t.Fatalf("RunQuery() error = %v", err)
			}

			if capture.sql != tt.wantSQL {
				t.Fatalf("QueryContext() SQL = %q, want %q", capture.sql, tt.wantSQL)
			}

			if !capture.hasLimit {
				t.Fatal("QueryContext() context has no deadline")
			}

			earliest := before.Add(tt.wantDeadline)
			latest := after.Add(tt.wantDeadline)

			if capture.deadline.Before(earliest) || capture.deadline.After(latest) {
				t.Fatalf("context deadline = %v, want between %v and %v", capture.deadline, earliest, latest)
			}
		})
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

func TestRunQueryReturnsImmediateRowsError(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("rows")

	res, err := RunQuery(
		context.Background(),
		immediateRowsErrorQuery{err: sentinel},
		func(rows *lazyQueryRows) driver.Rows { return rows },
		testDialect{},
		zap.NewNop(),
		"SELECT 1",
		nil,
		30*time.Millisecond,
	)
	if res != nil {
		t.Fatalf("RunQuery() result = %#v, want nil", res)
	}

	if !errors.Is(err, sentinel) {
		t.Fatalf("RunQuery() error = %v, want sentinel", err)
	}
}

func TestRunQueryDeadlineCoversRowIteration(t *testing.T) {
	t.Parallel()

	parent := context.Background()

	res, err := RunQuery(
		parent,
		lazyQuery{},
		func(rows *lazyQueryRows) driver.Rows { return rows },
		testDialect{},
		zap.NewNop(),
		"SELECT 1",
		nil,
		30*time.Millisecond,
	)
	if err != nil {
		t.Fatalf("RunQuery() error = %v", err)
	}
	defer res.Rows.Close()

	if res.Rows.Next() {
		t.Fatal("Next() = true, want deadline to stop row iteration")
	}

	if err := res.Rows.Err(); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Rows.Err() = %v, want context.DeadlineExceeded", err)
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
