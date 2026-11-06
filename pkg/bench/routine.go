package bench

import (
	"context"
	"errors"
	"time"

	"github.com/stroppy-io/stroppy/v6/pkg/driver"
)

// Routine requires the server routine to commit atomically or roll back before returning an error.
func (b *Bench) Routine(ctx context.Context, opts BeginOpts, query string, args map[string]any) error {
	if err := b.ensureDriver(ctx); err != nil {
		return err
	}

	isolation, err := ParseTxIsolation(string(opts.Isolation))
	if err != nil {
		return err
	}

	started := time.Now()

	backend, ok := b.drv.(driver.RoutineExecutor)
	if !ok {
		return b.BeginTx(ctx, opts, func(tx *Tx) error { return tx.Exec(ctx, query, args) })
	}

	result, runErr := backend.RunRoutine(b.operationContext(ctx), isolation, query, args)
	err = b.finishQuery(result, runErr)
	action := "commit"

	if err != nil {
		switch {
		case ambiguousRoutineOutcome(err):
			action = "unknown"
		case RoutineRollbackConfirmed(err):
			action = "rollback"
		default:
			action = "setup_error"
		}
	}

	if action == "commit" || action == "rollback" {
		b.root.txMetrics.record(b.vu, action, opts.Name, isolation)
	}

	b.root.txMetrics.recordTxEnd(b.vu, action, opts.Name, isolation, time.Since(started), 1, err == nil)

	if err != nil {
		var routine *driver.RoutineError
		if !errors.As(err, &routine) || ambiguousRoutineOutcome(err) {
			return &commitError{err: err}
		}
	}

	return err
}

func ambiguousRoutineOutcome(err error) bool {
	if err == nil {
		return false
	}

	// Inspect this node only: errors.As would stop at the first joined routine outcome.
	//nolint:errorlint // Every branch is inspected below.
	if routine, ok := err.(*driver.RoutineError); ok && routine.Ambiguous {
		return true
	}

	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		for _, cause := range joined.Unwrap() {
			if ambiguousRoutineOutcome(cause) {
				return true
			}
		}

		return false
	}

	return ambiguousRoutineOutcome(errors.Unwrap(err))
}

func RoutineRollbackConfirmed(err error) bool {
	var routine *driver.RoutineError

	return !ambiguousRoutineOutcome(err) && errors.As(err, &routine) && routine.RollbackConfirmed
}
