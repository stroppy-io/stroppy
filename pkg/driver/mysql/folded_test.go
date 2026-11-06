package mysql

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"testing"

	"github.com/stretchr/testify/require"
)

type foldedTestBackend struct {
	isolationTestConnection
	statements []string
	skip       bool
}

func (c *foldedTestBackend) QueryContext(
	ctx context.Context, query string, args []driver.NamedValue,
) (driver.Rows, error) {
	c.statements = append(c.statements, query)
	if c.skip && len(args) > 0 {
		return nil, driver.ErrSkip
	}

	return c.isolationTestConnection.QueryContext(ctx, query, args)
}

func (c *foldedTestBackend) ExecContext(
	_ context.Context, query string, args []driver.NamedValue,
) (driver.Result, error) {
	c.statements = append(c.statements, query)
	if c.skip && len(args) > 0 {
		return nil, driver.ErrSkip
	}

	return driver.RowsAffected(1), nil
}

type skippedPreparedStatement struct {
	mysqlStatement
	calls int
}

func (s *skippedPreparedStatement) ExecContext(context.Context, []driver.NamedValue) (driver.Result, error) {
	s.calls++

	return driver.RowsAffected(1), nil
}

func (s *skippedPreparedStatement) QueryContext(context.Context, []driver.NamedValue) (driver.Rows, error) {
	s.calls++

	return &isolationTestRows{value: int64(1)}, nil
}

func TestFoldedPreparedErrSkipFallsBack(t *testing.T) {
	for _, query := range []bool{false, true} {
		backend := &foldedTestBackend{skip: true}
		conn := &isolationConnection{mysqlConnection: backend, fold: true}
		tx, err := conn.BeginTx(t.Context(), driver.TxOptions{})
		require.NoError(t, err)

		prepared := &skippedPreparedStatement{}
		statement := &isolationStatement{mysqlStatement: prepared, connection: conn, query: "SELECT ?"}

		args := []driver.NamedValue{{Ordinal: 1, Value: "large"}}
		if query {
			rows, runErr := statement.QueryContext(t.Context(), args)
			require.NoError(t, runErr)
			require.NoError(t, rows.Close())
		} else {
			_, err = statement.ExecContext(t.Context(), args)
			require.NoError(t, err)
		}

		require.Equal(t, 1, prepared.calls)
		require.Equal(t, []string{"START TRANSACTION; SELECT ?", "START TRANSACTION"}, backend.statements)
		require.NoError(t, tx.Rollback())
	}
}

func TestFoldedBeginSharesFirstQuery(t *testing.T) {
	t.Parallel()

	backend := &foldedTestBackend{}
	conn := &isolationConnection{mysqlConnection: backend, fold: true, known: true, level: sql.LevelRepeatableRead}
	tx, err := conn.BeginTx(t.Context(), driver.TxOptions{Isolation: driver.IsolationLevel(sql.LevelRepeatableRead)})
	require.NoError(t, err)
	require.Empty(t, backend.statements)
	rows, err := conn.QueryContext(t.Context(), "SELECT 1", nil)
	require.NoError(t, err)
	require.NoError(t, rows.Close())
	require.Equal(t, []string{"START TRANSACTION; SELECT 1"}, backend.statements)
	require.NoError(t, tx.Commit())
	require.Equal(t, "COMMIT", backend.statements[1])
}

func TestFoldedEmptyTransactionDoesNotSendSQL(t *testing.T) {
	t.Parallel()

	backend := &foldedTestBackend{}
	conn := &isolationConnection{mysqlConnection: backend, fold: true}
	tx, err := conn.BeginTx(t.Context(), driver.TxOptions{})
	require.NoError(t, err)
	require.NoError(t, tx.Rollback())
	require.Empty(t, backend.statements)
	require.Nil(t, conn.pending)
}

func TestFoldedIsolationOverrideIsInFirstQuery(t *testing.T) {
	t.Parallel()

	backend := &foldedTestBackend{}
	conn := &isolationConnection{mysqlConnection: backend, fold: true, known: true, level: sql.LevelReadCommitted}
	tx, err := conn.BeginTx(t.Context(), driver.TxOptions{Isolation: driver.IsolationLevel(sql.LevelSerializable)})
	require.NoError(t, err)
	_, err = conn.ExecContext(t.Context(), "UPDATE t SET value=1", nil)
	require.NoError(t, err)
	require.Equal(t, "SET TRANSACTION ISOLATION LEVEL SERIALIZABLE; START TRANSACTION; UPDATE t SET value=1",
		backend.statements[0])
	require.NoError(t, tx.Rollback())
	require.Equal(t, "ROLLBACK", backend.statements[1])
}
