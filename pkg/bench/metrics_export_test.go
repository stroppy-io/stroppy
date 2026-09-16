package bench

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	collectorpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	"google.golang.org/protobuf/proto"
)

func TestOTLPShutdownExportsIdentityAndMetadataForShortRuns(t *testing.T) {
	t.Parallel()

	received := make(chan *collectorpb.ExportMetricsServiceRequest, 4)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/metrics" || r.Header.Get("Authorization") != "Bearer test-token" {
			w.WriteHeader(http.StatusUnauthorized)

			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)

			return
		}

		data := new(collectorpb.ExportMetricsServiceRequest)
		if err := proto.Unmarshal(body, data); err != nil {
			w.WriteHeader(http.StatusBadRequest)

			return
		}

		received <- data

		w.Header().Set("Content-Type", "application/x-protobuf")
	}))
	defer server.Close()

	ids := map[string]bool{}

	for _, segment := range []string{"first", "second"} {
		cfg := &MetricsConfig{
			HTTPEndpoint: strings.TrimPrefix(server.URL, "http://"), Insecure: true,
			Headers: "Authorization=Bearer%20test-token", RunID: "same-run",
			ResourceAttributes: map[string]string{"tenant": "test", "stroppy.segment": segment},
		}
		provider, _, prefix, err := newMeterProvider(context.Background(), cfg)
		require.NoError(t, err)

		registry := NewRegistry(provider.Meter("test"), prefix)
		counter, err := registry.NewMetric("iterations_total", Counter)
		require.NoError(t, err)
		trend, err := registry.NewMetric("query_duration", Trend)
		require.NoError(t, err)
		counter.add(context.Background(), 3, attributes("step", "workload"))
		trend.add(context.Background(), 2, attributes("step", "workload"))
		// Shorter than the periodic interval: Shutdown must flush the real OTLP exporter.
		require.NoError(t, provider.Shutdown(context.Background()))

		data := <-received
		require.Len(t, data.ResourceMetrics, 1)
		resource := data.ResourceMetrics[0]

		var instance string

		for _, attr := range resource.Resource.Attributes {
			if attr.Key == "service.instance.id" {
				instance = attr.Value.GetStringValue()
			}
		}

		require.NotEmpty(t, instance)
		require.False(t, ids[instance], "separate invocations reused a writer identity")
		ids[instance] = true

		for _, scope := range resource.ScopeMetrics {
			for _, metric := range scope.Metrics {
				attrs := map[string]string{}

				if sum := metric.GetSum(); sum != nil {
					require.Len(t, sum.DataPoints, 1)
					require.InDelta(t, 3.0, sum.DataPoints[0].GetAsDouble(), 0)

					for _, attr := range sum.DataPoints[0].Attributes {
						attrs[attr.Key] = attr.Value.GetStringValue()
					}
				} else {
					histogram := metric.GetHistogram()
					require.NotNil(t, histogram)
					require.Len(t, histogram.DataPoints, 1)

					for _, attr := range histogram.DataPoints[0].Attributes {
						attrs[attr.Key] = attr.Value.GetStringValue()
					}
				}

				require.Equal(t, map[string]string{
					"tenant": "test", "stroppy.segment": segment, "stroppy.run.id": "same-run", "step": "workload",
				}, attrs)
			}
		}
	}
}

func TestMetricMetadataPreservesPointDimensions(t *testing.T) {
	t.Parallel()

	got := mergeMetricAttributes(
		attribute.NewSet(attribute.String("step", "workload")),
		[]attribute.KeyValue{attribute.String("step", "metadata"), attribute.String("tenant", "test")},
	)
	value, _ := got.Value("step")
	require.Equal(t, "workload", value.AsString())
}
