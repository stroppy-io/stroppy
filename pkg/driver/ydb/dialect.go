package ydb

import (
	"errors"
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

func (ydbDialect) Convert(val any) (any, error) {
	switch v := val.(type) { //nolint:varnamelen // switch type assertion idiom
	case nil:
		return nil, nil //nolint:nilnil // allow to set nil in db
	case uuid.UUID:
		return v.String(), nil
	case time.Time:
		// Promote to *time.Time so toYDBValue's addressable-time case fires.
		// stdlib/std.daysToDate and Draw.date both return time.Time by value;
		// without this promotion the native BulkUpsert path would reject the
		// unaddressable value. Timestamp columns get TimestampValueFromTime;
		// Date columns accept it via YDB's implicit cast.
		return &v, nil
	case decimal.Decimal:
		return v.String(), nil
	case *decimal.Decimal:
		return v.String(), nil
	default:
		return v, nil
	}
}
