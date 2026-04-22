package ydb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"

	ydbsdk "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/config"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	"github.com/stroppy-io/stroppy/pkg/common/generate"
	"github.com/stroppy-io/stroppy/pkg/common/logger"
	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
	"github.com/stroppy-io/stroppy/pkg/driver"
	"github.com/stroppy-io/stroppy/pkg/driver/sqldriver"
	"github.com/stroppy-io/stroppy/pkg/driver/sqldriver/queries"
	"github.com/stroppy-io/stroppy/pkg/driver/stats"
)

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

	connOpts := buildConnectionOptions(lg, cfg, opts.DialFunc)

	nativeDB, err := ydbsdk.Open(ctx, cfg.GetUrl(), connOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to open ydb connection: %w", err)
	}

	connector, err := ydbsdk.Connector(nativeDB,
		ydbsdk.WithQueryService(true),
		ydbsdk.WithAutoDeclare(),
		ydbsdk.WithPositionalArgs(),
	)
	if err != nil {
		nativeDB.Close(ctx)

		return nil, fmt.Errorf("failed to create ydb connector: %w", err)
	}

	db := sql.OpenDB(connector)

	sqlCfg := cfg.GetSql()
	if err = sqldriver.ApplySQLConfig(db, sqlCfg); err != nil {
		db.Close()
		nativeDB.Close(ctx)

		return nil, fmt.Errorf("failed to apply SQL config: %w", err)
	}

	lg.Debug("Checking db connection...", zap.String("url", cfg.GetUrl()))

	if err = sqldriver.WaitForDB(ctx, lg, &sqldriver.DBPinger{DB: db}, 0); err != nil {
		db.Close()
		nativeDB.Close(ctx)

		return nil, err
	}

	const defaultBulkSize = 500

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
	case stroppy.InsertMethod_NATIVE:
		return d.insertValuesNative(ctx, builder)
	default:
		return nil, fmt.Errorf(
			"%w: %s",
			ErrUnsupportedInsertMethod,
			descriptor.GetMethod().String(),
		)
	}
}

// InsertSpec is not yet implemented for the ydb driver. The relational
// path lands per-driver in a later landing; until then this returns the
// framework's sentinel.
func (d *Driver) InsertSpec(
	_ context.Context,
	_ *dgproto.InsertSpec,
) (*stats.Query, error) {
	return nil, driver.ErrInsertSpecNotImplemented
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
