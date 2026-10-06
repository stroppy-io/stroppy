package bench

import (
	"context"
	"errors"
	"runtime"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/trace"
)

func TestMetricWritersCollectWhileRecording(t *testing.T) {
	provider, reader, prefix, err := newMeterProvider(t.Context(), &MetricsConfig{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })

	registry := newmetricRegistry(provider.Meter("test"), prefix)
	counter, err := registry.NewMetric("count", Counter)
	require.NoError(t, err)
	histogram, err := registry.NewMetric("duration", Trend)
	require.NoError(t, err)

	attrs := attributes("step", "workload")

	var workers sync.WaitGroup
	for worker := range 8 {
		workers.Go(func() {
			ctx := metricWorkerContext(t.Context(), worker, 8)
			for range 1000 {
				counter.add(ctx, 1, attrs)
				histogram.add(ctx, 1, attrs)
			}
		})
	}

	for range 10 {
		var data metricdata.ResourceMetrics
		require.NoError(t, reader.Collect(t.Context(), &data))
		mergeMetricWriters(&data)
	}

	workers.Wait()

	var data metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(t.Context(), &data))
	mergeMetricWriters(&data)
	require.InDelta(t, 8000, findSum(t, data, prefix+"count"), 0)
	points := findHistogram(t, data, prefix+"duration").DataPoints
	require.Len(t, points, 1)
	require.Equal(t, uint64(8000), points[0].Count)
	require.InDelta(t, 8000, points[0].Sum, 0)
}

func TestMetricWriterDimensionIsReserved(t *testing.T) {
	_, err := DescribeTest(Test{Name: "private-label", Define: func(d *Def) error {
		d.Metrics.Counter("count", LabelValues(metricWriterKey, "authored"))

		return nil
	}})

	var validation *ValidationError
	require.ErrorAs(t, err, &validation)
	_, err = metricsResource(&MetricsConfig{
		ResourceAttributes: map[string]string{metricWriterKey: "metadata"},
	})
	require.Error(t, err)
}

func TestMetricWriterFinalReportsHaveOnlyPublicSeries(t *testing.T) {
	test := Test{Name: "public-series", Define: func(d *Def) error {
		counter := d.Metrics.Counter("count")
		histogram := d.Metrics.Histogram("latency", Bounds(1, 2))
		action := func(ctx context.Context, _ *Bench) error {
			counter.Add(ctx, 1)
			histogram.Record(ctx, 1.5)

			return nil
		}
		d.Execution.Step("serial", action, SharedIterations(1, 3))
		d.Execution.Step("parallel", action, SharedIterations(8, 100))
		d.Report.Contribute("check", 1, func(snapshot ReportContext) (ReportContribution, error) {
			require.InDelta(t, 103, snapshot.Metrics["count"].Total, 0)
			require.Len(t, snapshot.Series["count"].Series, 2)

			return ReportContribution{Data: "checked"}, nil
		})

		return d.Execution.Err()
	}}
	result, err := RunTest(t.Context(), test, noopRunOptions())
	require.NoError(t, err)
	require.InDelta(t, 103, *result.Metrics["count"].Total, 0)
	require.Len(t, result.Metrics["count"].Series, 2)
	require.Len(t, result.Metrics["latency"].Series, 2)

	for _, metric := range result.Metrics {
		for _, series := range metric.Series {
			require.NotContains(t, series.Attributes, metricWriterKey)
		}
	}
}

