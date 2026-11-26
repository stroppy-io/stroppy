package bench

import (
	"context"
	"os"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/exemplar"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func metricExemplarFilter() exemplar.Filter {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("OTEL_METRICS_EXEMPLAR_FILTER"))) {
	case "always_on":
		return exemplar.AlwaysOnFilter
	case "always_off":
		return exemplar.AlwaysOffFilter
	default:
		return exemplar.TraceBasedFilter
	}
}

type metricSeries struct {
	attrs  attribute.Set
	shards map[int]*metricShard
}

type metricValues struct {
	count                    uint64
	total                    float64
	truth                    uint64
	min                      float64
	max                      float64
	buckets                  []uint64
	reservoir, trueReservoir exemplar.Reservoir
	exemplarSince            time.Time
}

// A shard lock keeps histogram fields consistent with concurrent collection.
// Padding prevents independent writers from sharing a cache line.
type metricShard struct {
	mu     sync.Mutex
	value  metricValues
	deltas []metricValues
	_      [128]byte
}

func (s *metricShard) record(
	ctx context.Context, value float64, typ metricType, bounds []float64, filter exemplar.Filter,
) {
	bucket := 0
	if typ == Trend {
		bucket = sort.SearchFloat64s(bounds, value)
	}

	sampled := filter(ctx)

	s.mu.Lock()
	s.value.record(value, typ, bucket)

	for i := range s.deltas {
		s.deltas[i].record(value, typ, bucket)
	}

	if sampled {
		s.value.offer(ctx, value, typ, bounds)

		for i := range s.deltas {
			s.deltas[i].offer(ctx, value, typ, bounds)
		}
	}
	s.mu.Unlock()
}

func (v *metricValues) offer(ctx context.Context, value float64, typ metricType, bounds []float64) {
	if v.reservoir == nil {
		if typ == Trend {
			v.reservoir = exemplar.NewHistogramReservoir(bounds)
		} else {
			v.reservoir = exemplar.NewFixedSizeReservoir(runtime.GOMAXPROCS(0))
		}
	}

	if typ == Rate {
		v.reservoir.Offer(ctx, time.Now(), exemplar.NewValue(float64(1)), nil)

		if value != 0 {
			if v.trueReservoir == nil {
				v.trueReservoir = exemplar.NewFixedSizeReservoir(runtime.GOMAXPROCS(0))
			}

			v.trueReservoir.Offer(ctx, time.Now(), exemplar.NewValue(float64(1)), nil)
		}

		return
	}

	v.reservoir.Offer(ctx, time.Now(), exemplar.NewValue(value), nil)
}

func (v *metricValues) record(value float64, typ metricType, bucket int) {
	switch typ {
	case Counter:
		v.total += value
	case Trend:
		v.total += value

		v.buckets[bucket]++
		if v.count == 0 || value < v.min {
			v.min = value
		}

		if v.count == 0 || value > v.max {
			v.max = value
		}
	case Rate:
		if value != 0 {
			v.truth++
		}
	case Gauge:
		v.total = value
	}

	v.count++
}

func collectNativeExemplars(reservoir exemplar.Reservoir, since time.Time) []metricdata.Exemplar[float64] {
	if reservoir == nil {
		return nil
	}

	var samples []exemplar.Exemplar
	reservoir.Collect(&samples)

	out := make([]metricdata.Exemplar[float64], 0, len(samples))
	for _, sample := range samples {
		if sample.Time.Before(since) {
			continue
		}

		out = append(out, metricdata.Exemplar[float64]{
			Value: sample.Value.Float64(), Time: sample.Time,
			TraceID: slices.Clone(sample.TraceID), SpanID: slices.Clone(sample.SpanID),
			FilteredAttributes: slices.Clone(sample.FilteredAttributes),
		})
	}

	return out
}

type collectedMetric struct {
	metricValues
	samples, trueSamples []metricdata.Exemplar[float64]
}

func (s *metricShard) snapshot(delta int) collectedMetric {
	s.mu.Lock()
	defer s.mu.Unlock()

	value := &s.value
	if delta >= 0 {
		value = &s.deltas[delta]
	}

	out := collectedMetric{
		metricValues: *value,
		samples:      collectNativeExemplars(value.reservoir, value.exemplarSince),
		trueSamples:  collectNativeExemplars(value.trueReservoir, value.exemplarSince),
	}

	out.buckets = slices.Clone(value.buckets)
	if delta >= 0 {
		clear(value.buckets)
		*value = metricValues{
			buckets: value.buckets, reservoir: value.reservoir, trueReservoir: value.trueReservoir,
			exemplarSince: time.Now(),
		}
	}

	return out
}

type metricProducer struct {
	registry *metricRegistry
	selector sdkmetric.TemporalitySelector
	delta    int
	mu       sync.Mutex
	started  time.Time
}

