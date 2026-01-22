package postgres

import (
	"context"
	"time"

	trmpgx "github.com/avito-tech/go-transaction-manager/drivers/pgxv5/v2"
	"github.com/avito-tech/go-transaction-manager/trm/v2/manager"
	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/durationpb"

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
		unit *stroppy.UnitDescriptor,
	) (*stroppy.DriverTransaction, error)
	AddGenerators(unit *stroppy.UnitDescriptor) error
	ValueToPgxValue(value *stroppy.Value) (any, error)
}

type Driver struct {
	logger  *zap.Logger
	pgxPool interface {
		Executor
		Close()
		CopyFrom(
			ctx context.Context,
			tableName pgx.Identifier,
			columnNames []string,
			rowSrc pgx.CopyFromSource,
		) (int64, error)
	}
	txManager  *manager.Manager
	txExecutor *TxExecutor

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

	d.txManager = manager.Must(trmpgx.NewDefaultFactory(connPool))
	d.txExecutor = NewTxExecutor(connPool)

	return d, nil
}

func (d *Driver) RunTransaction(
	ctx context.Context,
	unit *stroppy.UnitDescriptor,
) (*stroppy.DriverTransactionStat, error) {
	err := d.builder.AddGenerators(unit)
	if err != nil {
		return nil, err
	}

	transaction, err := d.GenerateNextUnit(ctx, unit)
	if err != nil {
		return nil, err
	}

	return d.runTransaction(ctx, transaction)
}

func (d *Driver) GenerateNextUnit(
	ctx context.Context,
	unit *stroppy.UnitDescriptor,
) (*stroppy.DriverTransaction, error) {
	return d.builder.Build(ctx, d.logger, unit)
}

func (d *Driver) runTransaction(
	ctx context.Context,
	transaction *stroppy.DriverTransaction,
) (*stroppy.DriverTransactionStat, error) {
	var (
		stat *stroppy.DriverTransactionStat
		err  error
	)

	if transaction.GetIsolationLevel() == stroppy.TxIsolationLevel_UNSPECIFIED {
		stat, err = d.runTransactionInternal(ctx, transaction, d.pgxPool)

		return stat, err
	}

	return stat, d.txManager.DoWithSettings(
		ctx,
		NewStroppyIsolationSettings(transaction),
		func(ctx context.Context) error {
			stat, err = d.runTransactionInternal(ctx, transaction, d.txExecutor)

			return err
		})
}

func (d *Driver) runTransactionInternal(
	ctx context.Context,
	transaction *stroppy.DriverTransaction,
	executor Executor,
) (*stroppy.DriverTransactionStat, error) {
	queryStats := make([]*stroppy.DriverQueryStat, 0, len(transaction.GetQueries()))
	txStart := time.Now()

	for _, query := range transaction.GetQueries() {
		values := make([]any, len(query.GetParams()))

		err := d.fillParamsToValues(query, values)
		if err != nil {
			return nil, err
		}

		start := time.Now()

		_, err = executor.Exec(ctx, query.GetRequest(), values...)
		if err != nil {
			return nil, err
		}

		queryStats = append(queryStats, &stroppy.DriverQueryStat{
			Name:         query.GetName(),
			ExecDuration: durationpb.New(time.Since(start)),
		})
	}

	return &stroppy.DriverTransactionStat{
		IsolationLevel: transaction.GetIsolationLevel(),
		ExecDuration:   durationpb.New(time.Since(txStart)),
		Queries:        queryStats,
	}, nil
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
