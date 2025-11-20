package sqlexec //nolint:testpackage // test package

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type MockSqlizer struct {
	mock.Mock
}

func (m *MockSqlizer) ToSql() ( //nolint:revive,nonamedreturns,stylecheck // cause lib
	sql string,
	args []interface{},
	err error,
) {
	callArgs := m.Called()

	return callArgs.String(0), callArgs.Get(1).([]interface{}), callArgs.Error(2)
}

func TestSqlizerExecutor_Exec(t *testing.T) {
	t.Parallel()

	defaultTr, err := pgxmock.NewConn()
	require.NoError(t, err)
	defer defaultTr.Close(context.Background())

	ctx := context.Background()
	sql := "INSERT INTO sqlbuild (column) VALUES ($1)"
	args := []interface{}{"value"}

	sqlizer := &MockSqlizer{}
	sqlizer.On("ToSql").Return(sql, args, nil)

	mockCtxGetter := new(MockCtxGetter)
	mockCtxGetter.On("DefaultTrOrDB", ctx, defaultTr).Return(defaultTr)

	executor := NewSqlizerExecutor(NewTxExecutor(defaultTr, WithCtxGetter(mockCtxGetter)))

	defaultTr.ExpectExec(regexp.QuoteMeta(sql)).
		WithArgs(args...).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	tag, err := executor.Exec(ctx, sqlizer)

	require.NoError(t, err)
	require.Equal(t, "INSERT1", tag.String())

	mockCtxGetter.AssertExpectations(t)
}

func TestSqlizerExecutor_Query(t *testing.T) {
	t.Parallel()

	defaultTr, err := pgxmock.NewConn()
	require.NoError(t, err)
	defer defaultTr.Close(context.Background())

	ctx := context.Background()
	sql := "SELECT column FROM sqlbuild"
	args := []interface{}{}

	sqlizer := &MockSqlizer{}
	sqlizer.On("ToSql").Return(sql, args, nil)

	mockCtxGetter := new(MockCtxGetter)
	mockCtxGetter.On("DefaultTrOrDB", ctx, defaultTr).Return(defaultTr)

	executor := NewSqlizerExecutor(NewTxExecutor(defaultTr, WithCtxGetter(mockCtxGetter)))

	rows := pgxmock.NewRows([]string{"column"}).AddRow("value")
	defaultTr.ExpectQuery(regexp.QuoteMeta(sql)).WithArgs(args...).WillReturnRows(rows)

	resultRows, err := executor.Query(ctx, sqlizer)
	require.NoError(t, err)

	defer resultRows.Close()

	require.NotNil(t, resultRows)

	mockCtxGetter.AssertExpectations(t)
}

func TestSqlizerExecutor_QueryRow(t *testing.T) {
	t.Parallel()

	defaultTr, err := pgxmock.NewConn()
	require.NoError(t, err)
	defer defaultTr.Close(context.Background())

	ctx := context.Background()
	sql := "SELECT column FROM sqlbuild WHERE column=$1"
	args := []interface{}{"value"}

	sqlizer := &MockSqlizer{}
	sqlizer.On("ToSql").Return(sql, args, nil)

	mockCtxGetter := new(MockCtxGetter)
	mockCtxGetter.On("DefaultTrOrDB", ctx, defaultTr).Return(defaultTr)

	executor := NewSqlizerExecutor(NewTxExecutor(defaultTr, WithCtxGetter(mockCtxGetter)))

	row := pgxmock.NewRows([]string{"column"}).AddRow("value")
	defaultTr.ExpectQuery(regexp.QuoteMeta(sql)).WithArgs(args...).WillReturnRows(row)

	resultRow := executor.QueryRow(ctx, sqlizer)

	var result string
	err = resultRow.Scan(&result)
	require.NoError(t, err)
	require.Equal(t, "value", result)

	mockCtxGetter.AssertExpectations(t)
}

func TestSqlizerExecutor_Exec_ToSql_Error(t *testing.T) {
	t.Parallel()

	mockSqlizer := new(MockSqlizer)
	sqlizerErr := errors.New("sqlizer error")
	mockSqlizer.On("ToSql").Return("", []interface{}(nil), sqlizerErr)

	executor := NewSqlizerExecutor(NewTxExecutor(nil))
	tag, err := executor.Exec(context.Background(), mockSqlizer)
	require.Error(t, err)
	require.Equal(t, pgconn.CommandTag{}, tag)

	mockSqlizer.AssertExpectations(t)
}

func TestSqlizerExecutor_Query_ToSql_Error(t *testing.T) {
	t.Parallel()

	mockSqlizer := new(MockSqlizer)
	sqlizerErr := errors.New("sqlizer error")
	mockSqlizer.On("ToSql").Return("", []interface{}(nil), sqlizerErr)

	executor := NewSqlizerExecutor(NewTxExecutor(nil))
	rows, err := executor.Query(context.Background(), mockSqlizer) //nolint:sqlclosecheck // need nil rows
	require.Error(t, err)
	require.Nil(t, rows)
	mockSqlizer.AssertExpectations(t)
}

func TestSqlizerExecutor_QueryRow_ToSql_Error(t *testing.T) {
	t.Parallel()

	mockSqlizer := new(MockSqlizer)
	sqlizerErr := errors.New("sqlizer error")
	mockSqlizer.On("ToSql").Return("", []interface{}(nil), sqlizerErr)

	executor := NewSqlizerExecutor(NewTxExecutor(nil))
	resultRow := executor.QueryRow(context.Background(), mockSqlizer)

	var result string

	err := resultRow.Scan(&result)
	require.Error(t, err)

	mockSqlizer.AssertExpectations(t)
}
