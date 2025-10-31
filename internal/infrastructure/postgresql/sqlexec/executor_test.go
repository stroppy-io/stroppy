package sqlexec //nolint:testpackage // test package
import (
	"context"
	"regexp"
	"testing"

	trmpgx "github.com/avito-tech/go-transaction-manager/pgxv5"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type MockCtxGetter struct {
	mock.Mock
}

func (m *MockCtxGetter) DefaultTrOrDB(ctx context.Context, db trmpgx.Tr) trmpgx.Tr { //nolint:ireturn // lib
	args := m.Called(ctx, db)

	return args.Get(0).(trmpgx.Tr)
}

func TestNewTxExecutor(t *testing.T) {
	t.Parallel()

	defaultTr, err := pgxmock.NewConn()
	require.NoError(t, err)
	defer defaultTr.Close(context.Background())

	executor := NewTxExecutor(defaultTr)

	require.NotNil(t, executor)
	require.Equal(t, defaultTr, executor.defaultTr)
	require.NotNil(t, executor.ctxGetter)
}

func TestTxExecutor_WithCtxGetter(t *testing.T) {
	t.Parallel()

	defaultTr, err := pgxmock.NewConn()
	require.NoError(t, err)
	defer defaultTr.Close(context.Background())

	mockCtxGetter := new(MockCtxGetter)
	executor := NewTxExecutor(defaultTr, WithCtxGetter(mockCtxGetter))

	require.NotNil(t, executor)
	require.Equal(t, defaultTr, executor.defaultTr)
	require.Equal(t, mockCtxGetter, executor.ctxGetter)
}

func TestTxExecutor_Exec(t *testing.T) {
	t.Parallel()

	defaultTr, err := pgxmock.NewConn()
	require.NoError(t, err)
	defer defaultTr.Close(context.Background())

	ctx := context.Background()
	sql := "INSERT INTO sqlbuild (column) VALUES ($1)"
	args := []interface{}{"value"}

	mockCtxGetter := new(MockCtxGetter)
	mockCtxGetter.On("DefaultTrOrDB", ctx, defaultTr).Return(defaultTr)

	executor := NewTxExecutor(defaultTr, WithCtxGetter(mockCtxGetter))

	defaultTr.ExpectExec(regexp.QuoteMeta(sql)).
		WithArgs(args...).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	tag, err := executor.Exec(ctx, sql, args...)

	require.NoError(t, err)
	require.Equal(t, "INSERT1", tag.String())

	mockCtxGetter.AssertExpectations(t)
}

func TestTxExecutor_Query(t *testing.T) {
	t.Parallel()

	defaultTr, err := pgxmock.NewConn()
	require.NoError(t, err)
	defer defaultTr.Close(context.Background())

	ctx := context.Background()
	sql := "SELECT column FROM sqlbuild"
	args := []interface{}{}

	mockCtxGetter := new(MockCtxGetter)
	mockCtxGetter.On("DefaultTrOrDB", ctx, defaultTr).Return(defaultTr)

	executor := NewTxExecutor(defaultTr, WithCtxGetter(mockCtxGetter))

	rows := pgxmock.NewRows([]string{"column"}).AddRow("value")
	defaultTr.ExpectQuery(regexp.QuoteMeta(sql)).WithArgs(args...).WillReturnRows(rows)

	resultRows, err := executor.Query(ctx, sql, args...)
	require.NoError(t, err)

	defer resultRows.Close()

	require.NotNil(t, resultRows)

	mockCtxGetter.AssertExpectations(t)
}

func TestTxExecutor_QueryRow(t *testing.T) {
	t.Parallel()

	defaultTr, err := pgxmock.NewConn()
	require.NoError(t, err)
	defer defaultTr.Close(context.Background())

	ctx := context.Background()
	sql := "SELECT column FROM sqlbuild WHERE column=$1"
	args := []interface{}{"value"}

	mockCtxGetter := new(MockCtxGetter)
	mockCtxGetter.On("DefaultTrOrDB", ctx, defaultTr).Return(defaultTr)

	executor := NewTxExecutor(defaultTr, WithCtxGetter(mockCtxGetter))

	row := pgxmock.NewRows([]string{"column"}).AddRow("value")
	defaultTr.ExpectQuery(regexp.QuoteMeta(sql)).WithArgs(args...).WillReturnRows(row)

	resultRow := executor.QueryRow(ctx, sql, args...)

	var result string
	err = resultRow.Scan(&result)
	require.NoError(t, err)
	require.Equal(t, "value", result)

	mockCtxGetter.AssertExpectations(t)
}
