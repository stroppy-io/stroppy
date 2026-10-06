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
	original *attribute.Set
	bound    metricAttributes
}

func (m *metric) bindSeries(id int, attrs metricAttributes) metricAttributes {
	key := metricWriterSeries{id, attrs.set.Equivalent()}
	if cached, exists := m.bindings.Load(key); exists {
		binding, _ := cached.(*metricWriterBinding)
		m.writers[id].Store(binding)

		return binding.bound
	}

	set := m.admitSeries(*attrs.set)
	key.attributes = set.Equivalent()

	var binding *metricWriterBinding
	if cached, exists := m.bindings.Load(key); exists {
		binding, _ = cached.(*metricWriterBinding)
	} else {
		if id < metricWriterLimit {
			values := append(set.ToSlice(), attribute.Int(metricWriterKey, id))
			set = attribute.NewSet(values...)
		}

		binding = &metricWriterBinding{attrs.set, metricAttributesFromSet(set)}
		stored, _ := m.bindings.LoadOrStore(key, binding)
		binding, _ = stored.(*metricWriterBinding)
	}

	m.writers[id].Store(binding)

	return binding.bound
}

func (m *metric) admitSeries(set attribute.Set) attribute.Set {
	m.seriesMu.Lock()
	defer m.seriesMu.Unlock()

	if m.series == nil {
		m.series = map[attribute.Distinct]struct{}{}
	}

	key := set.Equivalent()
	if _, exists := m.series[key]; exists {
		return set
	}

	if len(m.series) >= metricCardinalityLimit-1 {
		return attribute.NewSet(attribute.Bool("otel.metric.overflow", true))
	}

	m.series[key] = struct{}{}

	return set
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
