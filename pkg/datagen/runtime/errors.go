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

// ErrUnsupportedDegree is returned when a Relationship side declares a
// Degree kind the current runtime does not implement (only Fixed is
// supported in Stage C).
var ErrUnsupportedDegree = errors.New("runtime: unsupported degree")

// ErrUnsupportedStrategy is returned when a Relationship side declares
// a Strategy other than Sequential (Hash and Equitable land later).
var ErrUnsupportedStrategy = errors.New("runtime: unsupported strategy")

// ErrUnsupportedArity is returned when a Relationship declares more
// than two sides; higher arity is deferred to a later stage.
var ErrUnsupportedArity = errors.New("runtime: unsupported relationship arity")

// ErrTooManyRelationships is returned when a RelSource declares more
// than one Relationship; multiple-relationship composition is deferred.
var ErrTooManyRelationships = errors.New("runtime: multiple relationships unsupported")

// ErrUnknownRelationship is returned when RelSource.iter names a
// relationship absent from RelSource.relationships.
var ErrUnknownRelationship = errors.New("runtime: unknown relationship in iter")

// ErrMissingLookupPop is returned when the outer side of a
// Relationship is not declared as a LookupPop.
var ErrMissingLookupPop = errors.New("runtime: outer side must be declared as LookupPop")

// ErrOuterPopMismatch is returned when no side of a Relationship
// matches the RelSource's population (inner side) or when both sides
// match it.
var ErrOuterPopMismatch = errors.New("runtime: relationship sides do not pair with RelSource population")

// ErrUnknownBlockSlot is returned when a BlockRef references a slot
// not declared on the enclosing Side.
var ErrUnknownBlockSlot = errors.New("runtime: unknown block slot")

// ErrBlockSlotEval is returned when a BlockSlot expression itself
// fails to evaluate.
var ErrBlockSlotEval = errors.New("runtime: block slot evaluation failed")
