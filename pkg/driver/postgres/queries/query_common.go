package queries

import (
	"errors"
	"fmt"

	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
)

var (
	ErrNoParamGen       = errors.New("no generator for parameter")
	ErrUnknownParamType = errors.New("unknown parameter value type")
	ErrNilProtoValue    = errors.New("nil proto value type for parameter")
)

func GenParamValues(
	genIDs []GeneratorID,
	generators Generators,
) ([]*stroppy.Value, error) {
	var paramsValues []*stroppy.Value

	for _, genID := range genIDs {
		gen, ok := generators.Get(genID)

		if !ok {
			return nil, fmt.Errorf("%w: '%s'", ErrNoParamGen, genID)
		}

		protoValue, err := gen.Next()
		if err != nil {
			return nil, fmt.Errorf(
				"failed to generate value for parameter '%s': %w",
				genID,
				err,
			)
		}

		switch actual := protoValue.GetType().(type) {
		case nil:
			return nil, fmt.Errorf("%w: %s", ErrNilProtoValue, genID)
		case *stroppy.Value_List_:
			paramsValues = append(paramsValues, actual.List.GetValues()...)
		case *stroppy.Value_Bool,
			*stroppy.Value_Datetime,
			*stroppy.Value_Decimal,
			*stroppy.Value_Double,
			*stroppy.Value_Float,
			*stroppy.Value_Int32,
			*stroppy.Value_Int64,
			*stroppy.Value_Null,
			*stroppy.Value_String_,
			*stroppy.Value_Struct_,
			*stroppy.Value_Uint32,
			*stroppy.Value_Uint64,
			*stroppy.Value_Uuid:
			paramsValues = append(paramsValues, protoValue)
		default:
			return nil, fmt.Errorf("%w: '%T': value is '%v'", ErrUnknownParamType, actual, actual)
		}
	}

	return paramsValues, nil
}
