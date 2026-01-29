package picodata

import (
	"context"
	"fmt"

	"github.com/picodata/picodata-go"
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/pkg/common/logger"
	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	builder "github.com/stroppy-io/stroppy/pkg/driver/postgres/queries"
)

const (
	LoggerName = "picodata-driver"
)

type Pool interface {
	Exec(ctx context.Context, sql string, args ...any) (any, error)
	Close()
}

type poolAdapter struct {
	pool *picodata.Pool
}

func (p *poolAdapter) Exec(ctx context.Context, sql string, args ...any) (any, error) {
	return p.pool.Exec(ctx, sql, args...)
}

func (p *poolAdapter) Close() {
	p.pool.Close()
}

type QueryBuilder interface {
	Build(
		ctx context.Context,
		logger *zap.Logger,
		unit *stroppy.UnitDescriptor,
	) (*stroppy.DriverTransaction, error)
	AddGenerators(unit *stroppy.UnitDescriptor) error
	ValueToPgxValue(value *stroppy.Value) (any, error)
}

// Driver implements the driver.Driver interface for Picodata DB.
type Driver struct {
	logger  *zap.Logger
	pool    Pool
	builder QueryBuilder
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

	conn, err := picodata.New(ctx, cfg.GetUrl())
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Picodata: %w", err)
	}

	d.pool = &poolAdapter{pool: conn}

	queryBuilder, err := builder.NewQueryBuilder(0)
	if err != nil {
		return nil, err
	}
	d.builder = queryBuilder

	d.logger.Debug("Successfully connected to Picodata")

	return d, nil
}

// Teardown closes the connection to Picodata.
func (d *Driver) Teardown(_ context.Context) error {
	d.logger.Debug("Driver Teardown Start")

	if d.pool != nil {
		d.pool.Close()
	}

	d.logger.Debug("Driver Teardown End")

	return nil
}

func (d *Driver) GenerateNextUnit(
	ctx context.Context,
	unit *stroppy.UnitDescriptor,
) (*stroppy.DriverTransaction, error) {
	return d.builder.Build(ctx, d.logger, unit)
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
