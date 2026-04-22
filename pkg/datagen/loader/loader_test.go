package loader

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
)

// fakeInserter records every Insert call and tracks peak concurrent
// worker usage so tests can assert admission behavior without wiring a
// real driver.
type fakeInserter struct {
	hold       time.Duration // how long each Insert blocks
	err        error         // returned on every Insert
	errOnTable string        // when non-empty, only fail for this table

	mu         sync.Mutex
	observed   []call // workers seen per table, in call order
	active     int64  // live worker slots, summed across calls in flight
	peakActive int64  // high-water of active
}

type call struct {
	table   string
	workers int
}

func (f *fakeInserter) Insert(ctx context.Context, spec *dgproto.InsertSpec, workers int) error {
	f.mu.Lock()
	f.observed = append(f.observed, call{table: spec.GetTable(), workers: workers})

	f.active += int64(workers)
	if f.active > f.peakActive {
		f.peakActive = f.active
	}
	f.mu.Unlock()

	defer func() {
		f.mu.Lock()
		f.active -= int64(workers)
		f.mu.Unlock()
	}()

	if f.hold > 0 {
		select {
		case <-time.After(f.hold):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	if f.err != nil && (f.errOnTable == "" || f.errOnTable == spec.GetTable()) {
		return f.err
	}

	return nil
}

func (f *fakeInserter) calls() []call {
	f.mu.Lock()
	defer f.mu.Unlock()

	out := make([]call, len(f.observed))
	copy(out, f.observed)

	return out
}

func (f *fakeInserter) peak() int64 {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.peakActive
}

func makeSpec(table string, workers int32) *dgproto.InsertSpec {
	s := &dgproto.InsertSpec{Table: table}
	if workers >= 0 {
		s.Parallelism = &dgproto.Parallelism{Workers: workers}
	}

	return s
}

func TestNewValidation(t *testing.T) {
	t.Parallel()

	fake := &fakeInserter{}

	_, err := New(nil, 4, zap.NewNop())
	if !errors.Is(err, ErrNilInserter) {
		t.Fatalf("nil inserter: want ErrNilInserter, got %v", err)
	}

	_, err = New(fake, 0, zap.NewNop())
	if !errors.Is(err, ErrZeroCap) {
		t.Fatalf("zero cap: want ErrZeroCap, got %v", err)
	}

	_, err = New(fake, -3, zap.NewNop())
	if !errors.Is(err, ErrZeroCap) {
		t.Fatalf("negative cap: want ErrZeroCap, got %v", err)
	}

	l, err := New(fake, 8, nil)
	if err != nil {
		t.Fatalf("nil logger should be accepted: %v", err)
	}

	if l.Cap() != 8 {
		t.Fatalf("Cap(): got %d, want 8", l.Cap())
	}
}

func TestInsertNilSpec(t *testing.T) {
	t.Parallel()

	l, err := New(&fakeInserter{}, 4, zap.NewNop())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := l.Insert(context.Background(), nil); !errors.Is(err, ErrNilSpec) {
		t.Fatalf("nil spec: want ErrNilSpec, got %v", err)
	}
}

func TestInsertClampsWorkers(t *testing.T) {
	t.Parallel()

	fake := &fakeInserter{}

	l, err := New(fake, 4, zap.NewNop())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := l.Insert(context.Background(), makeSpec("foo", 100)); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	got := fake.calls()
	if len(got) != 1 {
		t.Fatalf("calls: got %d, want 1", len(got))
	}

	if got[0].workers != 4 {
		t.Fatalf("workers: got %d, want 4 (clamped)", got[0].workers)
	}
}

func TestInsertZeroWorkersDefaultsToOne(t *testing.T) {
	t.Parallel()

	fake := &fakeInserter{}

	l, err := New(fake, 4, zap.NewNop())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := l.Insert(context.Background(), makeSpec("zero", 0)); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	if err := l.Insert(context.Background(), makeSpec("neg", -1)); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	got := fake.calls()
	if len(got) != 2 {
		t.Fatalf("calls: got %d, want 2", len(got))
	}

	for _, c := range got {
		if c.workers != 1 {
			t.Fatalf("table %q: got workers=%d, want 1", c.table, c.workers)
		}
	}
}

func TestInsertNilParallelism(t *testing.T) {
	t.Parallel()

	fake := &fakeInserter{}

	l, err := New(fake, 8, zap.NewNop())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	spec := &dgproto.InsertSpec{Table: "npar"} // Parallelism left nil
	if err := l.Insert(context.Background(), spec); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	got := fake.calls()
	if len(got) != 1 || got[0].workers != 1 {
		t.Fatalf("nil parallelism: got %+v, want [{npar 1}]", got)
	}
}

func TestInsertConcurrentCaps(t *testing.T) {
	t.Parallel()

	fake := &fakeInserter{hold: 50 * time.Millisecond}

	l, err := New(fake, 5, zap.NewNop())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	specs := []*dgproto.InsertSpec{
		makeSpec("a", 3),
		makeSpec("b", 3),
		makeSpec("c", 3),
		makeSpec("d", 3),
	}

	if err := l.InsertConcurrent(context.Background(), specs); err != nil {
		t.Fatalf("InsertConcurrent: %v", err)
	}

	if got := fake.peak(); got > 5 {
		t.Fatalf("peak active workers = %d, want <= 5", got)
	}

	if len(fake.calls()) != 4 {
		t.Fatalf("want 4 calls, got %d", len(fake.calls()))
	}
}

func TestInsertConcurrentErrorCancels(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")
	fake := &fakeInserter{
		hold:       150 * time.Millisecond,
		err:        boom,
		errOnTable: "bad",
	}

	// Cap=1 forces serial admission so the failing spec goes first when
	// placed at the head; others block on the semaphore and observe the
	// canceled context.
	l, err := New(fake, 1, zap.NewNop())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	specs := []*dgproto.InsertSpec{
		makeSpec("bad", 1),
		makeSpec("other1", 1),
		makeSpec("other2", 1),
	}

	err = l.InsertConcurrent(context.Background(), specs)
	if !errors.Is(err, boom) {
		t.Fatalf("want boom, got %v", err)
	}
}

func TestInsertConcurrentEmpty(t *testing.T) {
	t.Parallel()

	fake := &fakeInserter{}

	l, err := New(fake, 2, zap.NewNop())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := l.InsertConcurrent(context.Background(), nil); err != nil {
		t.Fatalf("nil slice: %v", err)
	}

	if err := l.InsertConcurrent(context.Background(), []*dgproto.InsertSpec{}); err != nil {
		t.Fatalf("empty slice: %v", err)
	}

	if len(fake.calls()) != 0 {
		t.Fatalf("expected no inserts, got %d", len(fake.calls()))
	}
}

func TestInsertConcurrentNilSpec(t *testing.T) {
	t.Parallel()

	l, err := New(&fakeInserter{}, 2, zap.NewNop())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	err = l.InsertConcurrent(context.Background(), []*dgproto.InsertSpec{makeSpec("ok", 1), nil})
	if !errors.Is(err, ErrNilSpec) {
		t.Fatalf("want ErrNilSpec, got %v", err)
	}
}

func TestMaxWorkersFromEnv(t *testing.T) {
	// Not parallel: mutates process env.
	cases := []struct {
		name string
		set  bool
		val  string
		def  int
		want int
	}{
		{name: "unset", set: false, def: 7, want: 7},
		{name: "positive", set: true, val: "12", def: 3, want: 12},
		{name: "zero", set: true, val: "0", def: 9, want: 9},
		{name: "negative", set: true, val: "-1", def: 9, want: 9},
		{name: "non-numeric", set: true, val: "abc", def: 9, want: 9},
		{name: "empty", set: true, val: "", def: 5, want: 5},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.set {
				t.Setenv(envMaxWorkers, tc.val)
			} else {
				// Snapshot + remove for the duration of the subtest.
				prev, had := os.LookupEnv(envMaxWorkers)
				if err := os.Unsetenv(envMaxWorkers); err != nil {
					t.Fatalf("Unsetenv: %v", err)
				}

				t.Cleanup(func() {
					if had {
						_ = os.Setenv(envMaxWorkers, prev)
					}
				})
			}

			got := MaxWorkersFromEnv(tc.def)
			if got != tc.want {
				t.Fatalf("%s: got %d, want %d", tc.name, got, tc.want)
			}
		})
	}
}
