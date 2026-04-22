package stdlib

import (
	"fmt"
	"strconv"
)

func init() {
	registry["std.parseInt"] = parseIntFunc
	registry["std.parseFloat"] = parseFloatFunc
}

// parseIntFunc implements `std.parseInt(s string) → int64`. It bridges
// numeric dict columns that arrive as strings on the wire: dstparse
// emits every `DictRow.values` entry as a string, including columns
// whose logical type is integer (e.g. tpch n_regionkey). An empty or
// unparseable input returns ErrBadArg so the mistake surfaces at
// generation time.
func parseIntFunc(args []any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("%w: std.parseInt needs 1, got %d", ErrArity, len(args))
	}

	source, ok := toString(args[0])
	if !ok {
		return nil, fmt.Errorf("%w: std.parseInt arg 0: expected string, got %T", ErrArgType, args[0])
	}

	if source == "" {
		return nil, fmt.Errorf("%w: std.parseInt: empty input", ErrBadArg)
	}

	value, err := strconv.ParseInt(source, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("%w: std.parseInt: %q: %w", ErrBadArg, source, err)
	}

	return value, nil
}

// parseFloatFunc implements `std.parseFloat(s string) → float64`. See
// parseIntFunc for rationale.
func parseFloatFunc(args []any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("%w: std.parseFloat needs 1, got %d", ErrArity, len(args))
	}

	source, ok := toString(args[0])
	if !ok {
		return nil, fmt.Errorf("%w: std.parseFloat arg 0: expected string, got %T", ErrArgType, args[0])
	}

	if source == "" {
		return nil, fmt.Errorf("%w: std.parseFloat: empty input", ErrBadArg)
	}

	value, err := strconv.ParseFloat(source, 64)
	if err != nil {
		return nil, fmt.Errorf("%w: std.parseFloat: %q: %w", ErrBadArg, source, err)
	}

	return value, nil
}
