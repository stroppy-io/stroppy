package bench

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	collectorpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	"google.golang.org/protobuf/proto"
)

func TestNativeExportPreservesExponentialOverride(t *testing.T) {
	for _, temporality := range []string{"cumulative", "delta"} {
		t.Run(temporality, func(t *testing.T) {
			t.Setenv("OTEL_EXPORTER_OTLP_METRICS_DEFAULT_HISTOGRAM_AGGREGATION", "base2_exponential_bucket_histogram")
			t.Setenv("OTEL_EXPORTER_OTLP_METRICS_TEMPORALITY_PREFERENCE", temporality)

			received := make(chan *collectorpb.ExportMetricsServiceRequest, 4)

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)

				request := new(collectorpb.ExportMetricsServiceRequest)
				if err != nil || proto.Unmarshal(body, request) != nil {
					w.WriteHeader(http.StatusBadRequest)

					return
				}

				received <- request

				w.Header().Set("Content-Type", "application/x-protobuf")
			}))
			defer server.Close()

			provider, reader, registry, prefix, err := newMeterProvider(t.Context(), &MetricsConfig{
				HTTPEndpoint: strings.TrimPrefix(server.URL, "http://"), Insecure: true,
			})
			require.NoError(t, err)
			histogram, err := registry.newInstrument("latency", Trend, "ms", "latency", []float64{1, 2})
			require.NoError(t, err)
			require.NotNil(t, histogram.sdkHistogram)

			for worker, value := range []float64{0, 1, 2, 4} {
				histogram.add(metricWorkerContext(t.Context(), worker, 4), value, attributes("step", "workload"))
			}

			for range 2 {
				var data metricdata.ResourceMetrics
				require.NoError(t, reader.Collect(t.Context(), &data))
				point := findHistogram(t, data, prefix+"latency").DataPoints[0]
				require.Equal(t, uint64(4), point.Count)
				require.Equal(t, []float64{1, 2}, point.Bounds)
				require.Equal(t, []uint64{2, 1, 1}, point.BucketCounts)
			}

			require.NoError(t, provider.Shutdown(context.Background()))

			select {
			case request := <-received:
				var histograms []*metricspb.ExponentialHistogram

				for _, resource := range request.ResourceMetrics {
					for _, scope := range resource.ScopeMetrics {
						for _, metric := range scope.Metrics {
							if metric.Name == prefix+"latency" {
								require.Nil(t, metric.GetHistogram())
								histograms = append(histograms, metric.GetExponentialHistogram())
							}
						}
					}
				}

				require.Len(t, histograms, 1)
				histogram := histograms[0]
				require.NotNil(t, histogram)
				require.Len(t, histogram.DataPoints, 1)
				point := histogram.DataPoints[0]
				require.Equal(t, uint64(4), point.Count)
				require.InDelta(t, 7, point.GetSum(), 0)
				require.InDelta(t, 0, point.GetMin(), 0)
				require.InDelta(t, 4, point.GetMax(), 0)
				require.Len(t, point.Attributes, 1)
				require.Equal(t, "step", point.Attributes[0].Key)

				if temporality == "delta" {
					require.Equal(t, metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA,
						histogram.AggregationTemporality)
				} else {
					require.Equal(t, metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE,
						histogram.AggregationTemporality)
				}
			case <-time.After(time.Second):
				t.Fatal("exponential metrics not exported")
			}
		})
	}
}
