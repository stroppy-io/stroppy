package postgres

import (
	"context"
	"time"

	trmpgx "github.com/avito-tech/go-transaction-manager/drivers/pgxv5/v2"
	"github.com/avito-tech/go-transaction-manager/trm/v2/manager"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/stroppy-io/stroppy/pkg/common/logger"
	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto"
	"github.com/stroppy-io/stroppy/pkg/driver/postgres/pool"
	"github.com/stroppy-io/stroppy/pkg/driver/postgres/queries"
)

type QueryBuilder interface {
	Build(
		ctx context.Context,
		logger *zap.Logger,
		unit *stroppy.UnitDescriptor,
	) (*stroppy.DriverTransaction, error)
	ValueToPgxValue(value *stroppy.Value) (any, error)
}

type Driver struct {
	logger  *zap.Logger
	pgxPool interface {
		Executor
		Close()
	}
	txManager  *manager.Manager
	txExecutor *TxExecutor
	builder    QueryBuilder
}

func NewDriver(lg *zap.Logger) *Driver { //nolint: ireturn // allow
	if lg == nil {
		return &Driver{
			logger: logger.NewFromEnv().
				Named(pool.DriverLoggerName).
				WithOptions(zap.AddCallerSkip(1)),
		}
	}

	return &Driver{
		logger: lg,
	}
}

func (d *Driver) Initialize(ctx context.Context, runContext *stroppy.StepContext) error {
	connPool, err := pool.NewPool(
		ctx,
		runContext.GetConfig().GetDriver(),
		d.logger.Named(pool.LoggerName),
	)
	if err != nil {
		return err
	}

	d.pgxPool = connPool

	d.builder, err = queries.NewQueryBuilder(runContext)
	if err != nil {
		return err
	}

	d.txManager = manager.Must(trmpgx.NewDefaultFactory(connPool))
	d.txExecutor = NewTxExecutor(connPool)

	return nil
}

func (d *Driver) GenerateNextUnit(
	ctx context.Context,
	unit *stroppy.UnitDescriptor,
) (*stroppy.DriverTransaction, error) {
	return d.builder.Build(ctx, d.logger, unit)
}

func (d *Driver) RunTransaction(
	ctx context.Context,
	transaction *stroppy.DriverTransaction,
) (*stroppy.DriverTransactionStat, error) {
	var (
		stat *stroppy.DriverTransactionStat
		err  error
	)

	if transaction.GetIsolationLevel() == stroppy.TxIsolationLevel_TX_ISOLATION_LEVEL_UNSPECIFIED {
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
	queries := make([]*stroppy.DriverQueryStat, 0, len(transaction.GetQueries()))
	txStart := time.Now()

	for _, query := range transaction.GetQueries() {
		values := make([]any, len(query.GetParams()))

		for i, v := range query.GetParams() {
			val, err := d.builder.ValueToPgxValue(v)
			if err != nil {
				return nil, err
			}

			values[i] = val
		}

		start := time.Now()

		_, err := executor.Exec(ctx, query.GetRequest(), values...)
		if err != nil {
			return nil, err
		}

		queries = append(queries, &stroppy.DriverQueryStat{
			Name:         query.GetName(),
			ExecDuration: durationpb.New(time.Since(start)),
		})
	}

	return &stroppy.DriverTransactionStat{
		IsolationLevel: transaction.GetIsolationLevel(),
		ExecDuration:   durationpb.New(time.Since(txStart)),
		Queries:        queries,
	}, nil
}

func (d *Driver) Teardown(_ context.Context) error {
	d.pgxPool.Close()

	return nil
}
