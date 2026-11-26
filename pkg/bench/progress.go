package bench

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/v6/pkg/driver/insertprogress"
	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

const (
	progressInterval     = 10 * time.Second
	progressPendingLimit = 0.99
)

func progressSourceGauge(points []metricdata.DataPoint[float64]) bool {
	for _, point := range points {
		if point.Attributes.HasValue("unit") && point.Attributes.HasValue("activity") {
			return true
		}
	}

	return false
}

var progressIdentity = regexp.MustCompile(`^[a-zA-Z0-9_./-]{1,96}$`)

// ProgressUpdate describes one worker's current invocation, not successful work.
// Total is nil when the amount of work is unknown. Activity must be an identifier,
// never SQL text, arguments, or connection information.
type ProgressUpdate struct {
	Activity string
	Unit     string
	Current  float64
	Total    *float64
}

type workerProgress struct {
	update    ProgressUpdate
	started   time.Time
	iteration uint64
	completed float64
	insert    *insertprogress.Snapshot
	active    atomic.Bool
}

type runProgress struct {
	mu             sync.Mutex
	root           *rootState
	started        time.Time
	stopped        time.Time
	stepStarted    time.Time
	stepStopped    time.Time
	budgetStart    time.Time
	planned        int
	finished       int
	ratio          float64
	status         string
	step           string
	policy         Policy
	window         *measurement
	workers        []*workerProgress
	retained       map[string][]report.ProgressSource
	events         map[progressEvent]float64
	stop           chan struct{}
	done           chan struct{}
	lastLog        time.Time
	activityLogged bool
}

type progressEvent struct{ step, event string }

func policyProgressDescription(mode string) string {
	switch mode {
	case "constant-vus":
		return "elapsed / duration; draining reported separately"
	case "shared-iterations", "per-worker-iterations":
		return "finished / requested iterations; optional inner work"
	default:
		return "phase completion; runtime activity and optional native totals"
	}
}

// Progress publishes activity and native work within the current invocation.
// Repeated policies supply the outer invocation budget; callers report the inner
// work afresh each time. Call at activity start and after each completed unit.
func (b *Bench) Progress(ctx context.Context, update ProgressUpdate) {
	if ctx == nil || !progressIdentity.MatchString(update.Unit) ||
		(update.Activity != "" && !progressIdentity.MatchString(update.Activity)) ||
		math.IsNaN(update.Current) || math.IsInf(update.Current, 0) || update.Current < 0 {
		invalid("progress", inputError("context, bounded identifiers and a finite nonnegative current are required"))
	}

	if update.Total != nil {
		if math.IsNaN(*update.Total) || math.IsInf(*update.Total, 0) || *update.Total < 0 {
			invalid("progress", inputError("total must be finite and nonnegative"))
		}

		total := *update.Total
		update.Total = &total
	}

	if p := b.root.progress; p != nil {
		p.update(b.vu.worker, b.vu.iterScenario, update)
	}
}

func newRunProgress(root *rootState, steps []StepDescription) (*runProgress, error) {
	p := &runProgress{
		root: root, started: time.Now(), status: "running",
		events: make(map[progressEvent]float64), retained: make(map[string][]report.ProgressSource),
		stop: make(chan struct{}), done: make(chan struct{}),
	}
	for _, step := range steps {
		if root.stepFilter.enabled(step.Name) {
			p.planned++
		}
	}

	if err := p.registerMetrics(); err != nil {
		return nil, err
	}
	go p.watch()

	return p, nil
}

func (p *runProgress) registerMetrics() error {
	root := p.root
	names := []string{
		"progress_ratio", "progress_elapsed_seconds", "progress_step_elapsed_seconds",
		"progress_current", "progress_total", "progress_source_ratio", "progress_activity_elapsed_seconds",
	}
	instruments := make([]otelmetric.Observable, 0, len(names)+1)

	gauges := make([]otelmetric.Float64ObservableGauge, len(names))
	for index, name := range names {
		unit := "1"
		if index == 1 || index == 2 || index == 6 {
			unit = "s"
		}

		gauge, err := root.registry.meter.Float64ObservableGauge(root.metricsPrefix+name, otelmetric.WithUnit(unit))
		if err != nil {
			return err
		}

		gauges[index] = gauge
		instruments = append(instruments, gauge)
	}

	events, err := root.registry.meter.Float64ObservableCounter(root.metricsPrefix + "progress_events_total")
	if err != nil {
		return err
	}

	instruments = append(instruments, events)

	_, err = root.registry.meter.RegisterCallback(func(_ context.Context, observer otelmetric.Observer) error {
		p.mu.Lock()
		defer p.mu.Unlock()

		value := p.snapshotLocked(time.Now())
		observer.ObserveFloat64(gauges[0], value.Ratio)
		observer.ObserveFloat64(gauges[1], value.ElapsedSeconds)

		if value.Step != "" {
			observer.ObserveFloat64(gauges[2], value.StepElapsedSeconds,
				otelmetric.WithAttributes(attribute.String("step", value.Step)))
		}

		for _, source := range value.Sources {
			observeProgressSource(observer, gauges, source.Step, &source)
		}

		for key, count := range p.events {
			observer.ObserveFloat64(events, count, otelmetric.WithAttributes(
				attribute.String("step", key.step), attribute.String("event", key.event)))
		}

		return nil
	}, instruments...)

	return err
}

