package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/stroppy-io/stroppy/pkg/common/generate"
	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	pgqueries "github.com/stroppy-io/stroppy/pkg/driver/postgres/queries"
	"github.com/stroppy-io/stroppy/pkg/driver/sqldriver"
	sqlqueries "github.com/stroppy-io/stroppy/pkg/driver/sqldriver/queries"
	"github.com/stroppy-io/stroppy/pkg/driver/stats"
)

var ErrUnsupportedInsertMethod = errors.New("unsupported insert method for postgres driver")

// InsertValues inserts multiple rows into the database based on the descriptor.
// It supports three methods:
// - PLAIN_QUERY: executes individual INSERT statements for each row
// - PLAIN_BULK: executes batched bulk INSERT statements using multi-row VALUES syntax
// - COPY_FROM: uses PostgreSQL's COPY protocol for fast bulk insertion
func (d *Driver) InsertValues(
	ctx context.Context,
	descriptor *stroppy.InsertDescriptor,
) (*stats.Query, error) {
	builder, err := sqlqueries.NewQueryBuilder(
		d.logger,
		pgqueries.PgxDialect{},
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
		return d.insertValuesCopyFrom(ctx, builder)
	default:
		d.logger.Panic("unexpected proto.InsertMethod")

		return nil, nil //nolint:nilnil // unreachable after panic
	}
}

// insertValuesCopyFrom uses PostgreSQL's COPY protocol for fast bulk insertion.
// It streams values on-demand without loading all rows into memory.
func (d *Driver) insertValuesCopyFrom(
	ctx context.Context,
	builder *sqlqueries.QueryBuilder,
) (*stats.Query, error) {
	cols := builder.Columns()
	stream := newStreamingCopySource(builder)
	start := time.Now()

	if _, err := d.pool.CopyFrom(ctx, pgx.Identifier{builder.TableName()}, cols, stream); err != nil {
		return nil, err
	}

	return &stats.Query{Elapsed: time.Since(start)}, nil
}
