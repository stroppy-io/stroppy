package postgres

import (
	"context"
	"testing"

	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"
)

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
