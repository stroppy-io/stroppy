// Package loader is the cross-table scheduler for the datagen insert
// path. It admits per-spec work under a global weighted-semaphore cap so
// concurrent inserts share a single worker budget derived from the
// driver's connection pool. The Loader itself is driver-agnostic:
// workloads configure it with an Inserter adapter that knows how to run
// one InsertSpec against the target database.
package loader

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
)

// envMaxWorkers names the environment variable that overrides the
// default worker cap derived from the driver pool.
const envMaxWorkers = "STROPPY_MAX_LOAD_WORKERS"

// Inserter runs one InsertSpec, honoring the supplied worker count.
// Drivers implement this; the Loader stays DB-agnostic. The workers
// argument is already clamped to [1, totalWorkerCap] by the Loader, so
// implementations may use it directly as the chunk count.
type Inserter interface {
	Insert(ctx context.Context, spec *dgproto.InsertSpec, workers int) error
}

// Loader admits per-spec inserts under a global total-worker cap via a
// weighted semaphore. Insert is serial from the caller's POV;
// InsertConcurrent runs multiple specs in parallel and bounds their
// combined worker usage to totalWorkerCap.
type Loader struct {
	inserter Inserter
	cap      int
	sem      *semaphore.Weighted
	logger   *zap.Logger
}

// New constructs a Loader. totalWorkerCap must be > 0. A nil logger is
// rejected at the caller — pass zap.NewNop() when logging is unwanted —
// so that Insert never has to nil-check before emitting diagnostics.
func New(inserter Inserter, totalWorkerCap int, logger *zap.Logger) (*Loader, error) {
	if inserter == nil {
		return nil, ErrNilInserter
	}

	if totalWorkerCap <= 0 {
		return nil, ErrZeroCap
	}

	if logger == nil {
		logger = zap.NewNop()
	}

	return &Loader{
		inserter: inserter,
		cap:      totalWorkerCap,
		sem:      semaphore.NewWeighted(int64(totalWorkerCap)),
		logger:   logger,
	}, nil
}

// Cap reports the total worker budget the Loader admits against. Used
// by callers and tests to introspect the active limit without reaching
// into unexported fields.
func (l *Loader) Cap() int {
	return l.cap
}

// Insert runs one spec. It clamps spec.Parallelism.Workers into
// [1, totalWorkerCap], acquires that many weighted slots, invokes the
// configured Inserter, and releases on return. A nil Parallelism (or
// Workers <= 0) is treated as a request for a single worker.
func (l *Loader) Insert(ctx context.Context, spec *dgproto.InsertSpec) error {
	if spec == nil {
		return ErrNilSpec
	}

	workers := l.clampWorkers(spec)

	if err := l.sem.Acquire(ctx, int64(workers)); err != nil {
		return fmt.Errorf("loader: acquire %d slot(s) for %q: %w", workers, spec.GetTable(), err)
	}
	defer l.sem.Release(int64(workers))

	l.logger.Debug("loader: admit insert",
		zap.String("table", spec.GetTable()),
		zap.Int("workers", workers),
		zap.Int("cap", l.cap),
	)

	if err := l.inserter.Insert(ctx, spec, workers); err != nil {
		return fmt.Errorf("loader: insert %q: %w", spec.GetTable(), err)
	}

	return nil
}

// InsertConcurrent runs multiple specs concurrently. Each spec goes
// through the same admission as Insert; the shared semaphore bounds the
// combined active worker count across all in-flight inserts. First
// error wins, cancels sibling goroutines via the errgroup context, and
// is returned. Returns nil on success or when specs is empty.
func (l *Loader) InsertConcurrent(ctx context.Context, specs []*dgproto.InsertSpec) error {
	if len(specs) == 0 {
		return nil
	}

	for i, spec := range specs {
		if spec == nil {
			return fmt.Errorf("loader: specs[%d]: %w", i, ErrNilSpec)
		}
	}

	group, groupCtx := errgroup.WithContext(ctx)

	for _, spec := range specs {
		group.Go(func() error {
			return l.Insert(groupCtx, spec)
		})
	}

	if err := group.Wait(); err != nil {
		return err
	}

	return nil
}

// clampWorkers folds a spec's parallelism hint into the Loader's
// configured cap. A missing Parallelism or non-positive Workers maps to
// a single worker, matching the "one goroutine is always admissible"
// contract Insert relies on.
func (l *Loader) clampWorkers(spec *dgproto.InsertSpec) int {
	requested := 0

	if p := spec.GetParallelism(); p != nil {
		requested = int(p.GetWorkers())
	}

	if requested < 1 {
		requested = 1
	}

	if requested > l.cap {
		requested = l.cap
	}

	return requested
}

// MaxWorkersFromEnv returns the value of STROPPY_MAX_LOAD_WORKERS if the
// variable is set to a strictly positive integer, else defaultValue.
// Non-numeric, zero, and negative values fall back silently: callers
// must trust the default path rather than hard-fail on misconfig.
func MaxWorkersFromEnv(defaultValue int) int {
	raw, ok := os.LookupEnv(envMaxWorkers)
	if !ok {
		return defaultValue
	}

	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return defaultValue
	}

	if parsed <= 0 {
		return defaultValue
	}

	return parsed
}
