package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/pkg/common/logger"
	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"github.com/stroppy-io/stroppy/pkg/driver"
	"github.com/stroppy-io/stroppy/pkg/driver/postgres/pool"
	"github.com/stroppy-io/stroppy/pkg/driver/sqldriver"
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
	BeginTx(ctx context.Context, txOptions pgx.TxOptions) (pgx.Tx, error)
	Acquire(ctx context.Context) (*pgxpool.Conn, error)

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

func (d *Driver) Begin(ctx context.Context, isolation stroppy.TxIsolationLevel) (driver.Tx, error) {
	if isolation == stroppy.TxIsolationLevel_CONNECTION_ONLY {
		conn, err := d.pool.Acquire(ctx)
		if err != nil {
			return nil, err
		}

		return NewConnOnlyTx(conn, d.logger), nil
	}

	pgxTx, err := d.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: toTxIsoLevel(isolation)})
	if err != nil {
		return nil, err
	}

	return newTx(pgxTx, isolation, d), nil
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

