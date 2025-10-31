package application

import (
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"

	postgres "github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/postgresql"
	"github.com/stroppy-io/stroppy-cloud-panel/tools/sql/migrations"
)

const migrationsValkeyLockName = "komeet-migrations"

func NewDatabasePollWithTx(
	cfg *Config,
) (
	*pgxpool.Pool,
	error,
) {
	pgxPool, err := postgres.New(&cfg.Infra.Postgres)
	if err != nil {
		return nil, fmt.Errorf("failed to create Postgres pool: %w", err)
	}
	err = postgres.MigrateAtlas(pgxPool, migrations.Content)
	if err != nil {
		return nil, fmt.Errorf("failed to migrate Atlas: %w", err)
	}
	return pgxPool, LogicalMigrate(pgxPool)
}

func LogicalMigrate(pool *pgxpool.Pool) error {
	return nil
	//err := migrate.SetServerLimitsIfNotSet(exec, tx)
	//if err != nil {
	//	return err
	//}
	//return nil
}
