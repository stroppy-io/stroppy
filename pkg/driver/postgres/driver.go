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

	Ping(ctx context.Context) error
	Config() *pgxpool.Config
	Close()
}

type Driver struct {
	logger *zap.Logger
	pool   Executor
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

	d = &Driver{logger: lg}

	cfg := opts.Config

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
