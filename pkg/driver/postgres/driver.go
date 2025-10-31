package postgres

import (
	"context"
	"strings"
	"time"

	trmpgx "github.com/avito-tech/go-transaction-manager/drivers/pgxv5/v2"
	"github.com/avito-tech/go-transaction-manager/trm/v2/manager"
	"github.com/jackc/pgx/v5"
	cmap "github.com/orcaman/concurrent-map/v2"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/stroppy-io/stroppy/pkg/common/logger"
	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto"
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

	tableToCopyChannel cmap.ConcurrentMap[string, chan []any]
	copyFromStarter    func(tableName string, paramsNames []string) chan []any

	builder QueryBuilder
}

func NewDriver(
	ctx context.Context,
	lg *zap.Logger,
	cfg *stroppy.DriverConfig,
) (d *Driver, err error) { //nolint: ireturn // allow
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

// InsertValues inserts multiple rows into the database based on the descriptor.
// It supports two methods:
// - PLAIN_QUERY: executes individual INSERT statements for each row
// - COPY_FROM: uses PostgreSQL's COPY protocol for fast bulk insertion
// The count parameter specifies how many rows to insert.
func (d *Driver) InsertValues(
	ctx context.Context,
	descriptor *stroppy.InsertDescriptor,
	count int64,
) (*stroppy.DriverTransactionStat, error) {
	// Add generators for the descriptor
	unitDesc := &stroppy.UnitDescriptor{
		Type: &stroppy.UnitDescriptor_Insert{
			Insert: descriptor,
		},
	}

	err := d.builder.AddGenerators(unitDesc)
	if err != nil {
		return nil, err
	}

	txStart := time.Now()

	switch descriptor.GetMethod() {
	case stroppy.InsertMethod_PLAIN_QUERY:
		return d.insertValuesPlainQuery(ctx, descriptor, count, txStart)
	case stroppy.InsertMethod_COPY_FROM:
		return d.insertValuesCopyFrom(ctx, descriptor, count, txStart)
	default:
		d.logger.Panic("unexpected proto.InsertMethod")

		return nil, nil
	}
}

// insertValuesPlainQuery executes multiple INSERT statements sequentially.
// Each row is inserted with a separate pgx.Exec call.
func (d *Driver) insertValuesPlainQuery(
	ctx context.Context,
	descriptor *stroppy.InsertDescriptor,
	count int64,
	txStart time.Time,
) (*stroppy.DriverTransactionStat, error) {
	queryStart := time.Now()

	// Execute multiple inserts
	for range count {
		transaction, err := d.GenerateNextUnit(ctx, &stroppy.UnitDescriptor{
			Type: &stroppy.UnitDescriptor_Insert{
				Insert: descriptor,
			},
		})
		if err != nil {
			return nil, err
		}

		if len(transaction.GetQueries()) == 0 {
			continue
		}

		query := transaction.GetQueries()[0]
		values := make([]any, len(query.GetParams()))

		err = d.fillParamsToValues(query, values)
		if err != nil {
			return nil, err
		}

		_, err = d.pgxPool.Exec(ctx, query.GetRequest(), values...)
		if err != nil {
			return nil, err
		}
	}

	return &stroppy.DriverTransactionStat{
		IsolationLevel: stroppy.TxIsolationLevel_UNSPECIFIED,
		ExecDuration:   durationpb.New(time.Since(txStart)),
		Queries: []*stroppy.DriverQueryStat{{
			Name:         descriptor.GetName(),
			ExecDuration: durationpb.New(time.Since(queryStart)),
		}},
	}, nil
}

// insertValuesCopyFrom uses PostgreSQL's COPY protocol for fast bulk insertion.
// It streams values on-demand without loading all rows into memory, making it suitable
// for very large counts. Values are generated as the COPY protocol requests them.
func (d *Driver) insertValuesCopyFrom(
	ctx context.Context,
	descriptor *stroppy.InsertDescriptor,
	count int64,
	txStart time.Time,
) (*stroppy.DriverTransactionStat, error) {
	// Get column names
	cols := make([]string, 0, len(descriptor.GetParams()))
	for _, p := range descriptor.GetParams() {
		cols = append(cols, p.GetName())
	}

	for _, g := range descriptor.GetGroups() {
		for _, p := range g.GetParams() {
			cols = append(cols, p.GetName())
		}
	}

	queryStart := time.Now()

	_, err := d.pgxPool.CopyFrom(
		ctx,
		pgx.Identifier{descriptor.GetTableName()},
		cols,
		NewStreamingCopySource(d, descriptor, count),
	)
	if err != nil {
		return nil, err
	}

	return &stroppy.DriverTransactionStat{
		IsolationLevel: stroppy.TxIsolationLevel_UNSPECIFIED,
		ExecDuration:   durationpb.New(time.Since(txStart)),
		Queries: []*stroppy.DriverQueryStat{{
			Name:         descriptor.GetName(),
			ExecDuration: durationpb.New(time.Since(queryStart)),
		}},
	}, nil
}

// streamingCopySource implements pgx.CopyFromSource to generate values on-demand
// without loading all rows into memory.
type streamingCopySource struct {
	driver      *Driver
	descriptor  *stroppy.InsertDescriptor
	count       int64
	current     int64
	values      []any
	err         error
	transaction *stroppy.DriverTransaction
	unit        *stroppy.UnitDescriptor
}

func NewStreamingCopySource(
	d *Driver,
	descriptor *stroppy.InsertDescriptor,
	count int64,
) *streamingCopySource {

	return &streamingCopySource{
		driver:  d,
		count:   count,
		current: 0,
		values:  make([]any, strings.Count(queries.BadInsertSQL(descriptor), " ")),
		unit: &stroppy.UnitDescriptor{
			Type: &stroppy.UnitDescriptor_Insert{
				Insert: descriptor,
			},
		},
	}
}

// Next advances to the next row.
func (s *streamingCopySource) Next() bool {
	if s.current >= s.count {
		return false
	}

	// NOTE: known that ctx not used at query generatin
	s.transaction, s.err = s.driver.GenerateNextUnit(nil, s.unit)
	if s.err != nil {
		return false
	}

	s.err = s.driver.fillParamsToValues(s.transaction.GetQueries()[0], s.values)
	if s.err != nil {
		return false
	}

	s.current++
	return true
}

// Values returns the values for the current row.
func (s *streamingCopySource) Values() ([]any, error) {
	return s.values, nil
}

// Err returns any error that occurred during iteration.
func (s *streamingCopySource) Err() error {
	return s.err
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
	queries := make([]*stroppy.DriverQueryStat, 0, len(transaction.GetQueries()))
	txStart := time.Now()

	for _, query := range transaction.GetQueries() {
		values := make([]any, len(query.GetParams()))

		err := d.fillParamsToValues(query, values)
		if err != nil {
			return nil, err
		}

		start := time.Now()

		switch query.GetMethod() {
		case stroppy.InsertMethod_PLAIN_QUERY:
			_, err := executor.Exec(ctx, query.GetRequest(), values...)
			if err != nil {
				return nil, err
			}
		case stroppy.InsertMethod_COPY_FROM:
			// NOTE: ignores tx_level and sends a data trought the dedicated connection
			err := d.CopyFromQuery(query)
			if err != nil {
				return nil, err
			}
		default:
			d.logger.Panic("unexpected proto.InsertMethod")
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

	d.logger.Debug("Driver Teardown Close copy channels")
	d.CloseCopyChannels()

	d.logger.Debug("Driver Teardown Close pgxpool")
	d.pgxPool.Close()

	d.logger.Debug("Driver Teardown End")

	return nil
}
