package generate

import (
	"errors"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
	"google.golang.org/protobuf/proto"

	"github.com/stroppy-io/stroppy/pkg/common/generate/distribution"
	"github.com/stroppy-io/stroppy/pkg/common/generate/primitive"
	"github.com/stroppy-io/stroppy/pkg/common/generate/randstr"
	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
)

type ValueGenerator interface {
	Next() (*stroppy.Value, error)
}

type GenAbleStruct interface {
	GetGenerationRule() *stroppy.Generation_Rule
	GetName() string
}

var ErrNoGenerators = errors.New("no generators provided")

//nolint:gocognit // it's hard indeed
func NewTupleGenerator(
	seed uint64,
	genInfos []GenAbleStruct,
) ValueGenerator { //nolint:revive // revive is annoying to use
	if len(genInfos) == 0 {
		return valueGeneratorFn(func() (*stroppy.Value, error) { return nil, ErrNoGenerators })
	}

	// Result type to send both value and error through channel
	type result struct {
		vals []*stroppy.Value
		err  error
	}

	// Create buffered channel for results
	resultCh := make(chan result, 1)

	// Start goroutine to generate cartesian product
	go func() {
		defer close(resultCh)

		// Recursive function to iterate through all combinations
		var iterate func(depth int, current []*stroppy.Value) bool

		iterate = func(depth int, current []*stroppy.Value) bool {
			if depth == len(genInfos) {
				res := make([]*stroppy.Value, len(current))
				copy(res, current)

				resultCh <- result{vals: res, err: nil}

				return true
			}

			gen, err := NewValueGenerator(seed, genInfos[depth])
			if err != nil {
				resultCh <- result{vals: nil, err: err}

				return false
			}

			val, err := gen.Next()
			if err != nil {
				resultCh <- result{vals: nil, err: err}

				return false
			}

			for {
				current[depth] = val
				if !iterate(depth+1, current) {
					return false
				}

				newVal, err := gen.Next()
				if err != nil {
					resultCh <- result{vals: nil, err: err}

					return false
				}

				if proto.Equal(val, newVal) {
					return true
				}

				val = newVal
			}
		}

		iterate(0, make([]*stroppy.Value, len(genInfos)))
	}()

	// Return function that reads from channel
	return valueGeneratorFn(func() (*stroppy.Value, error) {
		res, ok := <-resultCh
		if !ok {
			// Channel closed, no more values
			return nil, nil
		}

		if res.err != nil {
			return nil, res.err
		}

		return &stroppy.Value{
			Type: &stroppy.Value_List_{List: &stroppy.Value_List{Values: res.vals}},
		}, nil
	})
}

func NewValueGenerator( //nolint: ireturn // need as lib part
	seed uint64,
	genInfo GenAbleStruct,
) (ValueGenerator, error) {
	gen, err := NewValueGeneratorByRule(seed, genInfo.GetGenerationRule())
	if err != nil {
		return nil, fmt.Errorf(
			"failed to create generator for entity '%s': %w",
			genInfo.GetName(),
			err,
		)
	}

	return gen, nil
}

