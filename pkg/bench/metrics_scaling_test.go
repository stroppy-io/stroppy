package bench

import (
	"context"
	"fmt"
	"runtime"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/exemplar"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/trace"
)

func BenchmarkMetricWriterCollection(b *testing.B) {
	provider, reader, prefix, err := newMeterProvider(b.Context(), &MetricsConfig{})
	require.NoError(b, err)
	b.Cleanup(func() { require.NoError(b, provider.Shutdown(context.Background())) })

	registry := newmetricRegistry(provider.Meter("benchmark"), prefix)
	histogram, err := registry.NewMetric("duration", Trend)
	require.NoError(b, err)
	counter, err := registry.NewMetric("count", Counter)
	require.NoError(b, err)

	for worker := range 24 {
		ctx := metricWorkerContext(b.Context(), worker, 24)

		for label := range 10 {
			attrs := attributes("label", strconv.Itoa(label))
			histogram.add(ctx, 1, attrs)
			counter.add(ctx, 1, attrs)
		}
	}

	var data metricdata.ResourceMetrics

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := reader.Collect(b.Context(), &data); err != nil {
			b.Fatal(err)
		}

		mergeMetricWriters(&data)
	}
}

func BenchmarkMetricWriterRecording(b *testing.B) {
	for _, sharded := range []bool{false, true} {
		b.Run(fmt.Sprintf("sharded=%t", sharded), func(b *testing.B) {
			provider, _, prefix, err := newMeterProvider(b.Context(), &MetricsConfig{})
			require.NoError(b, err)
			b.Cleanup(func() { require.NoError(b, provider.Shutdown(context.Background())) })

			histogram, err := newmetricRegistry(provider.Meter("benchmark"), prefix).NewMetric("duration", Trend)
			require.NoError(b, err)

			ctx := context.Background()

			var writer *metricWriter

			if sharded {
				ctx = metricWorkerContext(ctx, 0, 2)
				writer, _ = ctx.Value(metricWriterContextKey{}).(*metricWriter)
			}

			attrs := attributes("step", "workload")
			histogram.addWriter(ctx, 1, attrs, writer)
			b.ReportAllocs()
			b.ResetTimer()

			for b.Loop() {
				histogram.addWriter(ctx, 1, attrs, writer)
			}
		})
	}
}

// BenchmarkHistogramScaling isolates shared-series contention from caller state.
func BenchmarkHistogramScaling(b *testing.B) {
	for _, sharded := range []bool{false, true} {
		for _, exemplars := range []bool{false, true} {
			for _, cachedTrace := range []bool{false, true} {
				name := fmt.Sprintf("sharded=%t/exemplars=%t/cached-trace=%t", sharded, exemplars, cachedTrace)
				b.Run(name, func(b *testing.B) {
					providerOptions := []sdkmetric.Option{sdkmetric.WithReader(sdkmetric.NewManualReader())}
					if !exemplars {
						providerOptions = append(providerOptions, sdkmetric.WithExemplarFilter(exemplar.AlwaysOffFilter))
					}

					provider := sdkmetric.NewMeterProvider(providerOptions...)

					b.Cleanup(func() { require.NoError(b, provider.Shutdown(context.Background())) })

					histogram, err := provider.Meter("benchmark").Float64Histogram("duration",
						otelmetric.WithExplicitBucketBoundaries(durationMillisecondsBounds...))
					require.NoError(b, err)

					ctx, cancel := context.WithCancelCause(context.Background())
					defer cancel(nil)

					var workers atomic.Int64

					b.ReportAllocs()
					b.ResetTimer()
					b.RunParallel(func(pb *testing.PB) {
						worker := workers.Add(1)

						workerContext := ctx
						if cachedTrace {
							workerContext = trace.ContextWithSpan(ctx, trace.SpanFromContext(ctx))
						}

						attrs := []attribute.KeyValue{attribute.String("step", "workload")}
						if sharded {
							attrs = append(attrs, attribute.Int64("shard", worker))
						}

						options := []otelmetric.RecordOption{otelmetric.WithAttributes(attrs...)}
						for pb.Next() {
							histogram.Record(workerContext, 1, options...)
						}
					})
					b.ReportMetric(float64(runtime.GOMAXPROCS(0)), "workers")
				})
			}
		}
	}
}
