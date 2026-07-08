package pg

import (
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/stroppy-io/stroppy/next/driver"
)

// stmt is a reusable query handle. In the default (extended) path it carries the
// pgx prepared-statement name to execute by plus its description; in the simple
// path (server_prepare = false) it carries the SQL text instead and the
// executor runs it directly with no server-side prepare. The reusable bind
// buffer is shared by both paths.
type stmt struct {
	name   string                         // extended path: pgx prepared name
	text   string                         // simple path: raw SQL
	sd     *pgconn.StatementDescription   // extended path only
	simple bool
	args   driver.Args
}

var _ driver.Stmt = (*stmt)(nil)

// Bind returns the statement's reusable argument buffer, reset to empty.
func (s *stmt) Bind() *driver.Args { return s.args.Reset() }
