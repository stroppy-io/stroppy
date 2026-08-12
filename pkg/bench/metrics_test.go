package bench

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
)

func TestTrendUsesBoundedHistogram(t *testing.T) {
	t.Parallel()

	provider, reader, prefix, err := newMeterProvider(context.Background(), &MetricsConfig{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })

	registry := NewRegistry(provider.Meter("test"), prefix)
	trend, err := registry.NewMetric("run_query_duration", Trend)
	require.NoError(t, err)

	attrs := attributes("step", "workload")

	const observations = 1_000_000
	for range observations {
		trend.add(context.Background(), 1, attrs)
	}

	var data metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &data))

	histogram := findHistogram(t, data, prefix+"run_query_duration")
	require.Len(t, histogram.DataPoints, 1)
	require.Equal(t, uint64(observations), histogram.DataPoints[0].Count)
	require.Equal(t, durationMillisecondsBounds, histogram.DataPoints[0].Bounds)
	require.Len(t, histogram.DataPoints[0].BucketCounts, len(durationMillisecondsBounds)+1)
}

func TestRateUsesEventCounters(t *testing.T) {
	t.Parallel()

	provider, reader, prefix, err := newMeterProvider(context.Background(), &MetricsConfig{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })

	registry := NewRegistry(provider.Meter("test"), prefix)
	rate, err := registry.NewMetric("checks", Rate)
	require.NoError(t, err)

	attrs := attributes("step", "workload")
	rate.add(context.Background(), 0, attrs)
	rate.add(context.Background(), 1, attrs)
	rate.add(context.Background(), 1, attrs)

	var data metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &data))
	require.InDelta(t, 3.0, findSum(t, data, prefix+"checks_events_total"), 0)
	require.InDelta(t, 2.0, findSum(t, data, prefix+"checks_true_total"), 0)
}

func TestMeterProviderAcceptsNilConfig(t *testing.T) {
	t.Parallel()

	provider, reader, prefix, err := newMeterProvider(context.Background(), nil)
	require.NoError(t, err)
	require.NotNil(t, reader)
	require.Equal(t, defaultMetricsPrefix, prefix)
	require.NoError(t, provider.Shutdown(context.Background()))
}

func TestParseOTLPHeaders(t *testing.T) {
	t.Parallel()

	headers, err := parseOTLPHeaders("Authorization=Bearer%20token, x-tenant=benchmark")
	require.NoError(t, err)
	require.Equal(t, map[string]string{
		"Authorization": "Bearer token",
		"x-tenant":      "benchmark",
	}, headers)

	_, err = parseOTLPHeaders("Authorization=%ZZ")
	require.Error(t, err)
}

func TestGenericMetricCachesAttributes(t *testing.T) {
	t.Parallel()

	provider, _, prefix, err := newMeterProvider(context.Background(), &MetricsConfig{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })

	registry := NewRegistry(provider.Meter("test"), prefix)
	trend, err := registry.NewMetric("custom_duration", Trend)
	require.NoError(t, err)

	first := trend.taggedAttributes([]string{"step", "workload"})
	second := trend.taggedAttributes([]string{"step", "workload"})
	require.Same(t, &first.add[0], &second.add[0])

	odd := trend.taggedAttributes([]string{"step", "workload", "orphan"})
	require.Equal(t, first.set, odd.set)
}

func TestGenericMetricUsesOverflowAfterCardinalityLimit(t *testing.T) {
	t.Parallel()

	provider, _, prefix, err := newMeterProvider(context.Background(), &MetricsConfig{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })

	registry := NewRegistry(provider.Meter("test"), prefix)
	counter, err := registry.NewMetric("custom_total", Counter)
	require.NoError(t, err)
	counter.tagCount.Store(metricCardinalityLimit)

	got := counter.taggedAttributes([]string{"key", "new-value"})
	require.Equal(t, counter.overflow.set, got.set)
}

func TestGaugeSummarySumsAllSeries(t *testing.T) {
	t.Parallel()

	points := []metricdata.DataPoint[float64]{{Value: 2}, {Value: 3.5}}
	require.InDelta(t, 5.5, sumGauge(points), 0)
}

