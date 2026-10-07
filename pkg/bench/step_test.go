package bench

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	_ "github.com/stroppy-io/stroppy/v6/pkg/driver/noop"
)

// testBenchFixture wires a Bench to an observer logger and a fresh meter provider
// so step behavior (console records + metric step tag) can be asserted without a
// full Run. reader and prefix support metric collection.
type testBenchFixture struct {
	b         *Bench
	logs      *observer.ObservedLogs
	rootState *rootState
	reader    *sdkmetric.ManualReader
	prefix    string
}

func newTestBenchFixture(t *testing.T) *testBenchFixture {
	t.Helper()

	core, logs := observer.New(zapcore.InfoLevel)
	lg := zap.New(core)

	provider, reader, prefix, err := newMeterProvider(context.Background(), &MetricsConfig{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })

	rootState := &rootState{
		lg:         lg,
		registry:   newmetricRegistry(provider.Meter("test"), prefix),
		txMetrics:  &txMetrics{},
		stepFilter: newStepFilter(nil, nil),
	}

	return &testBenchFixture{
		b: &Bench{
			root: rootState,
			vu:   &VU{root: rootState, ctx: context.Background()},
			lg:   lg,
		},
		logs:      logs,
		rootState: rootState,
		reader:    reader,
		prefix:    prefix,
	}
}

func TestImmediateStepFiltering(t *testing.T) {
	test := Test{Name: "filters", Define: func(d *Def) error {
		d.Execution.Step("ordinary", func(context.Context, *Bench) error {
			t.Fatal("filtered step ran")

			return nil
		})
		d.Execution.Step("cleanup", func(context.Context, *Bench) error {
			t.Fatal("filtered cleanup ran")

			return nil
		}, Always(time.Second))

		return d.Execution.Err()
	}}
	options := noopRunOptions()
	options.Steps = []string{"other"}
	result, err := RunTest(t.Context(), test, options)
	require.NoError(t, err)
	require.Len(t, result.Steps, 2)

	for _, step := range result.Steps {
		require.Equal(t, "skipped", step.Status)
		require.Zero(t, step.Executions)
	}
}
