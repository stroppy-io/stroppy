package bench

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// metadataExporter keeps explicit run metadata on metric points as well as the
// resource. Prometheus remote-write otherwise retains it only in target_info.
// Enrichment happens at export time, never on the benchmark's hot path.
type metadataExporter struct {
	sdkmetric.Exporter
	attributes []attribute.KeyValue
}

func exportedAttributes(config *MetricsConfig) []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, len(config.ResourceAttributes)+1)
	for key, value := range config.ResourceAttributes {
		attrs = append(attrs, attribute.String(key, value))
	}

	if config.RunID != "" {
		attrs = append(attrs, attribute.String("stroppy.run.id", config.RunID))
	}

	set := attribute.NewSet(attrs...)

	return set.ToSlice()
}

func (e *metadataExporter) Export(ctx context.Context, data *metricdata.ResourceMetrics) error {
	for i := range data.ScopeMetrics {
		for j := range data.ScopeMetrics[i].Metrics {
			m := &data.ScopeMetrics[i].Metrics[j]
			switch values := m.Data.(type) {
			case metricdata.Sum[float64]:
				enrichPoints(values.DataPoints, e.attributes)
			case metricdata.Sum[int64]:
				enrichPoints(values.DataPoints, e.attributes)
			case metricdata.Gauge[float64]:
				enrichPoints(values.DataPoints, e.attributes)
			case metricdata.Gauge[int64]:
				enrichPoints(values.DataPoints, e.attributes)
			case metricdata.Histogram[float64]:
				for k := range values.DataPoints {
					values.DataPoints[k].Attributes = mergeMetricAttributes(values.DataPoints[k].Attributes, e.attributes)
				}
			case metricdata.Histogram[int64]:
				for k := range values.DataPoints {
					values.DataPoints[k].Attributes = mergeMetricAttributes(values.DataPoints[k].Attributes, e.attributes)
				}
			}
		}
	}

	return e.Exporter.Export(ctx, data)
}

func enrichPoints[N int64 | float64](points []metricdata.DataPoint[N], attrs []attribute.KeyValue) {
	for i := range points {
		points[i].Attributes = mergeMetricAttributes(points[i].Attributes, attrs)
	}
}

func mergeMetricAttributes(point attribute.Set, metadata []attribute.KeyValue) attribute.Set {
	// Explicit point dimensions (step, table, transaction) take precedence.
	attrs := append([]attribute.KeyValue(nil), metadata...)
	attrs = append(attrs, point.ToSlice()...)

	return attribute.NewSet(attrs...)
}
