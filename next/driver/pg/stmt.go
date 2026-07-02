package pg

import (
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/stroppy-io/stroppy/next/driver"
)

// stmt is a prepared handle: the pgx statement name to execute by, its
// description, and a reusable bind buffer.
type stmt struct {
	name string
	sd   *pgconn.StatementDescription
	args driver.Args
}

var _ driver.Stmt = (*stmt)(nil)

// Bind returns the statement's reusable argument buffer, reset to empty.
func (s *stmt) Bind() *driver.Args { return s.args.Reset() }
