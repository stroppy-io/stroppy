package queries

import (
	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	sqlqueries "github.com/stroppy-io/stroppy/pkg/driver/sqldriver/queries"
)

func collectInsertGenerators(
	seed uint64,
	descriptor *stroppy.InsertDescriptor,
) (Generators, error) {
	return sqlqueries.CollectInsertGenerators(seed, descriptor)
}
