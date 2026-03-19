package queries

import (
	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	sqlqueries "github.com/stroppy-io/stroppy/pkg/driver/sqldriver/queries"
)

func NewInsertValues(
	generators Generators,
	descriptor *stroppy.InsertDescriptor,
	valuesOut []any,
) error {
	genIDs := InsertGenIDs(descriptor)

	return GenParamValues(genIDs, generators, valuesOut)
}

func InsertSQL(descriptor *stroppy.InsertDescriptor) string {
	return sqlqueries.InsertSQL(pgxDialect{}, descriptor)
}

func InsertGenIDs(descriptor *stroppy.InsertDescriptor) []GeneratorID {
	return sqlqueries.InsertGenIDs(descriptor)
}

func InsertColumns(descriptor *stroppy.InsertDescriptor) []string {
	return sqlqueries.InsertColumns(descriptor)
}
