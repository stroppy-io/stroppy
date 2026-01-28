package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/pkg/common/logger"
	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"github.com/stroppy-io/stroppy/pkg/driver/postgres/pool"
	"github.com/stroppy-io/stroppy/pkg/driver/postgres/queries"
)

// TODO: performance issue by passing via interface?

type QueryBuilder interface {
	Build(
		ctx context.Context,
		logger *zap.Logger,
		insert *stroppy.InsertDescriptor,
	) (*stroppy.DriverTransaction, error)
	AddGenerators(insert *stroppy.InsertDescriptor) error
	ValueToPgxValue(value *stroppy.Value) (any, error)
}

type Executor interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	CopyFrom(
		ctx context.Context,
		tableName pgx.Identifier,
		columnNames []string,
		rowSrc pgx.CopyFromSource,
	) (int64, error)
	Close()
}

type Driver struct {
	logger  *zap.Logger
	pgxPool Executor
	builder QueryBuilder
}

//nolint:nonamedreturns // named returns for defer error handling
func NewDriver(
	ctx context.Context,
	lg *zap.Logger,
	cfg *stroppy.DriverConfig,
) (d *Driver, err error) {
	if lg == nil {
		d = &Driver{
			logger: logger.NewFromEnv().
				Named(pool.DriverLoggerName).
				WithOptions(zap.AddCallerSkip(0)),
		}
	} else {
		d = &Driver{
			logger: lg,
		}
	}

	connPool, err := pool.NewPool(
		ctx,
		cfg,
		d.logger.Named(pool.LoggerName),
	)
	if err != nil {
		return nil, err
	}

	d.logger.Debug("Checking db connection...", zap.String("url", cfg.GetUrl()))

	err = waitForDB(ctx, d.logger, connPool, dbConnectionTimeout)
	if err != nil {
		return nil, err
	}

	d.pgxPool = connPool

	d.builder, err = queries.NewQueryBuilder(0) // TODO: seed initialization after driver creation
	if err != nil {
		return nil, err
	}

	return d, nil
}

func (d *Driver) GenerateNextUnit(
	ctx context.Context,
	insert *stroppy.InsertDescriptor,
) (*stroppy.DriverTransaction, error) {
	return d.builder.Build(ctx, d.logger, insert)
}

func (d *Driver) fillParamsToValues(query *stroppy.DriverQuery, valuesOut []any) error {
	for i, v := range query.GetParams() {
		val, err := d.builder.ValueToPgxValue(v)
		if err != nil {
			return err
		}

		valuesOut[i] = val
	}

	return nil
}

func (d *Driver) Teardown(_ context.Context) error {
	d.logger.Debug("Driver Teardown Start")
	d.pgxPool.Close()
	d.logger.Debug("Driver Teardown End")

	return nil
}
