package tpcc

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/pkg/bench/testkit"
)

func TestPacingKeyingCancellation(t *testing.T) {
	w := &workload{pacing: true, vuStates: make([]*vuState, 1)}
	called := false
	test := bench.Test{Name: "keying-cancel", Define: func(d *bench.Def) error {
		d.Execution.Step("workload", func(ctx context.Context, b *bench.Bench) error {
			return w.runPaced(ctx, b, w.vuState(1, 1, 1), "new_order", func() error {
				called = true

				return nil
			})
		}, bench.ConstantWorkers(1, 20*time.Millisecond, bench.DrainTimeout(0)))

		return d.Execution.Err()
	}}
	start := time.Now()
	result, err := testkit.Run(t.Context(), test, bench.RunOptions{})
	require.NoError(t, err)
	require.Less(t, time.Since(start), time.Second)
	require.False(t, called)
	require.Equal(t, uint64(1), result.Errors.FailedIterations)
	require.Nil(t, result.Metrics["successful_transactions_total"].Total)
}

func TestPacingThinksAfterTerminalOutcome(t *testing.T) {
	const name = "test_terminal"

	thinkTimeMean[name] = 12

	t.Cleanup(func() { delete(thinkTimeMean, name) })

	sentinel := errors.New("terminal failure")
	for _, tc := range []struct {
		name   string
		err    error
		thinks bool
	}{
		{"success", nil, true},
		{"nonfatal", sentinel, true},
		{"fatal", bench.Fatal(sentinel), false},
		{"canceled", context.Canceled, false},
		{"deadline", context.DeadlineExceeded, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := &workload{pacing: true, vuStates: make([]*vuState, 1)}
			vs := w.vuState(1, 1, 1)

			control := (&workload{vuStates: make([]*vuState, 1)}).vuState(1, 1, 1)
			if tc.thinks {
				_ = thinkTime(control.picker, 12)
			}

			var outcome error

			test := bench.Test{Name: "thinking", Define: func(d *bench.Def) error {
				d.Execution.Step("workload", func(ctx context.Context, b *bench.Bench) error {
					idle, cancel := context.WithCancel(ctx)
					defer cancel()

					outcome = w.runPaced(idle, b, vs, name, func() error {
						time.AfterFunc(20*time.Millisecond, cancel)

						return tc.err
					})

					return outcome
				}, bench.SharedIterations(1, 1))

				return d.Execution.Err()
			}}
			start := time.Now()

			result, err := testkit.Run(t.Context(), test, bench.RunOptions{})
			if bench.IsFatalError(tc.err) {
				require.ErrorIs(t, err, tc.err)
			} else {
				require.NoError(t, err)
			}

			if tc.err == nil {
				require.Zero(t, result.Errors.FailedIterations)
				require.InDelta(t, 1, *result.Metrics["successful_transactions_total"].Total, 0)
			} else if !bench.IsFatalError(tc.err) {
				require.Equal(t, uint64(1), result.Errors.FailedIterations)
				require.InDelta(t, 0, *result.Metrics["successful_transactions_total"].Total, 0)
			}

			require.ErrorIs(t, outcome, tc.err)
			require.Equal(t, control.picker.Uint64(), vs.picker.Uint64())

			if tc.thinks {
				require.GreaterOrEqual(t, time.Since(start), 20*time.Millisecond)
			}

			require.Less(t, time.Since(start), time.Second)
		})
	}
}

func TestPacingSuccessBeforeCanceledThinking(t *testing.T) {
	const name = "test_completed"

	thinkTimeMean[name] = 12

	t.Cleanup(func() { delete(thinkTimeMean, name) })

	w := &workload{pacing: true, vuStates: make([]*vuState, 1)}
	test := bench.Test{Name: "think-drain", Define: func(d *bench.Def) error {
		d.Execution.Step("workload", func(ctx context.Context, b *bench.Bench) error {
			return w.runPaced(ctx, b, w.vuState(1, 1, 1), name, func() error { return nil })
		}, bench.ConstantWorkers(1, 20*time.Millisecond, bench.DrainTimeout(0)))

		return d.Execution.Err()
	}}
	result, err := testkit.Run(t.Context(), test, bench.RunOptions{})
	require.NoError(t, err)
	require.Zero(t, result.Errors.FailedIterations)
	require.InDelta(t, 1, *result.Metrics["successful_transactions_total"].Total, 0)
}

func TestSleepSecondsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	for _, seconds := range []float64{0, 100} {
		require.ErrorIs(t, sleepSeconds(ctx, seconds), context.Canceled)
	}
}
