package queries

import (
	"fmt"

	cmap "github.com/orcaman/concurrent-map/v2"

	"github.com/stroppy-io/stroppy/pkg/common/generate"
	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto"
)

type (
	GeneratorID string
	Generators  = cmap.ConcurrentMap[GeneratorID, generate.ValueGenerator]
)

func NewGeneratorID(queryID, paramID string) GeneratorID {
	return GeneratorID(fmt.Sprintf("%s:%s", queryID, paramID))
}

func (g GeneratorID) String() string {
	return string(g)
}

// should be [T ~R], but it's not allowed by syntax.
func out[R any, T any](xs []T) []R {
	res := make([]R, 0, len(xs))
	for _, x := range xs {
		res = append(res, any(x).(R)) //nolint:errcheck,forcetypeassert // allow panic
	}

	return res
}

func collectInsertGenerators(
	runContext *stroppy.StepContext,
	descriptor *stroppy.InsertDescriptor,
) (Generators, error) {
	generators := cmap.NewStringer[GeneratorID, generate.ValueGenerator]()

	for _, param := range descriptor.GetParams() {
		paramID := NewGeneratorID(
			descriptor.GetName(),
			param.GetName(),
		)

		generator, err := generate.NewValueGenerator(
			runContext.GetConfig().GetSeed(),
			param,
		)
		if err != nil {
			return generators, err
		}

		generators.Set(paramID, generator)
	}

	for _, group := range descriptor.GetGroups() {
		generator := generate.NewTupleGenerator(
			runContext.GetConfig().GetSeed(),
			out[generate.GenAbleStruct](group.GetParams()),
		)
		generators.Set(NewGeneratorID(descriptor.GetName(), group.GetName()), generator)
	}

	return generators, nil
}

func collectQueryGenerators(
	runContext *stroppy.StepContext,
	queryDescriptor *stroppy.QueryDescriptor,
) (Generators, error) {
	generators := cmap.NewStringer[GeneratorID, generate.ValueGenerator]()

	for _, param := range queryDescriptor.GetParams() {
		paramID := NewGeneratorID(
			queryDescriptor.GetName(),
			param.GetName(),
		)

		generator, err := generate.NewValueGenerator(
			runContext.GetConfig().GetSeed(),
			param,
		)
		if err != nil {
			return generators, err
		}

		generators.Set(paramID, generator)
	}

	for _, group := range queryDescriptor.GetGroups() {
		generator := generate.NewTupleGenerator(
			runContext.GetConfig().GetSeed(),
			out[generate.GenAbleStruct](group.GetParams()),
		)
		generators.Set(NewGeneratorID(queryDescriptor.GetName(), group.GetName()), generator)
	}

	return generators, nil
}

func CollectStepGenerators(
	runContext *stroppy.StepContext,
) (Generators, error) { //nolint: gocognit // allow
	generators := cmap.NewStringer[GeneratorID, generate.ValueGenerator]()

	for _, unit := range runContext.GetWorkload().GetUnits() {
		gens, err := collectUnitGenerators(unit.GetDescriptor_(), runContext)
		if err != nil {
			return generators, err
		}
		generators.MSet(gens.Items())
	}

	return generators, nil
}

func collectUnitGenerators(
	descriptor *stroppy.UnitDescriptor,
	runContext *stroppy.StepContext,
) (Generators, error) {
	switch typed := descriptor.GetType().(type) {
	case *stroppy.UnitDescriptor_Query:
		return collectQueryGenerators(runContext, typed.Query)
	case *stroppy.UnitDescriptor_Transaction:
		generators := cmap.NewStringer[GeneratorID, generate.ValueGenerator]()
		for _, query := range typed.Transaction.GetQueries() {
			gens, err := collectQueryGenerators(runContext, query)
			if err != nil {
				return generators, err
			}
			generators.MSet(gens.Items())
		}
		return generators, nil
	case *stroppy.UnitDescriptor_Insert:
		return collectInsertGenerators(runContext, typed.Insert)
	case *stroppy.UnitDescriptor_CreateTable: // do nothing
		return cmap.NewStringer[GeneratorID, generate.ValueGenerator](), nil
	default:
		panic(fmt.Sprintf("unknown type '%T' of descriptor isUnitDescriptor_Type", descriptor.GetType()))
	}
}
