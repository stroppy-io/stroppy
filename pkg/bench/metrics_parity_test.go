package bench

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestMetricCaseInsensitiveIdentity(t *testing.T) {
	for _, typ := range []metricType{Counter, Trend, Rate, Gauge} {
		t.Run(fmt.Sprint(typ), func(t *testing.T) {
			provider, reader, registry, prefix, err := newMeterProvider(t.Context(), nil)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })

			first, err := registry.newInstrument("latency", typ, "ms", "latency", []float64{1, 2})
			require.NoError(t, err)
			second, err := registry.newInstrument("Latency", typ, "ms", "latency", []float64{1, 2})
			require.NoError(t, err)
			first.add(t.Context(), 1, attributes())
			second.add(t.Context(), 2, attributes())

			var data metricdata.ResourceMetrics
			require.NoError(t, reader.Collect(t.Context(), &data))
			require.Len(t, data.ScopeMetrics, 1)

			if typ == Rate {
				require.Len(t, data.ScopeMetrics[0].Metrics, 2)
				require.InDelta(t, 2, findSum(t, data, prefix+"latency_events_total"), 0)
				require.InDelta(t, 2, findSum(t, data, prefix+"latency_true_total"), 0)
			} else {
				require.Len(t, data.ScopeMetrics[0].Metrics, 1)
				require.Equal(t, prefix+"latency", data.ScopeMetrics[0].Metrics[0].Name)

				switch typ {
				case Counter:
					require.InDelta(t, 3, findSum(t, data, prefix+"latency"), 0)
				case Trend:
					point := findHistogram(t, data, prefix+"latency").DataPoints[0]
					require.Equal(t, uint64(2), point.Count)
					require.InDelta(t, 3, point.Sum, 0)
				case Gauge:
					point := data.ScopeMetrics[0].Metrics[0].Data.(metricdata.Gauge[float64]).DataPoints[0]
					require.InDelta(t, 2, point.Value, 0)
				case Rate:
					t.Fatal("rate handled separately")
				}
			}

			_, err = registry.NewMetric("Latency", typ)
			require.ErrorIs(t, err, errMetricAlreadyRegistered)
		})
	}
}

func TestMetricAggregationMatchesSDK(t *testing.T) {
	for _, workers := range []int{1, 100, 256} {
		t.Run(fmt.Sprintf("workers=%d", workers), func(t *testing.T) {
			provider, reader, registry, prefix, err := newMeterProvider(t.Context(), &MetricsConfig{})
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })

			counter, err := registry.newInstrument("count", Counter, "{operation}", "completed operations", nil)
			require.NoError(t, err)
			histogram, err := registry.newInstrument("latency", Trend, "ms", "latency", []float64{0, 1, 2, 5})
			require.NoError(t, err)
			rate, err := registry.NewMetric("checks", Rate)
			require.NoError(t, err)
			gauge, err := registry.newInstrument("latest", Gauge, "By", "last value", nil)
			require.NoError(t, err)

			referenceReader := sdkmetric.NewManualReader()
			reference := sdkmetric.NewMeterProvider(sdkmetric.WithReader(referenceReader))

			t.Cleanup(func() { require.NoError(t, reference.Shutdown(context.Background())) })

			meter := reference.Meter("parity")
			referenceCounter, err := meter.Float64Counter(
				prefix+"count", otelmetric.WithUnit("{operation}"), otelmetric.WithDescription("completed operations"),
			)
			require.NoError(t, err)
			referenceHistogram, err := meter.Float64Histogram(
				prefix+"latency", otelmetric.WithUnit("ms"), otelmetric.WithDescription("latency"),
				otelmetric.WithExplicitBucketBoundaries(0, 1, 2, 5),
			)
			require.NoError(t, err)
			referenceTotal, err := meter.Float64Counter(prefix + "checks_events_total")
			require.NoError(t, err)
			referenceTrue, err := meter.Float64Counter(prefix + "checks_true_total")
			require.NoError(t, err)
			referenceGauge, err := meter.Float64Gauge(
				prefix+"latest", otelmetric.WithUnit("By"), otelmetric.WithDescription("last value"),
			)
			require.NoError(t, err)

			var group sync.WaitGroup
			for worker := range workers {
				group.Go(func() {
					ctx := metricWorkerContext(context.Background(), worker, workers)
					attrs := attributes("operation", strconv.Itoa(worker%3))

					for sample := range 64 {
						value := []float64{-1, 0, 0.1, 1, 1.5, 2, 5, 8}[sample%8]

						counter.add(ctx, 0.125, attrs)
						histogram.add(ctx, value, attrs)
						rate.add(ctx, float64(sample%2), attrs)
						referenceCounter.Add(ctx, 0.125, attrs.add...)
						referenceHistogram.Record(ctx, value, attrs.record...)
						referenceTotal.Add(ctx, 1, attrs.add...)

						if sample%2 != 0 {
							referenceTrue.Add(ctx, 1, attrs.add...)
						}
					}
				})
			}

			group.Wait()

			for _, value := range []float64{4, -2, 0, 8} {
				gauge.add(t.Context(), value, attributes())
				referenceGauge.Record(t.Context(), value)
			}

			for range 2 {
				var actual, expected metricdata.ResourceMetrics
				require.NoError(t, reader.Collect(t.Context(), &actual))
				require.NoError(t, referenceReader.Collect(t.Context(), &expected))
				mergeMetricWriters(&actual)
				assertMetricParity(t, actual, expected)
			}
		})
	}
}

