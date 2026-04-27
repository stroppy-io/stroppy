package picodata

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/stroppy-io/stroppy/pkg/common/logger"
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

func (m *mockPool) Ping(_ context.Context) error { return nil }
func (m *mockPool) Close()                       {}
func (m *mockPool) Config() *pgxpool.Config      { return nil }

func newTestDriver(pool Executor) *Driver {
	return &Driver{
		logger: logger.Global(),
		pool:   pool,
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
