package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"

	"github.com/stroppy-io/stroppy/pkg/driver"
)

func TestStatementQueryPreservesImmediateContextError(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	pgErr := &pgconn.PgError{Code: "57014"}
	query := statementQuery{query: func(context.Context, string, ...any) (pgx.Rows, error) {
		return nil, pgErr
	}}

	rows, err := query.QueryContext(ctx, "SELECT pg_sleep(10)")
	if rows != nil {
		rows.Close()
	}

	require.ErrorIs(t, err, context.Canceled)
	require.NotErrorIs(t, err, context.DeadlineExceeded)
	require.ErrorIs(t, err, pgErr)
}

type terminalErrorRows struct {
	pgx.Rows
	err error
}

func (*terminalErrorRows) Next() bool { return false }
func (r *terminalErrorRows) Err() error {
	return r.err
}

func TestStatementRowsPreserveParentCancellationBeforeCleanup(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name         string
		queryTimeout time.Duration
	}{
		{name: "timeout disabled"},
		{name: "timeout enabled", queryTimeout: time.Minute},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			parentCtx, cancelParent := context.WithCancel(context.Background())
			statementCtx := parentCtx
			cancelStatement := func() {}

			if tt.queryTimeout > 0 {
				statementCtx, cancelStatement = context.WithTimeout(parentCtx, tt.queryTimeout)
			}

			defer cancelStatement()

			pgErr := &pgconn.PgError{Code: "57014"}
			baseRows := pgxmock.NewRows([]string{"value"}).Kind()
			query := statementQuery{query: func(context.Context, string, ...any) (pgx.Rows, error) {
				return &terminalErrorRows{Rows: baseRows, err: pgErr}, nil
			}}

			rawRows, err := query.QueryContext(statementCtx, "SELECT value")
			require.NoError(t, err)

			rows := NewRows(rawRows)

			cancelParent()
			require.False(t, rows.Next())
			cancelStatement()

			err = rows.Err()
			require.ErrorIs(t, err, context.Canceled)
			require.NotErrorIs(t, err, context.DeadlineExceeded)
			require.ErrorIs(t, err, pgErr)
			require.Equal(t, driver.ErrorKindCanceled, (*Driver)(nil).ClassifyError(err).Kind)
		})
	}
}

func TestRunQuery_ReturnsRows(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)

	defer mock.Close()

	drv := newTestDriver(mock)

	mock.ExpectQuery("SELECT").
		WillReturnRows(
			mock.NewRows([]string{"id", "name"}).
				AddRow(1, "alice").
				AddRow(2, "bob"),
		)

	result, err := drv.RunQuery(context.Background(), "SELECT id, name FROM users", nil)
	require.NoError(t, err)
	require.NotNil(t, result.Stats)
	require.NotNil(t, result.Rows)

	require.Equal(t, []string{"id", "name"}, result.Rows.Columns())

	require.True(t, result.Rows.Next())
	require.Equal(t, []any{int(1), "alice"}, result.Rows.Values())

	require.True(t, result.Rows.Next())
	require.Equal(t, []any{int(2), "bob"}, result.Rows.Values())

	require.False(t, result.Rows.Next())
	require.NoError(t, result.Rows.Err())

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRunQuery_ReadAll(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)

	defer mock.Close()

	drv := newTestDriver(mock)

	mock.ExpectQuery("SELECT").
		WillReturnRows(
			mock.NewRows([]string{"val"}).
				AddRow(10).
				AddRow(20).
				AddRow(30),
		)

	result, err := drv.RunQuery(context.Background(), "SELECT val FROM t", nil)
	require.NoError(t, err)

	all := result.Rows.ReadAll(0)
	require.Len(t, all, 3)
	require.Equal(t, []any{int(10)}, all[0])
	require.Equal(t, []any{int(20)}, all[1])
	require.Equal(t, []any{int(30)}, all[2])

	require.NoError(t, result.Rows.Err())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRunQuery_ReadAllWithLimit(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)

	defer mock.Close()

	drv := newTestDriver(mock)

	mock.ExpectQuery("SELECT").
		WillReturnRows(
			mock.NewRows([]string{"val"}).
				AddRow(1).
				AddRow(2).
				AddRow(3).
				AddRow(4).
				AddRow(5),
		)

	result, err := drv.RunQuery(context.Background(), "SELECT val FROM t", nil)
	require.NoError(t, err)

	all := result.Rows.ReadAll(2)
	require.Len(t, all, 2)
	require.Equal(t, []any{int(1)}, all[0])
	require.Equal(t, []any{int(2)}, all[1])

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRunQuery_ExecStyleEmptyRows(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)

	defer mock.Close()

	drv := newTestDriver(mock)

	mock.ExpectQuery("INSERT").
		WillReturnRows(mock.NewRows([]string{}))

	result, err := drv.RunQuery(context.Background(), "INSERT INTO t (a) VALUES (1)", nil)
	require.NoError(t, err)
	require.NotNil(t, result.Stats)

	require.False(t, result.Rows.Next())
	require.Empty(t, result.Rows.Columns())
	require.NoError(t, result.Rows.Err())
	require.NoError(t, mock.ExpectationsWereMet())
}
