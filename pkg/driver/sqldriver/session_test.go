package sqldriver

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/v6/pkg/config"
	"github.com/stroppy-io/stroppy/v6/pkg/driver"
)

type sessionTestRows struct{ closed bool }

func (*sessionTestRows) Columns() []string   { return []string{"value"} }
func (*sessionTestRows) Next() bool          { return false }
func (*sessionTestRows) Values() []any       { return nil }
func (*sessionTestRows) ReadAll(int) [][]any { return nil }
func (*sessionTestRows) Err() error          { return nil }
func (r *sessionTestRows) Close() error {
	r.closed = true

	return nil
}

type sessionTestConnection struct {
	prepared, executed, statementClosed, committed, rolledBack, released int
	valid                                                                bool
	args                                                                 []any
	sql                                                                  string
	closeErr, commitErr                                                  error
}

func (c *sessionTestConnection) QueryContext(context.Context, string, ...any) (driver.Rows, error) {
	return &sessionTestRows{}, nil
}

func (c *sessionTestConnection) PrepareContext(_ context.Context, sql string) (PreparedQuery[driver.Rows], error) {
	c.prepared++
	c.sql = sql

	return &sessionTestStatement{c}, nil
}

func (c *sessionTestConnection) Begin(context.Context, config.TxIsolationLevel) (TxConn[driver.Rows], error) {
	return &sessionTestTransaction{c}, nil
}

func (c *sessionTestConnection) Close(context.Context) error {
	c.released++

	return nil
}
func (c *sessionTestConnection) Valid() bool { return c.valid }

type sessionTestStatement struct{ connection *sessionTestConnection }

func (p *sessionTestStatement) QueryContext(_ context.Context, args ...any) (driver.Rows, error) {
	p.connection.executed++
	p.connection.args = args

	return &sessionTestRows{}, nil
}

func (p *sessionTestStatement) Close(context.Context) error {
	p.connection.statementClosed++

	return p.connection.closeErr
}

type sessionTestTransaction struct{ *sessionTestConnection }

func (t *sessionTestTransaction) Commit(context.Context) error {
	t.committed++

	return t.commitErr
}

func (t *sessionTestTransaction) Rollback(context.Context) error {
	t.rolledBack++

	return nil
}

func TestSessionPreparesOnceAcrossTransactions(t *testing.T) {
	connection := &sessionTestConnection{valid: true}
	s := NewSession(connection, func(rows driver.Rows) driver.Rows { return rows }, qmark{}, zap.NewNop(), 0)
	p, err := s.Prepare(t.Context(), "update", "UPDATE t SET n = :n WHERE id = :id")
	require.NoError(t, err)
	require.Equal(t, "UPDATE t SET n = ? WHERE id = ?", connection.sql)

	for range 3 {
		tx, beginErr := s.Begin(t.Context(), config.TxIsolationLevelReadCommitted)
		require.NoError(t, beginErr)
		result, queryErr := tx.RunPrepared(t.Context(), p, map[string]any{"n": int64(2), "id": int64(1)})
		require.NoError(t, queryErr)
		require.NoError(t, result.Rows.Close())
		require.NoError(t, tx.Commit(t.Context()))
	}

	require.Equal(t, 1, connection.prepared)
	require.Equal(t, 3, connection.executed)
	require.Equal(t, []any{int64(2), int64(1)}, connection.args)
	require.NoError(t, p.Close(t.Context()))
	require.NoError(t, p.Close(t.Context()))
	_, err = p.RunQuery(t.Context(), nil)
	require.ErrorIs(t, err, driver.ErrPreparedClosed)
	require.NoError(t, s.Close(t.Context()))
	require.NoError(t, s.Close(t.Context()))
	require.Equal(t, 1, connection.statementClosed)
	require.Equal(t, 1, connection.released)
}

func TestSessionCursorAndTransactionCleanup(t *testing.T) {
	connection := &sessionTestConnection{valid: true}
	s := NewSession(connection, func(rows driver.Rows) driver.Rows { return rows }, qmark{}, zap.NewNop(), 0)
	p, err := s.Prepare(t.Context(), "one", "SELECT 1")
	require.NoError(t, err)
	tx, err := s.Begin(t.Context(), config.TxIsolationLevelReadCommitted)
	require.NoError(t, err)
	_, err = p.RunQuery(t.Context(), nil)
	require.ErrorIs(t, err, driver.ErrSessionTransaction)
	result, err := tx.RunPrepared(t.Context(), p, nil)
	require.NoError(t, err)
	require.ErrorIs(t, tx.Commit(t.Context()), driver.ErrSessionBusy)
	require.ErrorIs(t, p.Close(t.Context()), driver.ErrSessionBusy)
	require.NoError(t, s.Close(t.Context()))
	require.NoError(t, result.Rows.Close())
	require.Equal(t, 1, connection.rolledBack)
	require.Equal(t, 1, connection.statementClosed)
	require.Equal(t, 1, connection.released)
}

func TestPreparedCloseFailureRetainsCleanupOwnership(t *testing.T) {
	failure := errors.New("deallocation failed")
	connection := &sessionTestConnection{valid: true, closeErr: failure}
	s := NewSession(connection, func(rows driver.Rows) driver.Rows { return rows }, qmark{}, zap.NewNop(), 0)
	p, err := s.Prepare(t.Context(), "one", "SELECT 1")
	require.NoError(t, err)
	require.ErrorIs(t, p.Close(t.Context()), failure)
	require.Len(t, s.statements, 1)

	connection.closeErr = nil

	require.NoError(t, p.Close(t.Context()))
	require.Empty(t, s.statements)
	require.NoError(t, s.Close(t.Context()))
	require.Equal(t, 2, connection.statementClosed)
}

func TestSessionCommitFailureRollsBackBeforeReuse(t *testing.T) {
	failure := errors.New("commit failed")
	connection := &sessionTestConnection{valid: true, commitErr: failure}
	s := NewSession(connection, func(rows driver.Rows) driver.Rows { return rows }, qmark{}, zap.NewNop(), 0)
	tx, err := s.Begin(t.Context(), config.TxIsolationLevelReadCommitted)
	require.NoError(t, err)
	require.ErrorIs(t, tx.Commit(t.Context()), failure)
	require.Equal(t, 1, connection.rolledBack)
	require.NoError(t, s.Close(t.Context()))
}

func TestSessionInvalidatesWithoutReplay(t *testing.T) {
	connection := &sessionTestConnection{valid: true}
	s := NewSession(connection, func(rows driver.Rows) driver.Rows { return rows }, qmark{}, zap.NewNop(), 0)
	p, err := s.Prepare(t.Context(), "one", "SELECT 1")
	require.NoError(t, err)

	connection.valid = false
	_, err = p.RunQuery(t.Context(), nil)
	require.ErrorIs(t, err, driver.ErrSessionInvalidated)

	connection.valid = true
	_, err = p.RunQuery(t.Context(), nil)
	require.ErrorIs(t, err, driver.ErrSessionInvalidated)
	require.Zero(t, connection.executed)
	require.NoError(t, s.Close(t.Context()))
}
