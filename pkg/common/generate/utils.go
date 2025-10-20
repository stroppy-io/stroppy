package generate

import (
	"math/rand/v2"
	"time"

	"github.com/shopspring/decimal"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/stroppy-io/stroppy/pkg/common/generate/constraint"
	"github.com/stroppy-io/stroppy/pkg/common/generate/primitive"
	"github.com/stroppy-io/stroppy/pkg/common/logger"
	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto"
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

const Persent100 = 100

func wrapNilQuota( //nolint: ireturn // need from lib
	gen ValueGenerator,
	nullPercent uint32,
) ValueGenerator {
	percent := float64(nullPercent) / Persent100

	return valueGeneratorFn(func() (*stroppy.Value, error) {
		if rand.Float64() < percent { //nolint:gosec // performance in priority here (against crypto/rand)
			return &stroppy.Value{Type: &stroppy.Value_Null{Null: stroppy.Value_NULL_VALUE}}, nil
		}

		return gen.Next()
	})
}

func newConstValueGenerator[T primitive.Primitive](
	constant T,
	transformer valueTransformer[T],
) ValueGenerator {
	return valueGeneratorFn(func() (*stroppy.Value, error) {
		return transformer(constant)
	})
}
func newRangeGenerator[T primitive.Primitive](
	distribution primitiveGenerator[T],
	transformer valueTransformer[T],
) ValueGenerator {
	return valueGeneratorFn(func() (*stroppy.Value, error) {
		return transformer(distribution.Next())
	})
}

// Deprecated
// it was so beutifull refactor I don't want to kill it
func newValueGenerator[T primitive.Primitive]( //nolint: ireturn // need from lib
	distribution primitiveGenerator[T],
	transformer valueTransformer[T],
	nullPercent uint32,
	constant *T,
) ValueGenerator {
	var generator ValueGenerator

	if constant != nil {
		generator = newConstValueGenerator(*constant, transformer)
	}

	if distribution != nil {
		generator = newRangeGenerator(distribution, transformer)
	}

	if nullPercent > 0 {
		generator = wrapNilQuota(generator, nullPercent)
	}

	return generator
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

func boolToUint8(boolean bool) uint8 {

	val := uint8(0)
	if boolean {
		val = 1
	}

	return val
}

func dateTimePtrToTimePtr(dt *stroppy.DateTime) time.Time {
	val := dt.GetValue().AsTime()
	return val
}

func decimalPtrToDecimalPtr(d *stroppy.Decimal) decimal.Decimal {
	if d == nil {
		logger.Global().Sugar().Error("nil Decimal value", d.GetValue())
		var dd decimal.Decimal
		return dd
	}

	val, err := decimal.NewFromString(d.GetValue())
	if err != nil {
		logger.Global().Sugar().Error("can't parse decimal value", d.GetValue(), err)
	}

	return val
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
