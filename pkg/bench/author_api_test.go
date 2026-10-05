package bench

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

func TestNamedDriversAndFiniteMetricSchema(t *testing.T) {
	test := Test{Name: "named", Define: func(d *Def) error {
		left := d.Drivers.Declare("left", DriverConfig{Kind: DriverNoop})
		right := d.Drivers.Declare("right", DriverConfig{Kind: DriverNoop})
		counter := d.Metrics.Counter("checked", LabelValues("target", "left", "right"))
		latency := d.Metrics.Histogram("latency", Unit("s"), Bounds(.1, 1, 10))
		d.Execution.Step("compare", func(ctx context.Context, b *Bench) error {
			require.NoError(t, b.Database(left).Exec(ctx, "SELECT 1", nil))
			require.NoError(t, b.Database(right).Exec(ctx, "SELECT 1", nil))
			counter.Add(ctx, 1, LabelValue("target", "left"))
			counter.Add(ctx, 1, LabelValue("target", "right"))
			latency.Record(ctx, .5)

			return nil
		}, Use(left))

		return d.Execution.Err()
	}}
	result, err := RunTest(t.Context(), test, noopRunOptions())
	require.NoError(t, err)
	require.Len(t, result.Drivers, 3)
	require.Equal(
		t,
		[]string{"default", "left", "right"},
		[]string{
			result.Drivers[0].Name,
			result.Drivers[1].Name,
			result.Drivers[2].Name,
		},
	)
	require.Equal(t, "s", result.Metrics["latency"].Unit)
	require.Equal(t, []float64{.1, 1, 10}, result.Metrics["latency"].Bounds)
	require.Len(t, result.Metrics["checked"].Series, 2)

	description, err := DescribeTest(test)
	require.NoError(t, err)
	require.Len(t, description.Metrics, 2)
}

func TestInvalidFiniteLabelsReturnRecognizedError(t *testing.T) {
	test := Test{Name: "labels", Define: func(d *Def) error {
		counter := d.Metrics.Counter("count", LabelValues("kind", "known"))
		d.Execution.Step("work", func(ctx context.Context, b *Bench) error {
			counter.Add(ctx, 1, LabelValue("kind", "unknown"))

			return nil
		})

		return d.Execution.Err()
	}}
	_, err := RunTest(t.Context(), test, noopRunOptions())

	var validation *ValidationError
	require.ErrorAs(t, err, &validation)
}

func TestMultipleMeasuredStepsKeepSeparateWindows(t *testing.T) {
	test := Test{Name: "windows", Define: func(d *Def) error {
		action := func(context.Context, *Bench) error { return nil }
		d.Execution.Step("first", action, SharedIterations(2, 3))
		d.Execution.Step("second", action, SharedIterations(2, 5))

		return d.Execution.Err()
	}}
	result, err := RunTest(t.Context(), test, noopRunOptions())
	require.NoError(t, err)
	require.Len(t, result.Metrics["measurement_seconds"].Series, 2)
	require.Equal(t, int64(3), result.Executions["first"].Iterations)
	require.Equal(t, int64(5), result.Executions["second"].Iterations)
	require.Positive(t, result.Measurements["first"])
	require.Positive(t, result.Measurements["second"])
}

func TestInputEnvironmentIsSnapshotAcrossReplays(t *testing.T) {
	t.Setenv("ROWS", "7")

	calls := 0

	var observed []int

	test := Test{Name: "snapshot", Define: func(d *Def) error {
		rows, _ := d.Param.Int("rows", 1, "")
		observed = append(observed, rows)

		calls++
		if calls == 1 {
			t.Setenv("ROWS", "9")
		}

		d.Execution.Step("work", func(context.Context, *Bench) error { return nil })

		return nil
	}}
	_, err := RunTest(t.Context(), test, noopRunOptions())
	require.NoError(t, err)
	require.Equal(t, []int{7, 7}, observed)
}

func TestDirectReportCopiesPayload(t *testing.T) {
	test := Test{Name: "payload", Define: func(d *Def) error {
		value := map[string]int{"count": 1}
		d.Report.Put("example", 1, value)
		value["count"] = 2

		d.Execution.Step("work", func(context.Context, *Bench) error { return nil })

		return nil
	}}
	result, err := RunTest(t.Context(), test, noopRunOptions())
	require.NoError(t, err)
	require.JSONEq(t, `{"count":1}`, string(result.WorkloadReports[0].Data))
}

func TestFailureCleanupErrorsRemainDistinct(t *testing.T) {
	original := errors.New("operation")
	cleanup := errors.New("cleanup")
	test := Test{Name: "failure", Define: func(d *Def) error {
		d.Execution.Step("work", func(context.Context, *Bench) error { return original })
		d.Execution.Step(
			"cleanup",
			func(context.Context, *Bench) error { return cleanup },
			Always(time.Second),
		)

		return d.Execution.Err()
	}}
	result, err := RunTest(t.Context(), test, noopRunOptions())
	require.ErrorIs(t, err, original)
	require.ErrorIs(t, err, cleanup)
	require.Equal(t, report.StatusFailed, result.Status)
	require.Equal(t, "work", result.Failure.Phase)
}
