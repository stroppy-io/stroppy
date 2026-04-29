package stdlib_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/stroppy-io/stroppy/pkg/datagen/stdlib"
)

func TestUUIDSeeded(t *testing.T) {
	t.Parallel()

	t.Run("valid_v4_and_deterministic", func(t *testing.T) {
		t.Parallel()

		const key int64 = 42

		first, err := stdlib.Call("std.uuidSeeded", []any{key})
		require.NoError(t, err)
		second, err := stdlib.Call("std.uuidSeeded", []any{key})
		require.NoError(t, err)
		require.Equal(t, first, second, "same seed must produce same UUID")

		parsed, err := uuid.Parse(first.(string))
		require.NoError(t, err)
		require.Equal(t, uuid.Version(4), parsed.Version())
		require.Equal(t, uuid.RFC4122, parsed.Variant())
	})

	t.Run("distinct_seeds_diverge", func(t *testing.T) {
		t.Parallel()

		first, err := stdlib.Call("std.uuidSeeded", []any{int64(1)})
		require.NoError(t, err)
		second, err := stdlib.Call("std.uuidSeeded", []any{int64(2)})
		require.NoError(t, err)
		require.NotEqual(t, first, second)
	})

	t.Run("arity", func(t *testing.T) {
		t.Parallel()

		_, err := stdlib.Call("std.uuidSeeded", nil)
		require.ErrorIs(t, err, stdlib.ErrArity)
	})

	t.Run("type_error", func(t *testing.T) {
		t.Parallel()

		_, err := stdlib.Call("std.uuidSeeded", []any{"not-int"})
		require.ErrorIs(t, err, stdlib.ErrArgType)
	})
}
