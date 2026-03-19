package queries

import (
	"github.com/stroppy-io/stroppy/internal/common"
	"github.com/stroppy-io/stroppy/pkg/common/generate"
	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
)

func collectInsertGenerators(
	seed uint64,
	descriptor *stroppy.InsertDescriptor,
) (Generators, error) {
	generators := make(Generators)

	for _, param := range descriptor.GetParams() {
		paramID := param.GetName()

		generator, err := generate.NewValueGenerator(seed, param)
		if err != nil {
			return generators, err
		}

		generators[paramID] = generator
	}

	for _, group := range descriptor.GetGroups() {
		generator := generate.NewTupleGenerator(
			seed,
			common.Out[generate.GenAbleStruct](group.GetParams()),
		)
		generators[group.GetName()] = generator
	}

	return generators, nil
}
