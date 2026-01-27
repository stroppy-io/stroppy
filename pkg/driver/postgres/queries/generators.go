package queries

import (
	"fmt"

	cmap "github.com/orcaman/concurrent-map/v2"

	"github.com/stroppy-io/stroppy/pkg/common/generate"
	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
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

// Out type params should be [T ~R], but it's not allowed by syntax.
func Out[R any, T any](xs []T) []R {
	res := make([]R, 0, len(xs))
	for _, x := range xs {
		res = append(res, any(x).(R)) //nolint:errcheck,forcetypeassert // allow panic
	}

	return res
}

func collectInsertGenerators(
	seed uint64,
	descriptor *stroppy.InsertDescriptor,
) (Generators, error) {
	generators := cmap.NewStringer[GeneratorID, generate.ValueGenerator]()

	for _, param := range descriptor.GetParams() {
		paramID := NewGeneratorID(descriptor.GetTableName(), param.GetName())

		generator, err := generate.NewValueGenerator(seed, param)
		if err != nil {
			return generators, err
		}

		generators.Set(paramID, generator)
	}

	for _, group := range descriptor.GetGroups() {
		generator := generate.NewTupleGenerator(
			seed,
			Out[generate.GenAbleStruct](group.GetParams()),
		)
		generators.Set(NewGeneratorID(descriptor.GetTableName(), group.GetName()), generator)
	}

	return generators, nil
}

func collectUnitGenerators(
	descriptor *stroppy.InsertDescriptor,
	seed uint64,
) (Generators, error) {
	return collectInsertGenerators(seed, descriptor)
}
