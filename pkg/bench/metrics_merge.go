package bench

import (
	"runtime"
	"slices"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func mergeSumExemplars(current, incoming []metricdata.Exemplar[float64]) []metricdata.Exemplar[float64] {
	current = append(current, incoming...)

	limit := runtime.GOMAXPROCS(0)
	if len(current) > limit {
		slices.SortFunc(current, func(a, b metricdata.Exemplar[float64]) int { return b.Time.Compare(a.Time) })
		current = current[:limit]
	}

	return current
}

func mergeHistogramExemplars(
	current, incoming []metricdata.Exemplar[float64], bounds []float64,
) []metricdata.Exemplar[float64] {
	if len(bounds) == 0 {
		return mergeSumExemplars(current, incoming)
	}

	for _, sample := range incoming {
		bucket, _ := slices.BinarySearch(bounds, sample.Value)

		index := slices.IndexFunc(current, func(existing metricdata.Exemplar[float64]) bool {
			other, _ := slices.BinarySearch(bounds, existing.Value)

			return other == bucket
		})
		if index < 0 {
			current = append(current, sample)
		} else if sample.Time.After(current[index].Time) {
			current[index] = sample
		}
	}

	return current
}

// mergeMetricWriters removes private aggregation dimensions before observation/export.
func mergeMetricWriters(data *metricdata.ResourceMetrics) {
	for i := range data.ScopeMetrics {
		for j := range data.ScopeMetrics[i].Metrics {
			metric := &data.ScopeMetrics[i].Metrics[j]
			switch aggregation := metric.Data.(type) {
			case metricdata.Sum[float64]:
				aggregation.DataPoints = mergeSumPoints(aggregation.DataPoints)
				metric.Data = aggregation
			case metricdata.Histogram[float64]:
				aggregation.DataPoints = mergeHistogramPoints(aggregation.DataPoints)
				metric.Data = aggregation
			}
		}
	}
}

func publicMetricAttributes(set attribute.Set) attribute.Set {
	if _, exists := set.Value(attribute.Key(metricWriterKey)); !exists {
		return set
	}

	values := set.ToSlice()
	values = slices.DeleteFunc(values, func(value attribute.KeyValue) bool {
		return value.Key == attribute.Key(metricWriterKey)
	})

	return attribute.NewSet(values...)
}

func mergeSumPoints(points []metricdata.DataPoint[float64]) []metricdata.DataPoint[float64] {
	if !slices.ContainsFunc(points, func(point metricdata.DataPoint[float64]) bool {
		return point.Attributes.HasValue(attribute.Key(metricWriterKey))
	}) {
		return points
	}

	indexes := map[attribute.Distinct]int{}

	out := make([]metricdata.DataPoint[float64], 0, len(points))
	for _, point := range points {
		point.Attributes = publicMetricAttributes(point.Attributes)

		key := point.Attributes.Equivalent()
		if index, exists := indexes[key]; exists {
			target := &out[index]

			target.Value += point.Value
			if point.StartTime.Before(target.StartTime) {
				target.StartTime = point.StartTime
			}

			if point.Time.After(target.Time) {
				target.Time = point.Time
			}

			target.Exemplars = mergeSumExemplars(target.Exemplars, point.Exemplars)
		} else {
			indexes[key] = len(out)
			point.Exemplars = slices.Clone(point.Exemplars)
			out = append(out, point)
		}
	}

	return out
}

func mergeHistogramPoint(target, point *metricdata.HistogramDataPoint[float64]) {
	target.Count += point.Count

	target.Sum += point.Sum
	for bucket, count := range point.BucketCounts {
		target.BucketCounts[bucket] += count
	}

	if value, defined := point.Min.Value(); defined {
		if current, set := target.Min.Value(); !set || value < current {
			target.Min = metricdata.NewExtrema(value)
		}
	}

	if value, defined := point.Max.Value(); defined {
		if current, set := target.Max.Value(); !set || value > current {
			target.Max = metricdata.NewExtrema(value)
		}
	}

	if point.StartTime.Before(target.StartTime) {
		target.StartTime = point.StartTime
	}

	if point.Time.After(target.Time) {
		target.Time = point.Time
	}

	target.Exemplars = mergeHistogramExemplars(target.Exemplars, point.Exemplars, target.Bounds)
}

func mergeHistogramPoints(points []metricdata.HistogramDataPoint[float64]) []metricdata.HistogramDataPoint[float64] {
	if !slices.ContainsFunc(points, func(point metricdata.HistogramDataPoint[float64]) bool {
		return point.Attributes.HasValue(attribute.Key(metricWriterKey))
	}) {
		return points
	}

	indexes := map[attribute.Distinct]int{}

	out := make([]metricdata.HistogramDataPoint[float64], 0, len(points))
	for _, point := range points {
		point.Attributes = publicMetricAttributes(point.Attributes)

		key := point.Attributes.Equivalent()
		if index, exists := indexes[key]; exists {
			mergeHistogramPoint(&out[index], &point)
		} else {
			indexes[key] = len(out)
			point.Exemplars = slices.Clone(point.Exemplars)
			point.BucketCounts = slices.Clone(point.BucketCounts)
			out = append(out, point)
		}
	}

	return out
}
