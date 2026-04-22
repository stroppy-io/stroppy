package stdlib

import (
	"fmt"

	"github.com/stroppy-io/stroppy/pkg/datagen/seed"
)

func init() {
	registry["std.hashMod"] = hashMod
}

// hashMod implements `std.hashMod(n int64, k int64) → int64`.
// It returns `int64(splitmix64(uint64(n))) mod k`. The modulo is Go's
// default signed-remainder: sign of the result follows the dividend.
// That is acceptable because the numerator here is a bit-mixer output
// reinterpreted as signed, and callers using hashMod as a bucket index
// are expected to feed positive `k` and guard the call at the TS layer
// with `abs` when they need a non-negative result.
func hashMod(args []any) (any, error) {
	const wantArgs = 2
	if len(args) != wantArgs {
		return nil, fmt.Errorf("%w: std.hashMod needs %d, got %d", ErrArity, wantArgs, len(args))
	}

	num, ok := toInt64(args[0])
	if !ok {
		return nil, fmt.Errorf("%w: std.hashMod arg 0: expected int64, got %T", ErrArgType, args[0])
	}

	modulus, ok := toInt64(args[1])
	if !ok {
		return nil, fmt.Errorf("%w: std.hashMod arg 1: expected int64, got %T", ErrArgType, args[1])
	}

	if modulus <= 0 {
		return nil, fmt.Errorf("%w: std.hashMod k must be > 0, got %d", ErrBadArg, modulus)
	}

	mixed := int64(seed.SplitMix64(uint64(num))) //nolint:gosec // bit reinterpret is intentional

	return mixed % modulus, nil
}
