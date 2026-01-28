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
		`"%s" is valid only with "%s" set to "%s"`,
		descriptionCacheCapacityKey,
		defaultQueryExecModeKey,
		pgx.QueryExecModeCacheDescribe.String(),
	)

	ErrStatementCacheCapacityMissUse = fmt.Errorf(
		`"%s" is valid only with "%s" set to "%s"`,
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

	cfg, err := defaultPool(config.GetUrl())
	if err != nil {
		return nil, err
	}

	cfg, err = overrideWithDBSpecific(logger, cfg, cfgMap)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

func overrideWithDBSpecific(
	logger *zap.Logger,
	cfg *pgxpool.Config,
	cfgMap map[string]any,
) (*pgxpool.Config, error) {
	logLevel, ok := cfgMap[traceLogLevelKey]
	if !ok {
		logLevel = "error"
	}

	lvl, err := zapcore.ParseLevel( //nolint: forcetypeassert // allow panic
		logLevel.(string), //nolint: errcheck // allow panic
	)
	if err != nil {
		return nil, err
	}

	loggerTracer, err := newLoggerTracer(logger.WithOptions(
		zap.AddCallerSkip(1),
		zap.IncreaseLevel(lvl)))
	if err != nil {
		return nil, err
	}

	cfg.ConnConfig.Tracer = multitracer.New(loggerTracer)

	if maxConnLifetime, ok := cfgMap[maxConnLifetimeKey]; ok {
		d, err := time.ParseDuration( //nolint: forcetypeassert // allow panic
			maxConnLifetime.(string), //nolint: errcheck // allow panic
		)
		if err != nil {
			return nil, err
		}

		cfg.MaxConnLifetime = d
	}

	if maxConnIdleTime, ok := cfgMap[maxConnIdleTimeKey]; ok {
		d, err := time.ParseDuration( //nolint: forcetypeassert // allow panic
			maxConnIdleTime.(string), //nolint: errcheck // allow panic
		)
		if err != nil {
			return nil, err
		}

		cfg.MaxConnIdleTime = d
	}

	if maxConns, ok := cfgMap[maxConnsKey]; ok {
		cfg.MaxConns = maxConns.(int32) //nolint: errcheck,forcetypeassert // allow panic
	}

	if minConns, ok := cfgMap[minConnsKey]; ok {
		cfg.MinConns = minConns.(int32) //nolint: errcheck,forcetypeassert // allow panic
	}

	if minIdleConns, ok := cfgMap[minIdleConnsKey]; ok {
		cfg.MinIdleConns = minIdleConns.(int32) //nolint: errcheck,forcetypeassert // allow panic
	}

	if err = parsePgxOptimizations(cfgMap, cfg); err != nil {
		return nil, err
	}

	cfg.AfterConnect = func(_ context.Context, conn *pgx.Conn) error {
		pgxdecimal.Register(conn.TypeMap())

		return nil
	}

	return cfg, nil
}

func defaultPool(url string) (*pgxpool.Config, error) {
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
		cfg.MinConns = 0
	}

	// Disable connection lifetime limits
	if !strings.Contains(url, "pool_max_conn_lifetime") {
		cfg.MaxConnLifetime = 0 // Unlimited
	}

	if !strings.Contains(url, "pool_max_conn_idle_time") {
		cfg.MaxConnIdleTime = 0 // Never close idle connections
	}

	if !strings.Contains(url, "pool_health_check_period") {
		cfg.HealthCheckPeriod = 5 * time.Minute // Less frequent
	}

	return cfg, nil
}

func parsePgxOptimizations(cfgMap map[string]any, cfg *pgxpool.Config) error {
	var (
		err                  error
		defaultQueryExecMode pgx.QueryExecMode
	)

	if rawAny, exists := cfgMap[defaultQueryExecModeKey]; exists {
		rawStr := rawAny.(string) //nolint: errcheck,forcetypeassert // allow panic

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

		descriptionCacheCapacity := rawAny.(int32) //nolint: errcheck,forcetypeassert // allow panic
		cfg.ConnConfig.DescriptionCacheCapacity = int(descriptionCacheCapacity)
	}

	if rawAny, exists := cfgMap[statementCacheCapacityKey]; exists {
		if defaultQueryExecMode != pgx.QueryExecModeCacheStatement {
			return ErrStatementCacheCapacityMissUse
		}

		statementCacheCapacity := rawAny.(int32) //nolint: errcheck,forcetypeassert // allow panic
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
