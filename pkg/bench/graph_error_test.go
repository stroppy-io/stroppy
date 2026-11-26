package bench

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGraphCancellationKeepsJoinedFailures(t *testing.T) {
	failure := errors.New("rollback failed")
	for _, err := range []error{
		errors.Join(context.Canceled, failure),
		fmt.Errorf("cleanup: %w", errors.Join(context.Canceled, failure)),
	} {
		remaining := withoutGraphCancellation(err)
		require.ErrorIs(t, remaining, failure)
		require.NotErrorIs(t, remaining, context.Canceled)
	}

	require.NoError(t, withoutGraphCancellation(fmt.Errorf("stopped: %w", context.Canceled)))
}
