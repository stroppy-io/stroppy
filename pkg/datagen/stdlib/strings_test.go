package stdlib_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/stroppy-io/stroppy/pkg/datagen/stdlib"
)

func TestLower(t *testing.T) {
	t.Parallel()

	t.Run("ascii", func(t *testing.T) {
		t.Parallel()

		got, err := stdlib.Call("std.lower", []any{"Hello WORLD"})
		require.NoError(t, err)
		require.Equal(t, "hello world", got)
	})

	t.Run("preserves_nonletters", func(t *testing.T) {
		t.Parallel()

		got, err := stdlib.Call("std.lower", []any{"A1_B2!"})
		require.NoError(t, err)
		require.Equal(t, "a1_b2!", got)
	})

	t.Run("arity", func(t *testing.T) {
		t.Parallel()

		_, err := stdlib.Call("std.lower", []any{"a", "b"})
		require.ErrorIs(t, err, stdlib.ErrArity)
	})

	t.Run("type_error", func(t *testing.T) {
		t.Parallel()

		_, err := stdlib.Call("std.lower", []any{int64(1)})
		require.ErrorIs(t, err, stdlib.ErrArgType)
	})
}

func TestUpper(t *testing.T) {
	t.Parallel()

	t.Run("ascii", func(t *testing.T) {
		t.Parallel()

		got, err := stdlib.Call("std.upper", []any{"Hello world"})
		require.NoError(t, err)
		require.Equal(t, "HELLO WORLD", got)
	})

	t.Run("arity", func(t *testing.T) {
		t.Parallel()

		_, err := stdlib.Call("std.upper", nil)
		require.ErrorIs(t, err, stdlib.ErrArity)
	})

	t.Run("type_error", func(t *testing.T) {
		t.Parallel()

		_, err := stdlib.Call("std.upper", []any{true})
		require.ErrorIs(t, err, stdlib.ErrArgType)
	})
}

func TestSubstr(t *testing.T) {
	t.Parallel()

	t.Run("happy_path", func(t *testing.T) {
		t.Parallel()

		got, err := stdlib.Call("std.substr", []any{"abcdef", int64(1), int64(3)})
		require.NoError(t, err)
		require.Equal(t, "bcd", got)
	})

	t.Run("utf8_runes", func(t *testing.T) {
		t.Parallel()

		// "héllo" has 5 runes; rune-indexed substring from 1 of length 3
		// yields "éll", not a byte-sliced garble.
		got, err := stdlib.Call("std.substr", []any{"héllo", int64(1), int64(3)})
		require.NoError(t, err)
		require.Equal(t, "éll", got)
	})

	t.Run("negative_start_clamps", func(t *testing.T) {
		t.Parallel()

		got, err := stdlib.Call("std.substr", []any{"abc", int64(-2), int64(2)})
		require.NoError(t, err)
		require.Equal(t, "ab", got)
	})

	t.Run("start_past_end", func(t *testing.T) {
		t.Parallel()

		got, err := stdlib.Call("std.substr", []any{"abc", int64(5), int64(2)})
		require.NoError(t, err)
		require.Empty(t, got)
	})

	t.Run("length_overshoot", func(t *testing.T) {
		t.Parallel()

		got, err := stdlib.Call("std.substr", []any{"abc", int64(1), int64(99)})
		require.NoError(t, err)
		require.Equal(t, "bc", got)
	})

	t.Run("negative_length_empty", func(t *testing.T) {
		t.Parallel()

		got, err := stdlib.Call("std.substr", []any{"abc", int64(0), int64(-1)})
		require.NoError(t, err)
		require.Empty(t, got)
	})

	t.Run("arity", func(t *testing.T) {
		t.Parallel()

		_, err := stdlib.Call("std.substr", []any{"abc", int64(0)})
		require.ErrorIs(t, err, stdlib.ErrArity)
	})

	t.Run("type_on_source", func(t *testing.T) {
		t.Parallel()

		_, err := stdlib.Call("std.substr", []any{int64(1), int64(0), int64(1)})
		require.ErrorIs(t, err, stdlib.ErrArgType)
	})

	t.Run("type_on_start", func(t *testing.T) {
		t.Parallel()

		_, err := stdlib.Call("std.substr", []any{"abc", 1.0, int64(1)})
		require.ErrorIs(t, err, stdlib.ErrArgType)
	})

	t.Run("type_on_length", func(t *testing.T) {
		t.Parallel()

		_, err := stdlib.Call("std.substr", []any{"abc", int64(0), "1"})
		require.ErrorIs(t, err, stdlib.ErrArgType)
	})
}

func TestLen(t *testing.T) {
	t.Parallel()

	t.Run("ascii", func(t *testing.T) {
		t.Parallel()

		got, err := stdlib.Call("std.len", []any{"abc"})
		require.NoError(t, err)
		require.Equal(t, int64(3), got)
	})

	t.Run("utf8_runes", func(t *testing.T) {
		t.Parallel()

		got, err := stdlib.Call("std.len", []any{"héllo"})
		require.NoError(t, err)
		require.Equal(t, int64(5), got)
	})

	t.Run("empty", func(t *testing.T) {
		t.Parallel()

		got, err := stdlib.Call("std.len", []any{""})
		require.NoError(t, err)
		require.Equal(t, int64(0), got)
	})

	t.Run("arity", func(t *testing.T) {
		t.Parallel()

		_, err := stdlib.Call("std.len", nil)
		require.ErrorIs(t, err, stdlib.ErrArity)
	})

	t.Run("type_error", func(t *testing.T) {
		t.Parallel()

		_, err := stdlib.Call("std.len", []any{int64(1)})
		require.ErrorIs(t, err, stdlib.ErrArgType)
	})
}

func TestToString(t *testing.T) {
	t.Parallel()

	t.Run("int", func(t *testing.T) {
		t.Parallel()

		got, err := stdlib.Call("std.toString", []any{int64(42)})
		require.NoError(t, err)
		require.Equal(t, "42", got)
	})

	t.Run("float", func(t *testing.T) {
		t.Parallel()

		got, err := stdlib.Call("std.toString", []any{1.5})
		require.NoError(t, err)
		require.Equal(t, "1.5", got)
	})

	t.Run("bool", func(t *testing.T) {
		t.Parallel()

		got, err := stdlib.Call("std.toString", []any{true})
		require.NoError(t, err)
		require.Equal(t, "true", got)
	})

	t.Run("string_passthrough", func(t *testing.T) {
		t.Parallel()

		got, err := stdlib.Call("std.toString", []any{"abc"})
		require.NoError(t, err)
		require.Equal(t, "abc", got)
	})

	t.Run("time", func(t *testing.T) {
		t.Parallel()

		when := time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC)
		got, err := stdlib.Call("std.toString", []any{when})
		require.NoError(t, err)
		require.Contains(t, got, "2020-01-01")
	})

	t.Run("arity", func(t *testing.T) {
		t.Parallel()

		_, err := stdlib.Call("std.toString", nil)
		require.ErrorIs(t, err, stdlib.ErrArity)
	})
}
