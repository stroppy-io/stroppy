package bench

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/sdk/instrumentation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	collectorpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

type metricParityServer struct {
	collectorpb.UnimplementedMetricsServiceServer
	mu       sync.Mutex
	requests []*collectorpb.ExportMetricsServiceRequest
}

func (s *metricParityServer) Export(
	_ context.Context, request *collectorpb.ExportMetricsServiceRequest,
) (*collectorpb.ExportMetricsServiceResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.requests = append(s.requests, request)

	return &collectorpb.ExportMetricsServiceResponse{}, nil
}

func TestNativeOTLPMatchesSDK(t *testing.T) {
	for _, transport := range []string{"http", "grpc"} {
		t.Run(transport, func(t *testing.T) {
			received := &metricParityServer{}
			cfg := &MetricsConfig{Insecure: true, Prefix: "parity_"}

			if transport == "http" {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					body, err := io.ReadAll(r.Body)

					request := &collectorpb.ExportMetricsServiceRequest{}
					if err != nil || proto.Unmarshal(body, request) != nil {
						w.WriteHeader(http.StatusBadRequest)

						return
					}

					_, _ = received.Export(r.Context(), request)

					w.Header().Set("Content-Type", "application/x-protobuf")
				}))
				defer server.Close()

				cfg.HTTPEndpoint = strings.TrimPrefix(server.URL, "http://")
			} else {
				listener, err := net.Listen("tcp", "127.0.0.1:0")
				require.NoError(t, err)

				server := grpc.NewServer()

				collectorpb.RegisterMetricsServiceServer(server, received)
				go func() { _ = server.Serve(listener) }()

				defer server.Stop()

				cfg.GRPCEndpoint = listener.Addr().String()
			}

			for _, native := range []bool{false, true} {
				exporter, enabled, err := newMetricExporter(t.Context(), cfg)
				require.NoError(t, err)
				require.True(t, enabled)

				resource, err := metricsResource(cfg)
				require.NoError(t, err)

				reader := sdkmetric.NewManualReader()
				provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader), sdkmetric.WithResource(resource))
				meter := provider.Meter("parity")
				ctx := context.Background()
				attrs := attribute.NewSet(attribute.String("step", "workload"))

				var data metricdata.ResourceMetrics

				if native {
					registry := newmetricRegistry(cfg.Prefix)
					registry.scope = instrumentation.Scope{Name: "parity"}
					counter, createErr := registry.newInstrument("completed", Counter, "{operation}", "completed", nil)
					require.NoError(t, createErr)
					histogram, createErr := registry.newInstrument("latency", Trend, "ms", "latency", []float64{1, 2, 5})
					require.NoError(t, createErr)

					for _, value := range []float64{0, 1, 1.5, 2, 3, 5, 8} {
						counter.add(ctx, 1, metricAttributesFromSet(attrs))
						histogram.add(ctx, value, metricAttributesFromSet(attrs))
					}

					data.Resource = resource
					data.ScopeMetrics, err = registry.producer(sdkmetric.DefaultTemporalitySelector).Produce(ctx)
					require.NoError(t, err)
				} else {
					counter, createErr := meter.Float64Counter("parity_completed",
						otelmetric.WithUnit("{operation}"), otelmetric.WithDescription("completed"))
					require.NoError(t, createErr)
					histogram, createErr := meter.Float64Histogram("parity_latency",
						otelmetric.WithUnit("ms"), otelmetric.WithDescription("latency"),
						otelmetric.WithExplicitBucketBoundaries(1, 2, 5))
					require.NoError(t, createErr)

					for _, value := range []float64{0, 1, 1.5, 2, 3, 5, 8} {
						counter.Add(ctx, 1, otelmetric.WithAttributeSet(attrs))
						histogram.Record(ctx, value, otelmetric.WithAttributeSet(attrs))
					}

					require.NoError(t, reader.Collect(ctx, &data))
				}

				require.NoError(t, exporter.Export(ctx, &data))
				require.NoError(t, exporter.Shutdown(ctx))
				require.NoError(t, provider.Shutdown(ctx))
			}

			received.mu.Lock()
			defer received.mu.Unlock()

			require.Len(t, received.requests, 2)

			canonical := func(request *collectorpb.ExportMetricsServiceRequest) map[string]*metricspb.Metric {
				out := map[string]*metricspb.Metric{}

				for _, resource := range request.ResourceMetrics {
					for _, scope := range resource.ScopeMetrics {
						require.Equal(t, "parity", scope.Scope.Name)

						for _, metric := range scope.Metrics {
							for _, point := range metric.GetSum().GetDataPoints() {
								require.NotZero(t, point.StartTimeUnixNano)
								require.GreaterOrEqual(t, point.TimeUnixNano, point.StartTimeUnixNano)
								point.StartTimeUnixNano, point.TimeUnixNano = 0, 0
							}

							for _, point := range metric.GetHistogram().GetDataPoints() {
								require.NotZero(t, point.StartTimeUnixNano)
								require.GreaterOrEqual(t, point.TimeUnixNano, point.StartTimeUnixNano)
								point.StartTimeUnixNano, point.TimeUnixNano = 0, 0
							}

							out[metric.Name] = metric
						}
					}
				}

				return out
			}
			actual, expected := canonical(received.requests[1]), canonical(received.requests[0])
			require.Len(t, actual, len(expected))

			for name, metric := range expected {
				require.True(t, proto.Equal(metric, actual[name]), "%s output differs", name)
			}
		})
	}
}
