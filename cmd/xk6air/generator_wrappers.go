package xk6air

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stroppy-io/stroppy/internal/common"
	"github.com/stroppy-io/stroppy/pkg/common/generate"
	"github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"google.golang.org/protobuf/proto"
)

func NewGeneratorByRuleBin(seed uint64, ruleBytes []byte) any {
	seed = generate.ResolveSeed(seed)

	var rule stroppy.Generation_Rule
	err := proto.Unmarshal(ruleBytes, &rule)
	if err != nil {
		return err // TODO: wrap errors
	}

	gen, err := generate.NewValueGeneratorByRule(seed, &rule)
	if err != nil {
		return err
	}

	return GeneratorWrapper{generator: gen, seed: seed}
}

func NewGroupGeneratorByRulesBin(seed uint64, rulesBytes []byte) any {
	seed = generate.ResolveSeed(seed)

	var rules stroppy.QueryParamGroup
	err := proto.Unmarshal(rulesBytes, &rules)
	if err != nil {
		return err // TODO: wrap errors
	}

	gen := generate.NewTupleGenerator(seed, common.Out[generate.GenAbleStruct](rules.GetParams()))

	return GeneratorWrapper{generator: gen, seed: seed}
}

type GeneratorWrapper struct {
	generator generate.ValueGenerator
	seed      uint64
}

func (g *GeneratorWrapper) Next() any {
	v, _ := g.generator.Next()
	return toJSValue(v)
}

func toJSValue(v any) any {
	switch typed := v.(type) {
	case uuid.UUID:
		return typed.String()
	case *string:
		return *typed
	case *time.Time:
		return *typed
	case *decimal.Decimal:
		return typed.String()
	case []any:
		results := make([]any, len(typed))
		for i, vv := range typed {
			results[i] = toJSValue(vv)
		}

		return results
	default:
		return v
	}
}
