package bench

import (
	"context"
	"errors"
	"time"

	"github.com/stroppy-io/stroppy/v6/pkg/config"
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

	return b.finishRoutine(result, runErr, opts, isolation, started)
}

// RoutineRows retains results only after the atomic routine has finished successfully.
func (b *Bench) RoutineRows(
	ctx context.Context, opts BeginOpts, query string, args map[string]any,
) ([][]any, error) {
	if err := b.ensureDriver(ctx); err != nil {
		return nil, err
	}

	isolation, err := ParseTxIsolation(string(opts.Isolation))
	if err != nil {
		return nil, err
	}

	backend, ok := b.drv.(driver.RoutineRowsExecutor)
	if !ok {
		if _, atomic := b.drv.(driver.RoutineExecutor); atomic {
			return nil, inputError("driver %s does not support routine results", b.DriverTypeName())
		}

		var rows [][]any

		err := b.BeginTx(ctx, opts, func(tx *Tx) error {
			var queryErr error

			rows, queryErr = tx.RawRows(ctx, query, args)

			return queryErr
		})
		if err != nil {
			return nil, err
		}

		return rows, nil
	}

	started := time.Now()

	rows, result, runErr := backend.RunRoutineRows(b.operationContext(ctx), isolation, query, args)
	if err := b.finishRoutine(result, runErr, opts, isolation, started); err != nil {
		return nil, err
	}

	return rows, nil
}

func (b *Bench) finishRoutine(
	result *driver.QueryResult, runErr error, opts BeginOpts, isolation config.TxIsolationLevel, started time.Time,
) error {
	err := b.finishQuery(result, runErr)
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
