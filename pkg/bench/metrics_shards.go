package bench

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const (
	metricWriterKey   = "stroppy.internal.metric_writer"
	metricWriterLimit = 128
)

type metricWriterContextKey struct{}

// metricWriter isolates frequently updated SDK series without exporting worker labels.
type metricWriter struct {
	id int
}

type metricWriterSeries struct {
	writer     int
	attributes attribute.Distinct
}

type metricWriterBinding struct {
	original   *attribute.Set
	shard      *metricShard
	attributes metricAttributes
}

func (m *metric) bindSeries(id int, attrs metricAttributes) *metricWriterBinding {
	key := metricWriterSeries{id, attrs.set.Equivalent()}
	if cached, ok := m.bindings.Load(key); ok {
		binding, _ := cached.(*metricWriterBinding)
		m.writers[id].Store(binding)

		return binding
	}

	m.seriesMu.Lock()
	defer m.seriesMu.Unlock()

	if m.series == nil {
		m.series = map[attribute.Distinct]*metricSeries{}
	}

	set := *attrs.set
	seriesKey := set.Equivalent()

	series := m.series[seriesKey]
	if series == nil {
		if len(m.series) >= metricCardinalityLimit-1 {
			set = *m.overflow.set
			seriesKey = set.Equivalent()
			series = m.series[seriesKey]
		}

		if series == nil {
			series = &metricSeries{attrs: set, shards: map[int]*metricShard{}}
			m.series[seriesKey] = series
		}
	}

	shard := series.shards[id]
	if shard == nil {
		shard = &metricShard{deltas: make([]metricValues, m.deltaReaders)}
		if m.Type == Trend {
			shard.value.buckets = make([]uint64, len(m.bounds)+1)
			for i := range shard.deltas {
				shard.deltas[i].buckets = make([]uint64, len(m.bounds)+1)
			}
		}

		series.shards[id] = shard
	}

	key.attributes = seriesKey
	if cached, ok := m.bindings.Load(key); ok {
		binding, _ := cached.(*metricWriterBinding)
		m.writers[id].Store(binding)

		return binding
	}

	binding := &metricWriterBinding{original: attrs.set, shard: shard}
	if m.sdkHistogram != nil {
		binding.attributes = metricAttributesFromSet(set)
	}

	stored, _ := m.bindings.LoadOrStore(key, binding)
	binding, _ = stored.(*metricWriterBinding)
	m.writers[id].Store(binding)

	return binding
}

func metricWorkerContext(ctx context.Context, worker, workers int) context.Context {
	// Keep trace lookup above the shared cancellation context; sampled spans remain visible.
	ctx = trace.ContextWithSpan(ctx, trace.SpanFromContext(ctx))

	id := metricWriterLimit
	if workers > 1 {
		id = worker % metricWriterLimit
	}

	return context.WithValue(ctx, metricWriterContextKey{}, &metricWriter{id: id})
}
