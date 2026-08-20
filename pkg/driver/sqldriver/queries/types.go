package queries

import "time"

// Dialect abstracts database-specific SQL differences for database/sql drivers.
type Dialect interface {
	// Placeholder returns the SQL placeholder for the given 0-based parameter index.
	// For PostgreSQL: "$1", "$2", ...
	// For MySQL: "?", "?", ...
	Placeholder(index int) string

	// Convert converts a native Go value to a Go type suitable for binding to
	// the target database, both for insert rows and query arguments.
	Convert(v any) (any, error)

	// Deduplicate reports whether repeated named parameters should share
	// a single positional placeholder and a single value in the args slice.
	// PostgreSQL's wire protocol supports $1 back-references, so pgx returns true.
	// database/sql drivers (MySQL, etc.) require one value per placeholder, so they return false.
	Deduplicate() bool

	// StatementTimeoutHint returns sql with a server-side per-statement
	// timeout bound (e.g. an optimizer-hint comment) applied, or sql unchanged
	// when timeout is non-positive or the dialect has no such mechanism.
	// Dialects that bound execution server-side (MySQL MAX_EXECUTION_TIME)
	// keep the pooled connection reusable on timeout, where client-side context
	// cancellation alone would discard it.
	StatementTimeoutHint(sql string, timeout time.Duration) string
}
