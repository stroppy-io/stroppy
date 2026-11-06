package mysql

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"testing"

	gomysql "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
)

type isolationTestConnection struct {
	mysqlConnection
	begins   []driver.TxOptions
	value    any
	queryErr error
	closed   bool
	queries  int
}

func (c *isolationTestConnection) BeginTx(_ context.Context, opts driver.TxOptions) (driver.Tx, error) {
	c.begins = append(c.begins, opts)

	return isolationTestTx{}, nil
}

type isolationTestTx struct{}

func (isolationTestTx) Commit() error   { return nil }
func (isolationTestTx) Rollback() error { return nil }

func (c *isolationTestConnection) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	c.queries++

	return &isolationTestRows{value: c.value}, c.queryErr
}

func (c *isolationTestConnection) ExecContext(context.Context, string, []driver.NamedValue) (driver.Result, error) {
	return driver.RowsAffected(0), nil
}

func (c *isolationTestConnection) Close() error {
	c.closed = true

	return nil
}

type isolationTestRows struct {
	value any
	read  bool
}

func (*isolationTestRows) Columns() []string { return []string{"isolation"} }
func (*isolationTestRows) Close() error      { return nil }
func (r *isolationTestRows) Next(values []driver.Value) error {
	if r.read {
		return io.EOF
	}

	r.read = true
	values[0] = r.value

	return nil
}

type isolationTestConnector struct {
	driver.Connector
	connection *isolationTestConnection
}

func (c isolationTestConnector) Connect(context.Context) (driver.Conn, error) {
	return c.connection, nil
}

func TestIsolationConnectorReadsEachPhysicalConnection(t *testing.T) {
	t.Parallel()

	for _, value := range []any{"REPEATABLE-READ", []byte("REPEATABLE-READ")} {
		backend := &isolationTestConnection{value: value}
		connector := isolationConnector{isolationTestConnector{connection: backend}}
		conn, err := connector.Connect(t.Context())
		require.NoError(t, err)
		require.Equal(t, 1, backend.queries)

		begin, ok := conn.(driver.ConnBeginTx)
		require.True(t, ok)

		for _, level := range []sql.IsolationLevel{sql.LevelRepeatableRead, sql.LevelReadCommitted, sql.LevelDefault} {
			_, err = begin.BeginTx(t.Context(), driver.TxOptions{Isolation: driver.IsolationLevel(level)})
			require.NoError(t, err)
		}

		expected := []driver.TxOptions{{}, {Isolation: driver.IsolationLevel(sql.LevelReadCommitted)}, {}}
		require.Equal(t, expected, backend.begins)
		require.Equal(t, 1, backend.queries)

		backend.value = "READ-COMMITTED"
		conn, err = connector.Connect(t.Context())
		require.NoError(t, err)

		begin, ok = conn.(driver.ConnBeginTx)
		require.True(t, ok)
		_, err = begin.BeginTx(t.Context(), driver.TxOptions{Isolation: driver.IsolationLevel(sql.LevelRepeatableRead)})
		require.NoError(t, err)
		require.Equal(t, 2, backend.queries)
		require.Equal(t, driver.IsolationLevel(sql.LevelRepeatableRead), backend.begins[len(backend.begins)-1].Isolation)
	}
}

func TestIsolationConnectorPreservesUnsupportedDefaults(t *testing.T) {
	t.Parallel()

	backend := &isolationTestConnection{value: "SNAPSHOT"}
	connector := isolationConnector{isolationTestConnector{connection: backend}}
	conn, err := connector.Connect(t.Context())
	require.NoError(t, err)
	require.False(t, backend.closed)

	begin, ok := conn.(driver.ConnBeginTx)
	require.True(t, ok)
	_, err = begin.BeginTx(t.Context(), driver.TxOptions{Isolation: driver.IsolationLevel(sql.LevelReadCommitted)})
	require.NoError(t, err)
	require.Equal(t, driver.IsolationLevel(sql.LevelReadCommitted), backend.begins[0].Isolation)
	backend.queryErr = errors.New("unexpected isolation refresh")
	_, err = begin.BeginTx(t.Context(), driver.TxOptions{Isolation: driver.IsolationLevel(sql.LevelReadCommitted)})
	require.NoError(t, err)
	require.Len(t, backend.begins, 2)
	require.Equal(t, 1, backend.queries)
	require.Equal(t, backend.begins[0], backend.begins[1])
}

func TestIsolationConnectorClosesFailedInitialization(t *testing.T) {
	t.Parallel()

	backend := &isolationTestConnection{queryErr: errors.New("probe failed")}
	_, err := (isolationConnector{isolationTestConnector{connection: backend}}).Connect(t.Context())
	require.ErrorContains(t, err, "probe failed")
	require.True(t, backend.closed)
}

type legacyIsolationConnection struct {
	isolationTestConnection
	primaryErr error
	statements []string
}

func (c *legacyIsolationConnection) QueryContext(
	ctx context.Context, query string, args []driver.NamedValue,
) (driver.Rows, error) {
	c.statements = append(c.statements, query)
	if strings.Contains(query, "transaction_isolation") {
		return nil, c.primaryErr
	}

	return c.isolationTestConnection.QueryContext(ctx, query, args)
}

