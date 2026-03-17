package queries

import (
	"fmt"

	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	sqlqueries "github.com/stroppy-io/stroppy/pkg/driver/sqldriver/queries"
)

var _ sqlqueries.Dialect = pgxDialect{}

type pgxDialect struct{}

func (pgxDialect) Placeholder(index int) string {
	return fmt.Sprintf("$%d", index+1)
}

func (pgxDialect) ValueToAny(v *stroppy.Value) (any, error) {
	return ValueToAny(v)
}
