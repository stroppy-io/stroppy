package postgres

import (
	"context"
	"net"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/pkg/common/logger"
	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"github.com/stroppy-io/stroppy/pkg/driver/postgres/pool"
)

// TODO: performance issue by passing via interface?

type Executor interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	CopyFrom(
		ctx context.Context,
		tableName pgx.Identifier,
		columnNames []string,
		rowSrc pgx.CopyFromSource,
	) (int64, error)

	Config() *pgxpool.Config
	Close()
}

type Driver struct {
	logger  *zap.Logger
	pgxPool Executor
}

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

	// TODO: make waiting optional
	// TODO: think to float this waiting to the level of driver dispatching or k6-module
	err = waitForDB(ctx, d.logger, connPool, dbConnectionTimeout)
	if err != nil {
		return nil, err
	}

	d.pgxPool = connPool

	return d, nil
}

func (d *Driver) UpdateDialler(
	ctx context.Context,
	dialFunc func(ctx context.Context, network, addr string) (net.Conn, error),
) (err error) {
	cfg := d.pgxPool.Config()
	cfg.ConnConfig.DialFunc = dialFunc

	d.pgxPool.Close()
	d.pgxPool, err = pgxpool.NewWithConfig(ctx, cfg)

	return err
}

func (d *Driver) Teardown(_ context.Context) error {
	d.logger.Debug("Driver Teardown Start")
	d.pgxPool.Close()
	d.logger.Debug("Driver Teardown End")

	return nil
}
