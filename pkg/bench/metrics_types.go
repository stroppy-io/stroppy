package bench

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"
)

const tagPairSize = 2

var (
	errMetricAlreadyRegistered = errors.New("metric already registered")
	errUnknownMetricType       = errors.New("unknown metric type")
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
		0.1, 0.25, 0.5, 1, 2.5, 5, 10, 25, 50, 100,
		250, 500, 1000, 2500, 5000, 10000, 30000, 60000,
	}
	countBounds = []float64{0, 1, 2, 5, 10, 20, 50, 100, 250, 500, 1000}
)

type metricAttributes struct {
	set    attribute.Set
	add    []otelmetric.AddOption
	record []otelmetric.RecordOption
}

type metric struct {
	Name      string
	Type      metricType
	counter   otelmetric.Float64Counter
	histogram otelmetric.Float64Histogram
	gauge     otelmetric.Float64Gauge
	rateTrue  otelmetric.Float64Counter
	rateTotal otelmetric.Float64Counter
}

func (m *metric) add(ctx context.Context, value float64, attrs metricAttributes) {
	switch m.Type {
	case Counter:
		m.counter.Add(ctx, value, attrs.add...)
	case Trend:
		m.histogram.Record(ctx, value, attrs.record...)
	case Rate:
		m.rateTotal.Add(ctx, 1, attrs.add...)

		if value != 0 {
			m.rateTrue.Add(ctx, 1, attrs.add...)
		}
	case Gauge:
		m.gauge.Record(ctx, value, attrs.record...)
	}
}

type Registry struct {
	mu      sync.Mutex
	meter   otelmetric.Meter
	prefix  string
	metrics map[string]*metric
}

func NewRegistry(meter otelmetric.Meter, prefix string) *Registry {
	return &Registry{meter: meter, prefix: prefix, metrics: map[string]*metric{}}
}

func (r *Registry) NewMetric(name string, typ metricType) (*metric, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.metrics[name]; ok {
		return nil, fmt.Errorf("%w: %q", errMetricAlreadyRegistered, name)
	}

	m := &metric{Name: name, Type: typ}
	exportedName := r.prefix + name

	var err error

	switch typ {
	case Counter:
		m.counter, err = r.meter.Float64Counter(exportedName)
	case Trend:
		options := []otelmetric.Float64HistogramOption{
			otelmetric.WithExplicitBucketBoundaries(histogramBounds(name)...),
		}
		if strings.HasSuffix(name, "_duration") {
			options = append(options, otelmetric.WithUnit("ms"))
		}

		m.histogram, err = r.meter.Float64Histogram(exportedName, options...)
	case Rate:
		m.rateTotal, err = r.meter.Float64Counter(exportedName + "_events_total")
		if err == nil {
			m.rateTrue, err = r.meter.Float64Counter(exportedName + "_true_total")
		}
	case Gauge:
		m.gauge, err = r.meter.Float64Gauge(exportedName)
	default:
		err = fmt.Errorf("%w: %d", errUnknownMetricType, typ)
	}

	if err != nil {
		return nil, fmt.Errorf("create metric %q: %w", name, err)
	}

	r.metrics[name] = m

	return m, nil
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

	return metricAttributes{
		set:    set,
		add:    []otelmetric.AddOption{otelmetric.WithAttributeSet(set)},
		record: []otelmetric.RecordOption{otelmetric.WithAttributeSet(set)},
	}
}
