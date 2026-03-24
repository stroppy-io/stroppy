package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/pkg/common/generate"
	"github.com/stroppy-io/stroppy/pkg/common/logger"
	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"github.com/stroppy-io/stroppy/pkg/driver"
	"github.com/stroppy-io/stroppy/pkg/driver/postgres/pool"
	"github.com/stroppy-io/stroppy/pkg/driver/sqldriver"
	sqlqueries "github.com/stroppy-io/stroppy/pkg/driver/sqldriver/queries"
	"github.com/stroppy-io/stroppy/pkg/driver/stats"
)

const dbConnectionTimeout = 5 * time.Second

func init() {
	driver.RegisterDriver(
		stroppy.DriverConfig_DRIVER_TYPE_POSTGRES,
		func(ctx context.Context, opts driver.Options) (driver.Driver, error) {
			return NewDriver(ctx, opts)
		},
	)
}

type Executor interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	ExecContext(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryContext(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	CopyFrom(
		ctx context.Context,
		tableName pgx.Identifier,
		columnNames []string,
		rowSrc pgx.CopyFromSource,
	) (int64, error)

	Ping(ctx context.Context) error
	Config() *pgxpool.Config
	Close()
}

type Driver struct {
	logger   *zap.Logger
	pool     Executor
	bulkSize int
}

var _ driver.Driver = new(Driver)

func NewDriver(
	ctx context.Context,
	opts driver.Options,
) (d *Driver, err error) {
	lg := opts.Logger
	if lg == nil {
		lg = logger.NewFromEnv().
			Named(pool.DriverLoggerName).
			WithOptions(zap.AddCallerSkip(0))
	}

	const defaultBulkSize = 500

	d = &Driver{
		logger:   lg,
		bulkSize: defaultBulkSize,
	}

	cfg := opts.Config

	if cfg.BulkSize != nil {
		d.bulkSize = int(cfg.GetBulkSize())
	}

	d.pool, err = pool.NewPool(ctx, cfg, d.logger.Named(pool.LoggerName))
	if err != nil {
		return nil, err
	}

	// Apply DialFunc if provided (for k6 network metrics)
	if opts.DialFunc != nil {
		poolCfg := d.pool.Config()
		poolCfg.ConnConfig.DialFunc = opts.DialFunc

		d.pool.Close()

		d.pool, err = pool.NewWithConfig(ctx, poolCfg)
		if err != nil {
			return nil, err
		}
	}

	d.logger.Debug("Checking db connection...", zap.String("url", cfg.GetUrl()))

	// TODO: make waiting optional
	err = sqldriver.WaitForDB(ctx, d.logger, d.pool, dbConnectionTimeout)
	if err != nil {
		return nil, err
	}

	return d, nil
}

func (d *Driver) Teardown(_ context.Context) error {
	d.logger.Debug("Driver Teardown Start")
	d.pool.Close()
	d.logger.Debug("Driver Teardown End")

	return nil
}

// RunQuery executes sql with named :arg placeholders and returns rows cursor.
func (d *Driver) RunQuery(
	ctx context.Context,
	sql string,
	args map[string]any,
) (*driver.QueryResult, error) {
	return sqldriver.RunQuery(ctx, d.pool, NewRows, PgxDialect{}, d.logger, sql, args)
}

// InsertValues inserts multiple rows into the database based on the descriptor.
// It supports three methods:
// - PLAIN_QUERY: executes individual INSERT statements for each row
// - PLAIN_BULK: executes batched bulk INSERT statements using multi-row VALUES syntax
// - COPY_FROM: uses PostgreSQL's COPY protocol for fast bulk insertion.
func (d *Driver) InsertValues(
	ctx context.Context,
	descriptor *stroppy.InsertDescriptor,
) (*stats.Query, error) {
	builder, err := sqlqueries.NewQueryBuilder(
		d.logger,
		PgxDialect{},
		generate.ResolveSeed(descriptor.GetSeed()),
		descriptor,
	)
	if err != nil {
		return nil, fmt.Errorf("can't create query builder: %w", err)
	}

	switch descriptor.GetMethod() {
	case stroppy.InsertMethod_PLAIN_QUERY:
		return sqldriver.InsertPlainQuery(ctx, d.pool, builder)
	case stroppy.InsertMethod_PLAIN_BULK:
		return sqldriver.InsertPlainBulk(ctx, d.pool, builder, d.bulkSize)
	case stroppy.InsertMethod_COPY_FROM:
		return d.insertValuesCopyFrom(ctx, builder)
	default:
		d.logger.Panic("unexpected proto.InsertMethod")

		return nil, nil //nolint:nilnil // unreachable after panic
	}
}

// insertValuesCopyFrom uses PostgreSQL's COPY protocol for fast bulk insertion.
// It streams values on-demand without loading all rows into memory.
func (d *Driver) insertValuesCopyFrom(
	ctx context.Context,
	builder *sqlqueries.QueryBuilder,
) (*stats.Query, error) {
	cols := builder.Columns()
	stream := newStreamingCopySource(builder)
	start := time.Now()

	if _, err := d.pool.CopyFrom(ctx, pgx.Identifier{builder.TableName()}, cols, stream); err != nil {
		return nil, err
	}

	return &stats.Query{Elapsed: time.Since(start)}, nil
}
