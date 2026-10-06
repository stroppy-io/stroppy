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
	name    string
	started time.Time
	elapsed atomic.Int64
	writers []measurementWriter
}

const measurementWriterPadding = 128

// Padding keeps concurrently updated writers on separate cache lines.
type measurementWriter struct {
	transactions  atomic.Int64
	iterations    atomic.Int64
	queries       atomic.Int64
	transactional atomic.Bool
	_             [measurementWriterPadding]byte
}

type measurementTotals struct {
	transactions, iterations, queries int64
	transactional                     bool
}

func (m *measurement) writer(worker int) *measurementWriter {
	return &m.writers[worker%len(m.writers)]
}

func (m *measurement) totals() measurementTotals {
	var total measurementTotals

	for i := range m.writers {
		writer := &m.writers[i]
		total.transactions += writer.transactions.Load()
		total.iterations += writer.iterations.Load()
		total.queries += writer.queries.Load()
		total.transactional = total.transactional || writer.transactional.Load()
	}

	return total
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

func (r *rootState) startThroughput(name string, workers int) error {
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
				total := window.totals()

				options := []otelmetric.ObserveOption{otelmetric.WithAttributes(attribute.String("step", window.name))}
				if total.transactional {
					observer.ObserveFloat64(
						completed,
						float64(total.transactions),
						options...,
					)
					observer.ObserveFloat64(
						gauges[0],
						float64(total.transactions)/seconds,
						options...,
					)
				}

				observer.ObserveFloat64(
					gauges[1],
					float64(total.iterations)/seconds,
					options...,
				)
				observer.ObserveFloat64(
					gauges[2],
					float64(total.queries)/seconds,
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

	window := &measurement{
		name: name, started: time.Now(),
		writers: make([]measurementWriter, min(workers, metricWriterLimit)),
	}
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

	var writer *measurementWriter
	if outer && window != nil {
		writer = window.writer(b.vu.worker)
		if !writer.transactional.Load() {
			writer.transactional.Store(true)
		}
	}

	err := fn()
	if writer != nil && err == nil && b.vu.ctx.Err() == nil {
		writer.transactions.Add(1)
	}

	return err
}
