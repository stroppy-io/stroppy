package bench

import (
	"context"
	"strings"

	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// MetricSnapshot is a read-only copy of one metric's accumulated values at a
// run. Counter and Gauge metrics report Total; Trend histograms report Count,
// Sum, and the bucket layout (Bounds/BucketCounts) so callers can derive
// quantiles without retaining every raw observation.
type MetricSnapshot struct {
	Total   float64
	Count   uint64
	Sum     float64
	Bounds  []float64
	Buckets []uint64
}

// CollectedMetrics returns every collected metric keyed by its prefix-stripped
// name. Workloads call it from Teardown to post-process their own
// per-transaction counters and trends into a final report.
func (b *Bench) CollectedMetrics() (map[string]MetricSnapshot, error) {
	var data metricdata.ResourceMetrics

	if err := b.root.manualReader.Collect(context.Background(), &data); err != nil {
		return nil, err
	}

	out := make(map[string]MetricSnapshot)

	for _, scope := range data.ScopeMetrics {
		for _, metric := range scope.Metrics {
			snapshot, ok := snapshotMetric(metric)
			if !ok {
				continue
			}

			out[strings.TrimPrefix(metric.Name, b.root.metricsPrefix)] = snapshot
		}
	}

	return out, nil
}

// snapshotMetric converts one collected metric into a MetricSnapshot.
func snapshotMetric(metric metricdata.Metrics) (MetricSnapshot, bool) {
	switch aggregation := metric.Data.(type) {
	case metricdata.Sum[float64]:
		return MetricSnapshot{Total: sumPoints(aggregation.DataPoints)}, true
	case metricdata.Gauge[float64]:
		return MetricSnapshot{Total: sumPoints(aggregation.DataPoints)}, true
	case metricdata.Histogram[float64]:
		return histogramSnapshot(aggregation.DataPoints), true
	default:
		return MetricSnapshot{}, false
	}
}

// sumPoints totals the data points of a Counter or Gauge metric.
func sumPoints(points []metricdata.DataPoint[float64]) float64 {
	var total float64

	for _, point := range points {
		total += point.Value
	}

	return total
}

// histogramSnapshot aggregates a Trend metric's data points into one bucket
// layout, summing counts and reusing the first point's bounds.
func histogramSnapshot(points []metricdata.HistogramDataPoint[float64]) MetricSnapshot {
	snapshot := MetricSnapshot{}

	for _, point := range points {
		snapshot.Count += point.Count
		snapshot.Sum += point.Sum

		if len(snapshot.Bounds) == 0 {
			snapshot.Bounds = append([]float64(nil), point.Bounds...)
			snapshot.Buckets = make([]uint64, len(point.BucketCounts))
		}

		for i, bucketCount := range point.BucketCounts {
			snapshot.Buckets[i] += bucketCount
		}
	}

	return snapshot
}
