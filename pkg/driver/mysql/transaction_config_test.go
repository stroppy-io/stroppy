package mysql

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/v6/pkg/config"
)

func TestConnectorTransactionSettings(t *testing.T) {
	t.Parallel()

	for _, fold := range []bool{true, false} {
		level := "serializable"
		cfg := &config.DriverConfig{
			URL:          "user:password@tcp(localhost:3306)/db?parseTime=true",
			Transactions: &config.TransactionConfig{DefaultIsolation: &level, FoldBegin: &fold},
		}
		connector, err := prepareConnector(cfg, nil, zap.NewNop())
		require.NoError(t, err)

		wrapped, ok := connector.(isolationConnector)
		require.True(t, ok)
		require.Equal(t, fold, wrapped.fold)
		require.Equal(t, config.TxIsolationLevelSerializable, wrapped.defaultIsolation)
	}
}

func TestFoldedCancellationFailsBeforeQuery(t *testing.T) {
	t.Parallel()

	backend := &isolationTestConnection{}
	conn := &isolationConnection{mysqlConnection: backend, fold: true}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := conn.BeginTx(ctx, driver.TxOptions{Isolation: driver.IsolationLevel(sql.LevelSerializable)})
	require.ErrorIs(t, err, context.Canceled)
}
