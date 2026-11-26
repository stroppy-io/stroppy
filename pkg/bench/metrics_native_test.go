package bench

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	otelmetric "go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/trace"
)

func TestNativeMetricIndependentReaders(t *testing.T) {
	registry := newmetricRegistry("")
	cumulative := registry.producer(sdkmetric.DefaultTemporalitySelector)
	selector := func(sdkmetric.InstrumentKind) metricdata.Temporality { return metricdata.DeltaTemporality }
	first, second := registry.producer(selector), registry.producer(selector)
	histogram, err := registry.newInstrument("latency", Trend, "ms", "latency", []float64{1, 2})
	require.NoError(t, err)
	counter, err := registry.NewMetric("count", Counter)
	require.NoError(t, err)
	gauge, err := registry.NewMetric("latest", Gauge)
	require.NoError(t, err)

	collect := func(p *metricProducer, count uint64, sum float64, withGauge bool) {
		t.Helper()
		scopes, collectErr := p.Produce(t.Context())
		require.NoError(t, collectErr)

		data := metricdata.ResourceMetrics{ScopeMetrics: scopes}
		require.InDelta(t, sum, findSum(t, data, "count"), 0)
		point := findHistogram(t, data, "latency").DataPoints[0]
		require.Equal(t, count, point.Count)
		require.InDelta(t, sum, point.Sum, 0)

		for _, scope := range scopes {
			for _, metric := range scope.Metrics {
				if metric.Name == "latest" {
					require.True(t, withGauge)
					require.InDelta(t, 2, metric.Data.(metricdata.Gauge[float64]).DataPoints[0].Value, 0)
				}
			}
		}
	}

	for _, value := range []float64{1, 2} {
		histogram.add(t.Context(), value, attributes())
		counter.add(t.Context(), value, attributes())
		gauge.add(t.Context(), value, attributes())
	}

	collect(first, 2, 3, true)
	collect(cumulative, 2, 3, true)
	histogram.add(t.Context(), 5, attributes())
	counter.add(t.Context(), 5, attributes())
	collect(first, 1, 5, false)
	collect(second, 3, 8, true)
	collect(cumulative, 3, 8, true)

	for _, p := range []*metricProducer{first, second} {
		scopes, collectErr := p.Produce(t.Context())
		require.NoError(t, collectErr)
		require.Empty(t, scopes[0].Metrics)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err = cumulative.Produce(ctx)
	require.ErrorIs(t, err, context.Canceled)
}

func TestNativeHistogramConcurrentSnapshotsStayConsistent(t *testing.T) {
	provider, reader, registry, prefix, err := newMeterProvider(t.Context(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })

	histogram, err := registry.newInstrument("latency", Trend, "ms", "latency", []float64{1, 2})
	require.NoError(t, err)

	attrs := attributes()

	const workers, samples = 256, 1000

	var group sync.WaitGroup
	for worker := range workers {
		group.Go(func() {
			ctx := metricWorkerContext(context.Background(), worker, workers)
			for range samples {
				histogram.add(ctx, 1.5, attrs)
			}
		})
	}

	var previous uint64

	assertSnapshot := func() uint64 {
		t.Helper()

		var data metricdata.ResourceMetrics
		require.NoError(t, reader.Collect(t.Context(), &data))

		for _, scope := range data.ScopeMetrics {
			for _, metric := range scope.Metrics {
				if metric.Name != prefix+"latency" {
					continue
				}

				point := metric.Data.(metricdata.Histogram[float64]).DataPoints[0]
				require.GreaterOrEqual(t, point.Count, previous)
				require.Equal(t, []uint64{0, point.Count, 0}, point.BucketCounts)
				require.InDelta(t, float64(point.Count)*1.5, point.Sum, 0)
				previous = point.Count
			}
		}

		return previous
	}
	for range 50 {
		assertSnapshot()
	}

	group.Wait()
	require.Equal(t, uint64(workers*samples), assertSnapshot())
}

func TestNativeMetricWarmRecordingAllocations(t *testing.T) {
	for _, typ := range []metricType{Counter, Trend, Rate, Gauge} {
		t.Run(fmt.Sprint(typ), func(t *testing.T) {
			registry := newmetricRegistry("")
			metric, err := registry.NewMetric("measure", typ)
			require.NoError(t, err)

			ctx := metricWorkerContext(context.Background(), 0, 100)
			attrs := attributes("step", "workload")
			metric.add(ctx, 1, attrs)
			require.Zero(t, testing.AllocsPerRun(1000, func() { metric.add(ctx, 1, attrs) }))
		})
	}
}

func TestNativePublicMetricWarmRecordingAllocations(t *testing.T) {
	registry := newmetricRegistry("")
	metric, err := registry.NewMetric("latency", Trend)
	require.NoError(t, err)

	handle := &HistogramHandle{i: &instrument{
		metric: metric, labels: map[string][]string{"operation": {"read"}, "result": {"ok"}},
	}}
	ctx := context.WithValue(context.Background(), metricStepKey{}, "workload")
	labels := []Label{LabelValue("result", "ok"), LabelValue("operation", "read")}
	handle.Record(ctx, 1, labels...)
	require.Zero(t, testing.AllocsPerRun(1000, func() { handle.Record(ctx, 1, labels...) }))
}

func TestNativeOverflowWarmRecordingAllocations(t *testing.T) {
	registry := newmetricRegistry("")
	metric, err := registry.NewMetric("count", Counter)
	require.NoError(t, err)

	ctx := metricWorkerContext(context.Background(), 0, 100)
	for index := range metricCardinalityLimit {
		metric.add(ctx, 1, attributes("key", strconv.Itoa(index)))
	}

	first, second := attributes("key", "rejected-a"), attributes("key", "rejected-b")
	metric.add(ctx, 1, first)
	metric.add(ctx, 1, second)
	require.Zero(t, testing.AllocsPerRun(1000, func() {
		metric.add(ctx, 1, first)
		metric.add(ctx, 1, second)
	}))
}

func TestNativeDeltaSampledRecordingAllocations(t *testing.T) {
	for _, typ := range []metricType{Counter, Trend, Rate} {
		t.Run(fmt.Sprint(typ), func(t *testing.T) {
			registry := newmetricRegistry("")
			selector := func(sdkmetric.InstrumentKind) metricdata.Temporality { return metricdata.DeltaTemporality }
			producer := registry.producer(selector)
			metric, err := registry.NewMetric("sample", typ)
			require.NoError(t, err)

			sc := trace.NewSpanContext(trace.SpanContextConfig{
				TraceID: trace.TraceID{1}, SpanID: trace.SpanID{2}, TraceFlags: trace.FlagsSampled,
			})
			ctx := trace.ContextWithSpanContext(context.Background(), sc)
			attrs := attributes()
			metric.add(ctx, 1, attrs)
			shard := metric.writers[metricWriterLimit].Load().shard
			reservoir := shard.deltas[0].reservoir

			for range 3 {
				_, err = producer.Produce(t.Context())
				require.NoError(t, err)
				require.Same(t, reservoir, shard.deltas[0].reservoir)
				require.Zero(t, testing.AllocsPerRun(100, func() { metric.add(ctx, 1, attrs) }))
			}
		})
	}
}

func TestNativeDeltaDoesNotRepeatOldExemplars(t *testing.T) {
	registry := newmetricRegistry("")
	selector := func(sdkmetric.InstrumentKind) metricdata.Temporality { return metricdata.DeltaTemporality }
	producer := registry.producer(selector)
	histogram, err := registry.NewMetric("latency", Trend)
	require.NoError(t, err)

	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: trace.TraceID{1}, SpanID: trace.SpanID{2}, TraceFlags: trace.FlagsSampled,
	})
	histogram.add(trace.ContextWithSpanContext(context.Background(), sc), 1, attributes())

	scopes, err := producer.Produce(t.Context())
	require.NoError(t, err)
	point := findHistogram(t, metricdata.ResourceMetrics{ScopeMetrics: scopes}, "latency").DataPoints[0]
	require.NotEmpty(t, point.Exemplars)
	histogram.add(context.Background(), 8, attributes())

	scopes, err = producer.Produce(t.Context())
	require.NoError(t, err)
	point = findHistogram(t, metricdata.ResourceMetrics{ScopeMetrics: scopes}, "latency").DataPoints[0]
	require.Empty(t, point.Exemplars)
	require.Equal(t, uint64(1), point.Count)
	require.InDelta(t, 8, point.Sum, 0)
}

