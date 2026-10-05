package bench

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	_ "github.com/stroppy-io/stroppy/v6/pkg/driver/noop"
	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

func noopRunOptions() RunOptions {
	return RunOptions{
		Drivers: map[string]DriverConfig{"": {Kind: DriverNoop}},
		Metrics: &MetricsConfig{Quiet: true},
		Report:  &ReportOptions{},
	}
}

func newRuntimeTestRoot(t *testing.T) *rootState {
	t.Helper()

	root, err := newrootState(
		zap.NewNop(),
		context.Background(),
		nil,
		nil,
		&MetricsConfig{Quiet: true},
	)
	require.NoError(t, err)
	t.Cleanup(func() { root.errorReporter.stopAndWait(); root.shutdownMetrics() })

	return root
}

func TestRunContinuesAfterOrdinaryErrors(t *testing.T) {
	var calls atomic.Int64

	test := Test{Name: "errors", Define: func(d *Def) error {
		d.Execution.Step("work", func(context.Context, *Bench) error {
			calls.Add(1)

			return errors.New("ordinary")
		}, SharedIterations(3, 9))

		return d.Execution.Err()
	}}
	result, err := RunTest(t.Context(), test, noopRunOptions())
	require.NoError(t, err)
	require.Equal(t, int64(9), calls.Load())
	require.Equal(t, report.StatusCompletedWithErrors, result.Status)
	require.Equal(t, uint64(9), result.Errors.FailedIterations)
}

func TestRunFatalCancelsWorkersAndBlocksLaterWork(t *testing.T) {
	var (
		calls atomic.Int64
		later atomic.Bool
	)

	sentinel := errors.New("fatal")
	test := Test{Name: "fatal", Define: func(d *Def) error {
		d.Execution.Step("work", func(ctx context.Context, b *Bench) error {
			if calls.Add(1) == 1 {
				return Fatal(sentinel)
			}

			<-ctx.Done()

			return ctx.Err()
		}, SharedIterations(4, 100))
		d.Execution.Step("later", func(context.Context, *Bench) error {
			later.Store(true)

			return nil
		})

		return d.Execution.Err()
	}}
	result, err := RunTest(t.Context(), test, noopRunOptions())
	require.ErrorIs(t, err, sentinel)
	require.False(t, later.Load())
	require.Equal(t, "blocked", result.Steps[1].Status)
}

func TestDurationDrainAndCleanup(t *testing.T) {
	var (
		interrupted atomic.Bool
		cleaned     atomic.Bool
	)

	test := Test{Name: "drain", Define: func(d *Def) error {
		d.Execution.Step("work", func(ctx context.Context, b *Bench) error {
			<-ctx.Done()
			interrupted.Store(true)

			return ctx.Err()
		}, ConstantWorkers(1, 10*time.Millisecond, DrainTimeout(10*time.Millisecond)))
		d.Execution.Step("cleanup", func(ctx context.Context, b *Bench) error {
			cleaned.Store(ctx.Err() == nil)

			return nil
		}, Always(time.Second))

		return d.Execution.Err()
	}}
	result, err := RunTest(t.Context(), test, noopRunOptions())
	require.NoError(t, err)
	require.True(t, interrupted.Load())
	require.True(t, cleaned.Load())
	require.Equal(t, report.StatusCompletedWithErrors, result.Status)
}

func TestDrainExpiryCannotBecomeLogicalSuccess(t *testing.T) {
	test := Test{Name: "drain-success", Define: func(d *Def) error {
		d.Execution.Step("work", func(ctx context.Context, b *Bench) error {
			return b.LogicalOperation(func() error {
				<-ctx.Done()

				return nil
			})
		}, ConstantWorkers(1, 10*time.Millisecond, DrainTimeout(0)))

		return d.Execution.Err()
	}}
	result, err := RunTest(t.Context(), test, noopRunOptions())
	require.NoError(t, err)
	require.Equal(t, report.StatusCompletedWithErrors, result.Status)
	require.Equal(t, uint64(1), result.Errors.FailedIterations)
	require.InDelta(t, 0, *result.Metrics["successful_transactions_total"].Total, 0)
}

func TestParentCancellationAndDetachedCleanup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.WithValue(t.Context(), testContextKey{}, "retained"))

	var cleaned bool

	test := Test{Name: "cancel", Define: func(d *Def) error {
		d.Execution.Step("work", func(ctx context.Context, b *Bench) error {
			cancel()
			<-ctx.Done()

			return ctx.Err()
		}, SharedIterations(1, 1))
		d.Execution.Step("cleanup", func(ctx context.Context, b *Bench) error {
			_, deadline := ctx.Deadline()
			cleaned = ctx.Err() == nil && deadline && ctx.Value(testContextKey{}) == "retained"

			return nil
		}, Always(time.Second))

		return d.Execution.Err()
	}}
	result, err := RunTest(ctx, test, noopRunOptions())
	require.ErrorIs(t, err, context.Canceled)
	require.True(t, cleaned)
	require.Equal(t, report.StatusCanceled, result.Status)
}

func TestReportsBuilderAndDisabledMode(t *testing.T) {
	var calls atomic.Int64

	test := Test{Name: "report", Define: func(d *Def) error {
		d.Report.Contribute("example", 1, func(final ReportContext) (ReportContribution, error) {
			calls.Add(1)

			return ReportContribution{Data: map[string]string{"status": string(final.Status)}}, nil
		})
		d.Execution.Step("work", func(context.Context, *Bench) error { return nil })

		return d.Execution.Err()
	}}
	options := noopRunOptions()
	result, err := RunTest(t.Context(), test, options)
	require.NoError(t, err)
	require.Len(t, result.WorkloadReports, 1)
	require.Equal(t, int64(1), calls.Load())

	options.Report = nil
	result, err = RunTest(t.Context(), test, options)
	require.NoError(t, err)
	require.Nil(t, result)
	require.Equal(t, int64(1), calls.Load())
}

func TestRecognizedPanicsReturnAndUserPanicsPropagate(t *testing.T) {
	test := Test{Name: "invalid", Define: func(d *Def) error {
		d.Param.Int("bad_name", 1, "")

		return nil
	}}
	_, err := RunTest(t.Context(), test, noopRunOptions())
	require.Error(t, err)

	test = Test{Name: "panic", Define: func(d *Def) error {
		d.Execution.Step("work", func(context.Context, *Bench) error { panic("user panic") })

		return nil
	}}

	require.PanicsWithValue(
		t,
		"user panic",
		func() { _, _ = RunTest(t.Context(), test, noopRunOptions()) },
	)
}

func TestConcurrentRunsAreIsolated(t *testing.T) {
	test := Test{Name: "concurrent", Define: func(d *Def) error {
		count, _ := d.Param.Int64("count", 1, "")
		d.Execution.Step(
			"work",
			func(context.Context, *Bench) error { return nil },
			SharedIterations(2, count),
		)

		return d.Execution.Err()
	}}

	var wg sync.WaitGroup
	for _, count := range []int64{17, 29} {
		wg.Go(func() {
			options := noopRunOptions()
			options.Params.CLI = map[string]string{"count": fmtInt(count)}
			result, err := RunTest(t.Context(), test, options)
			require.NoError(t, err)
			require.InDelta(
				t,
				float64(count),
				*result.Metrics["iterations_total"].Total,
				0,
			)
		})
	}

	wg.Wait()
}
func fmtInt(value int64) string { return strconv.FormatInt(value, 10) }

type testContextKey struct{}
