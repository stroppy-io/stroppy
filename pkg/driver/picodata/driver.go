package picodata

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/picodata/picodata-go"
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/pkg/common/generate"
	"github.com/stroppy-io/stroppy/pkg/common/logger"
	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"github.com/stroppy-io/stroppy/pkg/driver"
	"github.com/stroppy-io/stroppy/pkg/driver/postgres"
	"github.com/stroppy-io/stroppy/pkg/driver/postgres/pool"
	"github.com/stroppy-io/stroppy/pkg/driver/sqldriver"
	sqlqueries "github.com/stroppy-io/stroppy/pkg/driver/sqldriver/queries"
	"github.com/stroppy-io/stroppy/pkg/driver/stats"
)

const (
	LoggerName          = "picodata-driver"
	dbConnectionTimeout = 5 * time.Second
)

func init() {
	driver.RegisterDriver(
		stroppy.DriverConfig_DRIVER_TYPE_PICODATA,
		func(ctx context.Context, opts driver.Options) (driver.Driver, error) {
			return NewDriver(ctx, opts)
		},
	)
}

type Executor interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	ExecContext(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryContext(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Ping(ctx context.Context) error
	Close()
	Config() *pgxpool.Config
}

// PoolX wraps *picodata.Pool and adds ExecContext/QueryContext to satisfy Executor.
type PoolX struct {
	*picodata.Pool
}

func (p *PoolX) ExecContext(
	ctx context.Context,
	sql string,
	args ...any,
) (pgconn.CommandTag, error) {
	return p.Exec(ctx, sql, args...)
}

func (p *PoolX) QueryContext(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return p.Query(ctx, sql, args...)
}

// Driver implements the driver.Driver interface for Picodata DB.
type Driver struct {
	logger   *zap.Logger
	pool     Executor
	bulkSize int
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

	const defaultBulkSize = 500

	cfg := opts.Config

	d = &Driver{
		logger:   lg,
		bulkSize: defaultBulkSize,
	}

	if cfg.BulkSize != nil {
		d.bulkSize = int(cfg.GetBulkSize())
	}

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

	d.pool = &PoolX{conn}

	d.logger.Debug("Checking db connection...", zap.String("url", cfg.GetUrl()))

	if err := sqldriver.WaitForDB(ctx, d.logger, d.pool, dbConnectionTimeout); err != nil {
		return nil, err
	}

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

// RunQuery executes sql with named :arg placeholders and returns rows cursor.
func (d *Driver) RunQuery(
	ctx context.Context,
	sql string,
	args map[string]any,
) (*driver.QueryResult, error) {
	return sqldriver.RunQuery(
		ctx,
		d.pool,
		postgres.NewRows,
		PicoDialect{},
		d.logger,
		sql,
		args,
	)
}

var ErrCopyFromUnsupported = errors.New("CopyFrom is not supported in Picodata yet")

// InsertValues inserts multiple rows into the database based on the descriptor.
// It supports two methods:
// - PLAIN_QUERY: executes individual INSERT statements for each row
// - PLAIN_BULK: executes batched bulk INSERT statements using multi-row VALUES syntax
// - COPY_FROM: unsupported.
func (d *Driver) InsertValues(
	ctx context.Context,
	descriptor *stroppy.InsertDescriptor,
) (*stats.Query, error) {
	builder, err := sqlqueries.NewQueryBuilder(
		d.logger,
		PicoDialect{},
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
		return nil, ErrCopyFromUnsupported
	default:
		d.logger.Panic("unexpected proto.InsertMethod")

		return nil, nil //nolint:nilnil // unreachable after panic
	}
}
