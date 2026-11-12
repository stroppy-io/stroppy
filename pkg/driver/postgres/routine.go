package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/v6/pkg/config"
	"github.com/stroppy-io/stroppy/v6/pkg/driver"
	"github.com/stroppy-io/stroppy/v6/pkg/driver/postgres/pool"
	"github.com/stroppy-io/stroppy/v6/pkg/driver/sqldriver"
	"github.com/stroppy-io/stroppy/v6/pkg/driver/stats"
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

	const bufferKey = "stroppy.routineArguments"

	buffer, _ := conn.Conn().PgConn().CustomData()[bufferKey].(*[]any)
	if buffer == nil {
		buffer = new([]any)
		conn.Conn().PgConn().CustomData()[bufferKey] = buffer
	}

	processed, values, err := sqldriver.ProcessArgsInto(PgxDialect{}, query, args, *buffer)
	if values != nil {
		*buffer = values
	}
	defer clear(*buffer)

	if err != nil {
		if !errors.Is(err, sqldriver.ErrExtraArgument) {
			return nil, &driver.RoutineError{Cause: fmt.Errorf("arguments processing error: %w", err)}
		}

		d.logger.Warn("unused query arguments ignored; query execution continues", zap.Error(err), zap.String("sql", query))
	}

	started := time.Now()
	rows, err := conn.Query(ctx, processed, values...)
	elapsed := time.Since(started)

	if rows != nil {
		rows.Close()
		err = driver.JoinErrors(err, rows.Err())
	}

	err = statementContextError(ctx, err)
	result := &struct {
		driver.QueryResult
		stats stats.Query
	}{stats: stats.Query{Elapsed: elapsed}}
	result.Stats = &result.stats

	if err != nil {
		backend, confirmed := errors.AsType[*pgconn.PgError](err)
		if confirmed && backend.Code == "40003" {
			confirmed = false
		}

		return &result.QueryResult, &driver.RoutineError{
			Cause: err, RollbackConfirmed: confirmed, Ambiguous: !confirmed && !pgconn.SafeToRetry(err),
		}
	}

	return &result.QueryResult, nil
}
