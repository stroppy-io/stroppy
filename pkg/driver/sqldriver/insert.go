package sqldriver

import (
	"context"
	"fmt"
	"time"

	"github.com/stroppy-io/stroppy/pkg/driver/sqldriver/queries"
	"github.com/stroppy-io/stroppy/pkg/driver/stats"
)

// InsertPlainQuery executes one INSERT per row.
func InsertPlainQuery[T any](
	ctx context.Context,
	db ExecContext[T],
	builder *queries.QueryBuilder,
) (*stats.Query, error) {
	start := time.Now()

	values := make([]any, len(builder.Columns()))
	query := builder.SQL()

	for range builder.Count() {
		if err := builder.Build(values); err != nil {
			return nil, fmt.Errorf("can't build query due to: %w", err)
		}

		if _, err := db.ExecContext(ctx, query, values...); err != nil {
			return nil, fmt.Errorf("error to execute query due to: %w", err)
		}
	}

	return &stats.Query{Elapsed: time.Since(start)}, nil
}

// InsertPlainBulk executes batched bulk INSERT statements.
// Each batch inserts up to bulkSize rows using multi-row VALUES syntax.
func InsertPlainBulk[T any](
	ctx context.Context,
	db ExecContext[T],
	builder *queries.QueryBuilder,
	bulkSize int,
) (*stats.Query, error) {
	start := time.Now()

	totalRows := int(builder.Count())
	colCount := len(builder.Columns())
	dialect := builder.Dialect()
	insert := builder.Insert()
	generators := builder.Generators()
	genIDs := queries.InsertGenIDs(insert)
	row := make([]any, colCount)

	for offset := 0; offset < totalRows; offset += bulkSize {
		batchRows := bulkSize
		if offset+batchRows > totalRows {
			batchRows = totalRows - offset
		}

		query := queries.BulkInsertSQL(dialect, insert, batchRows)
		allValues := make([]any, 0, batchRows*colCount)

		for range batchRows {
			if err := queries.GenParamValues(dialect, genIDs, generators, row); err != nil {
				return nil, fmt.Errorf("can't build query due to: %w", err)
			}

			allValues = append(allValues, row...)
		}

		if _, err := db.ExecContext(ctx, query, allValues...); err != nil {
			return nil, fmt.Errorf("error to execute bulk query due to: %w", err)
		}
	}

	return &stats.Query{Elapsed: time.Since(start)}, nil
}
