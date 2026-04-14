package ydb

import (
	"context"
	"fmt"
	"path"
	"time"

	"github.com/google/uuid"
	"github.com/ydb-platform/ydb-go-sdk/v3/table"
	"github.com/ydb-platform/ydb-go-sdk/v3/table/types"

	"github.com/stroppy-io/stroppy/pkg/driver/sqldriver/queries"
	"github.com/stroppy-io/stroppy/pkg/driver/stats"
)

// insertValuesNative uses YDB native BulkUpsert for fast non-transactional
// batch insertion via the underlying ydb-go-sdk driver.
func (d *Driver) insertValuesNative(
	ctx context.Context,
	builder *queries.QueryBuilder,
) (*stats.Query, error) {
	cols := builder.Columns()
	total := int(builder.Count())
	tablePath := path.Join(d.nativeDB.Name(), builder.TableName())

	start := time.Now()
	values := make([]any, len(cols))
	batch := make([]types.Value, 0, d.bulkSize)

	flush := func() error {
		if len(batch) == 0 {
			return nil
		}

		rows := types.ListValue(batch...)
		if err := d.nativeDB.Table().BulkUpsert(
			ctx, tablePath, table.BulkUpsertDataRows(rows),
		); err != nil {
			return fmt.Errorf("ydb bulk upsert: %w", err)
		}

		batch = batch[:0]

		return nil
	}

	for i := range total {
		if err := builder.Build(values); err != nil {
			return nil, fmt.Errorf("build row %d: %w", i, err)
		}

		fields := make([]types.StructValueOption, len(cols))
		for j, col := range cols {
			v, err := toYDBValue(values[j])
			if err != nil {
				return nil, fmt.Errorf("row %d col %q: %w", i, col, err)
			}

			fields[j] = types.StructFieldValue(col, v)
		}

		batch = append(batch, types.StructValue(fields...))

		if len(batch) >= d.bulkSize {
			if err := flush(); err != nil {
				return nil, err
			}
		}
	}

	if err := flush(); err != nil {
		return nil, err
	}

	return &stats.Query{Elapsed: time.Since(start)}, nil
}

// toYDBValue maps post-dialect Go values to native ydb types.Value.
// Generator layout (see pkg/common/generate/utils.go):
//   - numerics + bool → widened direct value (int64/uint64/float64/bool)
//     via intXToValue funcs (word-sized, no alloc)
//   - strings/datetimes → *string/*time.Time via newSlottedRangeGenerator
//   - uuid/decimal → stringified by ydbDialect.Convert before reaching here
func toYDBValue(val any) (types.Value, error) {
	switch typed := val.(type) {
	case bool:
		return types.BoolValue(typed), nil
	case int64:
		return types.Int64Value(typed), nil
	case uint64:
		return types.Uint64Value(typed), nil
	case float64:
		return types.DoubleValue(typed), nil
	case string:
		return types.TextValue(typed), nil
	case *string:
		return types.TextValue(*typed), nil
	case *time.Time:
		return types.TimestampValueFromTime(*typed), nil
	case *uuid.UUID:
		return types.TextValue(typed.String()), nil
	case uuid.UUID:
		return types.TextValue(typed.String()), nil
	case nil:
		return types.VoidValue(), nil
	default:
		return nil, fmt.Errorf("%w: %T", ErrUnsupportedType, val)
	}
}
