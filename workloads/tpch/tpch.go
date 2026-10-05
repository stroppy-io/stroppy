// Package tpch owns Stroppy's TPC-H implementation, tests, dialect SQL, and
// answer data. It loads eight tables through the canonical generator and runs
// q1–q22 once with §2.4 pinned defaults, and SF=1 answer validation
// (postgres only). Supports pg/mysql/pico/ydb dialect files; date shifts for pico/ydb
// are precomputed client-side.
package tpch

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/workloads/tpch/tpchgen"
)

var errScaleFactorMustBePositive = errors.New("SCALE_FACTOR must be positive")

type workload struct {
	sql           *bench.SQL
	driverType    bench.DriverTypeName
	isPicodata    bool
	needsEndDates bool
	scaleFactor   float64
	loadWorkers   int
	useUnlogged   bool
	ydbColumn     bool
	ydbStoreMode  string
	sqlFile       string

	params     map[string]map[string]any // final per-query params (end dates + q1 cutoff precomputed)
	m          map[string]*queryMetrics
	validation validationReport
}

type queryMetrics struct {
	duration     *bench.HistogramHandle
	runs         *bench.CounterHandle
	errors       *bench.CounterHandle
	elapsedTotal *bench.CounterHandle
}

var Test = bench.Test{Name: "tpch/tx", Define: define}

func init() { bench.Register(Test) }
func define(d *bench.Def) error {
	settings := bench.RunParameters(&d.Param, bench.RunDefaults{})
	w := &workload{}
	d.Report.Contribute("tpch.validation", 1, w.validationContribution)
	w.scaleFactor, _ = d.Param.Float64("scale-factor", 1, "TPC-H scale factor.")
	w.loadWorkers, _ = d.Param.Int("load-workers", 0, "Workers used to load each table.")
	w.useUnlogged, _ = d.Param.Bool("pg-unlogged", false, "Use unlogged tables while loading.")
	w.ydbStoreMode, _ = d.Param.String("ydb-store-mode", "column", "YDB table store mode.")

	w.sqlFile, _ = d.Param.String("sql-file", "", "SQL dialect file override.")
	if w.scaleFactor <= 0 {
		return fmt.Errorf("%w, got %v", errScaleFactorMustBePositive, w.scaleFactor)
	}

	db := d.Drivers.Declare("default", bench.DriverConfig{})
	w.driverType = db.Kind()
	w.useUnlogged = w.useUnlogged && w.driverType == bench.DriverPostgres
	w.ydbColumn = w.driverType == bench.DriverYDB && w.ydbStoreMode == "column"
	w.isPicodata = w.driverType == bench.DriverPicodata
	w.needsEndDates = w.isPicodata || w.driverType == bench.DriverYDB

	var err error
	if w.sqlFile != "" {
		w.sql, err = d.Queries.Override(w.sqlFile)
	} else {
		w.sql, err = d.Queries.Load(files, sqlFile(w.driverType, ""))
	}

	if err != nil {
		return err
	}

	w.m = w.initMetrics(&d.Metrics)

	w.params = w.buildParams()
	for _, name := range []string{"drop_schema", "create_schema"} {
		section := sqlSection{w, name}
		d.Execution.Step(name, section.Run)
	}

	if w.useUnlogged {
		section := sqlSection{w, "set_unlogged"}
		d.Execution.Step(section.name, section.Run)
	}

	d.Execution.Step("load_data", w.load)
	section := sqlSection{w, "create_indexes"}
	d.Execution.Step(section.name, section.Run)

	if w.useUnlogged {
		loggedSection := sqlSection{w, "set_logged"}
		d.Execution.Step(loggedSection.name, loggedSection.Run)
	}

	section = sqlSection{w, "analyze"}
	d.Execution.Step(section.name, section.Run)
	d.Execution.Step("validate_answers", w.validate)
	d.Execution.Step("workload", w.runQueries, settings.Policy())

	return d.Execution.Err()
}

type sqlSection struct {
	workload *workload
	name     string
}

func (s sqlSection) Run(ctx context.Context, b *bench.Bench) error {
	section := s.name
	if section == "create_schema" && s.workload.ydbColumn {
		section = "create_schema_column"
	}

	for _, q := range s.workload.sql.Section(section) {
		if err := b.Exec(ctx, q, nil); err != nil {
			return fmt.Errorf("%s: %w", section, err)
		}
	}

	return nil
}

func (w *workload) load(ctx context.Context, b *bench.Bench) error {
	for _, table := range tpchTables {
		source, err := tpchgen.NewBatchSource(table, w.scaleFactor)
		if err != nil {
			return err
		}

		if _, err = b.Insert(ctx, table, source, bench.LoadWorkers(max(w.loadWorkers, 1))); err != nil {
			return err
		}
	}

	return nil
}

func (w *workload) validate(ctx context.Context, b *bench.Bench) error {
	w.validation = validateAnswers(ctx, b, w.sql, w.params, w.scaleFactor, w.driverType)

	return nil
}

// initMetrics wires the per-query duration/counters (22 × 4).
func (w *workload) initMetrics(b *bench.MetricDeclarations) map[string]*queryMetrics {
	m := make(map[string]*queryMetrics, len(queryNames))
	for _, name := range queryNames {
		m[name] = &queryMetrics{
			duration:     b.Histogram("tpch_" + name + "_duration"),
			runs:         b.Counter("tpch_" + name + "_runs"),
			errors:       b.Counter("tpch_" + name + "_errors"),
			elapsedTotal: b.Counter("tpch_" + name + "_elapsed_total"),
		}
	}

	return m
}

// buildParams assembles per-query params: base §2.4 values, with pico/ydb end
// dates and the q1 picodata shipdate_cutoff precomputed once.
func (w *workload) buildParams() map[string]map[string]any {
	params := make(map[string]map[string]any, len(queryNames))
	base := queryParams(w.scaleFactor)

	for _, name := range queryNames {
		p := map[string]any{}
		for k, v := range base[name] {
			p[k] = v
		}

		p = withEndDates(p, w.needsEndDates)
		if name == "q1" && w.isPicodata {
			p["shipdate_cutoff"] = shiftDate("1998-12-01", -90, 0, 0)
		}

		params[name] = p
	}

	return params
}

// runQueries executes q1..q22 once each with pinned defaults, draining rows and
// recording per-query timing/error metrics. Rows are discarded (throughput pass).
func (w *workload) runQueries(ctx context.Context, b *bench.Bench) error {
	lg := b.Log

	for _, name := range queryNames {
		body, ok := w.sql.Query(name, "body")
		if !ok {
			lg.Info("query skipped", "query", name)

			continue
		}

		start := time.Now()
		_, err := b.RawRows(ctx, body, w.params[name])
		elapsed := time.Since(start).Milliseconds()
		w.recordAttempt(ctx, name, float64(elapsed), err != nil)

		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}

			b.RecordError(name, err)

			continue
		}

		lg.Info("query completed", "query", name, "milliseconds", elapsed)
	}

	return nil
}

func (w *workload) recordAttempt(ctx context.Context, name string, elapsedMs float64, failed bool) {
	qm := w.m[name]
	if qm == nil {
		return
	}

	qm.runs.Add(ctx, 1)
	qm.duration.Record(ctx, elapsedMs)
	qm.elapsedTotal.Add(ctx, elapsedMs)

	if failed {
		qm.errors.Add(ctx, 1)
	}
}

func (*workload) Teardown(_ context.Context, b *bench.Bench) error {
	return nil
}
