package sqldriver

import (
	"errors"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReusableBindingPreservesValidationAndConversion(t *testing.T) {
	t.Parallel()

	buffer := make([]any, 0, 5)

	for _, test := range []struct {
		sql  string
		args map[string]any
	}{
		{"SELECT :a, :b", map[string]any{"a": 1, "b": 2}},
		{"SELECT :a, :a", map[string]any{"a": 1}},
		{"SELECT :missing", map[string]any{"a": 1}},
		{"SELECT :a", map[string]any{"a": 1, "unused": 3}},
	} {
		wantSQL, wantArgs, wantErr := ProcessArgs(convertingDialect{}, test.sql, test.args)
		gotSQL, gotArgs, gotErr := ProcessArgsInto(convertingDialect{}, test.sql, test.args, buffer)
		require.Equal(t, wantSQL, gotSQL)
		require.True(t, reflect.DeepEqual(wantArgs, gotArgs))

		for _, kind := range []error{ErrMissedArgument, ErrExtraArgument} {
			require.Equal(t, errors.Is(wantErr, kind), errors.Is(gotErr, kind))
		}
	}
}

func TestReusableBindingOwnsItsDestination(t *testing.T) {
	t.Parallel()

	first := make([]any, 0, 1)
	second := make([]any, 0, 1)
	_, a, err := ProcessArgsInto(testDialect{}, "SELECT :a", map[string]any{"a": 1}, first)
	require.NoError(t, err)
	_, b, err := ProcessArgsInto(testDialect{}, "SELECT :a", map[string]any{"a": 2}, second)
	require.NoError(t, err)
	require.Equal(t, []any{1}, a)
	require.Equal(t, []any{2}, b)
}
