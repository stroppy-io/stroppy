package queries

import (
	"errors"
	"fmt"
)

var (
	ErrNoParamGen  = errors.New("no generator for parameter")
	ErrWrongLength = errors.New("len(valuesOut) != len(paramsValues)")
)

//nolint:gocognit // inherently complex: handles both scalar and list generator output
func GenParamValues(
	dialect Dialect,
	genIDs []GeneratorID,
	generators Generators,
	valuesOut []any,
) error {
	idx := 0

	for _, genID := range genIDs {
		gen, ok := generators[genID]
		if !ok {
			return fmt.Errorf("%w: '%s'", ErrNoParamGen, genID)
		}

		val, err := gen.Next()
		if err != nil {
			return fmt.Errorf("failed to generate value for parameter '%s': %w", genID, err)
		}

		switch actual := val.(type) {
		case []any:
			for _, v := range actual {
				if idx >= len(valuesOut) {
					return fmt.Errorf("%w", ErrWrongLength)
				}

				converted, err := dialect.Convert(v)
				if err != nil {
					return fmt.Errorf("can't convert [%d]: %w", idx, err)
				}

				valuesOut[idx] = converted
				idx++
			}
		default:
			if idx >= len(valuesOut) {
				return fmt.Errorf("%w", ErrWrongLength)
			}

			converted, err := dialect.Convert(val)
			if err != nil {
				return fmt.Errorf("can't convert [%d] = %v: %w", idx, val, err)
			}

			valuesOut[idx] = converted
			idx++
		}
	}

	if idx != len(valuesOut) {
		return fmt.Errorf("%d != %d: %w", idx, len(valuesOut), ErrWrongLength)
	}

	return nil
}
