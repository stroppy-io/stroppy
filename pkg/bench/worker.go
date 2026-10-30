package bench

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

func cleanupWorker(vu *VU, ctx context.Context, failures chan<- error, cancel context.CancelCauseFunc) {
	if err := vu.closeSessions(ctx); err != nil {
		failures <- err

		cancel(err)
	}
}

type stepWorker struct {
	b      *Bench
	action Action
}

func (e *Execution) newStepWorker(ctx context.Context, name string, worker int, opts *stepOptions) *stepWorker {
	//nolint:gosec // worker is nonnegative and bounded by the validated policy.
	vu := &VU{
		root: e.root, ctx: metricWorkerContext(ctx, worker, opts.policy.workers),
		vuid: uint64(worker) + 1, worker: worker, workers: opts.policy.workers,
		stepTag: name, initPhase: opts.policy.mode == "once",
	}
	vu.metricWriter, _ = vu.ctx.Value(metricWriterContextKey{}).(*metricWriter)
	vu.metricStepAttrs = e.root.txMetrics.stepAttributes(name)

	return &stepWorker{b: e.bench(vu, opts.driver.name)}
}

func (e *Execution) initializeWorkers(
	ctx context.Context, name string, opts *stepOptions,
) (workers []*stepWorker, err error) {
	ctx = context.WithValue(ctx, metricStepKey{}, name)

	startup, cancel := context.WithTimeout(ctx, defaultCleanupTimeout)
	defer cancel()

	workers = make([]*stepWorker, opts.policy.workers)

	var wg sync.WaitGroup

	failures := make(chan error, opts.policy.workers)

	panics := make(chan any, opts.policy.workers)
	for worker := range opts.policy.workers {
		state := e.newStepWorker(ctx, name, worker, opts)
		workers[worker] = state

		wg.Go(func() {
			defer func() {
				if value := recover(); value != nil {
					panics <- value

					cancel()
				}
			}()

			var initializeErr error

			state.action, initializeErr = opts.initialize(startup, state.b)
			if initializeErr == nil && state.action == nil {
				initializeErr = inputError("initializer returned nil action")
			}

			if initializeErr != nil {
				failures <- fmt.Errorf("worker %d initialization: %w", state.b.Worker(), initializeErr)

				cancel()
			}
		})
	}

	wg.Wait()
	close(failures)

	for failure := range failures {
		err = errors.Join(err, failure)
	}

	if err == nil {
		err = startup.Err()
	}

	select {
	case value := <-panics:
		for _, worker := range workers {
			_ = worker.b.vu.closeSessions(ctx)
		}

		panic(value)
	default:
	}

	return workers, err
}
