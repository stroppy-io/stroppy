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

// should be [T ~R], but it's not allowed by syntax
func out[R any, T any](xs []T) (res []R) {
	res = make([]R, 0, len(xs))
	for _, x := range xs {
		res = append(res, any(x).(R)) //nolint:forceassert // allow panic
	}
	return res
}

func collectQueryGenerators(
	runContext *stroppy.StepContext,
	queryDescriptor *stroppy.QueryDescriptor,
) (Generators, error) {
	generators := cmap.NewStringer[GeneratorID, generate.ValueGenerator]()
	for _, param := range queryDescriptor.GetParams() {
		// TODO: tuple generator
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

	for _, queryDescriptor := range runContext.GetWorkload().GetUnits() {
		var err error

		generators, err = collectUnitGenerators(queryDescriptor, runContext, generators)
		if err != nil {
			return generators, err
		}
	}

	return generators, nil
}

func collectUnitGenerators(
	queryDescriptor *stroppy.WorkloadUnitDescriptor,
	runContext *stroppy.StepContext,
	generators cmap.ConcurrentMap[GeneratorID, generate.ValueGenerator],
) (Generators, error) {
	switch queryDescriptor.GetDescriptor_().GetType().(type) {
	case *stroppy.UnitDescriptor_Query:
		gens, err := collectQueryGenerators(runContext, queryDescriptor.GetDescriptor_().GetQuery())
		if err != nil {
			return generators, err
		}

		generators.MSet(gens.Items())
	case *stroppy.UnitDescriptor_Transaction:
		for _, query := range queryDescriptor.GetDescriptor_().GetTransaction().GetQueries() {
			gens, err := collectQueryGenerators(runContext, query)
			if err != nil {
				return generators, err
			}

			generators.MSet(gens.Items())
		}
	}

	return generators, nil
}
