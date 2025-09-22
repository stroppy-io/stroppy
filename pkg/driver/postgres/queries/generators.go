package queries

import (
	"fmt"

	cmap "github.com/orcaman/concurrent-map/v2"

	"github.com/stroppy-io/stroppy/pkg/core/generate"
	stroppy "github.com/stroppy-io/stroppy/pkg/core/proto"
)

type (
	GeneratorID string
	Generators  = cmap.ConcurrentMap[GeneratorID, generate.ValueGenerator]
)

func NewGeneratorID(stepID, queryID, paramID string) GeneratorID {
	return GeneratorID(fmt.Sprintf("%s:%s:%s", stepID, queryID, paramID))
}

func (g GeneratorID) String() string {
	return string(g)
}

func collectQueryGenerators(
	runContext *stroppy.StepContext,
	queryDescriptor *stroppy.QueryDescriptor,
) (Generators, error) {
	generators := cmap.NewStringer[GeneratorID, generate.ValueGenerator]()

	for _, param := range queryDescriptor.GetParams() {
		paramID := NewGeneratorID(
			"", // TODO:  //runContext.GetStep().GetName(),
			queryDescriptor.GetName(),
			param.GetName(),
		)

		generator, err := generate.NewValueGenerator(
			runContext.GetGlobalConfig().GetRun().GetSeed(),
			1000000, // TODO: get proper amount
			param,
		)
		if err != nil {
			return generators, err
		}

		generators.Set(paramID, generator)
	}

	return generators, nil
}

func CollectStepGenerators(
	runContext *stroppy.StepContext,
) (Generators, error) { //nolint: gocognit // allow
	generators := cmap.NewStringer[GeneratorID, generate.ValueGenerator]()

	for _, step := range runContext.GetGlobalConfig().GetBenchmark().GetSteps() {
		for _, queryDescriptor := range step.GetUnits() {
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
		}
	}

	return generators, nil
}