func TestMetricWriterCardinalityAndBindingsStayBounded(t *testing.T) {
	provider, reader, prefix, err := newMeterProvider(t.Context(), &MetricsConfig{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })

	registry := newmetricRegistry(provider.Meter("test"), prefix)
	counter, err := registry.NewMetric("count", Counter)
	require.NoError(t, err)
	gauge, err := registry.NewMetric("latest", Gauge)
	require.NoError(t, err)
	contexts := []context.Context{
		metricWorkerContext(t.Context(), 0, 1),
		metricWorkerContext(t.Context(), 0, 2),
		metricWorkerContext(t.Context(), 1, 2),
	}

	const series = metricCardinalityLimit + 100
	for value := range series {
		attrs := attributes("key", strconv.Itoa(value))
		for _, ctx := range contexts {
			counter.add(ctx, 1, attrs)
			gauge.add(ctx, 1, attrs)
		}
	}

	var data metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(t.Context(), &data))
	mergeMetricWriters(&data)

	for _, scope := range data.ScopeMetrics {
		for _, metric := range scope.Metrics {
			switch points := metric.Data.(type) {
			case metricdata.Sum[float64]:
				require.Len(t, points.DataPoints, metricCardinalityLimit)

				var total, overflow float64
				for _, point := range points.DataPoints {
					total += point.Value
					if point.Attributes.HasValue("otel.metric.overflow") {
						overflow += point.Value
					}
				}

				require.InDelta(t, series*len(contexts), total, 0)
				require.InDelta(t, 101*len(contexts), overflow, 0)
			case metricdata.Gauge[float64]:
				require.Len(t, points.DataPoints, metricCardinalityLimit)
			}
		}
	}

	bindings := 0
	counter.bindings.Range(func(_, _ any) bool {
		bindings++

		return true
	})
	require.Equal(t, metricCardinalityLimit*len(contexts), bindings)
	require.Len(t, counter.series, metricCardinalityLimit-1)
}

func TestMetricWritersMergeDeltaCollectionAndCancelCause(t *testing.T) {
	selector := func(sdkmetric.InstrumentKind) metricdata.Temporality { return metricdata.DeltaTemporality }
	reader := sdkmetric.NewManualReader(sdkmetric.WithTemporalitySelector(selector))
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })

	registry := newmetricRegistry(provider.Meter("test"), "")
	counter, err := registry.NewMetric("count", Counter)
	require.NoError(t, err)
	histogram, err := registry.NewMetric("duration", Trend)
	require.NoError(t, err)
	parent, cancel := context.WithCancelCause(t.Context())
	ctx := metricWorkerContext(parent, 0, 2)
	cause := errors.New("test cancellation")
	cancel(cause)
	require.ErrorIs(t, context.Cause(ctx), cause)

	for _, amount := range []int{2, 3} {
		for worker := range 2 {
			ctx := metricWorkerContext(t.Context(), worker, 2)
			counter.add(ctx, float64(amount), attributes())
			histogram.add(ctx, float64(amount), attributes())
		}

		var data metricdata.ResourceMetrics
		require.NoError(t, reader.Collect(t.Context(), &data))
		mergeMetricWriters(&data)
		mergeMetricWriters(&data)
		require.InDelta(t, amount*2, findSum(t, data, "count"), 0)
		points := findHistogram(t, data, "duration").DataPoints
		require.Len(t, points, 1)
		require.Equal(t, uint64(2), points[0].Count)
		require.InDelta(t, amount*2, points[0].Sum, 0)
	}
}

func TestMetricWriterExemplarReservoirsStayBounded(t *testing.T) {
	provider, reader, prefix, err := newMeterProvider(t.Context(), &MetricsConfig{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })

	registry := newmetricRegistry(provider.Meter("test"), prefix)
	histogram, err := registry.newInstrument("latency", Trend, "", "", []float64{1, 2})
	require.NoError(t, err)
	counter, err := registry.NewMetric("count", Counter)
	require.NoError(t, err)

	span := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: trace.TraceID{1}, SpanID: trace.SpanID{2}, TraceFlags: trace.FlagsSampled,
	})

	ctx := trace.ContextWithSpanContext(t.Context(), span)
	for worker := range metricWriterLimit {
		workerContext := metricWorkerContext(ctx, worker, metricWriterLimit)
		for _, value := range []float64{0, 1.5, 3} {
			histogram.add(workerContext, value, attributes())
			counter.add(workerContext, value, attributes())
		}
	}

	var data metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(t.Context(), &data))
	mergeMetricWriters(&data)
	points := findHistogram(t, data, prefix+"latency").DataPoints
	require.Len(t, points, 1)
	require.Len(t, points[0].Exemplars, 3)

	for _, scope := range data.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if sum, ok := metric.Data.(metricdata.Sum[float64]); ok {
				require.LessOrEqual(t, len(sum.DataPoints[0].Exemplars), runtime.GOMAXPROCS(0))
			}
		}
	}
}

