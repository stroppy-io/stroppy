package bench

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/stroppy-io/stroppy/v6/pkg/driver"
	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

// Pool gauges describe snapshots, so adding different steps invents capacity.
func poolGauge(points []metricdata.DataPoint[float64]) bool {
	for _, point := range points {
		if _, exists := point.Attributes.Value("pool_scope"); exists {
			return true
		}
	}

	return false
}

func poolGaugeSummaries(name string, points []metricdata.DataPoint[float64]) []string {
	lines := make([]string, 0, len(points))
	for _, point := range points {
		labels := make([]string, 0, point.Attributes.Len())
		for _, value := range point.Attributes.ToSlice() {
			labels = append(labels, fmt.Sprintf("%s=%q", value.Key, value.Value.String()))
		}

		lines = append(lines, fmt.Sprintf("  %s{%s} %.3f", name, strings.Join(labels, ","), point.Value))
	}

	return lines
}

type poolObservation struct {
	step, database, kind string
	execution            int
	provider             driver.PoolStatistics
	baseline, latest     driver.PoolSnapshot
	finished             bool
	used                 bool
}

type poolTelemetry struct {
	mu        sync.Mutex
	step      string
	execution int
	current   map[string]*poolObservation
	history   []*poolObservation
}

func newPoolTelemetry(meter otelmetric.Meter, prefix string) (*poolTelemetry, error) {
	p := &poolTelemetry{}
	names := []string{
		"pool_stats_available", "pool_connections_open", "pool_connections_in_use",
		"pool_connections_idle", "pool_connections_max",
	}
	gauges := make([]otelmetric.Float64ObservableGauge, len(names))

	var instruments []otelmetric.Observable

	for i, name := range names {
		gauge, err := meter.Float64ObservableGauge(prefix + name)
		if err != nil {
			return nil, err
		}

		gauges[i] = gauge
		instruments = append(instruments, gauge)
	}

	names = []string{
		"pool_sql_capacity_waits_total", "pool_sql_capacity_wait_seconds_total",
		"pool_pgx_empty_acquires_total", "pool_pgx_empty_acquire_seconds_total", "pool_pgx_canceled_acquires_total",
	}

	counters := make([]otelmetric.Float64ObservableCounter, len(names))
	for i, name := range names {
		unit := "{acquisition}"
		if strings.Contains(name, "seconds") {
			unit = "s"
		}

		counter, err := meter.Float64ObservableCounter(prefix+name, otelmetric.WithUnit(unit))
		if err != nil {
			return nil, err
		}

		counters[i] = counter
		instruments = append(instruments, counter)
	}

	_, err := meter.RegisterCallback(func(_ context.Context, observer otelmetric.Observer) error {
		p.observe(observer, gauges, counters)

		return nil
	}, instruments...)

	return p, err
}

func (p *poolTelemetry) observe(
	observer otelmetric.Observer,
	gauges []otelmetric.Float64ObservableGauge,
	counters []otelmetric.Float64ObservableCounter,
) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, entry := range p.history {
		if !entry.used {
			continue
		}

		entry.sample()
		s := entry.latest
		attrs := otelmetric.WithAttributes(attribute.String("step", entry.step),
			attribute.String("database", entry.database), attribute.String("driver", entry.kind),
			attribute.String("pool_backend", s.Backend), attribute.String("pool_scope", s.Scope),
			attribute.Int("step_execution", entry.execution))

		available := float64(0)
		if s.Supported {
			available = 1
		}

		observer.ObserveFloat64(gauges[0], available, attrs)

		if !s.Supported {
			continue
		}

		for i, value := range []int64{s.Open, s.InUse, s.Idle, s.MaxConnections} {
			observer.ObserveFloat64(gauges[i+1], float64(value), attrs)
		}

		count, seconds, canceled := entry.deltas()

		switch s.Backend {
		case "database/sql":
			observer.ObserveFloat64(counters[0], count, attrs)
			observer.ObserveFloat64(counters[1], seconds, attrs)
		case "pgxpool":
			observer.ObserveFloat64(counters[2], count, attrs)
			observer.ObserveFloat64(counters[3], seconds, attrs)
			observer.ObserveFloat64(counters[4], canceled, attrs)
		}
	}
}

func (entry *poolObservation) sample() {
	if !entry.finished && entry.provider != nil {
		entry.latest = entry.provider.PoolStats()
	}
}

func (entry *poolObservation) deltas() (count, seconds, canceled float64) {
	return float64(max(0, entry.latest.WaitCount-entry.baseline.WaitCount)),
		max(0, (entry.latest.WaitDuration - entry.baseline.WaitDuration).Seconds()),
		float64(max(0, entry.latest.Canceled-entry.baseline.Canceled))
}

func (e *Execution) startPoolTelemetry(name string) {
	p := e.root.pools
	if p == nil {
		return
	}

	e.databaseMu.Lock()
	defer e.databaseMu.Unlock()

	p.mu.Lock()
	p.step = name
	p.execution++
	p.current = map[string]*poolObservation{}
	p.mu.Unlock()

	for database, slot := range e.databases {
		p.track(database, slot, false)
	}
}

func (p *poolTelemetry) use(name string, slot *databaseSlot) {
	p.track(name, slot, true)
}

func (p *poolTelemetry) track(name string, slot *databaseSlot, used bool) {
	if p == nil {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.step == "" {
		return
	}

	if entry := p.current[name]; entry != nil {
		entry.used = entry.used || used

		return
	}

	publicName := name
	if publicName == "" {
		publicName = "default"
	}

	provider, ok := slot.drv.(driver.PoolStatistics)

	stats := driver.PoolSnapshot{Reason: "driver does not expose connection-pool statistics"}
	if ok {
		stats = provider.PoolStats()
	}

	entry := &poolObservation{
		step: p.step, execution: p.execution, database: publicName, kind: slot.cfg.DriverType.String(),
		provider: provider, baseline: stats, latest: stats, used: used,
	}
	p.current[name] = entry
	p.history = append(p.history, entry)
}

func (p *poolTelemetry) stop() {
	if p == nil {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	for _, entry := range p.current {
		entry.sample()
		entry.finished = true
	}

	p.step = ""
	p.current = nil
}

func (p *poolTelemetry) reports() []report.Pool {
	if p == nil {
		return nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	out := make([]report.Pool, 0, len(p.history))
	for _, entry := range p.history {
		if !entry.used {
			continue
		}

		entry.sample()
		stats := entry.latest
		count, seconds, canceled := entry.deltas()

		value := report.Pool{
			Step: entry.step, Execution: entry.execution, Database: entry.database, Driver: entry.kind,
			Backend: stats.Backend, Scope: stats.Scope, Supported: stats.Supported, Reason: stats.Reason,
		}
		if stats.Supported {
			value.Open, value.InUse, value.Idle = &stats.Open, &stats.InUse, &stats.Idle
			value.MaxConnections = &stats.MaxConnections

			value.WaitCount, value.WaitSeconds = &count, &seconds
			if stats.Backend == "pgxpool" {
				value.Canceled = &canceled
			}
		}

		out = append(out, value)
	}

	return out
}
