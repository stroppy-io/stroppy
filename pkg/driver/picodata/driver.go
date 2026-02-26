package picodata

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/picodata/picodata-go"
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/pkg/common/logger"
	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"github.com/stroppy-io/stroppy/pkg/driver"
)

func init() {
	driver.RegisterDriver(
		stroppy.DriverConfig_DRIVER_TYPE_PICODATA,
		func(ctx context.Context, lg *zap.Logger, config *stroppy.DriverConfig) (driver.Driver, error) {
			return NewDriver(ctx, lg, config)
		},
	)
}

const (
	LoggerName = "picodata-driver"
)

type Executor interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Close()
	Config() *pgxpool.Config
}

// Driver implements the driver.Driver interface for Picodata DB.
type Driver struct {
	logger   *zap.Logger
	picoPool Executor
}

// NewDriver creates a new Picodata driver instance.
//
//nolint:nonamedreturns // named returns for defer error handling
func NewDriver(
	ctx context.Context,
	lg *zap.Logger,
	cfg *stroppy.DriverConfig,
) (d *Driver, err error) {
	if lg == nil {
		d = &Driver{
			logger: logger.NewFromEnv().
				Named(LoggerName).
				WithOptions(zap.AddCallerSkip(0)),
		}
	} else {
		d = &Driver{
			logger: lg,
		}
	}

	d.logger.Debug("Connecting to Picodata...", zap.String("url", cfg.GetUrl()))

	const (
		maxConnPerInstance = 20
	)

	conn, err := picodata.New(
		ctx,
		cfg.GetUrl(),
		picodata.WithDisableTopologyManaging(),
		picodata.WithMaxConnPerInstance(maxConnPerInstance),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Picodata: %w", err)
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

func (d *Driver) Configure(ctx context.Context, opts driver.Options) (err error) {
	if opts.DialFunc != nil {
		cfg := d.picoPool.Config()
		cfg.ConnConfig.DialFunc = opts.DialFunc

		d.picoPool.Close()

		newPool, err := picodata.NewWithConfig(ctx, cfg)
		if err != nil {
			return fmt.Errorf("can't start reconfigured picodataPool: %w", err)
		}

		d.picoPool = newPool
	}

	return nil
}
