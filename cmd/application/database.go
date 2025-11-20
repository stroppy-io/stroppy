package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/orm"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/crossplane"

	postgres "github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/postgres"
	"github.com/stroppy-io/stroppy-cloud-panel/tools/sql/migrations"
)

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
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tx, err := pgxPool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	err = LogicalMigrate(ctx, pgxPool, cfg)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to migrate logical schema: %w",
			errors.Join(err, tx.Rollback(ctx)),
		)
	}
	return pgxPool, tx.Commit(ctx)
}

func LogicalMigrate(ctx context.Context, pool *pgxpool.Pool, cfg *Config) error {
	quotaTableRepo := orm.NewQuotaTableRepository(func(ctx context.Context, operation orm.SqlOpType) orm.DB {
		return pool
	})
	writeQuotas := func(ctx context.Context, cloud crossplane.SupportedCloud, quotas map[string]int) error {
		for kind, value := range quotas {
			if quotaKind, ok := crossplane.Quota_Kind_value[kind]; ok {
				return quotaTableRepo.Exec(ctx, orm.QuotaTable.Insert().
					From(
						orm.QuotaTable.Cloud.Set(int32(cloud)),
						orm.QuotaTable.Kind.Set(quotaKind),
						orm.QuotaTable.Maximum.Set(int32(value)),
					).
					OnConflict(
						orm.QuotaTable.Cloud,
						orm.QuotaTable.Kind,
					).DoNothing())
			} else {
				return fmt.Errorf("unknown Quota_Kind: %s", quotaKind)
			}
		}
		return nil
	}
	for cfg.Service.Cloud.Yandex.Quotas != nil {
		err := writeQuotas(
			ctx,
			crossplane.SupportedCloud_SUPPORTED_CLOUD_YANDEX,
			cfg.Service.Cloud.Yandex.Quotas,
		)
		if err != nil {
			return err
		}
	}
	return nil
	//err := migrate.SetServerLimitsIfNotSet(exec, tx)
	//if err != nil {
	//	return err
	//}
	//return nil
}