func TestNativeHistogramBoundsMatchSDK(t *testing.T) {
	for _, bounds := range [][]float64{nil, {}, {1, 2}} {
		t.Run(fmt.Sprintf("%#v", bounds), func(t *testing.T) {
			registry := newmetricRegistry("")
			histogram, err := registry.newInstrument("latency", Trend, "ms", "", bounds)
			require.NoError(t, err)

			reader := sdkmetric.NewManualReader()
			provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

			t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })

			referenceBounds := bounds
			if bounds == nil {
				referenceBounds = histogramBounds("latency")
			}

			reference, err := provider.Meter("test").Float64Histogram("latency",
				otelmetric.WithExplicitBucketBoundaries(referenceBounds...))
			require.NoError(t, err)

			for _, value := range []float64{0, 1, 2, 7, 1000} {
				histogram.add(t.Context(), value, attributes())
				reference.Record(t.Context(), value)
			}

			scopes, err := registry.producer(sdkmetric.DefaultTemporalitySelector).Produce(t.Context())
			require.NoError(t, err)

			var data metricdata.ResourceMetrics
			require.NoError(t, reader.Collect(t.Context(), &data))
			expected := findHistogram(t, data, "latency").DataPoints[0]
			actual := findHistogram(t, metricdata.ResourceMetrics{ScopeMetrics: scopes}, "latency").DataPoints[0]
			require.Equal(t, expected.Bounds, actual.Bounds)
			require.Equal(t, expected.BucketCounts, actual.BucketCounts)
		})
	}
}

func TestNativeExemplarFiltersAndRateValues(t *testing.T) {
	for _, filter := range []string{"trace_based", "always_off", "always_on"} {
		t.Run(filter, func(t *testing.T) {
			t.Setenv("OTEL_METRICS_EXEMPLAR_FILTER", filter)

			registry := newmetricRegistry("")
			rate, err := registry.NewMetric("checks", Rate)
			require.NoError(t, err)

			sc := trace.NewSpanContext(trace.SpanContextConfig{
				TraceID: trace.TraceID{1}, SpanID: trace.SpanID{2}, TraceFlags: trace.FlagsSampled,
			})

			ctx := trace.ContextWithSpanContext(t.Context(), sc)
			for _, value := range []float64{0, 1, 0} {
				rate.add(ctx, value, attributes())
			}

			producer := registry.producer(sdkmetric.DefaultTemporalitySelector)
			scopes, err := producer.Produce(t.Context())
			require.NoError(t, err)

			for _, metric := range scopes[0].Metrics {
				point := metric.Data.(metricdata.Sum[float64]).DataPoints[0]
				if filter == "always_off" {
					require.Empty(t, point.Exemplars)
				} else {
					require.NotEmpty(t, point.Exemplars)

					for _, sample := range point.Exemplars {
						require.InDelta(t, 1, sample.Value, 0)
						require.Equal(t, sc.TraceID().String(), trace.TraceID(sample.TraceID).String())
					}
				}
			}
		})
	}
}
