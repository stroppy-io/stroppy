package stdlib_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stroppy-io/stroppy/pkg/datagen/stdlib"
)

func TestRegistryPopulated(t *testing.T) {
	t.Parallel()

	names := stdlib.Names()
	require.NotEmpty(t, names, "stdlib registry must be non-empty at package init")

	// Spec catalog (plan §5.6): 12 entries. Deviation is a source-level
	// review event, so this test breaks loudly when the set changes.
	want := []string{
		"std.format",
		"std.hashMod",
		"std.uuidSeeded",
		"std.daysToDate",
		"std.dateToDays",
		"std.lower",
		"std.upper",
		"std.substr",
		"std.len",
		"std.toString",
		"std.parseInt",
		"std.parseFloat",
	}
	require.ElementsMatch(t, want, names)
}

func TestCallUnknownFunction(t *testing.T) {
	t.Parallel()

	_, err := stdlib.Call("std.missing", nil)
	require.ErrorIs(t, err, stdlib.ErrUnknownFunction)
}

func TestCallDispatch(t *testing.T) {
	t.Parallel()

	// Round-trip through Call to make sure the dispatcher finds a known
	// function and returns its output verbatim.
	got, err := stdlib.Call("std.len", []any{"abc"})
	require.NoError(t, err)
	require.Equal(t, int64(3), got)
}

func TestCallErrorsPropagate(t *testing.T) {
	t.Parallel()

	_, err := stdlib.Call("std.hashMod", []any{int64(5)})
	require.ErrorIs(t, err, stdlib.ErrArity)
}
