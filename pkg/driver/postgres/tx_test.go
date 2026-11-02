package postgres

import (
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/v6/pkg/config"
)

func TestTransactionWrapperKeepsIndependentBackends(t *testing.T) {
	firstPool, err := pgxmock.NewPool()
	require.NoError(t, err)

	defer firstPool.Close()

	secondPool, err := pgxmock.NewPool()
	require.NoError(t, err)

	defer secondPool.Close()

	firstPool.ExpectBeginTx(pgx.TxOptions{})
	firstBackend, err := firstPool.Begin(t.Context())
	require.NoError(t, err)
	secondPool.ExpectBeginTx(pgx.TxOptions{})
	secondBackend, err := secondPool.Begin(t.Context())
	require.NoError(t, err)

	d := &Driver{logger: zap.NewNop()}
	first := newTx(firstBackend, config.TxIsolationLevelReadCommitted, d)
	second := newTx(secondBackend, config.TxIsolationLevelSerializable, d)
	require.NotSame(t, first, second)
	require.Equal(t, config.TxIsolationLevelReadCommitted, first.Isolation())
	require.Equal(t, config.TxIsolationLevelSerializable, second.Isolation())
	firstPool.ExpectCommit()
	require.NoError(t, first.Commit(t.Context()))
	secondPool.ExpectRollback()
	require.NoError(t, second.Rollback(t.Context()))
	require.NoError(t, firstPool.ExpectationsWereMet())
	require.NoError(t, secondPool.ExpectationsWereMet())
}