func observeProgressSource(
	observer otelmetric.Observer, gauges []otelmetric.Float64ObservableGauge, step string, source *report.ProgressSource,
) {
	attrs := otelmetric.WithAttributes(attribute.String("step", step),
		attribute.String("unit", source.Unit), attribute.Int("worker", source.Worker),
		attribute.String("activity", source.Activity))
	observer.ObserveFloat64(gauges[3], source.Current, attrs)

	if source.Total != nil {
		observer.ObserveFloat64(gauges[4], *source.Total, attrs)

		if *source.Total > 0 {
			observer.ObserveFloat64(gauges[5], min(source.Current / *source.Total, 1), attrs)
		}
	}

	observer.ObserveFloat64(gauges[6], source.ElapsedSeconds, attrs)
}

func (p *runProgress) watch() {
	defer close(p.done)

	ticker := time.NewTicker(progressInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.log()
		case <-p.stop:
			return
		}
	}
}

func (p *runProgress) begin(name string, policy Policy) {
	if p == nil {
		return
	}

	p.mu.Lock()
	p.step, p.policy, p.status = name, policy, "running"
	delete(p.retained, name)
	p.stepStarted = time.Now()
	p.stepStopped = time.Time{}
	p.budgetStart = time.Time{}
	p.window = nil
	p.activityLogged = false

	p.workers = make([]*workerProgress, policy.workers)
	for i := range p.workers {
		p.workers[i] = &workerProgress{}
	}

	p.planned = max(p.planned, p.finished+1)
	p.events[progressEvent{name, "started"}]++
	p.mu.Unlock()
	p.log()
}

func (p *runProgress) measuring(window *measurement) {
	p.mu.Lock()
	p.window = window
	p.budgetStart = time.Now()
	p.mu.Unlock()
}

func (p *runProgress) end(status StepStatus) {
	p.mu.Lock()
	now := time.Now()
	value := p.snapshotLocked(now)

	var sources []report.ProgressSource

	for _, source := range value.Sources {
		if source.Step == p.step {
			sources = append(sources, source)
		}
	}

	p.retained[p.step] = sources
	p.stepStopped = now
	p.status = string(status)

	p.events[progressEvent{p.step, string(status)}]++
	if status == Completed || status == CompletedWithErrors {
		p.finished++
	}
	p.mu.Unlock()
}

func (p *runProgress) update(worker int, iteration uint64, update ProgressUpdate) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if worker >= len(p.workers) {
		return
	}

	source := p.workers[worker]
	if source.update.Unit != update.Unit {
		source.completed = 0
	} else if source.iteration != iteration {
		source.completed += source.update.Current
	}

	changed := source.update.Activity != update.Activity && update.Activity != ""
	if changed || !source.active.Load() {
		source.started = time.Now()
	}

	if changed && update.Unit == "tables" {
		source.insert = nil
	}

	source.update, source.iteration = update, iteration
	source.active.Store(true)

	since := time.Since(p.lastLog)
	if (changed && (!p.activityLogged || since >= time.Second)) || since >= progressInterval {
		p.activityLogged = true
		p.logLocked(time.Now())
	}
}

func (p *runProgress) insert(worker int, snapshot *insertprogress.Snapshot) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if worker >= len(p.workers) {
		return
	}

	source := p.workers[worker]
	if snapshot.Event == insertprogress.EventCompleted && source.update.Unit == "tables" &&
		(source.insert == nil || source.insert.Event != insertprogress.EventCompleted) {
		source.update.Current++
	}

	value := *snapshot
	source.insert = &value
}

func (p *runProgress) iterationDone(worker int) {
	if p == nil {
		return
	}
	// The step owns the worker slice until all actions have joined.
	if source := p.workers[worker]; source.active.Load() {
		source.active.Store(false)
	}
}

func (p *runProgress) finish(status string) report.Progress {
	close(p.stop)
	<-p.done
	p.mu.Lock()
	defer p.mu.Unlock()

	p.stopped = time.Now()
	p.snapshotLocked(p.stopped)

	p.status = status
	if status == string(Completed) || status == string(CompletedWithErrors) {
		p.ratio = 1
	}

	p.events[progressEvent{"", status}]++
	p.logLocked(p.stopped)

	return p.snapshotLocked(p.stopped)
}

