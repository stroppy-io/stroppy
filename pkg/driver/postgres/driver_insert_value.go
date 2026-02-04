package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"github.com/stroppy-io/stroppy/pkg/driver/postgres/queries"
	"github.com/stroppy-io/stroppy/pkg/driver/stats"
)

// InsertValues inserts multiple rows into the database based on the descriptor.
// It supports two methods:
// - PLAIN_QUERY: executes individual INSERT statements for each row
// - COPY_FROM: uses PostgreSQL's COPY protocol for fast bulk insertion
// The count parameter specifies how many rows to insert.
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

// insertValuesPlainQuery executes multiple INSERT statements sequentially.
// Each row is inserted with a separate pgx.Exec call.
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

		if _, err := d.pgxPool.Exec(ctx, query, values...); err != nil {
			return nil, fmt.Errorf("error to execute query due to: %w", err)
		}
	}

	return &stats.Query{Elapsed: time.Since(start)}, nil
}

// insertValuesCopyFrom uses PostgreSQL's COPY protocol for fast bulk insertion.
// It streams values on-demand without loading all rows into memory, making it suitable
// for very large counts. Values are generated as the COPY protocol requests them.
func (d *Driver) insertValuesCopyFrom(
	ctx context.Context,
	builder *queries.QueryBuilder,
) (*stats.Query, error) {
	cols := builder.Columns()
	stream := newStreamingCopySource(builder)
	start := time.Now()

	if _, err := d.pgxPool.CopyFrom(ctx, pgx.Identifier{builder.TableName()}, cols, stream); err != nil {
		return nil, err
	}

	return &stats.Query{Elapsed: time.Since(start)}, nil
}
