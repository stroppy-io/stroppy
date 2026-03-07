package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/pkg/common/logger"
	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"github.com/stroppy-io/stroppy/pkg/driver"
	"github.com/stroppy-io/stroppy/pkg/driver/postgres/pool"
)

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
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
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
	logger  *zap.Logger
	pgxPool Executor
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

	d.pgxPool, err = pool.NewPool(ctx, cfg, d.logger.Named(pool.LoggerName))
	if err != nil {
		return nil, err
	}

	// Apply DialFunc if provided (for k6 network metrics)
	if opts.DialFunc != nil {
		poolCfg := d.pgxPool.Config()
		poolCfg.ConnConfig.DialFunc = opts.DialFunc

		d.pgxPool.Close()

		d.pgxPool, err = pgxpool.NewWithConfig(ctx, poolCfg)
		if err != nil {
			return nil, err
		}
	}

	d.logger.Debug("Checking db connection...", zap.String("url", cfg.GetUrl()))

	// TODO: make waiting optional
	err = waitForDB(ctx, d.logger, d.pgxPool, dbConnectionTimeout)
	if err != nil {
		return nil, err
	}

	return d, nil
}

func (d *Driver) Teardown(_ context.Context) error {
	d.logger.Debug("Driver Teardown Start")
	d.pgxPool.Close()
	d.logger.Debug("Driver Teardown End")

	return nil
}