func TestSessionIsolationLegacyVariableFallback(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		primaryErr error
		fallback   bool
	}{
		{"legacy variable", &gomysql.MySQLError{Number: 1193}, true},
		{"other backend error", &gomysql.MySQLError{Number: 1227}, false},
		{"cancellation", context.Canceled, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			conn := &legacyIsolationConnection{
				isolationTestConnection: isolationTestConnection{value: "READ-COMMITTED"},
				primaryErr:              tc.primaryErr,
			}

			level, err := readSessionIsolation(t.Context(), conn)
			if !tc.fallback {
				require.ErrorIs(t, err, tc.primaryErr)
				require.Len(t, conn.statements, 1)

				return
			}

			require.NoError(t, err)
			require.Equal(t, sql.LevelReadCommitted, level)
			require.Equal(t, []string{"SELECT @@SESSION.transaction_isolation", "SELECT @@SESSION.tx_isolation"},
				conn.statements)
		})
	}
}

func TestIsolationChangesDisableFastPathWithoutRefresh(t *testing.T) {
	t.Parallel()

	for _, query := range []string{
		"SET SESSION TRANSACTION ISOLATION LEVEL READ COMMITTED",
		"SET TRANSACTION ISOLATION LEVEL READ COMMITTED",
		"SET @@SESSION.transaction_isolation='READ-COMMITTED'",
		"SET SESSION tx_isolation='READ-COMMITTED'",
		"/* comment */ SET /* session */ TRANSACTION ISOLATION LEVEL READ COMMITTED",
		"SELECT 1; SET SESSION transaction_isolation='READ-COMMITTED'",
		"SET -- comment\n SESSION TRANSACTION ISOLATION LEVEL READ COMMITTED",
		"RESET CONNECTION", "EXECUTE session_change",
	} {
		t.Run(query, func(t *testing.T) {
			t.Parallel()

			backend := &isolationTestConnection{value: "READ-COMMITTED"}
			conn := &isolationConnection{mysqlConnection: backend, level: sql.LevelRepeatableRead, known: true}
			_, err := conn.ExecContext(t.Context(), query, nil)
			require.NoError(t, err)
			require.False(t, conn.known)
			_, err = conn.BeginTx(t.Context(), driver.TxOptions{Isolation: driver.IsolationLevel(sql.LevelRepeatableRead)})
			require.NoError(t, err)
			require.Zero(t, backend.queries)
			require.Equal(t, driver.IsolationLevel(sql.LevelRepeatableRead), backend.begins[0].Isolation)
			_, err = conn.BeginTx(t.Context(), driver.TxOptions{Isolation: driver.IsolationLevel(sql.LevelReadCommitted)})
			require.NoError(t, err)
			require.Zero(t, backend.queries)
			require.Equal(t, driver.IsolationLevel(sql.LevelReadCommitted), backend.begins[1].Isolation)
			_, err = conn.BeginTx(t.Context(), driver.TxOptions{})
			require.NoError(t, err)
			require.Zero(t, backend.begins[2].Isolation)
		})
	}
}

func (c *isolationTestConnection) PrepareContext(context.Context, string) (driver.Stmt, error) {
	return isolationTestStatement{}, nil
}

func (c *isolationTestConnection) Prepare(string) (driver.Stmt, error) {
	return isolationTestStatement{}, nil
}

type isolationTestStatement struct{ mysqlStatement }

func (isolationTestStatement) ExecContext(context.Context, []driver.NamedValue) (driver.Result, error) {
	return driver.RowsAffected(0), nil
}

func TestPreparedExecutionInvalidatesIsolationEachTime(t *testing.T) {
	t.Parallel()

	backend := &isolationTestConnection{value: "REPEATABLE-READ"}
	conn := &isolationConnection{mysqlConnection: backend, level: sql.LevelRepeatableRead, known: true}

	prepared, err := conn.PrepareContext(t.Context(), "SET TRANSACTION ISOLATION LEVEL READ COMMITTED")
	require.NoError(t, err)

	statement, ok := prepared.(driver.StmtExecContext)
	require.True(t, ok)

	for range 2 {
		_, err = statement.ExecContext(t.Context(), nil)
		require.NoError(t, err)
		require.False(t, conn.known)
		_, err = conn.BeginTx(t.Context(), driver.TxOptions{Isolation: driver.IsolationLevel(sql.LevelRepeatableRead)})
		require.NoError(t, err)
	}

	require.Zero(t, backend.queries)

	for _, opts := range backend.begins {
		require.Equal(t, driver.IsolationLevel(sql.LevelRepeatableRead), opts.Isolation)
	}
}

func TestIsolationCachePreservesWorkloadSQL(t *testing.T) {
	t.Parallel()

	for _, query := range []string{
		"SELECT 1", "SELECT 'a;b'", `SELECT "a;b"`, "SELECT `a;b` FROM t", "SELECT 'a'';b'",
		"INSERT INTO t VALUES (1)", "UPDATE t SET value=1", "DELETE FROM t",
		"CALL PAYMENT(1, 2)", "/* read-then-increment */ SELECT d_next_o_id FROM district FOR UPDATE",
		"SELECT 'back\\slash'", "SELECT 1; SELECT 2", "SET FOREIGN_KEY_CHECKS=0",
		"SELECT @@SESSION.transaction_isolation",
		"CREATE PROCEDURE workload() SELECT 1", "-- comment\nSELECT 1",
	} {
		backend := &isolationTestConnection{}
		conn := &isolationConnection{mysqlConnection: backend, level: sql.LevelRepeatableRead, known: true}
		_, err := conn.QueryContext(t.Context(), query, nil)
		require.NoError(t, err)
		_, err = conn.BeginTx(t.Context(), driver.TxOptions{Isolation: driver.IsolationLevel(sql.LevelRepeatableRead)})
		require.NoError(t, err)
		require.Zero(t, backend.begins[0].Isolation)
	}
}