func assertMetricParity(t *testing.T, actual, expected metricdata.ResourceMetrics) {
	t.Helper()

	flatten := func(data metricdata.ResourceMetrics) map[string]metricdata.Metrics {
		out := map[string]metricdata.Metrics{}

		for _, scope := range data.ScopeMetrics {
			for _, metric := range scope.Metrics {
				out[metric.Name] = metric
			}
		}

		return out
	}
	got, want := flatten(actual), flatten(expected)
	require.Len(t, got, len(want))

	for name, expectedMetric := range want {
		actualMetric, exists := got[name]
		require.True(t, exists, name)
		require.Equal(t, expectedMetric.Unit, actualMetric.Unit, name)
		require.Equal(t, expectedMetric.Description, actualMetric.Description, name)

		switch expectedData := expectedMetric.Data.(type) {
		case metricdata.Sum[float64]:
			actualData, ok := actualMetric.Data.(metricdata.Sum[float64])
			require.True(t, ok, name)
			require.Equal(t, expectedData.Temporality, actualData.Temporality, name)
			require.Equal(t, expectedData.IsMonotonic, actualData.IsMonotonic, name)
			assertPointParity(t, actualData.DataPoints, expectedData.DataPoints)
		case metricdata.Gauge[float64]:
			actualData, ok := actualMetric.Data.(metricdata.Gauge[float64])
			require.True(t, ok, name)
			assertPointParity(t, actualData.DataPoints, expectedData.DataPoints)
		case metricdata.Histogram[float64]:
			actualData, ok := actualMetric.Data.(metricdata.Histogram[float64])
			require.True(t, ok, name)
			require.Equal(t, expectedData.Temporality, actualData.Temporality, name)

			sortHistogram := func(points []metricdata.HistogramDataPoint[float64]) {
				slices.SortFunc(points, func(a, b metricdata.HistogramDataPoint[float64]) int {
					return compareAttributes(a.Attributes, b.Attributes)
				})
			}
			sortHistogram(actualData.DataPoints)
			sortHistogram(expectedData.DataPoints)
			require.Len(t, actualData.DataPoints, len(expectedData.DataPoints), name)

			for index, point := range expectedData.DataPoints {
				gotPoint := actualData.DataPoints[index]
				require.Equal(t, point.Attributes, gotPoint.Attributes, name)
				require.Equal(t, point.Count, gotPoint.Count, name)
				require.Equal(t, point.Bounds, gotPoint.Bounds, name)
				require.Equal(t, point.BucketCounts, gotPoint.BucketCounts, name)
				require.Equal(t, point.Min, gotPoint.Min, name)
				require.Equal(t, point.Max, gotPoint.Max, name)
				require.InDelta(t, point.Sum, gotPoint.Sum, max(1e-12, math.Abs(point.Sum)*1e-12), name)
			}
		default:
			t.Fatalf("unexpected aggregation %T", expectedData)
		}
	}
}

func compareAttributes(a, b attribute.Set) int {
	return slices.CompareFunc(a.ToSlice(), b.ToSlice(), func(x, y attribute.KeyValue) int {
		if x.Key != y.Key {
			return strings.Compare(string(x.Key), string(y.Key))
		}

		return strings.Compare(x.Value.String(), y.Value.String())
	})
}

func assertPointParity(t *testing.T, actual, expected []metricdata.DataPoint[float64]) {
	t.Helper()

	sortPoints := func(points []metricdata.DataPoint[float64]) {
		slices.SortFunc(points, func(a, b metricdata.DataPoint[float64]) int {
			return compareAttributes(a.Attributes, b.Attributes)
		})
	}
	sortPoints(actual)
	sortPoints(expected)
	require.Len(t, actual, len(expected))

	for index, point := range expected {
		require.Equal(t, point.Attributes, actual[index].Attributes)
		require.InDelta(t, point.Value, actual[index].Value, max(1e-12, math.Abs(point.Value)*1e-12))
	}
}
