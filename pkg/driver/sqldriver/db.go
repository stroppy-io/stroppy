package sqldriver

import (
	"context"
	"database/sql"
	"io"
)

// QueryContext is accepted by RunQuery.
type QueryContext interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// ExecContext is accepted by InsertPlainQuery and InsertPlainBulk.
type ExecContext interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// Compile-time checks: *sql.DB conforms to every thin interface.
var (
	_ QueryContext = (*sql.DB)(nil)
	_ ExecContext  = (*sql.DB)(nil)
	_ io.Closer    = (*sql.DB)(nil)
)
