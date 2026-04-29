package stdlib

import (
	"fmt"
	"strings"
)

// formatBadVerb is the substring Go's fmt package writes for unsatisfied
// verbs (missing arg, wrong type). Its presence turns the operation into
// a user-visible error rather than silently emitting "%!d(MISSING)" text.
const formatBadVerb = "%!"

func init() {
	registry["std.format"] = formatFunc
}

// formatFunc implements `std.format(fmt string, args... any) → string`.
// It wraps fmt.Sprintf with strict detection of fmt-verb errors: any
// output containing a "%!" sentinel is converted into ErrBadArg, so
// format mistakes surface during generation instead of silently
// poisoning output rows.
func formatFunc(args []any) (any, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("%w: std.format needs at least 1, got 0", ErrArity)
	}

	format, ok := toString(args[0])
	if !ok {
		return nil, fmt.Errorf("%w: std.format arg 0: expected string, got %T", ErrArgType, args[0])
	}

	out := fmt.Sprintf(format, args[1:]...)
	if strings.Contains(out, formatBadVerb) {
		return nil, fmt.Errorf("%w: std.format: bad verb or missing arg in %q -> %q", ErrBadArg, format, out)
	}

	return out, nil
}
