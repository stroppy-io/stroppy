// Package runtime iterates the rows of a RelSource flat population,
// evaluating the compiled Expr DAG at each row index and emitting values
// in the requested column order. It is the non-parallel, non-relational
// core that Stage B closes; cross-population wiring and null injection
// are added by later stages.
package runtime

import "errors"

// ErrInvalidSpec is returned by NewRuntime when the InsertSpec or its
// nested RelSource is nil, or when Population.Size is not positive.
var ErrInvalidSpec = errors.New("runtime: invalid InsertSpec")

// ErrMissingColumn is returned by NewRuntime when a name in column_order
// does not match any attr declared by the RelSource.
var ErrMissingColumn = errors.New("runtime: column in column_order not in attrs")

// ErrEmptyColumnOrder is returned by NewRuntime when RelSource.column_order
// is empty: a row with zero columns has no meaning for the loader.
var ErrEmptyColumnOrder = errors.New("runtime: column_order required")

// ErrSeekOutOfRange is returned by Seek when the requested index is
// negative or past Population.Size.
var ErrSeekOutOfRange = errors.New("runtime: seek out of range")
