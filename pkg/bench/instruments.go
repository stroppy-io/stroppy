package bench

import (
	"context"
	"math"
	"regexp"
	"slices"
	"sort"
)

// Label is one finite, declared metric dimension.
type Label struct {
	Name  string
	Value string
}

func LabelValue(name, value string) Label { return Label{name, value} }

// MetricOption supplies discoverable metric units, description, or label values.
type (
	MetricOption interface{ applyMetric(instrument *instrument) }
	metricOption func(*instrument)
)

func (o metricOption) applyMetric(i *instrument) { o(i) }
func Unit(unit string) MetricOption              { return metricOption(func(i *instrument) { i.unit = unit }) }

func MetricDescription(description string) MetricOption {
	return metricOption(func(i *instrument) { i.description = description })
}

func LabelValues(name string, values ...string) MetricOption {
	values = slices.Clone(values)

	return metricOption(func(i *instrument) {
		if i.labels == nil {
			i.labels = map[string][]string{}
		}

		if name == "" || len(values) == 0 {
			invalid("metric labels", inputError("name and values required"))
		}

		if _, ok := i.labels[name]; ok {
			invalid("metric labels", inputError("duplicate label key"))
		}

		seen := map[string]bool{}
		for _, value := range values {
			if value == "" || seen[value] {
				invalid("metric labels", inputError("empty or duplicate label value"))
			}

			seen[value] = true
		}

		if name == "step" {
			invalid("metric labels", inputError("step is framework-owned"))
		}

		i.labels[name] = values
	})
}

// MetricSchema is copied instrument metadata for discovery.
type MetricSchema struct {
	Name        string              `json:"name"`
	Kind        string              `json:"kind"`
	Unit        string              `json:"unit,omitempty"`
	Description string              `json:"description,omitempty"`
	Bounds      []float64           `json:"bounds,omitempty"`
	Labels      map[string][]string `json:"labels"`
}

// MetricDeclarations registers typed instruments during definition.
type MetricDeclarations struct {
	def    *Def
	names  map[string]bool
	schema []MetricSchema
}
type instrument struct {
	metric      *metric
	name        string
	unit        string
	description string
	bounds      []float64
	labels      map[string][]string
	root        *rootState
}

func (d *MetricDeclarations) declare(name string, kind metricType, options []MetricOption) *instrument {
	if !metricNamePattern.MatchString(name) {
		invalid("metric", inputError("name is required"))
	}

	if d.names == nil {
		d.names = map[string]bool{}
	}

	if d.names[name] {
		invalid("metric", inputError("duplicate %q", name))
	}

	d.names[name] = true
	i := &instrument{name: name, root: d.def.Execution.root}

	for _, option := range options {
		if option == nil {
			invalid("metric", inputError("nil option"))
		}

		option.applyMetric(i)
	}

	labelCopy := map[string][]string{}
	combinations := 1

	for name, values := range i.labels {
		labelCopy[name] = slices.Clone(values)
		if combinations > metricCardinalityLimit/len(values) {
			invalid("metric labels", inputError("label combinations exceed bounded metric capacity"))
		}

		combinations *= len(values)
	}

	if len(i.labels)*2+2 > maxCachedTagParts {
		invalid("metric labels", inputError("too many dimensions"))
	}

	kindName := map[metricType]string{
		Counter: "counter",
		Trend:   "histogram",
		Gauge:   "gauge",
		Rate:    "rate",
	}[kind]

	d.schema = append(d.schema, MetricSchema{
		Name:        name,
		Kind:        kindName,
		Unit:        i.unit,
		Description: i.description,
		Bounds:      slices.Clone(i.bounds),
		Labels:      labelCopy,
	})
	if i.root != nil {
		m, err := i.root.registry.newInstrument(name, kind, i.unit, i.description, i.bounds)
		if err != nil {
			invalid("metric", err)
		}

		i.metric = m
	}

	return i
}

func (i *instrument) record(ctx context.Context, value float64, labels []Label) {
	if i == nil || ctx == nil {
		invalid("metric recording", inputError("nil handle or context"))
	}

	if math.IsNaN(value) || math.IsInf(value, 0) {
		invalid("metric recording", inputError("value must be finite"))
	}

	tags := []string{}

	seen := map[string]bool{}
	for _, label := range labels {
		if seen[label.Name] {
			invalid("metric label", inputError("duplicate recording key"))
		}

		seen[label.Name] = true
		if !slices.Contains(i.labels[label.Name], label.Value) {
			invalid("metric label", inputError("undeclared %s=%s", label.Name, label.Value))
		}

		tags = append(tags, label.Name, label.Value)
	}

	if len(seen) != len(i.labels) {
		invalid("metric label", inputError("all declared labels are required"))
	}

	pairs := make([]Label, 0, len(labels))
	pairs = append(pairs, labels...)
	sort.Slice(pairs, func(a, b int) bool { return pairs[a].Name < pairs[b].Name })

	tags = tags[:0]
	for _, label := range pairs {
		tags = append(tags, label.Name, label.Value)
	}

	if i.metric == nil {
		return
	}

	if step, ok := ctx.Value(metricStepKey{}).(string); ok {
		tags = append(tags, "step", step)
	}

	if len(tags) > maxCachedTagParts {
		invalid("metric labels", inputError("too many label dimensions"))
	}

	i.metric.add(ctx, value, i.metric.taggedAttributes(tags))
}

type (
	metricStepKey   struct{}
	CounterHandle   struct{ i *instrument }
	HistogramHandle struct{ i *instrument }
	GaugeHandle     struct{ i *instrument }
	RateHandle      struct{ i *instrument }
)

func (d *MetricDeclarations) Counter(name string, opts ...MetricOption) *CounterHandle {
	return &CounterHandle{d.declare(name, Counter, opts)}
}

func (d *MetricDeclarations) Histogram(name string, opts ...MetricOption) *HistogramHandle {
	return &HistogramHandle{d.declare(name, Trend, opts)}
}

func (d *MetricDeclarations) Gauge(name string, opts ...MetricOption) *GaugeHandle {
	return &GaugeHandle{d.declare(name, Gauge, opts)}
}

func (d *MetricDeclarations) Rate(name string, opts ...MetricOption) *RateHandle {
	return &RateHandle{d.declare(name, Rate, opts)}
}

func (c *CounterHandle) Add(ctx context.Context, value float64, labels ...Label) {
	c.i.record(ctx, value, labels)
}

func (h *HistogramHandle) Record(ctx context.Context, value float64, labels ...Label) {
	h.i.record(ctx, value, labels)
}

func (g *GaugeHandle) Set(ctx context.Context, value float64, labels ...Label) {
	g.i.record(ctx, value, labels)
}

func (r *RateHandle) Record(ctx context.Context, value bool, labels ...Label) {
	number := 0.
	if value {
		number = 1
	}

	r.i.record(ctx, number, labels)
}

var metricNamePattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]*$`)

// Bounds sets explicit histogram boundaries in declared unit.
func Bounds(values ...float64) MetricOption {
	values = slices.Clone(values)

	return metricOption(func(i *instrument) {
		for index, value := range values {
			if math.IsNaN(value) || math.IsInf(value, 0) || (index > 0 && value <= values[index-1]) {
				invalid("histogram bounds", inputError("bounds must be finite and increasing"))
			}
		}

		i.bounds = values
	})
}
