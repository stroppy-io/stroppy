package picodata

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/picodata/picodata-go"
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/pkg/common/logger"
	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"github.com/stroppy-io/stroppy/pkg/driver"
	"github.com/stroppy-io/stroppy/pkg/driver/postgres/pool"
)

func init() {
	driver.RegisterDriver(
		stroppy.DriverConfig_DRIVER_TYPE_PICODATA,
		func(ctx context.Context, opts driver.Options) (driver.Driver, error) {
			return NewDriver(ctx, opts)
		},
	)
}

const (
	LoggerName = "picodata-driver"
)

type Executor interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Close()
	Config() *pgxpool.Config
}

// Driver implements the driver.Driver interface for Picodata DB.
type Driver struct {
	logger   *zap.Logger
	picoPool Executor
}

// NewDriver creates a new Picodata driver instance.
func NewDriver(
	ctx context.Context,
	opts driver.Options,
) (d *Driver, err error) {
	lg := opts.Logger
	if lg == nil {
		lg = logger.NewFromEnv().
			Named(LoggerName).
			WithOptions(zap.AddCallerSkip(0))
	}

	d = &Driver{logger: lg}

	cfg := opts.Config

	d.logger.Debug("Connecting to Picodata...", zap.String("url", cfg.GetUrl()))

	const maxConnPerInstance = 20

	parsedConfig, err := pool.ParseConfig(cfg, d.logger)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	conn, err := picodata.NewWithConfig(ctx,
		parsedConfig,
		picodata.WithDisableTopologyManaging(),
		picodata.WithMaxConnPerInstance(maxConnPerInstance),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Picodata: %w", err)
	}

	if opts.DialFunc != nil {
		poolCfg := conn.Config()
		poolCfg.ConnConfig.DialFunc = opts.DialFunc

		conn.Close()

		conn, err = picodata.NewWithConfig(ctx, poolCfg,
			picodata.WithDisableTopologyManaging(),
			picodata.WithMaxConnPerInstance(maxConnPerInstance),
		)
		if err != nil {
			return nil, fmt.Errorf("can't start reconfigured picodataPool: %w", err)
		}
	}

	d.picoPool = conn

	d.logger.Debug("Successfully connected to Picodata")

	return d, nil
}

// Teardown closes the connection to Picodata.
func (d *Driver) Teardown(_ context.Context) error {
	d.logger.Debug("Driver Teardown Start")

	if d.picoPool != nil {
		d.picoPool.Close()
	}

	d.logger.Debug("Driver Teardown End")

	return nil
}
