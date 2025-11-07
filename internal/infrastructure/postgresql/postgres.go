package postgres

import (
	"context"
	"io/fs"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5/multitracer"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/stroppy-io/stroppy-cloud-panel/internal/core/shutdown"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/postgresql/migrate/atlas"
)

const defaultMigrateDialect = "postgres"

type MigrationContent = fs.FS

func newPool(cfg *pgxpool.Config, loglevel string) (*pgxpool.Pool, error) {
	tr, err := newLoggerTracer(loglevel)
	if err != nil {
		return nil, err
	}
	cfg.ConnConfig.Tracer = multitracer.New(
		otelpgx.NewTracer(
			otelpgx.WithTrimSQLInSpanName(),
			otelpgx.WithIncludeQueryParameters(),
		), tr,
	)
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		return nil, err
	}
	registerMetrics(pool)
	shutdown.RegisterFn(pool.Close)
	return pool, nil
}

func NewFromString(config string, loglevel string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(config)
	if err != nil {
		return nil, err
	}
	return newPool(cfg, loglevel)
}

func New(config *Config) (*pgxpool.Pool, error) {
	cfg, err := config.Parse()
	if err != nil {
		return nil, err
	}
	return newPool(cfg, config.LogLevel)
}

func MigrateAtlas(pool *pgxpool.Pool, migrations ...MigrationContent) error {
	return atlas.AtlasMigrate(pool, migrations...)
}
