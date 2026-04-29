package stdlib_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/stroppy-io/stroppy/pkg/datagen/stdlib"
)

func TestDaysToDate(t *testing.T) {
	t.Parallel()

	t.Run("epoch", func(t *testing.T) {
		t.Parallel()

		got, err := stdlib.Call("std.daysToDate", []any{int64(0)})
		require.NoError(t, err)
		require.Equal(t, time.Date(1970, time.January, 1, 0, 0, 0, 0, time.UTC), got)
	})

	t.Run("positive", func(t *testing.T) {
		t.Parallel()

		// 2020-01-01 = 18262 days after epoch.
		got, err := stdlib.Call("std.daysToDate", []any{int64(18_262)})
		require.NoError(t, err)
		require.Equal(t, time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC), got)
	})

	t.Run("negative", func(t *testing.T) {
		t.Parallel()

		got, err := stdlib.Call("std.daysToDate", []any{int64(-1)})
		require.NoError(t, err)
		require.Equal(t, time.Date(1969, time.December, 31, 0, 0, 0, 0, time.UTC), got)
	})

	t.Run("arity", func(t *testing.T) {
		t.Parallel()

		_, err := stdlib.Call("std.daysToDate", []any{int64(1), int64(2)})
		require.ErrorIs(t, err, stdlib.ErrArity)
	})

	t.Run("type_error", func(t *testing.T) {
		t.Parallel()

		_, err := stdlib.Call("std.daysToDate", []any{1.5})
		require.ErrorIs(t, err, stdlib.ErrArgType)
	})
}

func TestDateToDays(t *testing.T) {
	t.Parallel()

	t.Run("epoch", func(t *testing.T) {
		t.Parallel()

		when := time.Date(1970, time.January, 1, 0, 0, 0, 0, time.UTC)
		got, err := stdlib.Call("std.dateToDays", []any{when})
		require.NoError(t, err)
		require.Equal(t, int64(0), got)
	})

	t.Run("truncates_intra_day", func(t *testing.T) {
		t.Parallel()

		// 2020-01-01 23:59:59 UTC should round down to 2020-01-01 = 18262.
		when := time.Date(2020, time.January, 1, 23, 59, 59, 0, time.UTC)
		got, err := stdlib.Call("std.dateToDays", []any{when})
		require.NoError(t, err)
		require.Equal(t, int64(18_262), got)
	})

	t.Run("negative_round_trip", func(t *testing.T) {
		t.Parallel()

		// Pre-epoch date truncates correctly: 1969-12-31 00:30:00 UTC -> -1.
		when := time.Date(1969, time.December, 31, 0, 30, 0, 0, time.UTC)
		got, err := stdlib.Call("std.dateToDays", []any{when})
		require.NoError(t, err)
		require.Equal(t, int64(-1), got)
	})

	t.Run("round_trip", func(t *testing.T) {
		t.Parallel()

		for _, days := range []int64{-730, -1, 0, 1, 10_000} {
			mid, err := stdlib.Call("std.daysToDate", []any{days})
			require.NoError(t, err)
			back, err := stdlib.Call("std.dateToDays", []any{mid})
			require.NoError(t, err)
			require.Equal(t, days, back)
		}
	})

	t.Run("arity", func(t *testing.T) {
		t.Parallel()

		_, err := stdlib.Call("std.dateToDays", nil)
		require.ErrorIs(t, err, stdlib.ErrArity)
	})

	t.Run("type_error", func(t *testing.T) {
		t.Parallel()

		_, err := stdlib.Call("std.dateToDays", []any{"2020-01-01"})
		require.ErrorIs(t, err, stdlib.ErrArgType)
	})
}
