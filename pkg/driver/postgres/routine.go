package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/stroppy-io/stroppy/v6/pkg/config"
	"github.com/stroppy-io/stroppy/v6/pkg/driver"
	"github.com/stroppy-io/stroppy/v6/pkg/driver/postgres/pool"
	"github.com/stroppy-io/stroppy/v6/pkg/driver/sqldriver"
)

func (d *Driver) RunRoutine(
	ctx context.Context, isolation config.TxIsolationLevel, query string, args map[string]any,
) (*driver.QueryResult, error) {
	ctx, cancel := sqldriver.StatementTimeout(ctx, d.queryTimeout)
	defer cancel()

	conn, err := d.pool.Acquire(pool.WithRoutineIsolation(ctx, isolation))
	if err != nil {
		return nil, &driver.RoutineError{Cause: err}
	}
	defer conn.Release()

	result, err := sqldriver.RunQuery(ctx, statementQuery{query: conn.Query}, NewRows,
		PgxDialect{}, d.logger, query, args, d.queryTimeout)
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
