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

	if backend, ok := b.drv.(driver.RoutineExecutor); ok {
		result, runErr := backend.RunRoutine(b.operationContext(ctx), isolation, query, args)
		err = b.finishQuery(result, runErr)
		committed := err == nil

		action := "commit"
		if err != nil {
			action = "rollback"
		}

		b.root.txMetrics.record(b.vu, action, opts.Name, isolation)
		b.root.txMetrics.recordTxEnd(b.vu, action, opts.Name, isolation, time.Since(started), 1, committed)

		if err != nil {
			var routine *driver.RoutineError
			if !errors.As(err, &routine) || routine.Ambiguous {
				return &commitError{err: err}
			}
		}

		return err
	}

	return b.BeginTx(ctx, opts, func(tx *Tx) error { return tx.Exec(ctx, query, args) })
}

func RoutineRollbackConfirmed(err error) bool {
	var routine *driver.RoutineError

	return errors.As(err, &routine) && routine.RollbackConfirmed
}
