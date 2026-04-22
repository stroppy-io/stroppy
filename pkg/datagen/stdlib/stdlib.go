// Package stdlib is the closed, reviewed catalog of `std.*` functions
// reachable from an Expr tree via Context.Call. The registry is populated
// by package-internal init() calls; there is no public Register hook,
// because admitting a new primitive requires a source edit and review.
package stdlib

import (
	"errors"
	"fmt"
)

// ErrUnknownFunction is returned by Call when the requested function name
// is not present in the registry.
var ErrUnknownFunction = errors.New("stdlib: unknown function")

// ErrArity is returned when a stdlib function receives the wrong number of
// arguments.
var ErrArity = errors.New("stdlib: wrong argument count")

// ErrArgType is returned when a stdlib function receives an argument of a
// type it cannot losslessly coerce into its expected type.
var ErrArgType = errors.New("stdlib: wrong argument type")

// ErrBadArg is returned when an argument has a valid type but a value the
// function rejects (e.g. non-positive divisor, empty format verb).
var ErrBadArg = errors.New("stdlib: bad argument")

// registry maps a function name to its implementation. It is populated
// exclusively by init() blocks in sibling files of this package; no
// runtime mutation path exists.
var registry = map[string]func([]any) (any, error){}

// Call dispatches a stdlib function by name with already-evaluated
// arguments. Returns ErrUnknownFunction if the name is not registered;
// any other error is produced by the function implementation.
func Call(name string, args []any) (any, error) {
	impl, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownFunction, name)
	}

	return impl(args)
}

// Names returns a sorted-free snapshot of registered function names.
// Intended for tests that verify the catalog is non-empty.
func Names() []string {
	out := make([]string, 0, len(registry))
	for name := range registry {
		out = append(out, name)
	}

	return out
}

// toInt64 coerces a value into an int64 without loss. Accepts the signed
// integer types and uint8/uint16/uint32. Rejects floats and strings: those
// conversions are user errors and must be explicit in the Expr tree.
func toInt64(value any) (int64, bool) {
	switch typed := value.(type) {
	case int64:
		return typed, true
	case int:
		return int64(typed), true
	case int32:
		return int64(typed), true
	case int16:
		return int64(typed), true
	case int8:
		return int64(typed), true
	case uint8:
		return int64(typed), true
	case uint16:
		return int64(typed), true
	case uint32:
		return int64(typed), true
	default:
		return 0, false
	}
}

// toString coerces a value into a string only when the source type is
// already a string. fmt-style rendering lives in std.toString, which is
// explicit.
func toString(value any) (string, bool) {
	typed, ok := value.(string)

	return typed, ok
}
