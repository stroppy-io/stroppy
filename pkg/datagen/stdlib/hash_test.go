package stdlib_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stroppy-io/stroppy/pkg/datagen/seed"
	"github.com/stroppy-io/stroppy/pkg/datagen/stdlib"
)

func TestHashMod(t *testing.T) {
	t.Parallel()

	t.Run("matches_splitmix_mod", func(t *testing.T) {
		t.Parallel()

		const (
			input   int64 = 0xDEADBEEF
			modulus int64 = 97
		)

		got, err := stdlib.Call("std.hashMod", []any{input, modulus})
		require.NoError(t, err)

		//nolint:gosec // bit reinterpret intentional, matches impl
		expected := int64(seed.SplitMix64(uint64(input))) % modulus
		require.Equal(t, expected, got)
	})

	t.Run("accepts_int32", func(t *testing.T) {
		t.Parallel()

		// Both args widen losslessly: int32 → int64.
		got, err := stdlib.Call("std.hashMod", []any{int32(42), int32(10)})
		require.NoError(t, err)
		require.IsType(t, int64(0), got)
	})

	t.Run("deterministic", func(t *testing.T) {
		t.Parallel()

		first, err := stdlib.Call("std.hashMod", []any{int64(1_234), int64(11)})
		require.NoError(t, err)
		second, err := stdlib.Call("std.hashMod", []any{int64(1_234), int64(11)})
		require.NoError(t, err)
		require.Equal(t, first, second)
	})

	t.Run("arity", func(t *testing.T) {
		t.Parallel()

		_, err := stdlib.Call("std.hashMod", []any{int64(1)})
		require.ErrorIs(t, err, stdlib.ErrArity)
	})

	t.Run("type_on_n", func(t *testing.T) {
		t.Parallel()

		_, err := stdlib.Call("std.hashMod", []any{"1", int64(10)})
		require.ErrorIs(t, err, stdlib.ErrArgType)
	})

	t.Run("type_on_k", func(t *testing.T) {
		t.Parallel()

		_, err := stdlib.Call("std.hashMod", []any{int64(1), 1.5})
		require.ErrorIs(t, err, stdlib.ErrArgType)
	})

	t.Run("k_zero", func(t *testing.T) {
		t.Parallel()

		_, err := stdlib.Call("std.hashMod", []any{int64(1), int64(0)})
		require.ErrorIs(t, err, stdlib.ErrBadArg)
	})

	t.Run("k_negative", func(t *testing.T) {
		t.Parallel()

		_, err := stdlib.Call("std.hashMod", []any{int64(1), int64(-5)})
		require.ErrorIs(t, err, stdlib.ErrBadArg)
	})
}
