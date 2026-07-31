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

	params map[string]map[string]any // final per-query params (end dates + q1 cutoff precomputed)
	m      map[string]*queryMetrics
}

type queryMetrics struct {
	duration     *bench.Metric
	runs         *bench.Metric
	errors       *bench.Metric
	elapsedTotal *bench.Metric
}

func init() { bench.Register(&workload{}) }

func (*workload) Name() string { return "tpch/tx" }

func (w *workload) Setup(ctx context.Context, b *bench.Bench) error {
	w.driverType = b.DriverTypeName()

	w.scaleFactor = bench.EnvFloat("SCALE_FACTOR", 1)
	if w.scaleFactor <= 0 {
		return fmt.Errorf("%w, got %v", errScaleFactorMustBePositive, w.scaleFactor)
	}

	w.loadWorkers = bench.EnvInt("LOAD_WORKERS", 0)
	w.useUnlogged = bench.Env("PG_UNLOGGED", "false") == "true" && w.driverType == bench.DriverPostgres
	w.ydbColumn = w.driverType == bench.DriverYDB && bench.Env("YDB_STORE_MODE", "column") == "column"
	w.isPicodata = w.driverType == bench.DriverPicodata
	w.needsEndDates = w.isPicodata || w.driverType == bench.DriverYDB
	w.sql = mustLoadSQL(w.driverType)

	// Per-query metrics (22 × 4).
	w.m = make(map[string]*queryMetrics, len(queryNames))
	for _, name := range queryNames {
		w.m[name] = &queryMetrics{
			duration:     b.Trend("tpch_" + name + "_duration"),
			runs:         b.Counter("tpch_" + name + "_runs"),
			errors:       b.Counter("tpch_" + name + "_errors"),
			elapsedTotal: b.Counter("tpch_" + name + "_elapsed_total"),
		}
	}

	// Final per-query params: base §2.4 values, with pico/ydb end dates and the q1
	// picodata shipdate_cutoff precomputed once.
	w.params = make(map[string]map[string]any, len(queryNames))
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

		w.params[name] = p
	}

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

	b.StepBegin("workload")

	return nil
}

func (w *workload) Iterate(ctx context.Context, b *bench.Bench) error {
	return b.Step("workload", func() error {
		w.runQueries(ctx, b)

		return nil
	})
}

// runQueries executes q1..q22 once each with pinned defaults, draining rows and
// recording per-query timing/error metrics. Rows are discarded (throughput pass).
func (w *workload) runQueries(ctx context.Context, b *bench.Bench) {
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
			lg.Infof("[tpch] %s: error in %dms %v", name, elapsed, err)

			continue
		}

		lg.Infof("[tpch] %s: ok in %dms", name, elapsed)
	}
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
	b.StepEnd("workload")

	return nil
}
