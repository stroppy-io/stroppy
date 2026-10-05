package bench

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"
)

// Each measured step owns a wall-clock window including its graceful drain.
type measurement struct {
	name          string
	started       time.Time
	elapsed       atomic.Int64
	transactions  atomic.Int64
	iterations    atomic.Int64
	queries       atomic.Int64
	transactional atomic.Bool
}

func (m *measurement) seconds() float64 {
	elapsed := m.elapsed.Load()
	if elapsed == 0 {
		elapsed = time.Since(m.started).Nanoseconds()
	}

	return float64(max(elapsed, 1)) / float64(time.Second)
}

type throughput struct {
	mu         sync.Mutex
	windows    []*measurement
	current    atomic.Pointer[measurement]
	registered bool
}

func (r *rootState) startThroughput(name string) error {
	t := &r.throughput
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.registered {
		meter := r.registry.meter
		names := []string{
			"tps",
			"iterations_per_second",
			"queries_per_second",
			"measurement_seconds",
		}
		units := []string{"{transaction}/s", "{iteration}/s", "{query}/s", "s"}

		gauges := make([]otelmetric.Float64ObservableGauge, len(names))
		for i, name := range names {
			gauge, err := meter.Float64ObservableGauge(r.metricsPrefix+name, otelmetric.WithUnit(units[i]))
			if err != nil {
				return fmt.Errorf("create throughput %s: %w", name, err)
			}

			gauges[i] = gauge
		}

		completed, err := meter.Float64ObservableCounter(r.metricsPrefix + "successful_transactions_total")
		if err != nil {
			return err
		}

		_, err = meter.RegisterCallback(func(_ context.Context, observer otelmetric.Observer) error {
			t.mu.Lock()
			defer t.mu.Unlock()

			for _, window := range t.windows {
				seconds := window.seconds()

				options := []otelmetric.ObserveOption{otelmetric.WithAttributes(attribute.String("step", window.name))}
				if window.transactional.Load() {
					observer.ObserveFloat64(
						completed,
						float64(window.transactions.Load()),
						options...,
					)
					observer.ObserveFloat64(
						gauges[0],
						float64(window.transactions.Load())/seconds,
						options...,
					)
				}

				observer.ObserveFloat64(
					gauges[1],
					float64(window.iterations.Load())/seconds,
					options...,
				)
				observer.ObserveFloat64(
					gauges[2],
					float64(window.queries.Load())/seconds,
					options...,
				)
				observer.ObserveFloat64(gauges[3], seconds, options...)
			}

			return nil
		}, gauges[0], gauges[1], gauges[2], gauges[3], completed)
		if err != nil {
			return err
		}

		t.registered = true
	}

	window := &measurement{name: name, started: time.Now()}
	t.windows = append(t.windows, window)
	t.current.Store(window)

	return nil
}

func (t *throughput) stop() {
	if window := t.current.Swap(nil); window != nil {
		window.elapsed.Store(max(time.Since(window.started).Nanoseconds(), 1))
	}
}

// LogicalOperation counts one successful logical operation, including its retries.
// Nested managed transactions do not double-count that operation.
func (b *Bench) LogicalOperation(fn func() error) error {
	outer := b.vu.logicalDepth == 0

	b.vu.logicalDepth++
	defer func() { b.vu.logicalDepth-- }()

	window := b.root.throughput.current.Load()
	if outer && window != nil {
		window.transactional.Store(true)
	}

	err := fn()
	if outer && window != nil && err == nil && b.vu.ctx.Err() == nil {
		window.transactions.Add(1)
	}

	return err
}
