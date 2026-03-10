package picodata

import (
	"context"
	"errors"
	"fmt"
	"time"

	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"github.com/stroppy-io/stroppy/pkg/driver/picodata/queries"
	"github.com/stroppy-io/stroppy/pkg/driver/stats"
)

var ErrCopyFromUnsupported = errors.New("CopyFrom is not supported in Picodata yet")

// InsertValues inserts multiple rows into the database based on the descriptor.
// It supports two methods:
// - PLAIN_QUERY: executes individual INSERT statements for each row
// - COPY_FROM: unsupported
// The count parameter specifies how many rows to insert.
// InsertValues(ctx context.Context, unit *stroppy.InsertDescriptor) (*stats.Query, error).
func (d *Driver) InsertValues(
	ctx context.Context,
	descriptor *stroppy.InsertDescriptor,
) (*stats.Query, error) {
	builder, err := queries.NewQueryBuilder(d.logger, 0, descriptor)
	if err != nil {
		return nil, fmt.Errorf("can't create query builder: %w", err)
	}

	switch descriptor.GetMethod() {
	case stroppy.InsertMethod_PLAIN_QUERY:
		return d.insertValuesPlainQuery(ctx, builder)
	case stroppy.InsertMethod_COPY_FROM:
		return d.insertValuesCopyFrom(ctx, builder)
	default:
		d.logger.Panic("unexpected proto.InsertMethod")

		return nil, nil //nolint:nilnil // unreachable after panic
	}
}

func (d *Driver) insertValuesPlainQuery(
	ctx context.Context,
	builder *queries.QueryBuilder,
) (*stats.Query, error) {
	start := time.Now()

	values := make([]any, len(builder.Columns()))
	query := builder.SQL()

	// Execute multiple inserts
	for range builder.Count() {
		if err := builder.Build(values); err != nil {
			return nil, fmt.Errorf("can't build query due to: %w", err)
		}

		if _, err := d.picoPool.Exec(ctx, query, values...); err != nil {
			return nil, fmt.Errorf("error to execute query due to: %w", err)
		}
	}

	return &stats.Query{Elapsed: time.Since(start)}, nil
}

func (d *Driver) insertValuesCopyFrom(
	_ context.Context,
	_ *queries.QueryBuilder,
) (*stats.Query, error) {
	return nil, ErrCopyFromUnsupported
}
