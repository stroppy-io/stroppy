package sqldriver

import (
	"context"
	"io"
)

// Teardown closes the database connection pool.
func Teardown(_ context.Context, db io.Closer) error {
	return db.Close()
}
