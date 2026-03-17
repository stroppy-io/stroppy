package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"time"

	gomysql "github.com/go-sql-driver/mysql"
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/pkg/common/generate"
	"github.com/stroppy-io/stroppy/pkg/common/logger"
	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"github.com/stroppy-io/stroppy/pkg/driver"
	"github.com/stroppy-io/stroppy/pkg/driver/sqldriver"
	"github.com/stroppy-io/stroppy/pkg/driver/sqldriver/queries"
	"github.com/stroppy-io/stroppy/pkg/driver/stats"
)

var ErrUnsupportedInsertMethod = errors.New("unsupported insert method for mysql driver")

func init() {
	driver.RegisterDriver(
		stroppy.DriverConfig_DRIVER_TYPE_MYSQL,
		func(ctx context.Context, opts driver.Options) (driver.Driver, error) {
			return NewDriver(ctx, opts)
		},
	)
}

type Driver struct {
	db      *sql.DB
	dialect queries.Dialect
	logger  *zap.Logger
	sqlCfg  *stroppy.DriverConfig_SqlConfig
}

var _ driver.Driver = (*Driver)(nil)

const dialerName = "stroppy"

func NewDriver(
	ctx context.Context,
	opts driver.Options,
) (*Driver, error) {
	lg := opts.Logger
	if lg == nil {
		lg = logger.NewFromEnv().Named("mysql")
	}

	cfg := opts.Config

	dsn, err := prepareDSN(cfg.GetUrl(), opts.DialFunc)
	if err != nil {
		return nil, err
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open mysql connection: %w", err)
	}

	sqlCfg := cfg.GetSql()
	applySQLConfig(db, sqlCfg)

	lg.Debug("Checking db connection...", zap.String("url", cfg.GetUrl()))

	if err = sqldriver.WaitForDB(ctx, lg, &dbPinger{db: db}, 0); err != nil {
		db.Close()

		return nil, err
	}

	return &Driver{
		db:      db,
		dialect: mysqlDialect{},
		logger:  lg,
		sqlCfg:  sqlCfg,
	}, nil
}

func prepareDSN(
	dsn string,
	dialFunc func(ctx context.Context, network, addr string) (net.Conn, error),
) (string, error) {
	if dialFunc == nil {
		return dsn, nil
	}

	mysqlCfg, err := gomysql.ParseDSN(dsn)
	if err != nil {
		return "", fmt.Errorf("failed to parse mysql DSN: %w", err)
	}

	mysqlCfg.DialFunc = dialFunc

	mysqlCfg.Net = dialerName

	return mysqlCfg.FormatDSN(), nil
}

func applySQLConfig(db *sql.DB, sqlCfg *stroppy.DriverConfig_SqlConfig) {
	if sqlCfg == nil {
		return
	}

	if sqlCfg.MaxOpenConns != nil {
		db.SetMaxOpenConns(int(sqlCfg.GetMaxOpenConns()))
	}

	if sqlCfg.MaxIdleConns != nil {
		db.SetMaxIdleConns(int(sqlCfg.GetMaxIdleConns()))
	}

	if sqlCfg.ConnMaxLifetime != nil {
		if d, err := time.ParseDuration(sqlCfg.GetConnMaxLifetime()); err == nil {
			db.SetConnMaxLifetime(d)
		}
	}

	if sqlCfg.ConnMaxIdleTime != nil {
		if d, err := time.ParseDuration(sqlCfg.GetConnMaxIdleTime()); err == nil {
			db.SetConnMaxIdleTime(d)
		}
	}
}

// dbPinger adapts *sql.DB to the Ping interface expected by WaitForDB.
type dbPinger struct {
	db *sql.DB
}

func (p *dbPinger) Ping(ctx context.Context) error {
	return p.db.PingContext(ctx)
}

func (d *Driver) InsertValues(
	ctx context.Context,
	descriptor *stroppy.InsertDescriptor,
) (*stats.Query, error) {
	builder, err := queries.NewQueryBuilder(
		d.logger,
		d.dialect,
		generate.ResolveSeed(descriptor.GetSeed()),
		descriptor,
	)
	if err != nil {
		return nil, fmt.Errorf("can't create query builder: %w", err)
	}

	switch descriptor.GetMethod() {
	case stroppy.InsertMethod_PLAIN_QUERY:
		return sqldriver.InsertPlainQuery(ctx, d.db, builder)
	case stroppy.InsertMethod_PLAIN_BULK:
		bulkSize := 1000
		if d.sqlCfg != nil && d.sqlCfg.BulkSize != nil {
			bulkSize = int(d.sqlCfg.GetBulkSize())
		}

		return sqldriver.InsertPlainBulk(ctx, d.db, builder, bulkSize)
	case stroppy.InsertMethod_COPY_FROM:
		return nil, fmt.Errorf("%w: COPY_FROM", ErrUnsupportedInsertMethod)
	default:
		return nil, fmt.Errorf(
			"%w: %s",
			ErrUnsupportedInsertMethod,
			descriptor.GetMethod().String(),
		)
	}
}

func (d *Driver) RunQuery(
	ctx context.Context,
	sqlStr string,
	args map[string]any,
) (*driver.QueryResult, error) {
	return sqldriver.RunQuery(ctx, d.db, d.dialect, d.logger, sqlStr, args)
}

func (d *Driver) Teardown(ctx context.Context) error {
	d.logger.Debug("Driver Teardown Start")
	err := sqldriver.Teardown(ctx, d.db)
	d.logger.Debug("Driver Teardown End")

	return err
}
