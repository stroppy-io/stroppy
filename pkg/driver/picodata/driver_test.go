package picodata

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/stroppy-io/stroppy/pkg/common/logger"
	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
)

type mockPool struct {
	execCalls []struct {
		SQL  string
		Args []any
	}
	execErr error
}

func (m *mockPool) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	m.execCalls = append(m.execCalls, struct {
		SQL  string
		Args []any
	}{SQL: sql, Args: args})

	return pgconn.NewCommandTag(""), m.execErr
}

func (m *mockPool) ExecContext(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	m.execCalls = append(m.execCalls, struct {
		SQL  string
		Args []any
	}{SQL: sql, Args: args})

	return pgconn.NewCommandTag(""), m.execErr
}

func (m *mockPool) Query(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
	return nil, nil //nolint:nilnil // mock
}

func (m *mockPool) QueryContext(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
	return nil, nil //nolint:nilnil // mock
}

func (m *mockPool) Ping(_ context.Context) error        { return nil }
func (m *mockPool) Close()                              {}
func (m *mockPool) Config() *pgxpool.Config             { return nil }

func newTestDriver(pool Executor) *Driver {
	return &Driver{
		logger: logger.Global(),
		pool:   pool,
	}
}

func ptr[T any](v T) *T {
	return &v
}

func TestDriver_InsertValuesPlainQuery(t *testing.T) {
	mock := &mockPool{}
	drv := newTestDriver(mock)

	ctx := context.Background()
	descriptor := &stroppy.InsertDescriptor{
		Count:     3,
		TableName: "test_table",
		Method:    stroppy.InsertMethod_PLAIN_QUERY.Enum(),
		Params: []*stroppy.QueryParamDescriptor{
			{
				Name: "id",
				GenerationRule: &stroppy.Generation_Rule{
					Kind: &stroppy.Generation_Rule_Int64Range{
						Int64Range: &stroppy.Generation_Range_Int64{
							Min: ptr[int64](1),
							Max: 100,
						},
					},
					Unique: ptr(true),
				},
			},
		},
	}

	stats, err := drv.InsertValues(ctx, descriptor)
	require.NoError(t, err)
	require.NotNil(t, stats)

	require.Len(t, mock.execCalls, 3, "expected 3 insert executions")

	for i, call := range mock.execCalls {
		require.Contains(t, strings.ToLower(call.SQL), "insert",
			"call %d: expected INSERT statement, got %q", i+1, call.SQL)
		require.Contains(t, strings.ToLower(call.SQL), "test_table",
			"call %d: expected test_table in SQL, got %q", i+1, call.SQL)
		require.Len(t, call.Args, 1, "call %d: expected 1 arg (id)", i+1)
	}
}

func TestDriver_Teardown(t *testing.T) {
	t.Run("teardown closes pool", func(t *testing.T) {
		mock := &mockPool{}
		drv := newTestDriver(mock)

		err := drv.Teardown(context.Background())
		require.NoError(t, err)
	})

	t.Run("teardown with nil pool does not panic", func(t *testing.T) {
		drv := &Driver{
			logger: logger.Global(),
			pool:   nil,
		}

		err := drv.Teardown(context.Background())
		require.NoError(t, err)
	})
}
