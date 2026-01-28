package protovalue

import (
	"github.com/shopspring/decimal"

	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
)

type Null = struct{}

func listValueToSlice(value *stroppy.Value_List) ([]any, error) {
	result := make([]any, 0, len(value.GetValues()))

	for _, v := range value.GetValues() {
		res, err := valueToAny(v)
		if err != nil {
			return nil, err
		}

		result = append(result, res)
	}

	return result, nil
}

func valueToAny(value *stroppy.Value) (any, error) {
	switch value.GetType().(type) {
	case *stroppy.Value_Null:
		return Null{}, nil
	case *stroppy.Value_Int32:
		return value.GetInt32(), nil
	case *stroppy.Value_Uint32:
		return value.GetUint32(), nil
	case *stroppy.Value_Int64:
		return value.GetInt64(), nil
	case *stroppy.Value_Uint64:
		return value.GetUint64(), nil
	case *stroppy.Value_Float:
		return value.GetFloat(), nil
	case *stroppy.Value_Double:
		return value.GetDouble(), nil
	case *stroppy.Value_String_:
		return value.GetString_(), nil
	case *stroppy.Value_Bool:
		return value.GetBool(), nil
	case *stroppy.Value_Decimal:
		dec, err := decimal.NewFromString(value.GetDecimal().GetValue())
		if err != nil {
			return nil, err
		}

		return dec, nil
	case *stroppy.Value_Uuid:
		return value.GetUuid().GetValue(), nil
	case *stroppy.Value_Datetime:
		return value.GetDatetime().GetValue().AsTime(), nil
	case *stroppy.Value_Struct_:
		return ValueStructToMap(value.GetStruct())
	case *stroppy.Value_List_:
		return listValueToSlice(value.GetList())
	default:
		panic("unknown value type")
	}
}

func ValueStructToMap(value *stroppy.Value_Struct) (map[string]any, error) {
	result := make(map[string]any, len(value.GetFields()))

	for _, filedValue := range value.GetFields() {
		val, err := valueToAny(filedValue)
		if err != nil {
			return nil, err
		}

		result[filedValue.GetKey()] = val
	}

	return result, nil
}