func TestHistogramQuantileUsesBoundedBuckets(t *testing.T) {
	t.Parallel()

	bounds := []float64{1, 5, 10}
	buckets := []uint64{1, 3, 5, 1}

	require.InDelta(t, 1.0, histogramQuantile(bounds, buckets, 10, 0), 0)
	require.InDelta(t, 5.0, histogramQuantile(bounds, buckets, 10, 0.25), 0)
	require.InDelta(t, 10.0, histogramQuantile(bounds, buckets, 10, 0.9), 0)
	require.InDelta(t, 10.0, histogramQuantile(bounds, buckets, 10, 1), 0)
	require.InDelta(t, 0.0, histogramQuantile(nil, nil, 0, 0.5), 0)
}

func BenchmarkTrendRecord(b *testing.B) {
	provider, _, prefix, err := newMeterProvider(context.Background(), &MetricsConfig{})
	require.NoError(b, err)
	b.Cleanup(func() { require.NoError(b, provider.Shutdown(context.Background())) })

	registry := NewRegistry(provider.Meter("benchmark"), prefix)
	trend, err := registry.NewMetric("run_query_duration", Trend)
	require.NoError(b, err)

	attrs := attributes("step", "workload")

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		trend.add(context.Background(), 1, attrs)
	}
}

func TestTransactionEndKeepsActionAndIsolationAttributes(t *testing.T) {
	provider, reader, prefix, err := newMeterProvider(context.Background(), &MetricsConfig{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })

	rootState := &RootState{
		registry:  NewRegistry(provider.Meter("test"), prefix),
		txMetrics: &txMetrics{},
	}
	vu := &VU{root: rootState, ctx: context.Background(), stepTag: "workload"}
	root = rootState

	rootState.txMetrics.recordTxEnd(
		vu, "commit", "payment", stroppy.TxIsolationLevel_READ_COMMITTED, time.Millisecond, 3, true,
	)

	var data metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &data))

	histogram := findHistogram(t, data, prefix+"tx_total_duration")
	require.Len(t, histogram.DataPoints, 1)
	attrs := histogram.DataPoints[0].Attributes
	require.Equal(t, "commit", attributeValue(attrs, "tx_action"))
	require.Equal(t, "read_committed", attributeValue(attrs, "tx_isolation"))
}

func BenchmarkMetricAdd(b *testing.B) {
	provider, _, prefix, err := newMeterProvider(context.Background(), &MetricsConfig{})
	require.NoError(b, err)
	b.Cleanup(func() { require.NoError(b, provider.Shutdown(context.Background())) })

	registry := NewRegistry(provider.Meter("benchmark"), prefix)
	trend, err := registry.NewMetric("custom_duration", Trend)
	require.NoError(b, err)

	metric := &Metric{m: trend}
	metric.Add(1, "step", "workload")

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		metric.Add(1, "step", "workload")
	}
}

func attributeValue(set attribute.Set, key string) string {
	value, ok := set.Value(attribute.Key(key))
	if !ok {
		return ""
	}

	return value.AsString()
}

func findHistogram(
	t *testing.T,
	data metricdata.ResourceMetrics,
	name string,
) metricdata.Histogram[float64] {
	t.Helper()

	for _, scope := range data.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name == name {
				histogram, ok := metric.Data.(metricdata.Histogram[float64])
				require.True(t, ok)

				return histogram
			}
		}
	}

	t.Fatalf("metric %q not found", name)

	return metricdata.Histogram[float64]{}
}

func findSum(t *testing.T, data metricdata.ResourceMetrics, name string) float64 {
	t.Helper()

	for _, scope := range data.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name != name {
				continue
			}

			sum, ok := metric.Data.(metricdata.Sum[float64])
			require.True(t, ok)

			var value float64
			for _, point := range sum.DataPoints {
				value += point.Value
			}

			return value
		}
	}

	t.Fatalf("metric %q not found", name)

	return 0
}