//nolint:funlen,cyclop,ireturn // need from lib
func NewValueGeneratorByRule(
	seed uint64,
	rule *stroppy.Generation_Rule,
) (ValueGenerator, error) {
	var generator ValueGenerator

	switch rule.GetKind().(type) {
	case *stroppy.Generation_Rule_FloatRange:
		generator = newRangeGenerator(
			primitive.NewNoTransformGenerator(
				distribution.NewDistributionGenerator[float32](
					rule.GetDistribution(),
					seed,
					rule.GetFloatRange(),
					false,
					rule.GetUnique(),
				),
			),
			float32ToValue,
		)
	case *stroppy.Generation_Rule_FloatConst:
		generator = newConstValueGenerator(rule.GetFloatConst(), float32ToValue)
	case *stroppy.Generation_Rule_DoubleRange:
		generator = newRangeGenerator(
			primitive.NewNoTransformGenerator(
				distribution.NewDistributionGenerator[float64](
					rule.GetDistribution(),
					seed,
					rule.GetDoubleRange(),
					false,
					rule.GetUnique(),
				)), float64ToValue)
	case *stroppy.Generation_Rule_DoubleConst:
		generator = newConstValueGenerator(rule.GetDoubleConst(), float64ToValue)
	case *stroppy.Generation_Rule_Int32Range:
		generator = newRangeGenerator(
			primitive.NewNoTransformGenerator(
				distribution.NewDistributionGenerator[int32](
					rule.GetDistribution(),
					seed,
					rule.GetInt32Range(),
					true,
					rule.GetUnique(),
				)),
			int32ToValue,
		)
	case *stroppy.Generation_Rule_Int32Const:
		generator = newConstValueGenerator(rule.GetInt32Const(), int32ToValue)
	case *stroppy.Generation_Rule_Int64Range:
		generator = newRangeGenerator(
			primitive.NewNoTransformGenerator(
				distribution.NewDistributionGenerator[int64](
					rule.GetDistribution(),
					seed,
					rule.GetInt64Range(),
					true,
					rule.GetUnique(),
				)),
			int64ToValue,
		)
	case *stroppy.Generation_Rule_Int64Const:
		generator = newConstValueGenerator(rule.GetInt64Const(), int64ToValue)
	case *stroppy.Generation_Rule_Uint32Range:
		generator = newRangeGenerator(
			primitive.NewNoTransformGenerator(
				distribution.NewDistributionGenerator[uint32](
					rule.GetDistribution(),
					seed,
					rule.GetUint32Range(),
					true,
					rule.GetUnique(),
				)),
			uint32ToValue,
		)
	case *stroppy.Generation_Rule_Uint32Const:
		generator = newConstValueGenerator(rule.GetUint32Const(), uint32ToValue)
	case *stroppy.Generation_Rule_Uint64Range:
		generator = newRangeGenerator(
			primitive.NewNoTransformGenerator(
				distribution.NewDistributionGenerator[uint64](
					rule.GetDistribution(),
					seed,
					rule.GetUint64Range(),
					true,
					rule.GetUnique(),
				)),
			uint64ToValue,
		)
	case *stroppy.Generation_Rule_Uint64Const:
		generator = newConstValueGenerator(rule.GetUint64Const(), uint64ToValue)
	case *stroppy.Generation_Rule_BoolRange:
		generator = newRangeGenerator(
			primitive.NewNoTransformGenerator(
				distribution.NewDistributionGenerator[uint8](
					rule.GetDistribution(),
					seed,
					newRangeWrapper[uint8](0, 1),
					true,
					rule.GetUnique(),
				)),
			uint8ToBoolValue,
		)
	case *stroppy.Generation_Rule_BoolConst:
		generator = newConstValueGenerator(boolToUint8(rule.GetBoolConst()), uint8ToBoolValue)
	case *stroppy.Generation_Rule_StringRange:
		strRange := rule.GetStringRange()
		generator = newRangeGenerator(
			randstr.NewStringGenerator(
				seed,
				distribution.NewDistributionGenerator[uint64](
					rule.GetDistribution(),
					seed,
					newRangeWrapper(strRange.GetMinLen(), strRange.GetMaxLen()),
					false,
					rule.GetUnique(),
				),
				alphabetToChars(strRange.GetAlphabet()),
				strRange.GetMaxLen(),
			),
			stringToValue,
		)
	case *stroppy.Generation_Rule_StringConst:
		generator = newConstValueGenerator(rule.GetStringConst(), stringToValue)
	case *stroppy.Generation_Rule_DatetimeRange:
		var err error

		generator, err = newDateTimeGenerator(
			rule.GetDistribution(),
			seed,
			rule.GetDatetimeRange(),
			rule.GetUnique(),
		)
		if err != nil {
			return nil, err
		}
	case *stroppy.Generation_Rule_DatetimeConst:
		generator = newConstValueGenerator(dateTimePtrToTime(rule.GetDatetimeConst()), dateTimeToValue)
	// TODO: make it better
	// case *stroppy.Generation_Rule_UuidRules:
	// 	return newUUIDGenerator(
	// 		rule.GetDistribution(),
	// 		seed,
	// 		rule.GetNullPercentage(),

	// 		rule.GetUuidRules().Constant, //nolint: protogetter // allow cause need pointer
	// 	), nil
	case *stroppy.Generation_Rule_DecimalRange:
		var err error

		generator, err = newDecimalGenerator(
			rule.GetDistribution(),
			seed,
			rule.GetDecimalRange(),
			rule.GetUnique(),
		)
		if err != nil {
			return nil, err
		}
	case *stroppy.Generation_Rule_DecimalConst:
		generator = newConstValueGenerator(decimalPtrToDecimal(rule.GetDecimalConst()), decimalToValue)
	default:
		return nil, fmt.Errorf("unknown rule type: %T, %v", rule, rule) //nolint: err113
	}

	if rule.GetNullPercentage() > 0 {
		generator = wrapNilQuota(generator, rule.GetNullPercentage())
	}

	return generator, nil
}

