package ydb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"time"

	ydbsdk "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/config"
	yc "github.com/ydb-platform/ydb-go-yc-metadata"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	"github.com/stroppy-io/stroppy/pkg/common/logger"
	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"github.com/stroppy-io/stroppy/pkg/driver"
	"github.com/stroppy-io/stroppy/pkg/driver/sqldriver"
	"github.com/stroppy-io/stroppy/pkg/driver/sqldriver/queries"
)

const primaryConnectTimeout = 3 * time.Second

var ErrUnsupportedInsertMethod = errors.New("unsupported insert method for ydb driver")

func init() {
	driver.RegisterDriver(
		stroppy.DriverConfig_DRIVER_TYPE_YDB,
		func(ctx context.Context, opts driver.Options) (driver.Driver, error) {
			return NewDriver(ctx, opts)
		},
	)
}

type Driver struct {
	db       *sql.DB
	nativeDB *ydbsdk.Driver
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
		lg = logger.NewFromEnv().Named("ydb")
	}

	cfg := opts.Config
	sqlCfg := cfg.GetSql()
	connOpts := buildConnectionOptions(lg, cfg, opts.DialFunc)

	primaryCtx, cancelPrimary := context.WithTimeout(ctx, primaryConnectTimeout)
	db, nativeDB, primaryErr := tryConnect(primaryCtx, lg, cfg, sqlCfg, connOpts, primaryConnectTimeout)

	cancelPrimary()

	if primaryErr != nil {
		lg.Warn("primary auth failed, retrying with Yandex Cloud metadata service",
			zap.Error(primaryErr))

		ycOpts := []ydbsdk.Option{yc.WithCredentials(), yc.WithInternalCA()}
		fallbackOpts := make([]ydbsdk.Option, 0, len(connOpts)+len(ycOpts))
		fallbackOpts = append(fallbackOpts, connOpts...)
		fallbackOpts = append(fallbackOpts, ycOpts...)

		var fallbackErr error

		db, nativeDB, fallbackErr = tryConnect(ctx, lg, cfg, sqlCfg, fallbackOpts, 0)
		if fallbackErr != nil {
			return nil, errors.Join(primaryErr, fmt.Errorf("yc metadata fallback: %w", fallbackErr))
		}
	}

	const defaultBulkSize = 2500

	bulkSize := defaultBulkSize
	if cfg.BulkSize != nil {
		bulkSize = int(cfg.GetBulkSize())
	}

	return &Driver{
		db:       db,
		nativeDB: nativeDB,
		dialect:  ydbDialect{},
		logger:   lg,
		sqlCfg:   sqlCfg,
		bulkSize: bulkSize,
	}, nil
}

func tryConnect(
	ctx context.Context,
	lg *zap.Logger,
	cfg *stroppy.DriverConfig,
	sqlCfg *stroppy.DriverConfig_SqlConfig,
	connOpts []ydbsdk.Option,
	pingTimeout time.Duration,
) (*sql.DB, *ydbsdk.Driver, error) {
	nativeDB, err := ydbsdk.Open(ctx, cfg.GetUrl(), connOpts...)
	if err != nil {
		return nil, nil, fmt.Errorf("open ydb connection: %w", err)
	}

	connector, err := ydbsdk.Connector(nativeDB,
		ydbsdk.WithQueryService(true),
		ydbsdk.WithAutoDeclare(),
		ydbsdk.WithPositionalArgs(),
	)
	if err != nil {
		nativeDB.Close(ctx)

		return nil, nil, fmt.Errorf("create ydb connector: %w", err)
	}

	db := sql.OpenDB(connector)

	if err = sqldriver.ApplySQLConfig(db, sqlCfg); err != nil {
		db.Close()
		nativeDB.Close(ctx)

		return nil, nil, fmt.Errorf("apply SQL config: %w", err)
	}

	lg.Debug("Checking db connection...", zap.String("url", cfg.GetUrl()))

	if err = sqldriver.WaitForDB(ctx, lg, &sqldriver.DBPinger{DB: db}, pingTimeout); err != nil {
		db.Close()
		nativeDB.Close(ctx)

		return nil, nil, err
	}

	return db, nativeDB, nil
}

func buildConnectionOptions(
	lg *zap.Logger,
	cfg *stroppy.DriverConfig,
	dialFunc func(ctx context.Context, network, addr string) (net.Conn, error),
) []ydbsdk.Option {
	var opts []ydbsdk.Option

	if dialFunc != nil {
		opts = append(opts, ydbsdk.With(
			config.WithGrpcOptions(grpc.WithContextDialer(
				func(ctx context.Context, addr string) (net.Conn, error) {
					return dialFunc(ctx, "tcp", addr)
				},
			)),
		))
	}

	if f := cfg.GetCaCertFile(); f != "" {
		lg.Debug("Using CA certificate", zap.String("file", f))
		opts = append(opts, ydbsdk.WithCertificatesFromFile(f))
	}

	if cfg.GetTlsInsecureSkipVerify() {
		lg.Warn("TLS certificate verification disabled (insecure)")

		opts = append(opts, ydbsdk.WithTLSSInsecureSkipVerify())
	}

	switch {
	case cfg.GetAuthToken() != "":
		lg.Debug("Using token authentication")

		opts = append(opts, ydbsdk.WithAccessTokenCredentials(cfg.GetAuthToken()))
	case cfg.GetAuthUser() != "":
		lg.Debug("Using static credentials", zap.String("user", cfg.GetAuthUser()))
		opts = append(opts, ydbsdk.WithStaticCredentials(cfg.GetAuthUser(), cfg.GetAuthPassword()))
	}

	return opts
}

func (d *Driver) Begin(ctx context.Context, isolation stroppy.TxIsolationLevel) (driver.Tx, error) {
	if isolation == stroppy.TxIsolationLevel_CONNECTION_ONLY {
		conn, err := d.db.Conn(ctx)
		if err != nil {
			return nil, err
		}

		return sqldriver.NewConnOnlyTx(conn, sqldriver.NewRows, d.dialect, d.logger, conn.Close), nil
	}

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

	if d.nativeDB != nil {
		err = errors.Join(err, d.nativeDB.Close(ctx))
	}

	d.logger.Debug("Driver Teardown End")

	return err
}
