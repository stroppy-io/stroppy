package mysql

import (
	"errors"

	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"github.com/stroppy-io/stroppy/pkg/driver/sqldriver/queries"
)

var _ queries.Dialect = mysqlDialect{}

var ErrUnsupportedType = errors.New("unsupported value type")

type mysqlDialect struct{}

func (mysqlDialect) Placeholder(_ int) string { return "?" }
func (mysqlDialect) Deduplicate() bool        { return false }

func (mysqlDialect) ValueToAny(value *stroppy.Value) (any, error) {
	switch typed := value.GetType().(type) {
	case *stroppy.Value_Null:
		return nil, nil //nolint:nilnil // allow to set nil in db
	case *stroppy.Value_Int32:
		return typed.Int32, nil
	case *stroppy.Value_Uint32:
		return typed.Uint32, nil
	case *stroppy.Value_Int64:
		return typed.Int64, nil
	case *stroppy.Value_Uint64:
		return typed.Uint64, nil
	case *stroppy.Value_Float:
		return typed.Float, nil
	case *stroppy.Value_Double:
		return typed.Double, nil
	case *stroppy.Value_String_:
		return typed.String_, nil
	case *stroppy.Value_Bool:
		return typed.Bool, nil
	case *stroppy.Value_Decimal:
		if value.GetDecimal() == nil {
			return nil, nil //nolint:nilnil // MySQL NULL decimal
		}

		return value.GetDecimal().GetValue(), nil
	case *stroppy.Value_Uuid:
		return value.GetUuid().GetValue(), nil
	case *stroppy.Value_Datetime:
		return value.GetDatetime().GetValue().AsTime(), nil
	default:
		return nil, ErrUnsupportedType
	}
}
