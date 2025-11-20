package postgres

import (
	"fmt"

	trmpgx "github.com/avito-tech/go-transaction-manager/pgxv5"
	"github.com/avito-tech/go-transaction-manager/trm/manager"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NewTXManager creates TX manager with pgxpool.Pool.
// Put constructor for TX manage here since manager depends on pgx5 driver
func NewTXManager(pool *pgxpool.Pool, settings *trmpgx.Settings) (*manager.Manager, error) {
	m, err := manager.New(trmpgx.NewDefaultFactory(pool), manager.WithSettings(settings))
	if err != nil {
		return nil, fmt.Errorf("manager.New: %w", err)
	}

	return m, nil
}

func GetGetter() *trmpgx.CtxGetter {
	return trmpgx.DefaultCtxGetter
}
