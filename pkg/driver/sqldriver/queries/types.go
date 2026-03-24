package queries

import (
	"github.com/stroppy-io/stroppy/pkg/common/generate"
	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
)

type (
	GeneratorID = string
	Generators  = map[GeneratorID]generate.ValueGenerator
)

// Dialect abstracts database-specific SQL differences for database/sql drivers.
type Dialect interface {
	// Placeholder returns the SQL placeholder for the given 0-based parameter index.
	// For PostgreSQL: "$1", "$2", ...
	// For MySQL: "?", "?", ...
	Placeholder(index int) string

	// ValueToAny converts a proto Value to a Go type suitable for the target database.
	ValueToAny(v *stroppy.Value) (any, error)

	// Deduplicate reports whether repeated named parameters should share
	// a single positional placeholder and a single value in the args slice.
	// PostgreSQL's wire protocol supports $1 back-references, so pgx returns true.
	// database/sql drivers (MySQL, etc.) require one value per placeholder, so they return false.
	Deduplicate() bool
}