func newDateTimeGenerator( //nolint: ireturn // need from lib
	distributeParams *stroppy.Generation_Distribution,
	seed uint64,
	ranges *stroppy.Generation_Range_DateTime,
	unique bool,
) (ValueGenerator, error) {
	var intRange [2]time.Time

	switch ranges.GetType().(type) {
	case *stroppy.Generation_Range_DateTime_String_:
		mins, err := time.Parse(time.RFC3339, ranges.GetString_().GetMin())
		if err != nil {
			return nil, fmt.Errorf("failed to parse time: %w", err)
		}

		maxs, err := time.Parse(time.RFC3339, ranges.GetString_().GetMin())
		if err != nil {
			return nil, fmt.Errorf("failed to parse time: %w", err)
		}

		intRange[0] = mins
		intRange[1] = maxs
	case *stroppy.Generation_Range_DateTime_TimestampPb_:
		intRange[0] = ranges.GetTimestampPb().GetMin().AsTime()
		intRange[1] = ranges.GetTimestampPb().GetMax().AsTime()
	case *stroppy.Generation_Range_DateTime_Timestamp:
		intRange[0] = time.Unix(int64(ranges.GetTimestamp().GetMin()), 0)
		intRange[1] = time.Unix(int64(ranges.GetTimestamp().GetMax()), 0)
	}

	atu := intRange[0].Unix()
	btu := intRange[1].Unix()
	diff := btu - atu

	return newRangeGenerator(
		primitive.NewGenerator(
			distribution.NewDistributionGenerator[int64](
				distributeParams,
				seed,
				newRangeWrapper(0, diff),
				true,
				unique,
			),
			func(d int64) time.Time {
				return time.Unix(d+atu, 0)
			},
		),
		dateTimeToValue,
	), nil
}

// FIXME: UUID generator
// func newUUIDGenerator( //nolint: ireturn // need from lib
// 	_ *stroppy.Generation_Distribution,
// 	seed uint64,
// 	nullPercentage uint32,
// 	constant *stroppy.Uuid,
// ) ValueGenerator {
// 	var byteSlice [32]byte

// 	binary.LittleEndian.PutUint64(byteSlice[:8], seed)
// 	prng := rand.NewChaCha8(byteSlice)

// 	if constant != nil {
// 		return valueGeneratorFn(func() (*stroppy.Value, error) {
// 			return &stroppy.Value{
// 				Type: &stroppy.Value_Uuid{
// 					Uuid: &stroppy.Uuid{
// 						Value: constant.GetValue(),
// 					},
// 				},
// 			}, nil
// 		})
// 	}

// 	return wrapNilQuota(valueGeneratorFn(func() (*stroppy.Value, error) {
// 		uid, err := uuid.NewRandomFromReader(prng)
// 		if err != nil {
// 			return nil, fmt.Errorf("failed to generate uuid: %w", err)
// 		}

// 		return &stroppy.Value{
// 			Type: &stroppy.Value_Uuid{
// 				Uuid: &stroppy.Uuid{
// 					Value: uid.String(),
// 				},
// 			},
// 		}, nil
// 	}), nullPercentage)
// }

func newDecimalGenerator( //nolint: ireturn // need from lib
	distributeParams *stroppy.Generation_Distribution,
	seed uint64,
	ranges *stroppy.Generation_Range_DecimalRange,
	unique bool,
) (ValueGenerator, error) {
	var decRanges [2]decimal.Decimal

	switch ranges.GetType().(type) {
	case *stroppy.Generation_Range_DecimalRange_Float:
		decRanges[0] = decimal.NewFromFloat(float64(ranges.GetFloat().GetMin()))
		decRanges[1] = decimal.NewFromFloat(float64(ranges.GetFloat().GetMax()))
	case *stroppy.Generation_Range_DecimalRange_Double:
		decRanges[0] = decimal.NewFromFloat(ranges.GetDouble().GetMin())
		decRanges[1] = decimal.NewFromFloat(ranges.GetDouble().GetMax())
	case *stroppy.Generation_Range_DecimalRange_String_:
		minDec, err := decimal.NewFromString(ranges.GetString_().GetMin())
		if err != nil {
			return nil, fmt.Errorf("failed to parse decimal: %w", err)
		}

		maxDec, err := decimal.NewFromString(ranges.GetString_().GetMax())
		if err != nil {
			return nil, fmt.Errorf("failed to parse decimal: %w", err)
		}

		decRanges[0] = minDec
		decRanges[1] = maxDec
	}

	return newRangeGenerator(
		primitive.NewGenerator(
			distribution.NewDistributionGenerator[float64](
				distributeParams,
				seed,
				newRangeWrapper(decRanges[0].InexactFloat64(), decRanges[1].InexactFloat64()),
				true,
				unique,
			),
			decimal.NewFromFloat,
		),
		decimalToValue,
	), nil
}
