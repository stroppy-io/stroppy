package bench

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
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

func TestParseOTLPHeaders(t *testing.T) {
	t.Parallel()

	require.Equal(t, map[string]string{
		"Authorization": "Bearer token",
		"x-tenant":      "benchmark",
	}, parseOTLPHeaders("Authorization=Bearer token, x-tenant=benchmark"))
}

func TestHistogramQuantileUsesBoundedBuckets(t *testing.T) {
	t.Parallel()

	bounds := []float64{1, 5, 10}
	buckets := []uint64{1, 3, 5, 1}

	require.InDelta(t, 1.0, histogramQuantile(bounds, buckets, 10, 0), 0)
	require.InDelta(t, 5.0, histogramQuantile(bounds, buckets, 10, 0.25), 0)
	require.InDelta(t, 10.0, histogramQuantile(bounds, buckets, 10, 0.9), 0)
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
