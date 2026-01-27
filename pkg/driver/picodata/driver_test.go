package picodata

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stroppy-io/stroppy/pkg/common/logger"
	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"github.com/stroppy-io/stroppy/pkg/driver/postgres/queries"
)

type mockPool struct {
	execCalls []struct {
		SQL  string
		Args []any
	}
	execErr error
}

func (m *mockPool) Exec(_ context.Context, sql string, args ...any) (any, error) {
	m.execCalls = append(m.execCalls, struct {
		SQL  string
		Args []any
	}{SQL: sql, Args: args})

	return nil, m.execErr
}

func (m *mockPool) Close() {}

func newMockPool(pool Pool) (*Driver, error) {
	builder, err := queries.NewQueryBuilder(0)
	if err != nil {
		return nil, err
	}

	return &Driver{
		logger:  logger.Global(),
		pool:    pool,
		builder: builder,
	}, nil
}

// -------------

func TestDriver_fillParamsToValues(t *testing.T) {
	mock := &mockPool{}
	drv, err := newMockPool(mock)
	require.NoError(t, err)

	tests := []struct {
		name       string
		query      *stroppy.DriverQuery
		wantLen    int
		wantErr    bool
		wantValues []any
	}{
		{
			name: "empty params",
			query: &stroppy.DriverQuery{
				Name:    "test",
				Request: "SELECT 1",
				Params:  nil,
			},
			wantLen: 0,
			wantErr: false,
		},
		{
			name: "single int param",
			query: &stroppy.DriverQuery{
				Name:    "test",
				Request: "SELECT * FROM users WHERE id = $1",
				Params: []*stroppy.Value{
					{Type: &stroppy.Value_Int64{Int64: 42}},
				},
			},
			wantLen:    1,
			wantErr:    false,
			wantValues: []any{int64(42)},
		},
		{
			name: "multiple params",
			query: &stroppy.DriverQuery{
				Name:    "test",
				Request: "SELECT * FROM users WHERE id = $1 AND name = $2",
				Params: []*stroppy.Value{
					{Type: &stroppy.Value_Int64{Int64: 1}},
					{Type: &stroppy.Value_String_{String_: "test"}},
				},
			},
			wantLen:    2,
			wantErr:    false,
			wantValues: []any{int64(1), "test"},
		},
		{
			name: "float param",
			query: &stroppy.DriverQuery{
				Name:    "test",
				Request: "SELECT * FROM products WHERE price > $1",
				Params: []*stroppy.Value{
					{Type: &stroppy.Value_Double{Double: 99.99}},
				},
			},
			wantLen:    1,
			wantErr:    false,
			wantValues: []any{99.99},
		},
		{
			name: "bool param",
			query: &stroppy.DriverQuery{
				Name:    "test",
				Request: "SELECT * FROM users WHERE active = $1",
				Params: []*stroppy.Value{
					{Type: &stroppy.Value_Bool{Bool: true}},
				},
			},
			wantLen:    1,
			wantErr:    false,
			wantValues: []any{true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			values := make([]any, len(tt.query.GetParams()))
			err := drv.fillParamsToValues(tt.query, values)

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			require.Len(t, values, tt.wantLen)

			if tt.wantValues != nil {
				for i, wantVal := range tt.wantValues {
					require.Equal(t, wantVal, values[i], "value at index %d", i)
				}
			}
		})
	}
}

func TestDriver_Teardown(t *testing.T) {
	t.Run("teardown closes pool", func(t *testing.T) {
		mock := &mockPool{}
		drv, err := newMockPool(mock)
		require.NoError(t, err)

		err = drv.Teardown(context.Background())
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
