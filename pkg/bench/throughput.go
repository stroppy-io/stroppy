package bench

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	otelmetric "go.opentelemetry.io/otel/metric"
)

// Throughput uses one wall-clock window shared by all VUs. Setup and teardown
// are outside this window; failed attempts and retry waits remain inside it.
type throughput struct {
	start         time.Time
	elapsed       atomic.Int64
	active        atomic.Bool
	transactions  atomic.Int64
	iterations    atomic.Int64
	queries       atomic.Int64
	transactional atomic.Bool
}

func (r *RootState) startThroughput() error {
	meter := r.registry.meter
	names := []string{"tps", "iterations_per_second", "queries_per_second", "measurement_seconds"}
	units := []string{"{transaction}/s", "{iteration}/s", "{query}/s", "s"}

	gauges := make([]otelmetric.Float64ObservableGauge, len(names))
	for i, name := range names {
		gauge, err := meter.Float64ObservableGauge(r.metricsPrefix+name, otelmetric.WithUnit(units[i]))
		if err != nil {
			return fmt.Errorf("create throughput metric %s: %w", name, err)
		}

		gauges[i] = gauge
	}

	completed, err := meter.Float64ObservableCounter(r.metricsPrefix + "successful_transactions_total")
	if err != nil {
		return err
	}

	t := &r.throughput
	t.start = time.Now()
	t.active.Store(true)

	_, err = meter.RegisterCallback(func(_ context.Context, observer otelmetric.Observer) error {
		seconds := t.seconds()
		if seconds <= 0 {
			return nil
		}

		if t.transactional.Load() {
			observer.ObserveFloat64(completed, float64(t.transactions.Load()))
			observer.ObserveFloat64(gauges[0], float64(t.transactions.Load())/seconds)
		}

		observer.ObserveFloat64(gauges[1], float64(t.iterations.Load())/seconds)
		observer.ObserveFloat64(gauges[2], float64(t.queries.Load())/seconds)
		observer.ObserveFloat64(gauges[3], seconds)

		return nil
	}, gauges[0], gauges[1], gauges[2], gauges[3], completed)

	return err
}

func (t *throughput) seconds() float64 {
	if elapsed := t.elapsed.Load(); elapsed > 0 {
		return time.Duration(elapsed).Seconds()
	}

	return time.Since(t.start).Seconds()
}

func (t *throughput) stop() {
	t.elapsed.Store(max(time.Since(t.start).Nanoseconds(), 1))
	t.active.Store(false)
}

// Transaction runs one logical workload transaction under the workload step.
// Place retry handling inside fn so a successful retry counts only once. An
// expected rollback may return nil when the workload defines it as successful.
// Filtered steps, failed transactions, setup and teardown do not increment TPS.
func (b *Bench) Transaction(fn func() error) error {
	return b.StepSilent("workload", func() error {
		t := &b.root.throughput

		measured := t.active.Load()
		if measured {
			t.transactional.Store(true)
		}

		err := fn()
		if measured && err == nil {
			t.transactions.Add(1)
		}

		return err
	})
}
