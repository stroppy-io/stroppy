package picodata

import (
	"context"
	"errors"
	"fmt"

	"github.com/stroppy-io/stroppy/pkg/common/generate"
	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	picoqeries "github.com/stroppy-io/stroppy/pkg/driver/picodata/queries"
	"github.com/stroppy-io/stroppy/pkg/driver/sqldriver"
	sqlqueries "github.com/stroppy-io/stroppy/pkg/driver/sqldriver/queries"
	"github.com/stroppy-io/stroppy/pkg/driver/stats"
)

var ErrCopyFromUnsupported = errors.New("CopyFrom is not supported in Picodata yet")

// InsertValues inserts multiple rows into the database based on the descriptor.
// It supports two methods:
// - PLAIN_QUERY: executes individual INSERT statements for each row
// - PLAIN_BULK: executes batched bulk INSERT statements using multi-row VALUES syntax
// - COPY_FROM: unsupported
func (d *Driver) InsertValues(
	ctx context.Context,
	descriptor *stroppy.InsertDescriptor,
) (*stats.Query, error) {
	builder, err := sqlqueries.NewQueryBuilder(
		d.logger,
		picoqeries.PicoDialect{},
		generate.ResolveSeed(descriptor.GetSeed()),
		descriptor,
	)
	if err != nil {
		return nil, fmt.Errorf("can't create query builder: %w", err)
	}

	switch descriptor.GetMethod() {
	case stroppy.InsertMethod_PLAIN_QUERY:
		return sqldriver.InsertPlainQuery(ctx, d.pool, builder)
	case stroppy.InsertMethod_PLAIN_BULK:
		return sqldriver.InsertPlainBulk(ctx, d.pool, builder, d.bulkSize)
	case stroppy.InsertMethod_COPY_FROM:
		return nil, ErrCopyFromUnsupported
	default:
		d.logger.Panic("unexpected proto.InsertMethod")

		return nil, nil //nolint:nilnil // unreachable after panic
	}
}
