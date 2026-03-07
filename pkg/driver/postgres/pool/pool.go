package pool

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	pgxdecimal "github.com/jackc/pgx-shopspring-decimal"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
)

const (
	LoggerName       = "pgx-pool"
	DriverLoggerName = "postgres-driver"
)

var ErrUnsupportedParam = fmt.Errorf("unsupported parameter")

func NewPool(
	ctx context.Context,
	config *stroppy.DriverConfig,
	logger *zap.Logger,
) (*pgxpool.Pool, error) {
	parsedConfig, err := parseConfig(config, logger)
	if err != nil {
		return nil, err
	}

	pool, err := pgxpool.NewWithConfig(ctx, parsedConfig)
	if err != nil {
		return nil, err
	}

	return pool, nil
}

func parseConfig(
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

	// NOTE: Testing purpose default query execution mode is "exec".
	// Stroppy aim is to test database performance, not the driver.
	if cfg.ConnConfig.DefaultQueryExecMode == 0 {
		cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeExec
	}

	logLevel := "error"
	if pg != nil && pg.GetTraceLogLevel() != "" {
		logLevel = pg.GetTraceLogLevel()
	}

	loggerTracer, err := NewLoggerTracer(
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
	if modeStr == "" {
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
				"%q is valid only with default_query_exec_mode set to %q",
				"description_cache_capacity",
				pgx.QueryExecModeCacheDescribe.String(),
			)
		}
		cfg.ConnConfig.DescriptionCacheCapacity = int(v)
	}

	if v := pg.GetStatementCacheCapacity(); v != 0 {
		if mode != pgx.QueryExecModeCacheStatement {
			return fmt.Errorf(
				"%q is valid only with default_query_exec_mode set to %q",
				"statement_cache_capacity",
				pgx.QueryExecModeCacheStatement.String(),
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

	return 0, fmt.Errorf(`"%s" invalid for "default_query_exec_mode" key; supported values are %v: %w`,
		modeStr,
		slices.Collect(maps.Keys(optMap)),
		ErrUnsupportedParam,
	)
}
