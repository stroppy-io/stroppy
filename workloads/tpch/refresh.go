package tpch

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/pkg/bench/mpp"
	"github.com/stroppy-io/stroppy/v6/pkg/report"
	"github.com/stroppy-io/stroppy/v6/workloads/tpch/dbgen"
	"github.com/stroppy-io/stroppy/v6/workloads/tpch/tpchgen"
)

const refreshBatchOrders = 100

var (
	errRefreshDriver   = errors.New("refresh functions require transactional retained sessions and logged tables")
	errRefreshScale    = errors.New("refresh-functions requires scale-factor in [0.01,100000]")
	errRefreshExisting = errors.New("existing data requires explicit refresh-set; use 1 only for unmodified data")
	errRefreshFresh    = errors.New("a fresh load requires refresh-set 1")
	errRefreshQuery    = errors.New("refresh stopped after query failure")
	errRefreshData     = errors.New("refresh dataset does not match refresh-set")
)

const refreshScopeReason = "RF-enabled phase only; " +
	"full sequencing, seed derivation, repeated runs and canonical scoring are absent"

type refreshAttempt struct {
	Set       int64   `json:"set"`
	Function  string  `json:"function"`
	Status    string  `json:"status"`
	Orders    int64   `json:"committed_orders"`
	Lineitems int64   `json:"committed_lineitems"`
	Seconds   float64 `json:"seconds"`
	Error     string  `json:"error,omitempty"`
	started   time.Time
	finished  time.Time
}

type refreshState struct {
	Mode               string           `json:"mode"`
	FirstSet           int64            `json:"first_set"`
	NextSet            int64            `json:"next_set,omitempty"`
	Pairs              int              `json:"expected_pairs"`
	Attempts           []refreshAttempt `json:"attempts"`
	Status             string           `json:"status"`
	MeasurementSeconds float64          `json:"measurement_seconds"`
	TPCCompliant       bool             `json:"tpc_compliant"`
	Reason             string           `json:"reason"`
}

func (w *workload) configureRefresh(d *bench.Def, selection streamSelection) error {
	if !selection.refresh {
		return nil
	}

	if !streamDriverSupported(w.driverType) || w.useUnlogged {
		return errRefreshDriver
	}

	if math.IsNaN(w.scaleFactor) || math.IsInf(w.scaleFactor, 0) || w.scaleFactor < 0.01 || w.scaleFactor > 100000 {
		return errRefreshScale
	}

	if !d.Execution.Enabled("load_data") && !selection.refreshSetExplicit {
		return errRefreshExisting
	}

	if d.Execution.Enabled("load_data") && selection.refreshSet != 1 {
		return errRefreshFresh
	}

	mode := "power"
	if selection.count > 1 {
		mode = "throughput"
	}

	w.refresh = &refreshState{
		Mode: mode, FirstSet: selection.refreshSet, NextSet: selection.refreshSet, Pairs: selection.count,
		Status: "not_started", Attempts: []refreshAttempt{}, Reason: refreshScopeReason,
	}
	w.refreshMetrics = declareRefreshMetrics(&d.Metrics)
	d.Report.Contribute("tpch.refresh", 1, w.refreshContribution)

	return nil
}

func declareRefreshMetrics(d *bench.MetricDeclarations) map[string]*queryMetrics {
	metrics := make(map[string]*queryMetrics)
	bounds := []float64{
		1, 10, 100, 1000, 10000, 60000, 300000, 1800000, 3600000, 14400000, 86400000,
		float64(time.Duration(1<<63-1)) / float64(time.Millisecond),
	}

	for _, name := range []string{"rf1", "rf2"} {
		prefix := "tpch_" + name
		metrics[name] = &queryMetrics{
			duration: d.Histogram(prefix+"_duration", bench.Unit("ms"), bench.Bounds(bounds...)),
			runs:     d.Counter(prefix + "_runs"), errors: d.Counter(prefix + "_errors"),
			elapsedTotal: d.Counter(prefix+"_elapsed_total", bench.Unit("ms")),
		}
	}

	return metrics
}

func (w *workload) executeRefreshStreams(d *bench.Def, dialect *mpp.Selection, selection streamSelection) {
	inserted, queried := make(chan struct{}), make(chan struct{})
	queryInitialize := w.streamInitializer(dialect)
	initialize := func(ctx context.Context, b *bench.Bench) (bench.Action, error) {
		if b.Worker() < selection.count {
			action, err := queryInitialize(ctx, b)
			if err == nil && selection.count == 1 {
				action = powerQueryAction(action, inserted, queried)
			}

			return action, err
		}

		if err := dialect.Step(func(context.Context, *bench.Bench) error { return nil })(ctx, b); err != nil {
			return nil, err
		}

		session, err := b.Session(ctx)
		if err != nil {
			return nil, err
		}

		dbgen.EnsureInit(w.scaleFactor)

		return w.refreshStreamAction(session, selection, inserted, queried), nil
	}
	d.Execution.WorkerStep("workload", initialize, bench.OncePerWorker(selection.count+1))
}

