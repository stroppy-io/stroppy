package bench

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stroppy-io/stroppy/v6/pkg/driver"
)

func TestRetrySuccessDoesNotAllocatePolicy(t *testing.T) {
	worker := &Bench{}
	ctx := context.Background()
	allocations := testing.AllocsPerRun(100, func() {
		require.NoError(t, worker.Retry(ctx, TxRetryPolicyOptions{MaxAttempts: 3}, func() error { return nil }))
	})
	require.Zero(t, allocations)
}

func TestRetrySnapshotsOverridesBeforeFirstAttempt(t *testing.T) {
	t.Parallel()

	worker := &Bench{}
	failure := errors.New("unknown")
	actions := make(ErrorActionMap)
	actions[driver.ErrorKindUnknown] = ErrorActionIgnore
	err := worker.Retry(t.Context(), TxRetryPolicyOptions{Actions: actions}, func() error {
		actions[driver.ErrorKindUnknown] = ErrorActionFatal

		return failure
	})
	require.NoError(t, err)
}

func TestRetryPreservesCallbacksAndCancellation(t *testing.T) {
	t.Parallel()

	worker := &Bench{}
	failure := errors.New("retry")
	actions := make(ErrorActionMap)
	actions[driver.ErrorKindUnknown] = ErrorActionRetry
	attempts, callbacks := 0, 0
	err := worker.Retry(t.Context(), TxRetryPolicyOptions{
		MaxAttempts: 3, Actions: actions,
		OnRetry: func(attempt int, cause error, _ RetryDecision) {
			callbacks++

			require.Equal(t, 2, attempt)
			require.ErrorIs(t, cause, failure)
		},
	}, func() error {
		attempts++
		if attempts == 1 {
			return failure
		}

		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 2, attempts)
	require.Equal(t, 1, callbacks)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, worker.Retry(ctx, TxRetryPolicyOptions{}, func() error {
		t.Fatal("canceled operation ran")

		return nil
	}), context.Canceled)
}
