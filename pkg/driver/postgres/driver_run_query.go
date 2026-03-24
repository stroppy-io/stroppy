package postgres

import (
	"context"

	"github.com/stroppy-io/stroppy/pkg/driver"
	pgqueries "github.com/stroppy-io/stroppy/pkg/driver/postgres/queries"
	"github.com/stroppy-io/stroppy/pkg/driver/sqldriver"
)

// RunQuery executes sql with named :arg placeholders and returns rows cursor.
func (d *Driver) RunQuery(
	ctx context.Context,
	sql string,
	args map[string]any,
) (*driver.QueryResult, error) {
	return sqldriver.RunQuery(ctx, d.pool, newRows, pgqueries.PgxDialect{}, d.logger, sql, args)
}
