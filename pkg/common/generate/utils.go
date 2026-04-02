package generate

import (
	"math/rand/v2"
	"time"

	"github.com/shopspring/decimal"

	"github.com/stroppy-io/stroppy/pkg/common/generate/constraint"
	"github.com/stroppy-io/stroppy/pkg/common/generate/primitive"
	"github.com/stroppy-io/stroppy/pkg/common/logger"
	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
)

type (
	primitiveGenerator[T primitive.Primitive] interface {
		Next() T
	}
	valueGeneratorFn                        func() (any, error)
	valueTransformer[T primitive.Primitive] func(T) (any, error)
)

func (f valueGeneratorFn) Next() (any, error) {
	return f()
}

const Persent100 = 100

func wrapNilQuota(
	gen ValueGenerator,
	nullPercent uint32,
) ValueGenerator {
	percent := float64(nullPercent) / Persent100

	return valueGeneratorFn(func() (any, error) {
		if rand.Float64() < percent { //nolint:gosec // performance in priority here (against crypto/rand)
			return nil, nil
		}

		return gen.Next()
	})
}

func newConstValueGenerator[T primitive.Primitive](
	constant T,
	transformer valueTransformer[T],
) ValueGenerator {
	return valueGeneratorFn(func() (any, error) {
		return transformer(constant)
	})
}

func newRangeGenerator[T primitive.Primitive](
	distribution primitiveGenerator[T],
	transformer valueTransformer[T],
) ValueGenerator {
	return valueGeneratorFn(func() (any, error) {
		return transformer(distribution.Next())
	})
}

type rangeWrapper[T constraint.Number] struct {
	min T
	max T
}

func newRangeWrapper[T constraint.Number](minVal, maxVal T) *rangeWrapper[T] {
	return &rangeWrapper[T]{min: minVal, max: maxVal}
}

func (r rangeWrapper[T]) GetMin() T {
	return r.min
}

func (r rangeWrapper[T]) GetMax() T {
	return r.max
}

// Values conversion ---------------------------------------------------------------------------------------------------

func float32ToValue(f float32) (any, error)         { return f, nil }
func float64ToValue(f float64) (any, error)         { return f, nil }
func uint8ToBoolValue(b uint8) (any, error)         { return b == 1, nil }
func uint32ToValue(i uint32) (any, error)           { return i, nil }
func uint64ToValue(i uint64) (any, error)           { return i, nil }
func int32ToValue(i int32) (any, error)             { return i, nil }
func int64ToValue(i int64) (any, error)             { return i, nil }
func stringToValue(s string) (any, error)           { return s, nil }
func decimalToValue(d decimal.Decimal) (any, error) { return d, nil }
func dateTimeToValue(t time.Time) (any, error)      { return t, nil }

func boolToUint8(boolean bool) uint8 {
	val := uint8(0)
	if boolean {
		val = 1
	}

	return val
}

func dateTimePtrToTime(dt *stroppy.DateTime) time.Time {
	val := dt.GetValue().AsTime()

	return val
}

func decimalPtrToDecimal(decimalPtr *stroppy.Decimal) decimal.Decimal {
	if decimalPtr == nil {
		logger.Global().Sugar().Error("nil Decimal value", decimalPtr.GetValue())

		return decimal.Decimal{}
	}

	val, err := decimal.NewFromString(decimalPtr.GetValue())
	if err != nil {
		logger.Global().Sugar().Error("can't parse decimal value", decimalPtr.GetValue(), err)
	}

	return val
}

func alphabetToChars(alphabet *stroppy.Generation_Alphabet) [][2]int32 {
	ranges := make([][2]int32, 0, len(alphabet.GetRanges()))
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
