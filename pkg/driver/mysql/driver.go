package mysql

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	godriver "database/sql/driver"
	"fmt"
	"net"
	"os"
	"time"

	gomysql "github.com/go-sql-driver/mysql"
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/pkg/common/logger"
	"github.com/stroppy-io/stroppy/pkg/config"
	"github.com/stroppy-io/stroppy/pkg/driver"
	"github.com/stroppy-io/stroppy/pkg/driver/sqldriver"
	"github.com/stroppy-io/stroppy/pkg/driver/sqldriver/queries"
)

func init() {
	driver.RegisterDriver(
		config.DriverTypeMySQL,
		func(ctx context.Context, opts driver.Options) (driver.Driver, error) {
			return NewDriver(ctx, opts)
		},
	)
}

type Driver struct {
	db           *sql.DB
	dialect      queries.Dialect
	logger       *zap.Logger
	sqlCfg       *config.SQLConfig
	bulkSize     int
	queryTimeout time.Duration
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

	connector, err := prepareConnector(cfg, opts.DialFunc, lg)
	if err != nil {
		return nil, err
	}

	db := sql.OpenDB(connector)

	sqlCfg := cfg.SQL
	if err = sqldriver.ApplySQLConfig(db, sqlCfg); err != nil {
		db.Close()

		return nil, fmt.Errorf("failed to apply SQL config: %w", err)
	}

	lg.Debug("Checking db connection...", zap.String("url", cfg.URL))

	if err = sqldriver.WaitForDB(ctx, lg, &sqldriver.DBPinger{DB: db}, 0); err != nil {
		db.Close()

		return nil, err
	}

	const defaultBulkSize = 2500

	bulkSize := defaultBulkSize
	if cfg.BulkSize != nil {
		bulkSize = int(cfg.GetBulkSize())
	}

	return &Driver{
		db:           db,
		dialect:      mysqlDialect{},
		logger:       lg,
		sqlCfg:       sqlCfg,
		bulkSize:     bulkSize,
		queryTimeout: opts.QueryTimeout,
	}, nil
}

func prepareConnector(
	driverCfg *config.DriverConfig,
	dialFunc func(ctx context.Context, network, addr string) (net.Conn, error),
	lg *zap.Logger,
) (godriver.Connector, error) {
	mysqlCfg, err := gomysql.ParseDSN(driverCfg.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse mysql DSN: %w", err)
	}

	applySecurityOverrides(lg, mysqlCfg, driverCfg)

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

// applySecurityOverrides applies proto-level security fields to the mysql
// config. DSN-derived values take precedence: overrides are applied only
// when the DSN did not already set the corresponding field.
func applySecurityOverrides(
	lg *zap.Logger,
	mysqlCfg *gomysql.Config,
	driverCfg *config.DriverConfig,
) {
	// auth_user / auth_password — override only when DSN had no user.
	if u := driverCfg.GetAuthUser(); u != "" && mysqlCfg.User == "" {
		lg.Debug("Using auth_user from proto config", zap.String("user", u))

		mysqlCfg.User = u
		mysqlCfg.Passwd = driverCfg.GetAuthPassword()
	}

	// TLS overrides — only when DSN did not configure TLS
	// (TLSConfig == "" and TLS == nil means no TLS from DSN).
	caCert := driverCfg.GetCaCertFile()
	skipVerify := driverCfg.GetTLSInsecureSkipVerify()

	if caCert == "" && !skipVerify {
		return
	}

	if mysqlCfg.TLSConfig != "" || mysqlCfg.TLS != nil {
		lg.Debug("TLS already configured via DSN, skipping proto TLS overrides")

		return
	}

	host, _, err := net.SplitHostPort(mysqlCfg.Addr)
	if err != nil {
		host = mysqlCfg.Addr
	}

	tlsCfg := &tls.Config{
		ServerName: host,
	}

	if skipVerify {
		lg.Warn("TLS certificate verification disabled (insecure)")

		tlsCfg.InsecureSkipVerify = true
	}

	if caCert != "" {
		lg.Debug("Using CA certificate", zap.String("file", caCert))

		pem, err := os.ReadFile(caCert)
		if err != nil {
			lg.Error("Failed to read CA certificate file", zap.Error(err))

			return
		}

		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			lg.Error("CA certificate file contains no valid certificates", zap.String("file", caCert))

			return
		}

		tlsCfg.RootCAs = pool
	}

	mysqlCfg.TLS = tlsCfg
}

func (d *Driver) Begin(ctx context.Context, isolation config.TxIsolationLevel) (driver.Tx, error) {
	if isolation == config.TxIsolationLevelConnectionOnly {
		conn, err := d.db.Conn(ctx)
		if err != nil {
			return nil, err
		}

		return sqldriver.NewConnOnlyTx(conn, sqldriver.NewRows, d.dialect, d.logger, d.queryTimeout, conn.Close), nil
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
		d.queryTimeout,
	), nil
}

func (d *Driver) RunQuery(
	ctx context.Context,
	sqlStr string,
	args map[string]any,
) (*driver.QueryResult, error) {
	return sqldriver.RunQuery(ctx, d.db, sqldriver.NewRows, d.dialect, d.logger, sqlStr, args, d.queryTimeout)
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