func (r *metricRegistry) producer(selector sdkmetric.TemporalitySelector) *metricProducer {
	r.mu.Lock()
	defer r.mu.Unlock()

	p := &metricProducer{registry: r, selector: selector, delta: -1, started: r.started}
	if selector(sdkmetric.InstrumentKindCounter) == metricdata.DeltaTemporality ||
		selector(sdkmetric.InstrumentKindHistogram) == metricdata.DeltaTemporality ||
		selector(sdkmetric.InstrumentKindGauge) == metricdata.DeltaTemporality {
		p.delta = r.deltaReaders

		r.deltaReaders++
		for _, metric := range r.metrics {
			metric.seriesMu.Lock()

			metric.deltaReaders = r.deltaReaders
			for _, series := range metric.series {
				for _, shard := range series.shards {
					shard.mu.Lock()
					shard.deltas = append(shard.deltas, metricValues{buckets: make([]uint64, len(shard.value.buckets))})
					shard.mu.Unlock()
				}
			}
			metric.seriesMu.Unlock()
		}
	}

	return p
}

func (p *metricProducer) Produce(ctx context.Context) ([]metricdata.ScopeMetrics, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	r := p.registry
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()

	out := metricdata.ScopeMetrics{Scope: r.scope}
	for _, metric := range r.metrics {
		kind := sdkmetric.InstrumentKindCounter

		switch metric.Type {
		case Counter, Rate:
			kind = sdkmetric.InstrumentKindCounter
		case Trend:
			kind = sdkmetric.InstrumentKindHistogram
		case Gauge:
			kind = sdkmetric.InstrumentKindGauge
		}

		temporality := p.selector(kind)
		delta := -1

		started := metric.started
		if temporality == metricdata.DeltaTemporality {
			delta, started = p.delta, p.started
		}

		out.Metrics = append(out.Metrics, metric.collect(r.prefix, started, now, temporality, delta)...)
	}

	p.started = now

	return []metricdata.ScopeMetrics{out}, nil
}

func (m *metric) collectSeries(series *metricSeries, delta int) collectedMetric {
	var total collectedMetric

	for _, shard := range series.shards {
		value := shard.snapshot(delta)
		if value.count == 0 {
			continue
		}

		if total.count == 0 {
			total.min, total.max = value.min, value.max
			total.buckets = make([]uint64, len(value.buckets))
		}

		total.count += value.count
		total.total += value.total
		total.truth += value.truth
		total.min = min(total.min, value.min)

		total.max = max(total.max, value.max)
		for i, count := range value.buckets {
			total.buckets[i] += count
		}

		if m.Type == Trend {
			total.samples = mergeHistogramExemplars(total.samples, value.samples, m.bounds)
		} else {
			total.samples = mergeSumExemplars(total.samples, value.samples)
			total.trueSamples = mergeSumExemplars(total.trueSamples, value.trueSamples)
		}
	}

	return total
}

func (m *metric) collect(
	prefix string, started, now time.Time, temporality metricdata.Temporality, delta int,
) []metricdata.Metrics {
	m.seriesMu.Lock()
	defer m.seriesMu.Unlock()

	var (
		points, truths []metricdata.DataPoint[float64]
		histograms     []metricdata.HistogramDataPoint[float64]
	)

	for _, series := range m.series {
		total := m.collectSeries(series, delta)
		if total.count == 0 {
			continue
		}

		if m.Type == Trend {
			histograms = append(histograms, metricdata.HistogramDataPoint[float64]{
				Attributes: series.attrs, StartTime: started, Time: now,
				Count: total.count, Sum: total.total, Bounds: slices.Clone(m.bounds), BucketCounts: total.buckets,
				Min: metricdata.NewExtrema(total.min), Max: metricdata.NewExtrema(total.max), Exemplars: total.samples,
			})

			continue
		}

		point := metricdata.DataPoint[float64]{
			Attributes: series.attrs, StartTime: started, Time: now, Value: total.total, Exemplars: total.samples,
		}
		if m.Type == Rate {
			point.Value = float64(total.count)
			if total.truth > 0 {
				truth := point
				truth.Value, truth.Exemplars = float64(total.truth), total.trueSamples
				truths = append(truths, truth)
			}
		}

		points = append(points, point)
	}

	return m.collectedMetrics(prefix, temporality, points, truths, histograms)
}

func (m *metric) collectedMetrics(
	prefix string, temporality metricdata.Temporality,
	points, truths []metricdata.DataPoint[float64], histograms []metricdata.HistogramDataPoint[float64],
) []metricdata.Metrics {
	metric := metricdata.Metrics{Name: prefix + m.Name, Unit: m.unit, Description: m.description}
	switch m.Type {
	case Trend:
		if len(histograms) == 0 {
			return nil
		}

		metric.Data = metricdata.Histogram[float64]{Temporality: temporality, DataPoints: histograms}
	case Gauge:
		if len(points) == 0 {
			return nil
		}

		metric.Data = metricdata.Gauge[float64]{DataPoints: points}
	case Counter, Rate:
		if len(points) == 0 {
			return nil
		}

		metric.Data = metricdata.Sum[float64]{Temporality: temporality, IsMonotonic: true, DataPoints: points}
		if m.Type == Rate {
			metric.Name += "_events_total"

			out := []metricdata.Metrics{metric}
			if len(truths) > 0 {
				out = append(out, metricdata.Metrics{
					Name: prefix + m.Name + "_true_total",
					Data: metricdata.Sum[float64]{Temporality: temporality, IsMonotonic: true, DataPoints: truths},
				})
			}

			return out
		}
	}

	return []metricdata.Metrics{metric}
}

var _ sdkmetric.Producer = (*metricProducer)(nil)
