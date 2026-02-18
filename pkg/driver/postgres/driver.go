package postgres

import (
	"context"
	"fmt"

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
		func(ctx context.Context, lg *zap.Logger, config *stroppy.DriverConfig) (driver.Driver, error) {
			return NewDriver(ctx, lg, config)
		},
	)
}

type Executor interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
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
	lg *zap.Logger,
	cfg *stroppy.DriverConfig,
) (d *Driver, err error) {
	d = &Driver{logger: lg}

	if lg == nil {
		d.logger = logger.NewFromEnv().
			Named(pool.DriverLoggerName).
			WithOptions(zap.AddCallerSkip(0))
	}

	d.pgxPool, err = pool.NewPool(ctx, cfg, d.logger.Named(pool.LoggerName))
	if err != nil {
		return nil, err
	}

	d.logger.Debug("Checking db connection...", zap.String("url", cfg.GetUrl()))

	// TODO: make waiting optional
	// TODO: think to float this waiting to the level of driver dispatching or k6-module
	err = waitForDB(ctx, d.logger, d.pgxPool, dbConnectionTimeout)
	if err != nil {
		return nil, err
	}

	return d, nil
}

func (d *Driver) Configure(ctx context.Context, opts driver.Options) (err error) {
	if opts.DialFunc != nil {
		cfg := d.pgxPool.Config()
		cfg.ConnConfig.DialFunc = opts.DialFunc

		d.pgxPool.Close()

		d.pgxPool, err = pgxpool.NewWithConfig(ctx, cfg)
		if err != nil {
			return fmt.Errorf("can't start reconfigured pgxpool: %w", err)
		}
	}

	return nil
}

func (d *Driver) Teardown(_ context.Context) error {
	d.logger.Debug("Driver Teardown Start")
	d.pgxPool.Close()
	d.logger.Debug("Driver Teardown End")

	return nil
}
