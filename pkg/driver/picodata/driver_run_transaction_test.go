package picodata

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
)

func TestDriver_runQueriesSequentially(t *testing.T) {
	t.Run("execute single query successfully", func(t *testing.T) {
		mock := &mockPool{}
		drv, err := newMockPool(mock)
		require.NoError(t, err)

		transaction := &stroppy.DriverTransaction{
			IsolationLevel: stroppy.TxIsolationLevel_UNSPECIFIED,
			Queries: []*stroppy.DriverQuery{
				{
					Name:    "query_1",
					Request: "SELECT 1",
					Params:  nil,
				},
			},
		}

		stats, err := drv.runQueriesSequentially(context.Background(), transaction)
		require.NoError(t, err)
		require.NotNil(t, stats)
		require.Len(t, stats.Queries, 1)
		require.Equal(t, "query_1", stats.Queries[0].Name)
		require.Equal(t, stroppy.TxIsolationLevel_UNSPECIFIED, stats.IsolationLevel)
	})

	t.Run("execute multiple queries successfully", func(t *testing.T) {
		mock := &mockPool{}
		drv, err := newMockPool(mock)
		require.NoError(t, err)

		transaction := &stroppy.DriverTransaction{
			IsolationLevel: stroppy.TxIsolationLevel_UNSPECIFIED,
			Queries: []*stroppy.DriverQuery{
				{
					Name:    "query_1",
					Request: "INSERT INTO users (id) VALUES ($1)",
					Params: []*stroppy.Value{
						{Type: &stroppy.Value_Int64{Int64: 1}},
					},
				},
				{
					Name:    "query_2",
					Request: "SELECT * FROM users WHERE id = $1",
					Params: []*stroppy.Value{
						{Type: &stroppy.Value_Int64{Int64: 1}},
					},
				},
			},
		}

		stats, err := drv.runQueriesSequentially(context.Background(), transaction)
		t.Log(stats)
		require.NoError(t, err)
		require.NotNil(t, stats)
		require.Len(t, stats.Queries, 2)
		require.Equal(t, "query_1", stats.Queries[0].Name)
		require.Equal(t, "query_2", stats.Queries[1].Name)

		require.Len(t, mock.execCalls, 2)
		require.Equal(t, "INSERT INTO users (id) VALUES ($1)", mock.execCalls[0].SQL)
		require.Equal(t, "SELECT * FROM users WHERE id = $1", mock.execCalls[1].SQL)
	})

	t.Run("handles empty transaction", func(t *testing.T) {
		mock := &mockPool{}
		drv, err := newMockPool(mock)
		require.NoError(t, err)

		transaction := &stroppy.DriverTransaction{
			IsolationLevel: stroppy.TxIsolationLevel_UNSPECIFIED,
			Queries:        nil,
		}

		stats, err := drv.runQueriesSequentially(context.Background(), transaction)
		require.NoError(t, err)
		require.NotNil(t, stats)
		require.Len(t, stats.Queries, 0)
		require.Len(t, mock.execCalls, 0)
	})

	t.Run("returns error on query execution failure", func(t *testing.T) {
		mock := &mockPool{
			execErr: errors.New("syntax error"),
		}
		drv, err := newMockPool(mock)
		require.NoError(t, err)

		transaction := &stroppy.DriverTransaction{
			IsolationLevel: stroppy.TxIsolationLevel_UNSPECIFIED,
			Queries: []*stroppy.DriverQuery{
				{
					Name:    "invalid_query",
					Request: "invalidsql",
					Params:  nil,
				},
			},
		}

		stats, err := drv.runQueriesSequentially(context.Background(), transaction)
		require.Error(t, err)
		require.Nil(t, stats)
		require.Contains(t, err.Error(), "syntax error")
	})
}
