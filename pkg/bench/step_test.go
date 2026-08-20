package bench

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	_ "github.com/stroppy-io/stroppy/pkg/driver/noop"
)

// testStepBench builds a Bench wired to an observer logger and a fresh meter
// provider so step behavior (console records + metric step tag) can be asserted
// without a full Run. The returned reader and prefix are for metric collection.
func testStepBench(
	t *testing.T,
) (*Bench, *observer.ObservedLogs, *RootState, *sdkmetric.ManualReader, string) {
	t.Helper()

	core, logs := observer.New(zapcore.InfoLevel)
	lg := zap.New(core)

	provider, reader, prefix, err := newMeterProvider(context.Background(), &MetricsConfig{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })

	rootState := &RootState{
		lg:         lg,
		registry:   NewRegistry(provider.Meter("test"), prefix),
		txMetrics:  &txMetrics{},
		stepFilter: newStepFilter(),
	}

	return &Bench{
		root: rootState,
		vu:   &VU{root: rootState, ctx: context.Background()},
		lg:   lg,
	}, logs, rootState, reader, prefix
}

func TestStepLogsStartAndEnd(t *testing.T) {
	b, logs, _, _, _ := testStepBench(t)

	require.NoError(t, b.Step("load_data", func() error { return nil }))

	require.Equal(t, 2, logs.Len())
	require.Equal(t, "Start of 'load_data' step", logs.All()[0].Message)
	require.True(t, strings.Contains(logs.All()[1].Message, "End of 'load_data' step"))
	require.Empty(t, b.vu.stepTag) // tag cleared after the step
}

func TestStepSilentKeepsMetricTagWithoutLogging(t *testing.T) {
	b, logs, rootState, reader, prefix := testStepBench(t)

	// stepBegin/stepEnd read the package-global root for NotifyStep (a no-op), so
	// install the test root the same way metrics_test.go does.
	previousRoot := root
	root = rootState
	t.Cleanup(func() { root = previousRoot })

	var tagDuring string
	err := b.StepSilent("workload", func() error {
		tagDuring = b.vu.stepTag
		rootState.txMetrics.recordQueryResult(b.vu, time.Millisecond, nil)

		return nil
	})
	require.NoError(t, err)

	require.Equal(t, "workload", tagDuring, "step tag must be set for the iteration metrics")
	require.Empty(t, b.vu.stepTag, "step tag must be cleared after the step")
	require.Zero(t, logs.Len(), "a silent step must not emit start/end records")

	var data metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &data))
	hist := findHistogram(t, data, prefix+"run_query_duration")
	require.Len(t, hist.DataPoints, 1)
	require.Equal(t, "workload", attributeValue(hist.DataPoints[0].Attributes, "step"))
}

func TestStepAndStepSilentRespectFilter(t *testing.T) {
	t.Run("loud skip is logged", func(t *testing.T) {
		b, logs, rootState, _, _ := testStepBench(t)
		rootState.stepFilter.only = map[string]struct{}{"load_data": {}}

		var ran atomic.Bool
		require.NoError(t, b.Step("workload", func() error { ran.Store(true); return nil }))

		require.False(t, ran.Load(), "filtered step must not run")
		require.Equal(t, 1, logs.Len())
		require.Equal(t, "Skipping step 'workload'", logs.All()[0].Message)
	})

	t.Run("silent skip is quiet", func(t *testing.T) {
		b, logs, rootState, _, _ := testStepBench(t)
		rootState.stepFilter.only = map[string]struct{}{"load_data": {}}

		var ran atomic.Bool
		require.NoError(t, b.StepSilent("workload", func() error { ran.Store(true); return nil }))

		require.False(t, ran.Load(), "filtered step must not run")
		require.Zero(t, logs.Len(), "silent skip must not emit records")
	})
}

// stepLifecycleWorkload exhibits the uniform workload-step contract: one loud
// setup step (load_data) and one silent per-iteration step (workload). Its
// iteration runs one query so per-iteration query metrics carry the step tag.
type stepLifecycleWorkload struct{}

func (stepLifecycleWorkload) Name() string { return "test/step-lifecycle" }

func (stepLifecycleWorkload) Define(*Def) error { return nil }

func (stepLifecycleWorkload) Setup(_ context.Context, b *Bench) error {
	return b.Step("load_data", func() error { return nil })
}

func (stepLifecycleWorkload) Iterate(ctx context.Context, b *Bench) error {
	return b.StepSilent("workload", func() error {
		_, err := b.QueryValue(ctx, "SELECT 1", nil)

		return err
	})
}

func (stepLifecycleWorkload) Teardown(context.Context, *Bench) error { return nil }

func TestWorkloadRunLogVolumeIsBounded(t *testing.T) {
	Register(func() Workload { return stepLifecycleWorkload{} })

	runAndCount := func(iterations string) (*observer.ObservedLogs, int) {
		core, logs := observer.New(zapcore.InfoLevel)
		lg := zap.New(core)

		err := Run(
			context.Background(),
			"test/step-lifecycle",
			map[int]*stroppy.DriverConfig{0: {DriverType: stroppy.DriverConfig_DRIVER_TYPE_NOOP}},
			nil,
			ParamInputs{CLI: map[string]string{"iterations": iterations, "vus": "1"}},
			lg,
			&MetricsConfig{},
		)
		require.NoError(t, err)

		return logs, logs.Len()
	}

	base, baseCount := runAndCount("100")
	_, highCount := runAndCount("10000")

	// The per-iteration workload step is silent, so a 100x increase in iteration
	// count must not change the log volume: only the one setup step records.
	require.Equal(t, baseCount, highCount, "log volume must be independent of iteration count")

	workloadRecords, setupStarts, setupEnds := 0, 0, 0
	for _, entry := range base.All() {
		if strings.Contains(entry.Message, "workload' step") {
			workloadRecords++
		}

		if entry.Message == "Start of 'load_data' step" {
			setupStarts++
		}

		if strings.HasPrefix(entry.Message, "End of 'load_data' step") {
			setupEnds++
		}
	}

	require.Zero(t, workloadRecords, "no per-iteration start/end workload records")
	require.Equal(t, 1, setupStarts, "setup step emits exactly one start record")
	require.Equal(t, 1, setupEnds, "setup step emits exactly one end record")
}
