package pool

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
	"time"

	pgxdecimal "github.com/jackc/pgx-shopspring-decimal"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
)

const (
	LoggerName       = "pgx-pool"
	DriverLoggerName = "postgres-driver"
)

var (
	ErrUnsupportedParam        = errors.New("unsupported parameter")
	ErrInvalidExecModeForParam = errors.New(
		"parameter is valid only with specific default_query_exec_mode",
	)
)

type PoolX struct {
	*pgxpool.Pool
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

func NewPool(
	ctx context.Context,
	config *stroppy.DriverConfig,
	logger *zap.Logger,
) (*PoolX, error) {
	parsedConfig, err := ParseConfig(config, logger)
	if err != nil {
		return nil, err
	}

	pool, err := pgxpool.NewWithConfig(ctx, parsedConfig)
	if err != nil {
		return nil, err
	}

	return &PoolX{pool}, nil
}

func NewWithConfig(ctx context.Context, config *pgxpool.Config) (*PoolX, error) {
	pool, err := pgxpool.NewWithConfig(ctx, config)

	return &PoolX{pool}, err
}

func ParseConfig(
	config *stroppy.DriverConfig,
	logger *zap.Logger,
) (*pgxpool.Config, error) {
	cfg, err := pgxpool.ParseConfig(config.GetUrl())
	if err != nil {
		return nil, err
	}

	// Disable connection lifetime limits
	// NOTE: unfortunately "MaxConnLifetime = 0" != "no lifetime limits".
	// "MaxConnLifetime = 0" == spam with expired connections.
	if !strings.Contains(config.GetUrl(), "pool_max_conn_lifetime") {
		const oneDay = 24 * time.Hour

		cfg.MaxConnLifetime = oneDay // Nearly never
	}

	pg := config.GetPostgres()
	if pg != nil {
		if err := applyPostgresConfig(cfg, pg); err != nil {
			return nil, err
		}
	}

	logLevel := "error"
	if pg != nil && pg.GetTraceLogLevel() != "" {
		logLevel = pg.GetTraceLogLevel()
	}

	applySecurityOverrides(logger, cfg.ConnConfig, config)

	loggerTracer, err := newLoggerTracer(
		logger.WithOptions(zap.AddCallerSkip(1), zap.IncreaseLevel(mustParseLevel(logLevel))))
	if err != nil {
		return nil, err
	}

	cfg.ConnConfig.Tracer = loggerTracer
	cfg.AfterConnect = func(_ context.Context, conn *pgx.Conn) error {
		pgxdecimal.Register(conn.TypeMap())

		return nil
	}

	return cfg, nil
}

func mustParseLevel(s string) zap.AtomicLevel {
	lvl, err := zap.ParseAtomicLevel(s)
	if err != nil {
		return zap.NewAtomicLevelAt(zap.ErrorLevel)
	}

	return lvl
}

func applyPostgresConfig(cfg *pgxpool.Config, pg *stroppy.DriverConfig_PostgresConfig) error {
	if v := pg.GetMaxConnLifetime(); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return err
		}

		cfg.MaxConnLifetime = d
	}

	if v := pg.GetMaxConnIdleTime(); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return err
		}

		cfg.MaxConnIdleTime = d
	}

	if v := pg.GetMaxConns(); v != 0 {
		cfg.MaxConns = v
	}

	if v := pg.GetMinConns(); v != 0 {
		cfg.MinConns = v
	}

	if v := pg.GetMinIdleConns(); v != 0 {
		cfg.MinIdleConns = v
	}

	if err := parsePgxOptimizations(pg, cfg); err != nil {
		return err
	}

	return nil
}

func parsePgxOptimizations(pg *stroppy.DriverConfig_PostgresConfig, cfg *pgxpool.Config) error {
	modeStr := pg.GetDefaultQueryExecMode()
	if modeStr == "" { // set by default
		// NOTE: Testing purpose default query execution mode is "exec".
		// Stroppy aim is to test database performance, not the driver.
		cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeExec

		return nil
	}

	mode, err := parseDefaultQueryExecMode(modeStr)
	if err != nil {
		return err
	}

	cfg.ConnConfig.DefaultQueryExecMode = mode

	if v := pg.GetDescriptionCacheCapacity(); v != 0 {
		if mode != pgx.QueryExecModeCacheDescribe {
			return fmt.Errorf(
				"%q is valid only with default_query_exec_mode set to %q: %w",
				"description_cache_capacity",
				pgx.QueryExecModeCacheDescribe.String(),
				ErrInvalidExecModeForParam,
			)
		}

		cfg.ConnConfig.DescriptionCacheCapacity = int(v)
	}

	if v := pg.GetStatementCacheCapacity(); v != 0 {
		if mode != pgx.QueryExecModeCacheStatement {
			return fmt.Errorf(
				"%q is valid only with default_query_exec_mode set to %q: %w",
				"statement_cache_capacity",
				pgx.QueryExecModeCacheStatement.String(),
				ErrInvalidExecModeForParam,
			)
		}

		cfg.ConnConfig.StatementCacheCapacity = int(v)
	}

	return nil
}

func parseDefaultQueryExecMode(modeStr string) (pgx.QueryExecMode, error) {
	optMap := map[string]pgx.QueryExecMode{
		"cache_statement": pgx.QueryExecModeCacheStatement,
		"cache_describe":  pgx.QueryExecModeCacheDescribe,
		"describe_exec":   pgx.QueryExecModeDescribeExec,
		"exec":            pgx.QueryExecModeExec,
		"simple_protocol": pgx.QueryExecModeSimpleProtocol,
	}
	if mode, exists := optMap[modeStr]; exists {
		return mode, nil
	}

	return 0, fmt.Errorf(
		`"%s" invalid for "default_query_exec_mode" key; supported values are %v: %w`,
		modeStr,
		slices.Collect(maps.Keys(optMap)),
		ErrUnsupportedParam,
	)
}

// applySecurityOverrides applies proto-level security fields to the pgx
// connection config. DSN-derived values take precedence: overrides are
// applied only when the DSN did not already set the corresponding field.
func applySecurityOverrides(
	lg *zap.Logger,
	connCfg *pgx.ConnConfig,
	driverCfg *stroppy.DriverConfig,
) {
	// auth_user / auth_password — override only when DSN had no user.
	if u := driverCfg.GetAuthUser(); u != "" && connCfg.User == "" {
		lg.Debug("Using auth_user from proto config", zap.String("user", u))

		connCfg.User = u
		connCfg.Password = driverCfg.GetAuthPassword()
	}

	// TLS overrides — only when DSN did not configure TLS (sslmode=disable
	// or absent → TLSConfig == nil).
	caCert := driverCfg.GetCaCertFile()
	skipVerify := driverCfg.GetTlsInsecureSkipVerify()

	if caCert == "" && !skipVerify {
		return
	}

	if connCfg.TLSConfig != nil {
		lg.Debug("TLS already configured via DSN, skipping proto TLS overrides")

		return
	}

	tlsCfg := &tls.Config{
		ServerName: connCfg.Host,
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

	connCfg.TLSConfig = tlsCfg
}
