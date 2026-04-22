package stdlib_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stroppy-io/stroppy/pkg/datagen/stdlib"
)

func TestParseInt(t *testing.T) {
	t.Parallel()

	t.Run("happy_path", func(t *testing.T) {
		t.Parallel()

		got, err := stdlib.Call("std.parseInt", []any{"42"})
		require.NoError(t, err)
		require.Equal(t, int64(42), got)
	})

	t.Run("negative", func(t *testing.T) {
		t.Parallel()

		got, err := stdlib.Call("std.parseInt", []any{"-7"})
		require.NoError(t, err)
		require.Equal(t, int64(-7), got)
	})

	t.Run("arity", func(t *testing.T) {
		t.Parallel()

		_, err := stdlib.Call("std.parseInt", nil)
		require.ErrorIs(t, err, stdlib.ErrArity)

		_, err = stdlib.Call("std.parseInt", []any{"1", "2"})
		require.ErrorIs(t, err, stdlib.ErrArity)
	})

	t.Run("wrong_type", func(t *testing.T) {
		t.Parallel()

		_, err := stdlib.Call("std.parseInt", []any{int64(5)})
		require.ErrorIs(t, err, stdlib.ErrArgType)
	})

	t.Run("empty_input", func(t *testing.T) {
		t.Parallel()

		_, err := stdlib.Call("std.parseInt", []any{""})
		require.ErrorIs(t, err, stdlib.ErrBadArg)
	})

	t.Run("unparseable", func(t *testing.T) {
		t.Parallel()

		_, err := stdlib.Call("std.parseInt", []any{"12.5"})
		require.ErrorIs(t, err, stdlib.ErrBadArg)

		_, err = stdlib.Call("std.parseInt", []any{"abc"})
		require.ErrorIs(t, err, stdlib.ErrBadArg)
	})
}

func TestParseFloat(t *testing.T) {
	t.Parallel()

	t.Run("happy_path", func(t *testing.T) {
		t.Parallel()

		got, err := stdlib.Call("std.parseFloat", []any{"3.14"})
		require.NoError(t, err)

		asFloat, ok := got.(float64)
		require.True(t, ok, "expected float64, got %T", got)
		require.InDelta(t, 3.14, asFloat, 1e-12)
	})

	t.Run("integer_string", func(t *testing.T) {
		t.Parallel()

		got, err := stdlib.Call("std.parseFloat", []any{"100"})
		require.NoError(t, err)

		asFloat, ok := got.(float64)
		require.True(t, ok, "expected float64, got %T", got)
		require.InDelta(t, 100.0, asFloat, 1e-12)
	})

	t.Run("arity", func(t *testing.T) {
		t.Parallel()

		_, err := stdlib.Call("std.parseFloat", nil)
		require.ErrorIs(t, err, stdlib.ErrArity)

		_, err = stdlib.Call("std.parseFloat", []any{"1", "2"})
		require.ErrorIs(t, err, stdlib.ErrArity)
	})

	t.Run("wrong_type", func(t *testing.T) {
		t.Parallel()

		_, err := stdlib.Call("std.parseFloat", []any{3.14})
		require.ErrorIs(t, err, stdlib.ErrArgType)
	})

	t.Run("empty_input", func(t *testing.T) {
		t.Parallel()

		_, err := stdlib.Call("std.parseFloat", []any{""})
		require.ErrorIs(t, err, stdlib.ErrBadArg)
	})

	t.Run("unparseable", func(t *testing.T) {
		t.Parallel()

		_, err := stdlib.Call("std.parseFloat", []any{"not-a-number"})
		require.ErrorIs(t, err, stdlib.ErrBadArg)
	})
}
