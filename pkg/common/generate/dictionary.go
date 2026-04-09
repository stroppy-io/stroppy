package generate

import (
	"errors"
	"fmt"

	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
)

// newStringDictionaryGenerator builds a generator that picks from a fixed
// list of strings on each Next() call.
//
// When the dictionary carries an `index` sub-rule, the sub-rule drives the
// pick: its Next() must produce integer values, which are wrapped modulo
// len(values) to tolerate over/underflow. Any integer kind the runtime
// emits (int32/int64/uint32/uint64/...) is accepted via toInt64.
//
// When `index` is omitted, an internal monotonic counter cycles through
// `values` in order, producing values[0], values[1], ..., values[n-1],
// values[0], ... on successive calls. This is the path used by TPC-C
// population of C_LAST for the first 1000 customers per district, where
// each district needs exactly the same 1000 syllable strings in order.
func newStringDictionaryGenerator(
	seed uint64,
	dict *stroppy.Generation_StringDictionary,
) (ValueGenerator, error) {
	values := dict.GetValues()
	if len(values) == 0 {
		return nil, ErrNoGenerators
	}

	idxRule := dict.GetIndex()
	if idxRule == nil {
		// Internal cycling counter.
		var counter uint64

		n := uint64(len(values))

		return valueGeneratorFn(func() (any, error) {
			v := values[counter%n]
			counter++

			return v, nil
		}), nil
	}

	// Sub-rule-driven index.
	idxGen, err := NewValueGeneratorByRule(seed, idxRule)
	if err != nil {
		return nil, fmt.Errorf("string_dictionary index: %w", err)
	}

	numValues := int64(len(values))

	return valueGeneratorFn(func() (any, error) {
		raw, err := idxGen.Next()
		if err != nil {
			return nil, err
		}

		idx, err := toInt64(raw)
		if err != nil {
			return nil, fmt.Errorf("string_dictionary index must be integer: %w", err)
		}

		// Safe modulo for negatives: (-1 mod n) should be n-1, not -1.
		idx = ((idx % numValues) + numValues) % numValues

		return values[idx], nil
	}), nil
}

// toInt64 normalises any integer-kind value produced by a sub-generator to
// int64 for indexing. Range generators emit pointer-to-T because the tuple
// generator stores the primitive in a closure slot (see
// newSlottedRangeGenerator), so accept both value and pointer forms.
func toInt64(value any) (int64, error) {
	switch typed := value.(type) {
	case int:
		return int64(typed), nil
	case int8:
		return int64(typed), nil
	case int16:
		return int64(typed), nil
	case int32:
		return int64(typed), nil
	case int64:
		return typed, nil
	case uint:
		return int64(typed), nil //nolint:gosec // index domain fits comfortably in int64
	case uint8:
		return int64(typed), nil
	case uint16:
		return int64(typed), nil
	case uint32:
		return int64(typed), nil
	case uint64:
		return int64(typed), nil //nolint:gosec // index domain fits comfortably in int64
	case *int32:
		return int64(*typed), nil
	case *int64:
		return *typed, nil
	case *uint32:
		return int64(*typed), nil
	case *uint64:
		return int64(*typed), nil //nolint:gosec // index domain fits comfortably in int64
	default:
		return 0, fmt.Errorf("%w: %T", errToInt64Unsupported, value)
	}
}

var errToInt64Unsupported = errors.New("cannot convert to int64")
