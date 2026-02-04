package queries

import (
	"errors"
	"fmt"

	"github.com/stroppy-io/stroppy/pkg/common/generate"
	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
)

var (
	ErrNoParamGen       = errors.New("no generator for parameter")
	ErrUnknownParamType = errors.New("unknown parameter value type")
	ErrNilProtoValue    = errors.New("nil proto value type for parameter")
	ErrWrongLength      = errors.New("len(valuesOut) != len(paramsValues)")
)

type (
	GeneratorID = string
	Generators  = map[GeneratorID]generate.ValueGenerator
)

func GenParamValues(
	genIDs []GeneratorID,
	generators Generators,
	valuesOut []any,
) error {
	var paramsValues []*stroppy.Value

	for _, genID := range genIDs {
		gen, ok := generators[genID]

		if !ok {
			return fmt.Errorf("%w: '%s'", ErrNoParamGen, genID)
		}

		protoValue, err := gen.Next()
		if err != nil {
			return fmt.Errorf(
				"failed to generate value for parameter '%s': %w",
				genID,
				err,
			)
		}

		switch actual := protoValue.GetType().(type) {
		case nil:
			return fmt.Errorf("%w: %s", ErrNilProtoValue, genID)
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
			return fmt.Errorf("%w: '%T': value is '%v'", ErrUnknownParamType, actual, actual)
		}
	}

	if len(valuesOut) != len(paramsValues) {
		return fmt.Errorf("%d != %d: %w", len(valuesOut), len(paramsValues), ErrWrongLength)
	}

	var err error
	for i := range paramsValues {
		valuesOut[i], err = ValueToAny(paramsValues[i])
		if err != nil {
			return fmt.Errorf("can't convert [%d] = %v, due to: %w", i, paramsValues, err)
		}
	}

	return nil
}