func (p *runProgress) snapshotLocked(now time.Time) report.Progress {
	if !p.stopped.IsZero() {
		now = p.stopped
	}

	value := report.Progress{
		Ratio: p.ratio, Status: p.status, Step: p.step,
		ElapsedSeconds: now.Sub(p.started).Seconds(), Method: "equal selected phase weights; best-effort work estimate",
	}

	stepNow := now
	if !p.stepStopped.IsZero() {
		stepNow = p.stepStopped
	}

	if !p.stepStarted.IsZero() {
		value.StepElapsedSeconds = stepNow.Sub(p.stepStarted).Seconds()
	}

	fraction := p.policyProgress(stepNow, &value)
	for worker, state := range p.workers {
		inner := p.workerProgress(worker, state, stepNow, &value)
		if p.policy.mode == "once" {
			fraction = max(fraction, inner)
		} else if p.policy.mode != "constant-vus" && p.policy.
			iterations > 0 && state.active.Load() {
			fraction += inner / float64(p.policy.iterations)
		}
	}

	value.Status = p.status
	for index := range value.Sources {
		value.Sources[index].Step = p.step
	}

	for step, sources := range p.retained {
		if step != p.step {
			value.Sources = append(value.Sources, sources...)
		}
	}

	if !p.stepStopped.IsZero() {
		fraction = 0
	}

	if p.planned > 0 && p.stopped.IsZero() {
		// Reserve the terminal percentage for teardown and runtime-discovered work.
		p.ratio = max(p.ratio, min(progressPendingLimit, (float64(p.finished)+min(fraction, 1))/float64(p.planned)))
	}

	value.Ratio = p.ratio

	return value
}

func (p *runProgress) policyProgress(now time.Time, value *report.Progress) float64 {
	if p.window == nil || p.policy.mode == "once" {
		return 0
	}

	current, total := float64(p.window.totals().iterations), float64(p.policy.iterations)

	unit := "iterations"
	if p.policy.mode == "constant-vus" {
		unit, total = "seconds", p.policy.duration.Seconds()

		current = max(0, now.Sub(p.budgetStart).Seconds())
		if current >= total && p.status == "running" {
			p.status = "draining"
			p.events[progressEvent{p.step, "draining"}]++
		}

		current = min(current, total)
	}

	value.Sources = append(value.Sources, report.ProgressSource{Unit: unit, Current: current, Total: &total})
	if total <= 0 {
		return 0
	}

	return min(current/total, 1)
}

func (p *runProgress) workerProgress(worker int, state *workerProgress, now time.Time, value *report.Progress) float64 {
	update := state.update
	inner := 0.0

	if update.Unit != "" {
		if update.Total != nil && *update.Total > 0 {
			inner = min(update.Current / *update.Total, 1)
		}

		value.Sources = append(value.Sources, p.nativeSource(worker, state, now))
	}

	load := state.insert
	if load == nil {
		return inner
	}

	total := float64(load.TotalRows)

	var known *float64
	if total > 0 {
		known = &total
	}

	value.Sources = append(value.Sources, report.ProgressSource{
		Worker: worker, Unit: "rows", Activity: load.Table,
		Current: float64(load.ConfirmedRows), Total: known, ElapsedSeconds: load.Elapsed.Seconds(),
	})
	if p.policy.mode != "once" || known == nil {
		return inner
	}

	rows := min(float64(load.ConfirmedRows)/total, 1)
	if update.Unit == "tables" && update.Total != nil && *update.Total > 0 {
		if load.Event == insertprogress.EventCompleted {
			rows = 0
		}

		return min((update.Current+rows) / *update.Total, 1)
	}

	if update.Unit == "" {
		return rows
	}

	return inner
}

func (p *runProgress) nativeSource(worker int, state *workerProgress, now time.Time) report.ProgressSource {
	update := state.update

	current, total := update.Current, update.Total
	if p.policy.mode != "once" && total != nil {
		current += state.completed

		switch {
		case p.policy.mode == "shared-iterations" && p.policy.workers == 1:
			planned := *total * float64(p.policy.iterations)
			total = &planned
		case p.policy.mode != "per-worker-iterations":
			// Shared scheduling does not assign a fixed count to each worker.
			total = nil
		}
	}

	return report.ProgressSource{
		Worker: worker, Unit: update.Unit, Activity: update.Activity,
		Current: current, Total: total, ElapsedSeconds: max(0, now.Sub(state.started).Seconds()),
	}
}

func (p *runProgress) log() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.logLocked(time.Now())
}

func (p *runProgress) logLocked(now time.Time) {
	value := p.snapshotLocked(now)
	p.lastLog = now

	fields := []zap.Field{
		zap.Float64("percent", value.Ratio*percentScale), zap.String("status", value.Status),
		zap.String("step", value.Step), zap.Float64("elapsed_seconds", value.ElapsedSeconds),
	}
	for _, source := range value.Sources {
		if source.Step != value.Step {
			continue
		}

		label := source.Unit + "_" + strconv.Itoa(source.Worker)

		text := fmt.Sprintf("%g", source.Current)
		if source.Total != nil {
			text += fmt.Sprintf("/%g", *source.Total)
		}

		fields = append(fields, zap.String(label, text))
		if source.Activity != "" {
			fields = append(fields, zap.String("activity_"+strconv.Itoa(source.Worker), source.Activity),
				zap.Float64("activity_elapsed_seconds_"+strconv.Itoa(source.Worker), source.ElapsedSeconds))
		}
	}

	p.root.lg.Info("run progress", fields...)
}
