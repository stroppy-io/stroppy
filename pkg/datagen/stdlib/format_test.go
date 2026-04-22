package stdlib_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stroppy-io/stroppy/pkg/datagen/stdlib"
)

func TestFormat(t *testing.T) {
	t.Parallel()

	t.Run("happy_path", func(t *testing.T) {
		t.Parallel()

		got, err := stdlib.Call("std.format", []any{"%s=%d", "x", int64(7)})
		require.NoError(t, err)
		require.Equal(t, "x=7", got)
	})

	t.Run("no_verbs", func(t *testing.T) {
		t.Parallel()

		got, err := stdlib.Call("std.format", []any{"literal"})
		require.NoError(t, err)
		require.Equal(t, "literal", got)
	})

	t.Run("arity_zero", func(t *testing.T) {
		t.Parallel()

		_, err := stdlib.Call("std.format", nil)
		require.ErrorIs(t, err, stdlib.ErrArity)
	})

	t.Run("type_on_fmt", func(t *testing.T) {
		t.Parallel()

		_, err := stdlib.Call("std.format", []any{int64(1)})
		require.ErrorIs(t, err, stdlib.ErrArgType)
	})

	t.Run("missing_arg", func(t *testing.T) {
		t.Parallel()

		_, err := stdlib.Call("std.format", []any{"%s=%d", "x"})
		require.ErrorIs(t, err, stdlib.ErrBadArg)
	})

	t.Run("bad_verb", func(t *testing.T) {
		t.Parallel()

		_, err := stdlib.Call("std.format", []any{"%d", "notnum"})
		require.ErrorIs(t, err, stdlib.ErrBadArg)
	})
}
