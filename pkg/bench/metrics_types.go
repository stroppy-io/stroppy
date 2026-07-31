package bench

// stroppy-owned metrics substrate — mirrors the subset of go.k6.io/k6/metrics
// that pkg/bench consumes, so the k6 dependency can be dropped from the Go
// runtime path. Tags are carried for parity but the floor sink ignores them.

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

var errMetricAlreadyRegistered = errors.New("metric already registered")

type metricType int

const (
	Counter metricType = iota
	Trend
	Rate
)

type metric struct {
	Name string
	Type metricType
}

type Registry struct {
	mu      sync.Mutex
	metrics map[string]*metric
}

func NewRegistry() *Registry {
	return &Registry{metrics: map[string]*metric{}}
}

func (r *Registry) NewMetric(name string, t metricType) (*metric, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.metrics[name]; ok {
		return nil, fmt.Errorf("%w: %q", errMetricAlreadyRegistered, name)
	}

	m := &metric{Name: name, Type: t}
	r.metrics[name] = m

	return m, nil
}

// RootTagSet returns an empty tag set sentinel.
func (r *Registry) RootTagSet() *TagSet { return &TagSet{} }

// TagSet is an immutable, copy-on-extend set of tag key/value pairs.
type TagSet struct {
	pairs [][2]string
}

func (t *TagSet) With(k, v string) *TagSet {
	if t == nil {
		t = &TagSet{}
	}

	out := &TagSet{pairs: make([][2]string, 0, len(t.pairs)+1)}
	out.pairs = append(out.pairs, t.pairs...)
	out.pairs = append(out.pairs, [2]string{k, v})

	return out
}

type Sample struct {
	Metric *metric
	Tags   *TagSet
	Time   time.Time
	Value  float64
}

type SampleContainer interface{ GetSamples() []Sample }

type sampleContainer []Sample

func (s sampleContainer) GetSamples() []Sample { return []Sample(s) }

// PushIfNotDone sends s on ch unless ctx is already done. Blocks until either.
func PushIfNotDone(ctx context.Context, ch chan<- SampleContainer, s Sample) {
	select {
	case <-ctx.Done():
	case ch <- sampleContainer{s}:
	}
}
