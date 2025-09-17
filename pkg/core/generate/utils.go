package generate

import (
	"time"

	"github.com/shopspring/decimal"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/stroppy-io/stroppy-core/pkg/generate/constraint"
	"github.com/stroppy-io/stroppy-core/pkg/generate/primitive"
	stroppy "github.com/stroppy-io/stroppy-core/pkg/proto"
)

type (
	primitiveGenerator[T primitive.Primitive] interface {
		Next() T
	}
	valueGeneratorFn                        func() (*stroppy.Value, error)
	valueTransformer[T primitive.Primitive] func(T) (*stroppy.Value, error)
)

func (f valueGeneratorFn) Next() (*stroppy.Value, error) {
	return f()
}

func newNilQuota(quota uint64) func() bool {
	quotaBuf := quota
	done := false

	return func() bool {
		if quotaBuf == 0 {
			done = true

			return done
		}

		quotaBuf--

		return done
	}
}

const Persent100 = 100

func wrapNilQuota( //nolint: ireturn // need from lib
	gen ValueGenerator,
	nullPercent uint32,
	size uint64,
) ValueGenerator {
	if nullPercent > 0 {
		nilQuota := newNilQuota(size * uint64(nullPercent) / Persent100)

		return valueGeneratorFn(func() (*stroppy.Value, error) {
			if !nilQuota() {
				return &stroppy.Value{
					Type: &stroppy.Value_Null{
						Null: stroppy.Value_NULL_VALUE,
					},
				}, nil
			}

			return gen.Next()
		})
	}

	return gen
}

func newValueGenerator[T primitive.Primitive]( //nolint: ireturn // need from lib
	distribution primitiveGenerator[T],
	transformer valueTransformer[T],
	nullPercent uint32,
	size uint64,
	constant *T,
) ValueGenerator {
	if constant != nil {
		return valueGeneratorFn(func() (*stroppy.Value, error) {
			return transformer(*constant)
		})
	}

	return wrapNilQuota(valueGeneratorFn(func() (*stroppy.Value, error) {
		return transformer(distribution.Next())
	}), nullPercent, size)
}

type rangeWrapper[T constraint.Number] struct {
	min T
	max T
}

func newRangeWrapper[T constraint.Number](minVal, maxVal T) *rangeWrapper[T] {
	return &rangeWrapper[T]{min: minVal, max: maxVal}
}

func (r rangeWrapper[T]) GetMin() T { //nolint: ireturn // generic
	return r.min
}

func (r rangeWrapper[T]) GetMax() T { //nolint: ireturn // generic
	return r.max
}

// Values conversion ---------------------------------------------------------------------------------------------------

func float32ToValue(f float32) (*stroppy.Value, error) {
	return &stroppy.Value{
		Type: &stroppy.Value_Float{
			Float: f,
		},
	}, nil
}

func float64ToValue(f float64) (*stroppy.Value, error) {
	return &stroppy.Value{
		Type: &stroppy.Value_Double{
			Double: f,
		},
	}, nil
}

func uint8ToBoolValue(b uint8) (*stroppy.Value, error) {
	return &stroppy.Value{
		Type: &stroppy.Value_Bool{
			Bool: b == 1,
		},
	}, nil
}

func uint32ToValue(i uint32) (*stroppy.Value, error) {
	return &stroppy.Value{
		Type: &stroppy.Value_Uint32{
			Uint32: i,
		},
	}, nil
}

func uint64ToValue(i uint64) (*stroppy.Value, error) {
	return &stroppy.Value{
		Type: &stroppy.Value_Uint64{
			Uint64: i,
		},
	}, nil
}

func int32ToValue(i int32) (*stroppy.Value, error) {
	return &stroppy.Value{
		Type: &stroppy.Value_Int32{
			Int32: i,
		},
	}, nil
}

func int64ToValue(i int64) (*stroppy.Value, error) {
	return &stroppy.Value{
		Type: &stroppy.Value_Int64{
			Int64: i,
		},
	}, nil
}

func stringToValue(s string) (*stroppy.Value, error) {
	return &stroppy.Value{
		Type: &stroppy.Value_String_{
			String_: s,
		},
	}, nil
}

func decimalToValue(d decimal.Decimal) (*stroppy.Value, error) {
	return &stroppy.Value{
		Type: &stroppy.Value_Decimal{
			Decimal: &stroppy.Decimal{
				Value: d.String(),
			},
		},
	}, nil
}

func dateTimeToValue(t time.Time) (*stroppy.Value, error) {
	return &stroppy.Value{
		Type: &stroppy.Value_Datetime{
			Datetime: &stroppy.DateTime{
				Value: timestamppb.New(t),
			},
		},
	}, nil
}

func boolPtrToUint8Ptr(boolean *bool) *uint8 {
	if boolean == nil {
		return nil
	}

	val := uint8(0)
	if *boolean {
		val = 1
	}

	return &val
}

func dateTimePtrToTimePtr(dt *stroppy.DateTime) *time.Time {
	if dt == nil {
		return nil
	}

	val := dt.GetValue().AsTime()

	return &val
}

func decimalPtrToDecimalPtr(d *stroppy.Decimal) *decimal.Decimal {
	if d == nil {
		return nil
	}

	val, err := decimal.NewFromString(d.GetValue())
	if err != nil {
		return nil
	}

	return &val
}

func alphabetToChars(alphabet *stroppy.Generation_Alphabet) [][2]int32 {
	ranges := make([][2]int32, 0)
	for _, rg := range alphabet.GetRanges() {
		ranges = append(
			ranges,
			[2]int32{
				int32(rg.GetMin()), //nolint: gosec // allow
				int32(rg.GetMax()), //nolint: gosec// allow
			},
		)
	}

	return ranges
}
