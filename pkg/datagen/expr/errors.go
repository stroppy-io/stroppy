// Package expr is the Expr-tree evaluator for the datagen framework.
// It is a pure dispatcher: given an Expr and a Context, it returns the
// evaluated Go value or an error. Stdlib function bodies live in a
// separate package and reach the evaluator through Context.Call.
package expr

import "errors"

// ErrBadExpr is returned when an Expr is nil or carries no kind.
var ErrBadExpr = errors.New("expr: bad or empty expression")

// ErrUnknownCol is returned by Context.LookupCol when a ColRef names an
// attribute that has not been evaluated yet in the current row scratch.
var ErrUnknownCol = errors.New("expr: unknown column")

// ErrDictMissing is returned by Context.LookupDict when an opaque dict
// key is not present in the enclosing InsertSpec.dicts map.
var ErrDictMissing = errors.New("expr: dict missing")

// ErrDivByZero is returned by BinOp DIV when the divisor evaluates to zero.
var ErrDivByZero = errors.New("expr: division by zero")

// ErrModByZero is returned by BinOp MOD when the divisor evaluates to zero.
var ErrModByZero = errors.New("expr: modulo by zero")

// ErrTypeMismatch is returned when an operator receives operands whose
// types it cannot handle (for example ordering comparison on bools, or a
// non-bool condition passed to If).
var ErrTypeMismatch = errors.New("expr: type mismatch")

// ErrUnknownCall is returned by Context.Call when the named function is
// not registered with the stdlib dispatcher.
var ErrUnknownCall = errors.New("expr: unknown call")

// ErrBadDraw is returned by StreamDraw when the draw descriptor is nil,
// carries no arm, or violates its per-arm contract (empty alphabet,
// min > max, unknown column in a joint dict, etc.).
var ErrBadDraw = errors.New("expr: bad stream draw")

// ErrBadChoose is returned by Choose when no branch is declared, when a
// branch weight is non-positive, or when the cumulative weight is zero.
var ErrBadChoose = errors.New("expr: bad choose")