func powerQueryAction(action bench.Action, inserted <-chan struct{}, queried chan<- struct{}) bench.Action {
	return func(ctx context.Context, b *bench.Bench) error {
		if err := waitRefreshPhase(ctx, inserted); err != nil {
			return err
		}

		if err := action(ctx, b); err != nil {
			return err
		}

		close(queried)

		return nil
	}
}

func waitRefreshPhase(ctx context.Context, ready <-chan struct{}) error {
	select {
	case <-ready:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (w *workload) refreshStreamAction(
	session *bench.Session, selection streamSelection, inserted chan<- struct{}, queried <-chan struct{},
) bench.Action {
	return func(ctx context.Context, _ *bench.Bench) error {
		for pair := range selection.count {
			set := selection.refreshSet + int64(pair)
			if err := w.runRefresh(ctx, session, set, "rf1"); err != nil {
				return bench.Fatal(err)
			}

			if selection.count == 1 {
				close(inserted)

				if err := waitRefreshPhase(ctx, queried); err != nil {
					return err
				}
			}

			if w.queryFailures.Load() {
				return bench.Fatal(errRefreshQuery)
			}

			if err := w.runRefresh(ctx, session, set, "rf2"); err != nil {
				return bench.Fatal(err)
			}
		}

		return nil
	}
}

func nextRefreshSet(set int64) int64 {
	if set >= lastExecutableRefreshSet {
		return 0
	}

	return set + 1
}

func (w *workload) runRefresh(ctx context.Context, session *bench.Session, set int64, function string) (err error) {
	if set < 1 || set > lastExecutableRefreshSet {
		return errRefreshRange
	}

	attempt := refreshAttempt{Set: set, Function: function, Status: "running", started: time.Now()}
	defer func() {
		attempt.finished = time.Now()
		attempt.Seconds = attempt.finished.Sub(attempt.started).Seconds()
		attempt.Status = "completed"
		metrics := w.refreshMetrics[function]
		metrics.runs.Add(ctx, 1)
		metrics.duration.Record(ctx, attempt.Seconds*1000)
		metrics.elapsedTotal.Add(ctx, attempt.Seconds*1000)

		failed := 0.
		if err != nil {
			attempt.Status, attempt.Error, failed = "failed", err.Error(), 1
		}

		metrics.errors.Add(ctx, failed)

		w.refresh.Attempts = append(w.refresh.Attempts, attempt)
		if err == nil && function == "rf2" {
			w.refresh.NextSet = nextRefreshSet(set)
		}
	}()

	generator, err := tpchgen.NewRefreshGenerator(w.scaleFactor, set)
	if err != nil {
		return err
	}

	if function == "rf1" {
		return insertRefresh(ctx, session, generator, &attempt)
	}

	return deleteRefresh(ctx, session, generator, &attempt)
}

func (w *workload) refreshWindow() (first, last time.Time, complete bool) {
	complete = len(w.refresh.Attempts) == 2*w.refresh.Pairs && !w.queryFailures.Load()
	include := func(start, end time.Time) {
		if !start.IsZero() && (first.IsZero() || start.Before(first)) {
			first = start
		}

		if end.After(last) {
			last = end
		}
	}

	for i := range w.refresh.Attempts {
		attempt := &w.refresh.Attempts[i]
		complete = complete && attempt.Status == "completed"
		include(attempt.started, attempt.finished)
	}

	for i := range w.streams {
		stream := &w.streams[i]
		complete = complete && stream.Complete && stream.Errors == 0
		include(stream.started, stream.finished)
	}

	return first, last, complete
}

func (w *workload) refreshContribution(snapshot bench.ReportContext) (bench.ReportContribution, error) {
	value := *w.refresh

	value.Attempts = append([]refreshAttempt(nil), w.refresh.Attempts...)
	for i := range value.Attempts {
		value.Attempts[i].Error = snapshot.Diagnostic(value.Attempts[i].Error)
	}

	if len(value.Attempts) == 0 && snapshot.Status == report.StatusCompleted {
		value.NextSet = 0

		return bench.ReportContribution{
			Status: report.WorkloadReportSkipped, Reason: "refresh workload was not started", Data: value,
		}, nil
	}

	first, last, complete := w.refreshWindow()
	value.Status = "complete"

	status := report.WorkloadReportStatus("")
	if !complete || snapshot.Status != report.StatusCompleted {
		value.Status, value.NextSet, status = "incomplete", 0, report.WorkloadReportError
	}

	if !first.IsZero() && !last.IsZero() {
		value.MeasurementSeconds = last.Sub(first).Seconds()
	}

	return bench.ReportContribution{Status: status, Data: value}, nil
}
