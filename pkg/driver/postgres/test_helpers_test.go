package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pashagolub/pgxmock/v4"

	"github.com/stroppy-io/stroppy/pkg/common/logger"
)

// mockExecutor wraps pgxmock.PgxPoolIface to satisfy the Executor interface
// by adding ExecContext/QueryContext shims (which delegate to Exec/Query).
type mockExecutor struct {
	pgxmock.PgxPoolIface
}

func (m *mockExecutor) ExecContext(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return m.Exec(ctx, sql, args...)
}

func (m *mockExecutor) QueryContext(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return m.Query(ctx, sql, args...)
}

func (m *mockExecutor) Config() *pgxpool.Config { return nil }

type testDriver struct {
	*Driver
}

func newTestDriver(mockPool pgxmock.PgxPoolIface) *testDriver {
	return &testDriver{
		Driver: &Driver{
			logger: logger.Global(),
			pool:   &mockExecutor{mockPool},
		},
	}
}
