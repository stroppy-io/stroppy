package bench

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
)

const (
	defaultMetricsPrefix        = "stroppy_"
	defaultMetricExportInterval = 10 * time.Second
	metricCardinalityLimit      = 2000
)

type MetricsConfig struct {
	GRPCEndpoint       string
	HTTPEndpoint       string
	HTTPPath           string
	Headers            string
	Insecure           bool
	Prefix             string
	ServiceVersion     string
	RunID              string
	ResourceAttributes map[string]string
}

func newMeterProvider(
	ctx context.Context,
	config *MetricsConfig,
) (*sdkmetric.MeterProvider, *sdkmetric.ManualReader, string, error) {
	manualReader := sdkmetric.NewManualReader()
	options := []sdkmetric.Option{
		sdkmetric.WithReader(manualReader),
		sdkmetric.WithCardinalityLimit(metricCardinalityLimit),
	}

	res, err := metricsResource(config)
	if err != nil {
		return nil, nil, "", fmt.Errorf("create metrics resource: %w", err)
	}

	options = append(options, sdkmetric.WithResource(res))

	exporter, enabled, err := newMetricExporter(ctx, config)
	if err != nil {
		return nil, nil, "", err
	}

	if enabled {
		options = append(options, sdkmetric.WithReader(sdkmetric.NewPeriodicReader(
			exporter,
			sdkmetric.WithInterval(metricExportInterval()),
		)))
	}

	prefix := config.Prefix
	if prefix == "" {
		prefix = defaultMetricsPrefix
	}

	return sdkmetric.NewMeterProvider(options...), manualReader, prefix, nil
}

func newMetricExporter(ctx context.Context, config *MetricsConfig) (sdkmetric.Exporter, bool, error) {
	headers := parseOTLPHeaders(config.Headers)

	if config.GRPCEndpoint != "" {
		options := []otlpmetricgrpc.Option{otlpmetricgrpc.WithEndpoint(config.GRPCEndpoint)}
		if config.Insecure {
			options = append(options, otlpmetricgrpc.WithInsecure())
		}

		if len(headers) > 0 {
			options = append(options, otlpmetricgrpc.WithHeaders(headers))
		}

		exporter, err := otlpmetricgrpc.New(ctx, options...)
		if err != nil {
			return nil, false, fmt.Errorf("create OTLP gRPC metrics exporter: %w", err)
		}

		return exporter, true, nil
	}

	if config.HTTPEndpoint == "" {
		return nil, false, nil
	}

	options := []otlpmetrichttp.Option{otlpmetrichttp.WithEndpoint(config.HTTPEndpoint)}
	if config.HTTPPath != "" {
		options = append(options, otlpmetrichttp.WithURLPath(config.HTTPPath))
	}

	if config.Insecure {
		options = append(options, otlpmetrichttp.WithInsecure())
	}

	if len(headers) > 0 {
		options = append(options, otlpmetrichttp.WithHeaders(headers))
	}

	exporter, err := otlpmetrichttp.New(ctx, options...)
	if err != nil {
		return nil, false, fmt.Errorf("create OTLP HTTP metrics exporter: %w", err)
	}

	return exporter, true, nil
}

func metricsResource(config *MetricsConfig) (*resource.Resource, error) {
	attrs := []attribute.KeyValue{semconv.ServiceName("stroppy")}
	if config.ServiceVersion != "" {
		attrs = append(attrs, semconv.ServiceVersion(config.ServiceVersion))
	}

	if config.RunID != "" {
		attrs = append(attrs, attribute.String("stroppy.run.id", config.RunID))
	}

	for key, value := range config.ResourceAttributes {
		attrs = append(attrs, attribute.String(key, value))
	}

	return resource.Merge(resource.Default(), resource.NewSchemaless(attrs...))
}

func parseOTLPHeaders(raw string) map[string]string {
	if raw == "" {
		return nil
	}

	headers := make(map[string]string)

	for part := range strings.SplitSeq(raw, ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if ok && key != "" {
			headers[key] = value
		}
	}

	return headers
}

func metricExportInterval() time.Duration {
	raw := os.Getenv("OTEL_METRIC_EXPORT_INTERVAL")
	if raw == "" {
		return defaultMetricExportInterval
	}

	milliseconds, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || milliseconds <= 0 {
		return defaultMetricExportInterval
	}

	return time.Duration(milliseconds) * time.Millisecond
}
