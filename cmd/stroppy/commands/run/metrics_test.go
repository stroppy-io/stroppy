package run

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stroppy-io/stroppy/pkg/config"
)

func TestMetricsConfig(t *testing.T) {
	t.Parallel()

	grpcEndpoint := "collector:4317"
	httpEndpoint := "collector:4318"
	httpPath := "/custom/metrics"
	headers := "Authorization=secret"
	insecure := true
	prefix := "bench_"

	config := &config.RunConfig{Global: &config.GlobalConfig{
		RunId:    "run-42",
		Metadata: map[string]string{"environment": "test"},
		Exporter: &config.ExporterConfig{OtlpExport: &config.OtlpExport{
			OtlpGrpcEndpoint:        &grpcEndpoint,
			OtlpHttpEndpoint:        &httpEndpoint,
			OtlpHttpExporterUrlPath: &httpPath,
			OtlpHeaders:             &headers,
			OtlpEndpointInsecure:    &insecure,
			OtlpMetricsPrefix:       &prefix,
		}},
	}}

	got := metricsConfig(config)
	require.Equal(t, grpcEndpoint, got.GRPCEndpoint)
	require.Equal(t, httpEndpoint, got.HTTPEndpoint)
	require.Equal(t, httpPath, got.HTTPPath)
	require.Equal(t, headers, got.Headers)
	require.True(t, got.Insecure)
	require.Equal(t, prefix, got.Prefix)
	require.Equal(t, "run-42", got.RunID)
	require.Equal(t, map[string]string{"environment": "test"}, got.ResourceAttributes)
}

func TestMetricsConfigWithGlobalWithoutExporter(t *testing.T) {
	t.Parallel()

	got := metricsConfig(&config.RunConfig{Global: &config.GlobalConfig{RunId: "run-42"}})
	require.Equal(t, "run-42", got.RunID)
	require.Empty(t, got.GRPCEndpoint)
	require.Empty(t, got.HTTPEndpoint)
}

func TestMetricsConfigWithoutFile(t *testing.T) {
	t.Parallel()

	got := metricsConfig(nil)
	require.Empty(t, got.GRPCEndpoint)
	require.Empty(t, got.HTTPEndpoint)
}
