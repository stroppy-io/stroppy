package postgres

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/stroppy-io/stroppy/v6/pkg/config"
	"github.com/stroppy-io/stroppy/v6/pkg/driver"
)

func (d *Driver) routineDriver(ctx context.Context, isolation config.TxIsolationLevel) (*Driver, error) {
	d.routineMu.Lock()
	if d.routineClosed {
		d.routineMu.Unlock()

		return nil, pgx.ErrTxClosed
	}

	if isolation == config.TxIsolationLevelUnspecified {
		d.routineMu.Unlock()

		return d, nil
	}

	if existing := d.routines[isolation]; existing != nil {
		d.routineMu.Unlock()

		return existing, nil
	}
	d.routineMu.Unlock()

	name, err := driver.IsolationSQL(isolation)
	if err != nil {
		return nil, err
	}

	cfg := *d.options.Config
	value := strings.ToLower(strings.ReplaceAll(name, " ", "_"))

	transaction := config.TransactionConfig{}
	if cfg.Transactions != nil {
		transaction = *cfg.Transactions
	}

	transaction.DefaultIsolation = &value
	cfg.Transactions = &transaction
	options := d.options
	options.Config = &cfg

	backend, err := NewDriver(ctx, options)
	if err != nil {
		return nil, err
	}

	d.routineMu.Lock()
	if d.routineClosed {
		d.routineMu.Unlock()

		return nil, driver.JoinErrors(pgx.ErrTxClosed, backend.Teardown(context.WithoutCancel(ctx)))
	}

	if existing := d.routines[isolation]; existing != nil {
		d.routineMu.Unlock()

		return existing, backend.Teardown(context.WithoutCancel(ctx))
	}

	if d.routines == nil {
		d.routines = make(map[config.TxIsolationLevel]*Driver)
	}

	d.routines[isolation] = backend
	d.routineMu.Unlock()

	return backend, nil
}

func (d *Driver) RunRoutine(
	ctx context.Context, isolation config.TxIsolationLevel, query string, args map[string]any,
) (*driver.QueryResult, error) {
	backend, err := d.routineDriver(ctx, isolation)
	if err != nil {
		return nil, &driver.RoutineError{Cause: err}
	}

	result, err := backend.RunQuery(ctx, query, args)
	if result != nil && result.Rows != nil {
		err = driver.JoinErrors(err, result.Rows.Close(), result.Rows.Err())
	}

	if err != nil {
		backend, confirmed := errors.AsType[*pgconn.PgError](err)
		if confirmed && backend.Code == "40003" {
			confirmed = false
		}

		return result, &driver.RoutineError{
			Cause: err, RollbackConfirmed: confirmed, Ambiguous: !confirmed && !pgconn.SafeToRetry(err),
		}
	}

	return result, nil
}
