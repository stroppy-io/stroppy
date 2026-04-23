package ydb

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/ydb-platform/ydb-go-sdk/v3/table/types"
)

// toYDBValue maps post-dialect Go values to native ydb types.Value.
//   - numerics + bool → widened direct value (int64/uint64/float64/bool)
//     via intXToValue funcs (word-sized, no alloc)
//   - strings/datetimes → *string/*time.Time via the shared runtime
//   - uuid/decimal → stringified by ydbDialect.Convert before reaching here.
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
