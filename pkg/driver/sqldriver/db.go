package sqldriver

import (
	"context"
	"database/sql"
	"io"
)

// QueryContext is accepted by RunQuery.
type QueryContext[R any] interface {
	QueryContext(ctx context.Context, query string, args ...any) (R, error)
}

// ExecContext is accepted by InsertPlainQuery and InsertPlainBulk.
type ExecContext[T any] interface {
	ExecContext(ctx context.Context, query string, args ...any) (T, error)
}

// Compile-time checks: *sql.DB conforms to every thin interface.
var (
	_ QueryContext[*sql.Rows] = (*sql.DB)(nil)
	_ ExecContext[sql.Result] = (*sql.DB)(nil)
	_ io.Closer               = (*sql.DB)(nil)
)