func TestMetricWritersMergeCountersHistogramsAndRates(t *testing.T) {
	provider, reader, prefix, err := newMeterProvider(t.Context(), &MetricsConfig{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })

	registry := newmetricRegistry(provider.Meter("test"), prefix)
	counter, err := registry.NewMetric("count", Counter)
	require.NoError(t, err)
	histogram, err := registry.newInstrument("latency", Trend, "s", "latency", []float64{1, 2})
	require.NoError(t, err)
	rate, err := registry.NewMetric("checks", Rate)
	require.NoError(t, err)

	var workers sync.WaitGroup
	for worker := range 4 {
		workers.Go(func() {
			ctx := metricWorkerContext(t.Context(), worker, 4)

			for range 100 {
				attrs := attributes("step", "workload")
				counter.add(ctx, 1, attrs)
				histogram.add(ctx, float64(worker), attrs)
				rate.add(ctx, float64(worker%2), attrs)
			}
		})
	}

	workers.Wait()

	for range 2 {
		var data metricdata.ResourceMetrics
		require.NoError(t, reader.Collect(t.Context(), &data))
		mergeMetricWriters(&data)
		require.InDelta(t, 400, findSum(t, data, prefix+"count"), 0)
		require.InDelta(t, 400, findSum(t, data, prefix+"checks_events_total"), 0)
		require.InDelta(t, 200, findSum(t, data, prefix+"checks_true_total"), 0)
		histogram := findHistogram(t, data, prefix+"latency")
		require.Len(t, histogram.DataPoints, 1)
		point := histogram.DataPoints[0]
		require.Equal(t, uint64(400), point.Count)
		require.InDelta(t, 600, point.Sum, 0)
		require.Equal(t, []uint64{200, 100, 100}, point.BucketCounts)
		minimum, set := point.Min.Value()
		require.True(t, set)
		require.InDelta(t, 0, minimum, 0)

		maximum, set := point.Max.Value()
		require.True(t, set)
		require.InDelta(t, 3, maximum, 0)

		_, private := point.Attributes.Value(attribute.Key(metricWriterKey))
		require.False(t, private)
		require.Equal(t, "workload", attributeValue(point.Attributes, "step"))
	}
}

func TestMetricWriterKeepsSampledExemplars(t *testing.T) {
	provider, reader, prefix, err := newMeterProvider(t.Context(), &MetricsConfig{})
	require.NoError(t, err)

	defer provider.Shutdown(context.Background())

	registry := newmetricRegistry(provider.Meter("test"), prefix)
	histogram, err := registry.NewMetric("duration", Trend)
	require.NoError(t, err)

	span := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: trace.TraceID{1}, SpanID: trace.SpanID{2}, TraceFlags: trace.FlagsSampled,
	})
	ctx := trace.ContextWithSpanContext(t.Context(), span)
	ctx = metricWorkerContext(ctx, 0, 2)
	histogram.add(ctx, 1, attributes("step", "workload"))

	var data metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(ctx, &data))
	mergeMetricWriters(&data)
	points := findHistogram(t, data, prefix+"duration").DataPoints
	require.Len(t, points, 1)
	require.NotEmpty(t, points[0].Exemplars)

	id := span.TraceID()
	require.Equal(t, id[:], points[0].Exemplars[0].TraceID)
}

func TestMetricMergeTimesAndGaugeSemantics(t *testing.T) {
	start := time.Now().Add(-time.Minute)
	points := []metricdata.DataPoint[float64]{
		{Attributes: attribute.NewSet(attribute.Int(metricWriterKey, 0)), Value: 1, StartTime: start, Time: start},
		{
			Attributes: attribute.NewSet(attribute.Int(metricWriterKey, 1)), Value: 2,
			StartTime: start.Add(time.Second), Time: start.Add(2 * time.Second),
		},
	}
	out := mergeSumPoints(points)
	require.Len(t, out, 1)
	require.Equal(t, start, out[0].StartTime)
	require.Equal(t, start.Add(2*time.Second), out[0].Time)
	provider, reader, prefix, err := newMeterProvider(t.Context(), &MetricsConfig{})
	require.NoError(t, err)

	defer provider.Shutdown(context.Background())

	registry := newmetricRegistry(provider.Meter("test"), prefix)
	gauge, err := registry.NewMetric("latest", Gauge)
	require.NoError(t, err)

	for worker := range 2 {
		gauge.add(metricWorkerContext(t.Context(), worker, 4), float64(worker), attributes())
	}

	var data metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(t.Context(), &data))
	mergeMetricWriters(&data)
	require.InDelta(t, 1, *reportMetrics(data, prefix)["latest"].Total, 0)
}
