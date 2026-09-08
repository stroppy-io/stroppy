package ydb

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/stroppy-io/stroppy/pkg/driver/sqldriver/queries"
)

var _ queries.Dialect = ydbDialect{}

var ErrUnsupportedType = errors.New("unsupported value type")

type ydbDialect struct{}

func (ydbDialect) Placeholder(_ int) string { return "?" }
func (ydbDialect) Deduplicate() bool        { return false }

// StatementTimeoutHint returns sql unchanged; YDB honors context cancellation
// for its per-statement deadline.
func (ydbDialect) StatementTimeoutHint(sql string, _ time.Duration) (string, bool) {
	return sql, false
}

// StatementDeadline returns timeout unchanged; there is no server-side hint
// to outlast.
func (ydbDialect) StatementDeadline(timeout time.Duration) time.Duration { return timeout }

func (ydbDialect) Convert(val any) (any, error) {
	switch v := val.(type) { //nolint:varnamelen // switch type assertion idiom
	case nil:
		return nil, nil //nolint:nilnil // allow to set nil in db
	case uuid.UUID:
		return v.String(), nil
	case time.Time:
		// Promote to *time.Time so toYDBValue's addressable-time case fires.
		// Workload parameters and generated rows may carry time.Time by value;
		// without this promotion the native BulkUpsert path would reject the
		// unaddressable value. Timestamp columns get TimestampValueFromTime;
		// Date columns accept it via YDB's implicit cast.
		return &v, nil
	case decimal.Decimal:
		return v.String(), nil
	case *decimal.Decimal:
		return v.String(), nil
	case []any:
		return convertAnySlice(v)
	default:
		return v, nil
	}
}

// convertAnySlice converts heterogeneous parameter slices into types the YDB SDK
// can declare natively. Workloads build lists as []any with individually typed
// elements. The SDK's reflect-based parameter binder cannot
// resolve interface{} as a list element type, so we collapse []any to a
// concrete typed slice here. WithAutoDeclare can then emit stable declarations
// such as `DECLARE $pN AS List<Int64>;`, which preserves server plan-cache hits.
func convertAnySlice(arr []any) (any, error) {
	// Empty list: YDB rejects empty IN-comparisons. Pass through unchanged
	// so the SDK surfaces a useful error if the caller missed an empty
	// guard; we don't want to silently turn this into a typed empty list.
	if len(arr) == 0 {
		return arr, nil
	}

	// Choose target element type from the first non-nil element. All-nil
	// arrays fall through to the same "unsupported" error the SDK would
	// emit on its own, just with a clearer message.
	var sample any

	for _, x := range arr {
		if x != nil {
			sample = x

			break
		}
	}

	switch sample.(type) {
	case int64, int, int32, float64:
		out := make([]int64, len(arr))

		for i, elem := range arr {
			switch ev := elem.(type) {
			case int64:
				out[i] = ev
			case int:
				out[i] = int64(ev)
			case int32:
				out[i] = int64(ev)
			case float64:
				// TPC-C list values are integral; convert them so the typed
				// slice stays []int64.
				out[i] = int64(ev)
			default:
				return nil, fmt.Errorf(
					"%w: list element %d type %T not int64-compatible",
					ErrUnsupportedType, i, elem,
				)
			}
		}

		return out, nil
	case string:
		out := make([]string, len(arr))

		for i, elem := range arr {
			str, okStr := elem.(string)
			if !okStr {
				return nil, fmt.Errorf(
					"%w: list element %d type %T not string",
					ErrUnsupportedType, i, elem,
				)
			}

			out[i] = str
		}

		return out, nil
	default:
		return nil, fmt.Errorf(
			"%w: list element type %T not supported by YDB Convert",
			ErrUnsupportedType, sample,
		)
	}
}
