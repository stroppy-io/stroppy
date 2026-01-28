package queries

import (
	"github.com/stroppy-io/stroppy/pkg/common/generate"
	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
)

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
	generators := make(Generators)

	for _, param := range descriptor.GetParams() {
		paramID := GeneratorID(param.GetName())

		generator, err := generate.NewValueGenerator(seed, param)
		if err != nil {
			return generators, err
		}

		generators[paramID] = generator
	}

	for _, group := range descriptor.GetGroups() {
		generator := generate.NewTupleGenerator(
			seed,
			Out[generate.GenAbleStruct](group.GetParams()),
		)
		generators[GeneratorID(group.GetName())] = generator
	}

	return generators, nil
}
