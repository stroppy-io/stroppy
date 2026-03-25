package mysql

import (
	"context"
	"database/sql"
	godriver "database/sql/driver"
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
	db       *sql.DB
	dialect  queries.Dialect
	logger   *zap.Logger
	sqlCfg   *stroppy.DriverConfig_SqlConfig
	bulkSize int
}

var _ driver.Driver = (*Driver)(nil)

func NewDriver(
	ctx context.Context,
	opts driver.Options,
) (*Driver, error) {
	lg := opts.Logger
	if lg == nil {
		lg = logger.NewFromEnv().Named("mysql")
	}

	cfg := opts.Config

	connector, err := prepareConnector(cfg.GetUrl(), opts.DialFunc, lg)
	if err != nil {
		return nil, err
	}

	db := sql.OpenDB(connector)

	sqlCfg := cfg.GetSql()
	applySQLConfig(db, sqlCfg)

	lg.Debug("Checking db connection...", zap.String("url", cfg.GetUrl()))

	if err = sqldriver.WaitForDB(ctx, lg, &dbPinger{db: db}, 0); err != nil {
		db.Close()

		return nil, err
	}

	const defaultBulkSize = 500

	bulkSize := defaultBulkSize
	if cfg.BulkSize != nil {
		bulkSize = int(cfg.GetBulkSize())
	}

	return &Driver{
		db:       db,
		dialect:  mysqlDialect{},
		logger:   lg,
		sqlCfg:   sqlCfg,
		bulkSize: bulkSize,
	}, nil
}

func prepareConnector(
	dsn string,
	dialFunc func(ctx context.Context, network, addr string) (net.Conn, error),
	lg *zap.Logger,
) (godriver.Connector, error) {
	mysqlCfg, err := gomysql.ParseDSN(dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to parse mysql DSN: %w", err)
	}

	if dialFunc != nil {
		mysqlCfg.DialFunc = dialFunc
	}

	if lg != nil {
		mysqlCfg.Logger = &zapMySQLLogger{z: lg}
	}

	connector, err := gomysql.NewConnector(mysqlCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create mysql connector: %w", err)
	}

	return connector, nil
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

func (d *Driver) Begin(ctx context.Context, isolation stroppy.TxIsolationLevel) (driver.Tx, error) {
	sqlTx, err := d.db.BeginTx(ctx, &sql.TxOptions{Isolation: sqldriver.IsolationToSQL(isolation)})
	if err != nil {
		return nil, err
	}

	return sqldriver.NewTx(
		&sqldriver.SQLTxAdapter{Tx: sqlTx},
		sqldriver.NewRows,
		isolation,
		d.dialect,
		d.logger,
	), nil
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
		return sqldriver.InsertPlainBulk(ctx, d.db, builder, d.bulkSize)
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
	return sqldriver.RunQuery(ctx, d.db, sqldriver.NewRows, d.dialect, d.logger, sqlStr, args)
}

func (d *Driver) Teardown(ctx context.Context) error {
	d.logger.Debug("Driver Teardown Start")
	err := sqldriver.Teardown(ctx, d.db)
	d.logger.Debug("Driver Teardown End")

	return err
}

type zapMySQLLogger struct{ z *zap.Logger }

func (l *zapMySQLLogger) Print(v ...any) {
	l.z.Error("mysql", zap.String("msg", fmt.Sprint(v...)))
}
