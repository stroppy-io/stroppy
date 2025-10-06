package generate

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"google.golang.org/protobuf/proto"

	"github.com/stroppy-io/stroppy/pkg/common/generate/distribution"
	"github.com/stroppy-io/stroppy/pkg/common/generate/primitive"
	"github.com/stroppy-io/stroppy/pkg/common/generate/randstr"
	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto"
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
) valueGeneratorFn { //nolint:revive // revive is annoying to use
	if len(genInfos) == 0 {
		return func() (*stroppy.Value, error) { return nil, ErrNoGenerators }
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
	return func() (*stroppy.Value, error) {
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
	}
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

func NewValueGeneratorByRule( //nolint: funlen,ireturn // need from lib
	seed uint64,
	rule *stroppy.Generation_Rule,
) (ValueGenerator, error) {
	switch rule.GetType().(type) {
	case *stroppy.Generation_Rule_FloatRules:
		return newValueGenerator(
			primitive.NewNoTransformGenerator(
				distribution.NewDistributionGenerator[float32](
					rule.GetDistribution(),
					seed,
					rule.GetFloatRules().GetRange(),
					false,
					rule.GetUnique(),
				)),
			float32ToValue,
			rule.GetNullPercentage(),
			rule.GetFloatRules().Constant, //nolint: protogetter // allow cause need pointer
		), nil
	case *stroppy.Generation_Rule_DoubleRules:
		return newValueGenerator(
			primitive.NewNoTransformGenerator(
				distribution.NewDistributionGenerator[float64](
					rule.GetDistribution(),
					seed,
					rule.GetDoubleRules().GetRange(),
					false,
					rule.GetUnique(),
				)),
			float64ToValue,
			rule.GetNullPercentage(),
			rule.GetDoubleRules().Constant, //nolint: protogetter // allow cause need pointer
		), nil
	case *stroppy.Generation_Rule_Int32Rules:
		return newValueGenerator(
			primitive.NewNoTransformGenerator(
				distribution.NewDistributionGenerator[int32](
					rule.GetDistribution(),
					seed,
					rule.GetInt32Rules().GetRange(),
					true,
					rule.GetUnique(),
				)),
			int32ToValue,
			rule.GetNullPercentage(),

			rule.GetInt32Rules().Constant, //nolint: protogetter // allow cause need pointer
		), nil
	case *stroppy.Generation_Rule_Int64Rules:
		return newValueGenerator(
			primitive.NewNoTransformGenerator(
				distribution.NewDistributionGenerator[int64](
					rule.GetDistribution(),
					seed,
					rule.GetInt64Rules().GetRange(),
					true,
					rule.GetUnique(),
				)),
			int64ToValue,
			rule.GetNullPercentage(),

			rule.GetInt64Rules().Constant, //nolint: protogetter // allow cause need pointer
		), nil

	case *stroppy.Generation_Rule_Uint32Rules:
		return newValueGenerator(
			primitive.NewNoTransformGenerator(
				distribution.NewDistributionGenerator[uint32](
					rule.GetDistribution(),
					seed,
					rule.GetUint32Rules().GetRange(),
					true,
					rule.GetUnique(),
				)),
			uint32ToValue,
			rule.GetNullPercentage(),

			rule.GetUint32Rules().Constant, //nolint: protogetter // allow cause need pointer
		), nil
	case *stroppy.Generation_Rule_Uint64Rules:
		return newValueGenerator(
			primitive.NewNoTransformGenerator(
				distribution.NewDistributionGenerator[uint64](
					rule.GetDistribution(),
					seed,
					rule.GetUint64Rules().GetRange(),
					true,
					rule.GetUnique(),
				)),
			uint64ToValue,
			rule.GetNullPercentage(),

			rule.GetUint64Rules().Constant, //nolint: protogetter // allow cause need pointer
		), nil
	case *stroppy.Generation_Rule_BoolRules:
		return newValueGenerator(
			primitive.NewNoTransformGenerator(
				distribution.NewDistributionGenerator[uint8](
					rule.GetDistribution(),
					seed,
					newRangeWrapper[uint8](0, 1),
					true,
					rule.GetUnique(),
				)),
			uint8ToBoolValue,
			rule.GetNullPercentage(),

			boolPtrToUint8Ptr(rule.GetBoolRules().Constant), //nolint: protogetter // allow cause need pointer
		), nil
	case *stroppy.Generation_Rule_StringRules:
		return newValueGenerator(
			randstr.NewStringGenerator(
				seed,
				distribution.NewDistributionGenerator[uint64](
					rule.GetDistribution(),
					seed,
					rule.GetStringRules().GetLenRange(),
					false,
					rule.GetUnique(),
				),
				alphabetToChars(rule.GetStringRules().GetAlphabet()),
				rule.GetStringRules().GetLenRange().GetMax(),
			),
			stringToValue,
			rule.GetNullPercentage(),

			rule.GetStringRules().Constant, //nolint: protogetter // allow cause need pointer
		), nil
	case *stroppy.Generation_Rule_DatetimeRules:
		return newDateTimeGenerator(
			rule.GetDistribution(),
			seed,
			rule.GetDatetimeRules().GetRange(),
			rule.GetUnique(),
			rule.GetNullPercentage(),

			rule.GetDatetimeRules().Constant, //nolint: protogetter // allow cause need pointer
		)
	case *stroppy.Generation_Rule_UuidRules:
		return newUUIDGenerator(
			rule.GetDistribution(),
			seed,
			rule.GetNullPercentage(),

			rule.GetUuidRules().Constant, //nolint: protogetter // allow cause need pointer
		), nil
	case *stroppy.Generation_Rule_DecimalRules:
		return newDecimalGenerator(
			rule.GetDistribution(),
			seed,
			rule.GetDecimalRules().GetRange(),
			rule.GetUnique(),
			rule.GetNullPercentage(),

			rule.GetDecimalRules().Constant, //nolint: protogetter // allow cause need pointer
		)
	}

	return nil, fmt.Errorf("unknown rule type: %T", rule) //nolint: err113
}

func newDateTimeGenerator( //nolint: ireturn // need from lib
	distributeParams *stroppy.Generation_Distribution,
	seed uint64,
	ranges *stroppy.Generation_Range_DateTimeRange,
	unique bool,
	nullPercentage uint32,

	constant *stroppy.DateTime,
) (ValueGenerator, error) {
	var intRange [2]time.Time
	switch ranges.GetType().(type) {
	case *stroppy.Generation_Range_DateTimeRange_Default_:
		intRange[1] = ranges.GetDefault().GetMax().GetValue().AsTime()
		intRange[0] = ranges.GetDefault().GetMin().GetValue().AsTime()
	case *stroppy.Generation_Range_DateTimeRange_String_:
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
	case *stroppy.Generation_Range_DateTimeRange_TimestampPb_:
		intRange[0] = ranges.GetTimestampPb().GetMin().AsTime()
		intRange[1] = ranges.GetTimestampPb().GetMax().AsTime()
	case *stroppy.Generation_Range_DateTimeRange_Timestamp_:
		intRange[0] = time.Unix(int64(ranges.GetTimestamp().GetMin()), 0)
		intRange[1] = time.Unix(int64(ranges.GetTimestamp().GetMax()), 0)
	}

	atu := intRange[0].Unix()
	btu := intRange[1].Unix()
	diff := btu - atu

	return newValueGenerator(
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
		nullPercentage,

		dateTimePtrToTimePtr(constant),
	), nil
}

func newUUIDGenerator( //nolint: ireturn // need from lib
	_ *stroppy.Generation_Distribution,
	seed uint64,
	nullPercentage uint32,
	constant *stroppy.Uuid,
) ValueGenerator {
	var byteSlice [32]byte

	binary.LittleEndian.PutUint64(byteSlice[:8], seed)
	prng := rand.NewChaCha8(byteSlice)

	if constant != nil {
		return valueGeneratorFn(func() (*stroppy.Value, error) {
			return &stroppy.Value{
				Type: &stroppy.Value_Uuid{
					Uuid: &stroppy.Uuid{
						Value: constant.GetValue(),
					},
				},
			}, nil
		})
	}

	return wrapNilQuota(valueGeneratorFn(func() (*stroppy.Value, error) {
		uid, err := uuid.NewRandomFromReader(prng)
		if err != nil {
			return nil, fmt.Errorf("failed to generate uuid: %w", err)
		}

		return &stroppy.Value{
			Type: &stroppy.Value_Uuid{
				Uuid: &stroppy.Uuid{
					Value: uid.String(),
				},
			},
		}, nil
	}), nullPercentage)
}

func newDecimalGenerator( //nolint: ireturn // need from lib
	distributeParams *stroppy.Generation_Distribution,
	seed uint64,
	ranges *stroppy.Generation_Range_DecimalRange,
	unique bool,
	nullPercentage uint32,

	constant *stroppy.Decimal,
) (ValueGenerator, error) {
	var decRanges [2]decimal.Decimal

	switch ranges.GetType().(type) {
	case *stroppy.Generation_Range_DecimalRange_Default_:
		minDec, err := decimal.NewFromString(ranges.GetDefault().GetMin().GetValue())
		if err != nil {
			return nil, fmt.Errorf("failed to parse decimal: %w", err)
		}

		maxDec, err := decimal.NewFromString(ranges.GetDefault().GetMax().GetValue())
		if err != nil {
			return nil, fmt.Errorf("failed to parse decimal: %w", err)
		}

		decRanges[0] = minDec
		decRanges[1] = maxDec
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

	return newValueGenerator(
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
		nullPercentage,
		decimalPtrToDecimalPtr(constant),
	), nil
}
