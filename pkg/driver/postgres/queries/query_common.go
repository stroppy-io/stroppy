package queries

import (
	sqlqueries "github.com/stroppy-io/stroppy/pkg/driver/sqldriver/queries"
)

type (
	GeneratorID = sqlqueries.GeneratorID
	Generators  = sqlqueries.Generators
)

func GenParamValues(
	genIDs []GeneratorID,
	generators Generators,
	valuesOut []any,
) error {
	return sqlqueries.GenParamValues(pgxDialect{}, genIDs, generators, valuesOut)
}
