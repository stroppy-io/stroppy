package picodata

import (
	"context"

	"github.com/stroppy-io/stroppy/pkg/driver"
	picoqeries "github.com/stroppy-io/stroppy/pkg/driver/picodata/queries"
	"github.com/stroppy-io/stroppy/pkg/driver/sqldriver"
)

// RunQuery executes sql with named :arg placeholders and returns rows cursor.
func (d *Driver) RunQuery(
	ctx context.Context,
	sql string,
	args map[string]any,
) (*driver.QueryResult, error) {
	return sqldriver.RunQuery(ctx, d.pool, newRows, picoqeries.PicoDialect{}, d.logger, sql, args)
}
