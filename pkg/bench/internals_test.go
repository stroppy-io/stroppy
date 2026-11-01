package bench

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/v6/pkg/config"
	"github.com/stroppy-io/stroppy/v6/pkg/driver"
)

func TestTransactionAttributesWarmAllocations(t *testing.T) {
	var metrics txMetrics

	want := metrics.txAttributes("workload", "commit", "transfer", "read_committed")

	allocations := testing.AllocsPerRun(1000, func() {
		got := metrics.txAttributes("workload", "commit", "transfer", "read_committed")
		if got.set != want.set {
			t.Fatal("warm lookup did not reuse transaction attributes")
		}
	})
	require.Zero(t, allocations)
}

func TestTransactionLazyPolicy(t *testing.T) {
	for _, test := range []struct {
		name        string
		maxAttempts int
		failures    int
		wantBodies  int
		wantRetries []int
	}{
		{name: "success", maxAttempts: 4, wantBodies: 1},
		{name: "default budget", failures: 1, wantBodies: 1},
		{name: "one retry", maxAttempts: 4, failures: 1, wantBodies: 2, wantRetries: []int{2}},
		{name: "several retries", maxAttempts: 4, failures: 3, wantBodies: 4, wantRetries: []int{2, 3, 4}},
		{name: "exhausted", maxAttempts: 3, failures: 5, wantBodies: 3, wantRetries: []int{2, 3}},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := newRuntimeTestRoot(t)
			tx := &commitFailTx{}
			work := &Bench{
				root: root, vu: &VU{root: root, ctx: t.Context()}, lg: zap.NewNop(),
				drv: &commitFailDriver{tx: tx, facts: driver.ErrorFacts{Kind: driver.ErrorKindSerialization}},
				cfg: &config.DriverConfig{DriverType: config.DriverTypePostgres},
			}
			conflict := errors.New("conflict")
			bodies := 0

			var retries []int

			err := work.Transaction(t.Context(), TransactionOptions{
				Isolation: IsoReadCommitted,
				Retry: TxRetryPolicyOptions{
					MaxAttempts: test.maxAttempts,
					OnRetry: func(attempt int, err error, decision RetryDecision) {
						require.ErrorIs(t, err, conflict)
						require.Equal(t, driver.ErrorKindSerialization, decision.Facts.Kind)

						retries = append(retries, attempt)
					},
				},
			}, func(context.Context, *Tx) error {
				bodies++
				if bodies <= test.failures {
					return conflict
				}

				return nil
			})
			if test.failures >= test.wantBodies {
				require.ErrorIs(t, err, conflict)
			} else {
				require.NoError(t, err)
			}

			require.Equal(t, test.wantBodies, bodies)
			require.Equal(t, test.wantRetries, retries)
			require.Equal(t, uint64(len(retries)), root.errorReporter.snapshot().retryAttempts)
		})
	}
}

func TestTransactionLazyPolicyBackoff(t *testing.T) {
	root := newRuntimeTestRoot(t)
	work := &Bench{
		root: root, vu: &VU{root: root, ctx: t.Context()}, lg: zap.NewNop(),
		drv: &commitFailDriver{tx: &commitFailTx{}, facts: driver.ErrorFacts{
			Kind: driver.ErrorKindSerialization, Backoff: true,
		}},
		cfg: &config.DriverConfig{DriverType: config.DriverTypePostgres},
	}

	var delays []float64

	attempts := 0
	err := work.Transaction(t.Context(), TransactionOptions{
		Retry: TxRetryPolicyOptions{
			MaxAttempts: 3, BaseDelaySeconds: 0.0001,
			OnRetry: func(_ int, _ error, decision RetryDecision) {
				delays = append(delays, decision.DelaySeconds)
			},
		},
	}, func(context.Context, *Tx) error {
		attempts++
		if attempts < 3 {
			return errors.New("conflict")
		}

		return nil
	})
	require.NoError(t, err)
	require.Len(t, delays, 2)
	require.GreaterOrEqual(t, delays[0], 0.0001)
	require.LessOrEqual(t, delays[0], 0.00012)
	require.GreaterOrEqual(t, delays[1], 0.0002)
	require.LessOrEqual(t, delays[1], 0.00024)
}

func TestWorkerContextsKeepCancellationCause(t *testing.T) {
	ctx, cancel := context.WithCancelCause(t.Context())
	defer cancel(nil)

	started := make(chan context.Context, 4)
	cause := errors.New("stop workers")
	test := Test{Name: "worker-cancellation", Define: func(d *Def) error {
		d.Execution.Step("workload", func(ctx context.Context, _ *Bench) error {
			started <- ctx

			<-ctx.Done()
			require.ErrorIs(t, context.Cause(ctx), cause)

			return ctx.Err()
		}, SharedIterations(4, 4))

		return d.Execution.Err()
	}}

	go func() {
		for range 4 {
			<-started
		}

		cancel(cause)
	}()

	_, err := RunTest(ctx, test, noopRunOptions())
	require.ErrorIs(t, err, context.Canceled)
}

func TestTransactionPolicySnapshotsOverrides(t *testing.T) {
	root := newRuntimeTestRoot(t)
	work := &Bench{
		root: root, vu: &VU{root: root, ctx: t.Context()}, lg: zap.NewNop(),
		drv: &commitFailDriver{tx: &commitFailTx{}, facts: driver.ErrorFacts{Kind: driver.ErrorKindUnknown}},
		cfg: &config.DriverConfig{DriverType: config.DriverTypePostgres},
	}
	overrides := DefaultErrorActions()
	overrides[driver.ErrorKindUnknown] = ErrorActionIgnore
	err := work.Transaction(t.Context(), TransactionOptions{
		Isolation: IsoReadCommitted, Retry: TxRetryPolicyOptions{Actions: overrides},
	}, func(context.Context, *Tx) error {
		overrides[driver.ErrorKindUnknown] = ErrorActionError

		return errors.New("ignored")
	})
	require.NoError(t, err)
}
