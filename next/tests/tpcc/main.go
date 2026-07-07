// Command tpcc is the stroppy-next TPC-C port: the flagship acceptance test for
// the engine. It loads a TPC-C dataset (§4.3.3 population rules) via parallel
// columnar COPY, runs the five-transaction mix (§2.4-2.8) as a closed loop of
// warehouse-bound terminals, and validates population and consistency — all
// declared as a single Define pass and run as one step DAG.
//
//	STROPPY_DRIVER_URL=postgres://stroppy@127.0.0.1:5432/postgres go run ./tests/tpcc
//	WAREHOUSES=2 DURATION=30s go run ./tests/tpcc
//	go run ./tests/tpcc -plan        # print the step DAG
//	go run ./tests/tpcc -probe       # machine-readable description
//
// # SDK gap (per-transaction metrics) — instruments declared, recording lands in D6
//
// The built-in per-step servicetime histogram aggregates all five transaction
// types. The D7 Define spine now lets an author declare per-tx instruments
// ([bench.Def.Histogram]/[Counter]) before the metrics registry freezes, but the
// phase-3 registration that assigns their handles is D6's job, so this port
// still counts per-transaction outcomes in per-VU state and reports counts only
// (see report).
package main

import (
	"fmt"
	"time"

	_ "embed"

	"github.com/stroppy-io/stroppy/next/bench"
	"github.com/stroppy-io/stroppy/next/driver"
	"github.com/stroppy-io/stroppy/next/sqlfile"
)

//go:embed tpcc.sql
var tpccSQL []byte

// tpccSeed is the spec-representative root seed in its uint64 form: world-gen
// and executor configs (repro_test) consume it directly. The seed param's
// "canonical"/"fixed" keyword (F6) resolves to canonicalSeed, its string form,
// which is Test.Seed. TPC-C's canonical seed is 1.
const (
	tpccSeed       uint64 = 1
	canonicalSeed        = "1"
)

// options are the tpcc tunables. Define declares each as a typed param and
// copies the resolved value here so the legacy helpers (buildSteps, report)
// read them unchanged.
type options struct {
	Warehouses  int64
	LoadWorkers int
	VUs         int
	Duration    time.Duration
	TxIsolation string
	DoValidate  bool
}

// validate enforces sane bounds and a known isolation level.
func (o *options) validate() error {
	if o.Warehouses < 1 {
		return fmt.Errorf("WAREHOUSES must be >= 1, got %d", o.Warehouses)
	}
	if o.LoadWorkers < 1 {
		return fmt.Errorf("LOAD_WORKERS must be >= 1, got %d", o.LoadWorkers)
	}
	if o.VUs < 1 {
		return fmt.Errorf("VUS must be >= 1, got %d", o.VUs)
	}
	if _, ok := driver.ParseIsolation(o.TxIsolation); !ok {
		return fmt.Errorf("TX_ISOLATION %q is not a known level", o.TxIsolation)
	}
	return nil
}

func main() {
	o := &options{}
	t := &bench.Test{
		Name: "tpcc",
		Seed: canonicalSeed,
		Define: func(d *bench.Def) error {
			// One declarative pass: each param resolves at registration, so the
			// values feed straight into validation, the world and the executor
			// policies below.
			o.Warehouses = d.Param.Int64("warehouses", 1, "warehouse count (scale factor)").Value()
			o.LoadWorkers = d.Param.Int("load_workers", 4, "parallel COPY workers per load step").Value()
			o.VUs = d.Param.Int("vus", 4, "closed-loop virtual users (terminals per warehouse)").Value()
			o.Duration = d.Param.Duration("duration", 30*time.Second, "workload duration").Value()
			o.TxIsolation = d.Param.String("tx_isolation", "read_committed", "transaction isolation level").Value()
			o.DoValidate = d.Param.Bool("validate", true, "run population and consistency validation").Value()
			if err := o.validate(); err != nil {
				return err
			}

			// tpcc.sql is the reference (pg) dialect today, registered as the
			// generic fallback so every kind resolves without an override; the
			// SDK resolves per active kind (override -> per-kind -> generic).
			qs := d.Queries("tpcc", tpccSQL)
			file, err := qs.File()
			if err != nil {
				return fmt.Errorf("tpcc: %w", err)
			}

			iso, _ := driver.ParseIsolation(o.TxIsolation)
			d.Driver("main", "pg")
			return buildSteps(d, o, d.Seed(), iso, file)
		},
	}
	bench.Main(t)
}

// buildSteps assembles the tpcc step DAG against d:
//
//	drop_schema(Silent) -> create_schema -> load_* (Pool, per table, concurrent)
//	  -> create_indexes -> validate_population(If VALIDATE)
//	  -> workload(Closed) -> {check_consistency(If VALIDATE), report}
//
// workload uses AfterAny(validate_population, create_indexes) so a skipped
// validation (VALIDATE=false) still lets the workload run — a Skipped node fails
// an After gate but AfterAny is satisfied by create_indexes succeeding, while
// still ordering the workload after validation completes.
func buildSteps(d *bench.Def, o *options, seed uint64, iso driver.Isolation, file *sqlfile.File) error {
	w := newWorld(seed, o.Warehouses)
	q := resolveTxQueries(file)

	dropQs := file.Section("drop_schema")
	createQs := file.Section("create_schema")
	idxQs := file.Section("create_indexes")

	d.Step("drop_schema", multiExec(dropQs)).OnErr(bench.ModeSilent).Uses("main")
	d.Step("create_schema", multiExec(createQs)).After("drop_schema").Uses("main")

	// One Pool load step per table (item once; the rest scale with W). Row
	// content is keyed by the global cycle, so the chunk count (sized by
	// LOAD_WORKERS) changes only parallelism, never the data.
	nChunks := o.LoadWorkers * 8
	loadNames := make([]string, 0, len(tables()))
	for _, tbl := range tables() {
		items := chunkRanges(tbl.cycles(w), nChunks)
		d.Step(tbl.step(), &loadHandler{w: w, tbl: tbl}).
			Pool(o.LoadWorkers, items...).
			After("create_schema").Uses("main")
		loadNames = append(loadNames, tbl.step())
	}

	d.Step("create_indexes", multiExec(idxQs)).After(loadNames...).Uses("main")

	d.Step("validate_population", validatePopulation(w)).
		After("create_indexes").
		If(func(r *bench.Run) bool { return o.DoValidate }).
		Uses("main")

	d.Step("workload", &workloadHandler{w: w, q: q, iso: iso}).
		Closed(o.VUs, o.Duration).
		AfterAny("validate_population", "create_indexes").
		Uses("main")

	d.Step("check_consistency", checkConsistency()).
		After("workload").
		If(func(r *bench.Run) bool { return o.DoValidate }).
		Uses("main")

	d.Step("report", report(o)).After("workload").Uses("main")

	d.Variant("full")
	return nil
}

// multiExec returns a run-once handler that prepares and executes every query in
// order for side effect (DDL groups: drop, create, index).
func multiExec(qs []*sqlfile.Query) bench.Handler {
	return bench.FuncOnce(func(vu *bench.VU) error {
		conn, err := vu.Conn()
		if err != nil {
			return err
		}
		for _, q := range qs {
			st, err := conn.Prepare(vu.Ctx(), q)
			if err != nil {
				return err
			}
			if err := conn.Exec(vu.Ctx(), st); err != nil {
				return err
			}
		}
		return nil
	})
}
