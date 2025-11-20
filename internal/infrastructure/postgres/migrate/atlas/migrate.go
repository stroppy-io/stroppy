package atlas

import (
	"context"
	"errors"
	"fmt"
	"io/fs"

	"ariga.io/atlas/sql/migrate"
	"ariga.io/atlas/sql/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy-cloud-panel/internal/core/logger"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/core/mfs"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/postgres/sqlexec"
)

func AtlasMigrate(pool *pgxpool.Pool, migrations ...fs.FS) error {
	lg := logger.NewStructLogger("atlas-migrate")
	revision, err := NewRevisionReaderWriter(sqlexec.NewTxExecutor(pool))
	if err != nil {
		return fmt.Errorf("failed to create revision reader writer in AtlasMigrate: %v", err)
	}
	executor := stdlib.OpenDBFromPool(pool)
	pg, err := postgres.Open(executor)
	if err != nil {
		return fmt.Errorf("failed to open postgres in AtlasMigrate: %v", err)
	}
	migrator, err := migrate.NewExecutor(pg,
		NewEmbedDir(mfs.MergeMultiple(migrations...)),
		revision,
		migrate.WithAllowDirty(true),
		migrate.WithLogger(newMigrationLogger(lg)),
	)
	if err != nil {
		return fmt.Errorf("failed to create migrator in AtlasMigrate: %v", err)
	}
	err = migrator.ExecuteN(context.Background(), 0)
	if err != nil {
		if errors.Is(err, migrate.ErrNoPendingFiles) {
			lg.Info("no pending migrations in AtlasMigrate to apply")
			return nil
		}
		lg.Error("failed to apply migrations in AtlasMigrate: %v", zap.Error(err))
		return err
	}
	return nil
}
