package pool

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	pgxdecimal "github.com/jackc/pgx-shopspring-decimal"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/multitracer"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"github.com/stroppy-io/stroppy/pkg/utils/protovalue"
)

const (
	LoggerName       = "pgx-pool"
	DriverLoggerName = "postgres-driver"
)

const (
	traceLogLevelKey   = "trace_log_level"
	maxConnLifetimeKey = "max_conn_lifetime"
	maxConnIdleTimeKey = "max_conn_idle_time"
	maxConnsKey        = "max_conns"
	minConnsKey        = "min_conns"
	minIdleConnsKey    = "min_idle_conns"

	defaultQueryExecModeKey     = "default_query_exec_mode"
	descriptionCacheCapacityKey = "description_cache_capacity"
	statementCacheCapacityKey   = "statement_cache_capacity"
)

var (
	ErrUnsupportedParam                = errors.New("unsupported parameter")
	ErrDescriptionCacheCapacityMissUse = fmt.Errorf(
		"%q is valid only with %q set to %q",
		descriptionCacheCapacityKey,
		defaultQueryExecModeKey,
		pgx.QueryExecModeCacheDescribe.String(),
	)

	ErrStatementCacheCapacityMissUse = fmt.Errorf(
		"%q is valid only with %q set to %q",
		statementCacheCapacityKey,
		defaultQueryExecModeKey,
		pgx.QueryExecModeCacheStatement.String(),
	)
)

func parseConfig(
	config *stroppy.DriverConfig,
	logger *zap.Logger,
) (*pgxpool.Config, error) {
	cfgMap, err := protovalue.ValueStructToMap(config.GetDbSpecific())
	if err != nil {
		return nil, err
	}

	var cfg *pgxpool.Config
	if config.GetConnectionType().GetSingleConnPerVu() != nil {
		cfg, err = defaultSingleConnConfig(config.GetUrl())
		if err != nil {
			return nil, err
		}
	} else if shared := config.GetConnectionType().GetSharedPool(); shared != nil {
		cfg, err = defaultConfig(config.GetUrl())
		if err != nil {
			return nil, err
		}
		if shared.GetSharedConnections() != 0 {
			cfg.MaxConns = shared.SharedConnections
			cfg.MinConns = shared.SharedConnections
		} else {
			logger.Info("shared_connections set to default by pgx", zap.Int32("shared_connections", cfg.MaxConns))
		}
	} else {
		cfg, err = defaultConfig(config.GetUrl())
		if err != nil {
			return nil, err
		}
	}

	// Disable connection lifetime limits
	// NOTE: unfortunately "MaxConnLifetime = 0" != "no lifetime limits".
	// "MaxConnLifetime = 0" == spam with expired connections.
	if !strings.Contains(config.GetUrl(), "pool_max_conn_lifetime") {
		cfg.MaxConnLifetime = 24 * time.Hour // Nearly never
	}

	err = overrideWithDBSpecific(cfg, cfgMap)
	if err != nil {
		return nil, err
	}

	logLevel := "error"
	if overrideLevel, ok := cfgMap[traceLogLevelKey]; ok {
		logLevel, _ = overrideLevel.(string)
	}

	lvl, err := zapcore.ParseLevel(logLevel)
	if err != nil {
		return nil, err
	}

	loggerTracer, err := newLoggerTracer(
		logger.WithOptions(zap.AddCallerSkip(1), zap.IncreaseLevel(lvl)))
	if err != nil {
		return nil, err
	}

	cfg.ConnConfig.Tracer = multitracer.New(loggerTracer)
	cfg.AfterConnect = func(_ context.Context, conn *pgx.Conn) error {
		pgxdecimal.Register(conn.TypeMap())

		return nil
	}

	return cfg, nil
}

func overrideWithDBSpecific(
	cfg *pgxpool.Config,
	cfgMap map[string]any,
) error {
	if maxConnLifetime, ok := cfgMap[maxConnLifetimeKey]; ok {
		d, err := time.ParseDuration( //nolint:forcetypeassert // allow panic
			maxConnLifetime.(string), //nolint:errcheck // allow panic
		)
		if err != nil {
			return err
		}

		cfg.MaxConnLifetime = d
	}

	if maxConnIdleTime, ok := cfgMap[maxConnIdleTimeKey]; ok {
		d, err := time.ParseDuration( //nolint:forcetypeassert // allow panic
			maxConnIdleTime.(string), //nolint:errcheck // allow panic
		)
		if err != nil {
			return err
		}

		cfg.MaxConnIdleTime = d
	}

	if maxConns, ok := cfgMap[maxConnsKey]; ok {
		cfg.MaxConns, _ = maxConns.(int32)
	}

	if minConns, ok := cfgMap[minConnsKey]; ok {
		cfg.MinConns, _ = minConns.(int32)
	}

	if minIdleConns, ok := cfgMap[minIdleConnsKey]; ok {
		cfg.MinIdleConns, _ = minIdleConns.(int32)
	}

	if err := parsePgxOptimizations(cfgMap, cfg); err != nil {
		return err
	}

	return nil
}

func defaultConfig(url string) (*pgxpool.Config, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

func defaultSingleConnConfig(url string) (*pgxpool.Config, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}

	url = strings.ToLower(url)

	// Single stable connection, unless URL specified it
	if !strings.Contains(url, "pool_max_conns") {
		cfg.MaxConns = 1
	}

	if !strings.Contains(url, "pool_min_conns") {
		cfg.MinConns = 1
	}

	return cfg, nil
}

func parsePgxOptimizations(cfgMap map[string]any, cfg *pgxpool.Config) error {
	var (
		err                  error
		defaultQueryExecMode pgx.QueryExecMode
	)

	if rawAny, exists := cfgMap[defaultQueryExecModeKey]; exists {
		rawStr, _ := rawAny.(string)

		defaultQueryExecMode, err = parseDefaultQueryExecMode(rawStr)
		if err != nil {
			return err
		}

		cfg.ConnConfig.DefaultQueryExecMode = defaultQueryExecMode
	} else {
		// NOTE: Testing purpose default query execution mode is "exec".
		// Stroppy aim is to test database performance, not the driver.
		// So by default pgx's driver level optimizations disabled.
		// Second potentially useful value is "simple_protocol".
		// e.g. If some pg-like db not support extended binary protocol.
		cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeExec
	}

	if rawAny, exists := cfgMap[descriptionCacheCapacityKey]; exists {
		if defaultQueryExecMode != pgx.QueryExecModeCacheDescribe {
			return ErrDescriptionCacheCapacityMissUse
		}

		descriptionCacheCapacity, _ := rawAny.(int32)
		cfg.ConnConfig.DescriptionCacheCapacity = int(descriptionCacheCapacity)
	}

	if rawAny, exists := cfgMap[statementCacheCapacityKey]; exists {
		if defaultQueryExecMode != pgx.QueryExecModeCacheStatement {
			return ErrStatementCacheCapacityMissUse
		}

		statementCacheCapacity, _ := rawAny.(int32)
		cfg.ConnConfig.StatementCacheCapacity = int(statementCacheCapacity)
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

	return 0, fmt.Errorf(`"%s" invalid for "%s" key; supported values are %v: %w`,
		modeStr, defaultQueryExecModeKey,
		slices.Collect(maps.Keys(optMap)),
		ErrUnsupportedParam,
	)
}

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
