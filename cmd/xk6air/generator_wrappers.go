package xk6air

import (
	"github.com/stroppy-io/stroppy/pkg/common/generate"
	"github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"github.com/stroppy-io/stroppy/pkg/driver/postgres/queries"
	"google.golang.org/protobuf/proto"
)

func NewGeneratorByRuleBin(seed uint64, ruleBytes []byte) any {
	var rule stroppy.Generation_Rule
	err := proto.Unmarshal(ruleBytes, &rule)
	if err != nil {
		return err // TODO: wrap errors
	}
	gen, err := generate.NewValueGeneratorByRule(seed, &rule)
	if err != nil {
		return err
	} else {
		return GeneratorWrapper{
			generator: gen,
			seed:      seed,
		}
	}
}

func NewGroupGeneratorByRulesBin(seed uint64, rulesBytes []byte) any {
	var rules stroppy.QueryParamGroup
	err := proto.Unmarshal(rulesBytes, &rules)
	if err != nil {
		return err // TODO: wrap errors
	}
	gen := generate.NewTupleGenerator(
		seed,
		queries.Out[generate.GenAbleStruct](rules.GetParams()),
	)
	return GeneratorWrapper{
		generator: gen,
		seed:      seed,
	}
}

type GeneratorWrapper struct {
	generator generate.ValueGenerator
	seed      uint64
}

func (g *GeneratorWrapper) Next() any {
	v, _ := g.generator.Next()
	return UnwrapValue(v)
}

func UnwrapValue(v *stroppy.Value) any {
	var result any
	switch t := v.GetType().(type) {
	case *stroppy.Value_Bool:
		result = t.Bool
	case *stroppy.Value_Datetime:
		result = t.Datetime
	case *stroppy.Value_Decimal:
		result = t.Decimal
	case *stroppy.Value_Double:
		result = t.Double
	case *stroppy.Value_Float:
		result = t.Float
	case *stroppy.Value_Int32:
		result = t.Int32
	case *stroppy.Value_Int64:
		result = t.Int64
	case *stroppy.Value_List_:
		results := make([]any, 0, len(t.List.GetValues()))
		for _, vv := range t.List.GetValues() {
			results = append(results, UnwrapValue(vv))
		}
		result = results
	case *stroppy.Value_Null:
		result = t.Null
	case *stroppy.Value_String_:
		result = t.String_
	case *stroppy.Value_Struct_:
		result = t.Struct
	case *stroppy.Value_Uint32:
		result = t.Uint32
	case *stroppy.Value_Uint64:
		result = t.Uint64
	case *stroppy.Value_Uuid:
		result = t.Uuid
	default:
		panic("unexpected stroppy.isValue_Type")
	}
	return result
}
