// Package tpch is the Go-native port of workloads/tpch/tx.ts: the relational load of
// the 8 TPC-H tables via the ported dbgen generator (bench.InsertTpch) plus the q1–q22
// business queries run once with §2.4 pinned defaults, and SF=1 answer validation
// (postgres only). Supports pg/mysql/pico/ydb dialect files; date shifts for pico/ydb
// are precomputed client-side.
package tpch

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/stroppy-io/stroppy/pkg/bench"
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

	params map[string]map[string]any // final per-query params (end dates + q1 cutoff precomputed)
	m      map[string]*queryMetrics
}

type queryMetrics struct {
	duration     *bench.Metric
	runs         *bench.Metric
	errors       *bench.Metric
	elapsedTotal *bench.Metric
}

func init() { bench.Register(func() bench.Workload { return &workload{} }) }

func (*workload) Name() string { return "tpch/tx" }

func (w *workload) Define(d *bench.Def) error {
	w.scaleFactor = d.Param.Float64("scale-factor", 1, "TPC-H scale factor.").Value()
	w.loadWorkers = d.Param.Int("load-workers", 0, "Workers used to load each table.").Value()
	w.useUnlogged = d.Param.Bool("pg-unlogged", false, "Use unlogged PostgreSQL tables while loading.").Value()
	w.ydbStoreMode = d.Param.String("ydb-store-mode", "column", "YDB table store mode.").Value()
	w.sqlFile = d.Param.String("sql-file", "", "SQL dialect file override.").Value()

	if w.scaleFactor <= 0 {
		return fmt.Errorf("%w, got %v", errScaleFactorMustBePositive, w.scaleFactor)
	}

	return nil
}

func (w *workload) Setup(ctx context.Context, b *bench.Bench) error {
	w.driverType = b.DriverTypeName()

	w.initConfig()

	// Per-query metrics (22 × 4).
	w.m = w.initMetrics(b)

	// Final per-query params: base §2.4 values, with pico/ydb end dates and the
	// q1 picodata shipdate_cutoff precomputed once.
	w.params = w.buildParams()

	runSection := func(name string) error {
		for _, q := range w.sql.Section(name) {
			if err := b.Exec(ctx, q, nil); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
		}

		return nil
	}

	type step struct {
		name string
		fn   func() error
	}

	var steps []step

	addStep := func(name string, fn func() error) { steps = append(steps, step{name, fn}) }

	addStep("drop_schema", func() error { return runSection("drop_schema") })
	addStep("create_schema", func() error {
		section := "create_schema"
		if w.ydbColumn {
			section = "create_schema_column"
		}

		return runSection(section)
	})

	if w.useUnlogged {
		addStep("set_unlogged", func() error { return runSection("set_unlogged") })
	}

	addStep("load_data", func() error {
		for _, table := range tpchTables {
			if _, err := b.InsertTpch(ctx, table, w.scaleFactor, w.loadWorkers); err != nil {
				return err
			}
		}

		return nil
	})
	addStep("create_indexes", func() error { return runSection("create_indexes") })

	if w.useUnlogged {
		addStep("set_logged", func() error { return runSection("set_logged") })
	}

	addStep("analyze", func() error { return runSection("analyze") })
	addStep("validate_answers", func() error {
		validateAnswers(ctx, b, w.sql, w.params, w.scaleFactor, w.driverType)

		return nil
	})

	for _, s := range steps {
		if err := b.Step(s.name, s.fn); err != nil {
			return err
		}
	}

	return nil
}

// initConfig resolves driver-specific workload configuration.
func (w *workload) initConfig() {
	w.useUnlogged = w.useUnlogged && w.driverType == bench.DriverPostgres
	w.ydbColumn = w.driverType == bench.DriverYDB && w.ydbStoreMode == "column"
	w.isPicodata = w.driverType == bench.DriverPicodata
	w.needsEndDates = w.isPicodata || w.driverType == bench.DriverYDB
	w.sql = mustLoadSQL(w.driverType, w.sqlFile)
}

// initMetrics wires the per-query duration/counters (22 × 4).
func (w *workload) initMetrics(b *bench.Bench) map[string]*queryMetrics {
	m := make(map[string]*queryMetrics, len(queryNames))
	for _, name := range queryNames {
		m[name] = &queryMetrics{
			duration:     b.Trend("tpch_" + name + "_duration"),
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

func (w *workload) Iterate(ctx context.Context, b *bench.Bench) error {
	return b.StepSilent("workload", func() error {
		return w.runQueries(ctx, b)
	})
}

// runQueries executes q1..q22 once each with pinned defaults, draining rows and
// recording per-query timing/error metrics. Rows are discarded (throughput pass).
func (w *workload) runQueries(ctx context.Context, b *bench.Bench) error {
	lg := b.Logger().Sugar()

	for _, name := range queryNames {
		body, ok := w.sql.Query(name, "body")
		if !ok {
			lg.Infof("[tpch] %s: skipped (no body in SQL file)", name)

			continue
		}

		start := time.Now()
		_, err := b.QueryRows(ctx, body, w.params[name])
		elapsed := time.Since(start).Milliseconds()
		w.recordAttempt(name, float64(elapsed), err != nil)

		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}

			b.RecordQueryError(name, err)

			continue
		}

		lg.Infof("[tpch] %s: ok in %dms", name, elapsed)
	}

	return nil
}

func (w *workload) recordAttempt(name string, elapsedMs float64, failed bool) {
	qm := w.m[name]
	if qm == nil {
		return
	}

	qm.runs.Add(1)
	qm.duration.Add(elapsedMs)
	qm.elapsedTotal.Add(elapsedMs)

	if failed {
		qm.errors.Add(1)
	}
}

func (*workload) Teardown(_ context.Context, b *bench.Bench) error {
	return nil
}
