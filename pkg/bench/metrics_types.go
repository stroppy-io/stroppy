package bench

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/sdk/instrumentation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/exemplar"
)

const (
	tagPairSize       = 2
	maxCachedTagParts = 16
)

var (
	errMetricAlreadyRegistered = inputError("metric already registered")
	errUnknownMetricType       = inputError("unknown metric type")
)

type metricType int

const (
	Counter metricType = iota
	Trend
	Rate
	Gauge
)

var (
	durationMillisecondsBounds = []float64{
		0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 25, 50, 100,
		250, 500, 1000, 2500, 5000, 10000, 20000, 30000, 60000,
	}
	countBounds = []float64{0, 1, 2, 5, 10, 20, 50, 100, 250, 500, 1000}
)

type metricAttributes struct {
	set    *attribute.Set
	add    []otelmetric.AddOption
	record []otelmetric.RecordOption
}

type metricTagKey struct {
	count uint8
	parts [maxCachedTagParts]string
}

type metric struct {
	Name           string
	Type           metricType
	unit           string
	description    string
	bounds         []float64
	started        time.Time
	deltaReaders   int
	exemplarFilter exemplar.Filter
	emptyAttrs     metricAttributes
	overflow       metricAttributes
	tagAttrs       sync.Map
	tagCount       atomic.Int64
	seriesMu       sync.Mutex
	series         map[attribute.Distinct]*metricSeries
	writers        [metricWriterLimit + 1]atomic.Pointer[metricWriterBinding]
	bindings       sync.Map
}

func (m *metric) add(ctx context.Context, value float64, attrs metricAttributes) {
	writer, _ := ctx.Value(metricWriterContextKey{}).(*metricWriter)
	m.addWriter(ctx, value, attrs, writer)
}

func (m *metric) addWriter(ctx context.Context, value float64, attrs metricAttributes, writer *metricWriter) {
	if attrs.set == nil {
		attrs = m.emptyAttrs
	}

	id := metricWriterLimit
	if writer != nil && m.Type != Gauge {
		id = writer.id
	}

	binding := m.writers[id].Load()
	if binding == nil || binding.original != attrs.set {
		binding = m.bindSeries(id, attrs)
	}

	binding.shard.record(ctx, value, m.Type, m.bounds, m.exemplarFilter)
}

type metricRegistry struct {
	mu           sync.Mutex
	meter        otelmetric.Meter
	prefix       string
	metrics      map[string]*metric
	started      time.Time
	scope        instrumentation.Scope
	deltaReaders int
}

func newmetricRegistry(prefix string) *metricRegistry {
	return &metricRegistry{prefix: prefix, metrics: map[string]*metric{}, started: time.Now()}
}

func (r *metricRegistry) NewMetric(name string, typ metricType) (*metric, error) {
	return r.newInstrument(name, typ, "", "", nil)
}

func (r *metricRegistry) newInstrument(
	name string,
	typ metricType,
	unit, description string,
	bounds []float64,
) (*metric, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.metrics[name]; ok {
		return nil, fmt.Errorf("%w: %q", errMetricAlreadyRegistered, name)
	}

	if typ < Counter || typ > Gauge {
		return nil, fmt.Errorf("%w: %d", errUnknownMetricType, typ)
	}

	if typ == Trend {
		if bounds == nil {
			bounds = histogramBounds(name)
		} else if len(bounds) == 0 {
			aggregation := sdkmetric.DefaultAggregationSelector(sdkmetric.InstrumentKindHistogram)
			defaults, _ := aggregation.(sdkmetric.AggregationExplicitBucketHistogram)
			bounds = defaults.Boundaries
		}

		bounds = slices.Clone(bounds)

		if unit == "" && strings.HasSuffix(name, "_duration") {
			unit = "ms"
		}
	}

	if typ == Rate {
		unit, description = "", ""
	}

	m := &metric{
		Name: name, Type: typ, unit: unit, description: description,
		bounds: bounds, started: r.started, deltaReaders: r.deltaReaders, exemplarFilter: metricExemplarFilter(),
		emptyAttrs: attributes(),
		overflow:   metricAttributesFromSet(attribute.NewSet(attribute.Bool("otel.metric.overflow", true))),
	}

	r.metrics[name] = m

	return m, nil
}

func (m *metric) taggedAttributes(tags []string) metricAttributes {
	if len(tags) == 0 {
		return m.emptyAttrs
	}

	if len(tags)%tagPairSize != 0 {
		tags = tags[:len(tags)-1]
	}

	if len(tags) == 0 {
		return m.emptyAttrs
	}

	if len(tags) > maxCachedTagParts {
		return m.overflow
	}

	var key metricTagKey

	key.count = uint8(len(tags)) //nolint:gosec // bounded by maxCachedTagParts
	copy(key.parts[:], tags)

	if cached, ok := m.tagAttrs.Load(key); ok {
		if attrs, valid := cached.(metricAttributes); valid {
			return attrs
		}
	}

	if m.tagCount.Add(1) > metricCardinalityLimit {
		m.tagCount.Add(-1)

		return m.overflow
	}

	attrs := attributes(tags...)

	cached, loaded := m.tagAttrs.LoadOrStore(key, attrs)
	if loaded {
		m.tagCount.Add(-1)
	}

	if result, valid := cached.(metricAttributes); valid {
		return result
	}

	return m.overflow
}

func histogramBounds(name string) []float64 {
	if strings.HasSuffix(name, "_duration") {
		return durationMillisecondsBounds
	}

	return countBounds
}

func attributes(tags ...string) metricAttributes {
	var set attribute.Set

	if len(tags) >= tagPairSize {
		pairs := make([]attribute.KeyValue, 0, len(tags)/tagPairSize)
		for i := 0; i+1 < len(tags); i += tagPairSize {
			pairs = append(pairs, attribute.String(tags[i], tags[i+1]))
		}

		set = attribute.NewSet(pairs...)
	} else {
		set = attribute.NewSet()
	}

	return metricAttributesFromSet(set)
}

func metricAttributesFromSet(set attribute.Set) metricAttributes {
	return metricAttributes{
		set:    &set,
		add:    []otelmetric.AddOption{otelmetric.WithAttributeSet(set)},
		record: []otelmetric.RecordOption{otelmetric.WithAttributeSet(set)},
	}
}
