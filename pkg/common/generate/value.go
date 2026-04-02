package generate

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"math/rand/v2"
	"reflect"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/stroppy-io/stroppy/pkg/common/generate/distribution"
	"github.com/stroppy-io/stroppy/pkg/common/generate/primitive"
	"github.com/stroppy-io/stroppy/pkg/common/generate/randstr"
	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
)

type ValueGenerator interface {
	Next() (any, error)
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
		return valueGeneratorFn(func() (any, error) { return nil, ErrNoGenerators })
	}

	count := len(genInfos)

	type depthState struct {
		gen ValueGenerator
		val any
	}

	state := make([]depthState, count)
	started := false
	done := false

	resetFrom := func(from int) error {
		for idx := from; idx < count; idx++ {
			gen, err := NewValueGenerator(seed, genInfos[idx])
			if err != nil {
				return err
			}

			val, err := gen.Next()
			if err != nil {
				return err
			}

			state[idx] = depthState{gen, val}
		}

		return nil
	}

	emit := func() []any {
		vals := make([]any, count)
		for i, s := range state {
			vals[i] = s.val
		}

		return vals
	}

	return valueGeneratorFn(func() (any, error) {
		if done {
			return nil, nil
		}

		if !started {
			started = true

			if err := resetFrom(0); err != nil {
				return nil, err
			}

			return emit(), nil
		}

		for depth := count - 1; depth >= 0; depth-- {
			newVal, err := state[depth].gen.Next()
			if err != nil {
				return nil, err
			}

			if !reflect.DeepEqual(newVal, state[depth].val) {
				state[depth].val = newVal

				if err := resetFrom(depth + 1); err != nil {
					return nil, err
				}

				return emit(), nil
			}
		}

		done = true

		return nil, nil
	})
}

func NewValueGenerator(
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

//nolint:funlen,cyclop // need from lib
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
	case *stroppy.Generation_Rule_UuidRandom:
		generator = newUUIDGenerator(nil)
	case *stroppy.Generation_Rule_UuidConst:
		generator = newUUIDGenerator(rule.GetUuidConst()) //nolint: protogetter // need pointer
	case *stroppy.Generation_Rule_UuidSeeded:
		generator = newUUIDSeededGenerator(seed)
	case *stroppy.Generation_Rule_UuidSeq:
		var err error

		generator, err = newUUIDSequentialGenerator(rule.GetUuidSeq())
		if err != nil {
			return nil, err
		}
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

func newDateTimeGenerator(
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

func newUUIDSeededGenerator(seed uint64) ValueGenerator {
	var byteSlice [32]byte

	binary.LittleEndian.PutUint64(byteSlice[:8], seed)
	prng := rand.NewChaCha8(byteSlice)

	return valueGeneratorFn(func() (any, error) {
		uid, err := uuid.NewRandomFromReader(prng)
		if err != nil {
			return nil, fmt.Errorf("failed to generate seeded uuid: %w", err)
		}

		return uid, nil
	})
}

func newUUIDSequentialGenerator(
	uuidSeqRange *stroppy.Generation_Range_UuidSeq,
) (ValueGenerator, error) {
	var startBytes [16]byte // nil UUID by default

	if minUUID := uuidSeqRange.GetMin(); minUUID != nil {
		uid, err := uuid.Parse(minUUID.GetValue())
		if err != nil {
			return nil, fmt.Errorf("failed to parse min uuid: %w", err)
		}

		startBytes = uid
	}

	maxUID, err := uuid.Parse(uuidSeqRange.GetMax().GetValue())
	if err != nil {
		return nil, fmt.Errorf("failed to parse max uuid: %w", err)
	}

	current := new(big.Int).SetBytes(startBytes[:])
	end := new(big.Int).SetBytes(maxUID[:])
	one := big.NewInt(1)

	return valueGeneratorFn(func() (any, error) {
		b := current.Bytes()

		var uid [16]byte

		copy(uid[16-len(b):], b) // right-align into big-endian 128-bit

		if current.Cmp(end) > 0 {
			// at the end should return same value, this semantic used by [NewTupleGenerator]
			// silly, but works for now
			return uuid.UUID(uid), nil
		}

		current.Add(current, one)

		return uuid.UUID(uid), nil
	}), nil
}

func newUUIDGenerator(constant *stroppy.Uuid) ValueGenerator {
	if constant != nil {
		uid, err := uuid.Parse(constant.GetValue())

		return valueGeneratorFn(func() (any, error) {
			if err != nil {
				return nil, fmt.Errorf("failed to parse const uuid: %w", err)
			}

			return uid, nil
		})
	}

	return valueGeneratorFn(func() (any, error) {
		uid, err := uuid.NewRandom()
		if err != nil {
			return nil, fmt.Errorf("failed to generate uuid: %w", err)
		}

		return uid, nil
	})
}

func newDecimalGenerator(
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
