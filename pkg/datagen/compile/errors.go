// Package compile performs compile-time validation of RelSource attrs.
// It walks each attr's Expr tree to extract ColRef dependencies, then
// produces a topologically ordered view of the attrs with producers
// preceding consumers. Consumers of this package are the runtime
// evaluator (it reads attrs in Order) and workload authors via error
// feedback when a spec is malformed.
package compile

import "errors"

// ErrCycle reports a cyclic dependency among attrs: at least one attr
// transitively depends on itself. The error message names the attrs
// that remained unordered after topological sort.
var ErrCycle = errors.New("compile: cyclic dependency in attrs")

// ErrUnknownRef reports an Expr that references an attribute name not
// present in the RelSource.
var ErrUnknownRef = errors.New("compile: unknown column reference")

// ErrDuplicateAttr reports two or more attrs sharing the same name.
var ErrDuplicateAttr = errors.New("compile: duplicate attr name")

// ErrNilAttr reports a nil entry in the attrs slice.
var ErrNilAttr = errors.New("compile: nil attr")
