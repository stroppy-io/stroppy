package stdlib

import (
	"fmt"
	"unicode/utf8"
)

// asciiCaseShift is the constant delta between 'A' and 'a' in ASCII.
const asciiCaseShift byte = 'a' - 'A'

func init() {
	registry["std.lower"] = lowerFunc
	registry["std.upper"] = upperFunc
	registry["std.substr"] = substrFunc
	registry["std.len"] = lenFunc
	registry["std.toString"] = toStringFunc
}

// lowerFunc implements `std.lower(s string) → string`. Only ASCII
// letters are folded; non-ASCII bytes pass through untouched. The spec
// catalog is deliberately ASCII-only to stay byte-stable across
// locales; a Unicode lowercase primitive can be added to the catalog
// later if a workload needs it.
func lowerFunc(args []any) (any, error) {
	source, err := singleString(args, "std.lower")
	if err != nil {
		return nil, err
	}

	buf := []byte(source)
	for i, char := range buf {
		if char >= 'A' && char <= 'Z' {
			buf[i] = char + asciiCaseShift
		}
	}

	return string(buf), nil
}

// upperFunc implements `std.upper(s string) → string`. ASCII-only for
// the same reason as lowerFunc.
func upperFunc(args []any) (any, error) {
	source, err := singleString(args, "std.upper")
	if err != nil {
		return nil, err
	}

	buf := []byte(source)
	for i, char := range buf {
		if char >= 'a' && char <= 'z' {
			buf[i] = char - asciiCaseShift
		}
	}

	return string(buf), nil
}

// substrFunc implements `std.substr(s string, i int64, n int64) → string`.
// Both indexes are in runes. Out-of-range indices clamp to the string
// bounds: a negative i starts at rune 0, and a length that overshoots
// the end stops at the end. A negative length is treated as zero.
func substrFunc(args []any) (any, error) {
	const wantArgs = 3
	if len(args) != wantArgs {
		return nil, fmt.Errorf("%w: std.substr needs %d, got %d", ErrArity, wantArgs, len(args))
	}

	source, ok := toString(args[0])
	if !ok {
		return nil, fmt.Errorf("%w: std.substr arg 0: expected string, got %T", ErrArgType, args[0])
	}

	start, ok := toInt64(args[1])
	if !ok {
		return nil, fmt.Errorf("%w: std.substr arg 1: expected int64, got %T", ErrArgType, args[1])
	}

	length, ok := toInt64(args[2])
	if !ok {
		return nil, fmt.Errorf("%w: std.substr arg 2: expected int64, got %T", ErrArgType, args[2])
	}

	runes := []rune(source)
	total := int64(len(runes))

	if start < 0 {
		start = 0
	}

	if start >= total || length <= 0 {
		return "", nil
	}

	end := start + length
	if end > total {
		end = total
	}

	return string(runes[start:end]), nil
}

// lenFunc implements `std.len(s string) → int64` as the count of runes
// in the UTF-8 encoding of s.
func lenFunc(args []any) (any, error) {
	source, err := singleString(args, "std.len")
	if err != nil {
		return nil, err
	}

	return int64(utf8.RuneCountInString(source)), nil
}

// toStringFunc implements `std.toString(x any) → string` via fmt.Sprint.
// time.Time uses its MarshalText form so that SCD2 and date columns
// render ISO-8601 regardless of how they entered the Expr tree.
func toStringFunc(args []any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("%w: std.toString needs 1, got %d", ErrArity, len(args))
	}

	return fmt.Sprint(args[0]), nil
}

// singleString centralizes the arity + type check shared by lower,
// upper and len.
func singleString(args []any, fn string) (string, error) {
	if len(args) != 1 {
		return "", fmt.Errorf("%w: %s needs 1, got %d", ErrArity, fn, len(args))
	}

	source, ok := toString(args[0])
	if !ok {
		return "", fmt.Errorf("%w: %s arg 0: expected string, got %T", ErrArgType, fn, args[0])
	}

	return source, nil
}
