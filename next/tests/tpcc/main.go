// Command tpcc is the stroppy-next TPC-C port: the flagship acceptance test for
// the engine. It loads a TPC-C dataset (§4.3.3 population rules) via parallel
// columnar COPY, runs the five-transaction mix (§2.4-2.8) as a closed loop of
// warehouse-bound terminals, and validates population and consistency — all as a
// single step DAG.
//
//	STROPPY_DRIVER_URL=postgres://stroppy@127.0.0.1:5432/postgres go run ./tests/tpcc
//	WAREHOUSES=2 DURATION=30s go run ./tests/tpcc
//	go run ./tests/tpcc -plan        # print the step DAG
//	go run ./tests/tpcc -probe       # machine-readable description
//
// # SDK gap (per-transaction metrics)
//
// The built-in per-step servicetime histogram aggregates all five transaction
// types. Per-transaction-type latency histograms are not available: user
// instruments must be registered before the metrics registry freezes (at
// executor materialize, before any Handler.Init), but a Handler has no access to
// the registry — Test/VU expose no registration hook. This port therefore counts
// per-transaction outcomes in per-VU state and reports counts only (see report).
// A registration hook reachable from Init (or a Test-level instrument
// declaration) is the M7 fix.
package main

import (
	"fmt"
	"log"
	"time"

	_ "embed"

	"github.com/stroppy-io/stroppy/next/bench"
	"github.com/stroppy-io/stroppy/next/driver"
	"github.com/stroppy-io/stroppy/next/sqlfile"
)

//go:embed pg.sql
var pgSQL []byte

//go:embed tpcc.sql
var extraSQL []byte

// tpccSeed is the run's declared root seed. The world's generation constants and
// the SDK's per-step rng streams (via Test.Seed) both derive from it, so they
// stay consistent. The -seed flag overrides the SDK streams but not the world
// constants; the built-in tests use the default, and a run is reproducible for a
// fixed seed regardless.
const tpccSeed = 1

// options are the tpcc tunables, filled from the environment by the SDK.
type options struct {
	Warehouses  int64         `env:"WAREHOUSES" default:"1"`
	LoadWorkers int           `env:"LOAD_WORKERS" default:"4"`
	VUs         int           `env:"VUS" default:"4"`
	Duration    time.Duration `env:"DURATION" default:"30s"`
	TxIsolation string        `env:"TX_ISOLATION" default:"read_committed"`
	DoValidate  bool          `env:"VALIDATE" default:"true"`
}

// Validate enforces sane bounds and a known isolation level.
func (o *options) Validate() error {
	if o.Warehouses < 1 {
		return fmt.Errorf("WAREHOUSES must be >= 1, got %d", o.Warehouses)
	}
	if o.LoadWorkers < 1 {
		return fmt.Errorf("LOAD_WORKERS must be >= 1, got %d", o.LoadWorkers)
	}
	if o.VUs < 1 {
		return fmt.Errorf("VUS must be >= 1, got %d", o.VUs)
	}
	if _, ok := isolationByName(o.TxIsolation); !ok {
		return fmt.Errorf("TX_ISOLATION %q is not a known level", o.TxIsolation)
	}
	return nil
}

// isolationByName maps a TX_ISOLATION string to a driver.Isolation.
func isolationByName(name string) (driver.Isolation, bool) {
	switch name {
	case "db_default":
		return driver.DBDefault, true
	case "read_uncommitted":
		return driver.ReadUncommitted, true
	case "read_committed":
		return driver.ReadCommitted, true
	case "repeatable_read":
		return driver.RepeatableRead, true
	case "serializable":
		return driver.Serializable, true
	case "conn":
		return driver.ConnectionOnly, true
	case "none":
		return driver.None, true
	default:
		return 0, false
	}
}

func main() {
	o := &options{}
	if err := bench.LoadOptions(o); err != nil {
		log.Fatalf("tpcc: %v", err)
	}
	iso, _ := isolationByName(o.TxIsolation)

	file, err := sqlfile.Parse(pgSQL)
	if err != nil {
		log.Fatalf("tpcc: parse pg.sql: %v", err)
	}
	extra, err := sqlfile.Parse(extraSQL)
	if err != nil {
		log.Fatalf("tpcc: parse tpcc.sql: %v", err)
	}

	t := &bench.Test{
		Name:    "tpcc",
		Seed:    tpccSeed,
		Opts:    o,
		Drivers: []bench.DriverSlot{{Name: "main", Kind: "pg"}},
		Steps:   buildSteps(o, iso, file, extra),
	}
	bench.Main(t)
}

// buildSteps assembles the tpcc step DAG:
//
//	drop_schema(Silent) -> create_schema -> load_* (Pool, per table, concurrent)
//	  -> create_indexes -> validate_population(If VALIDATE)
//	  -> workload(Closed) -> {check_consistency(If VALIDATE), report}
//
// workload uses AfterAny(validate_population, create_indexes) so a skipped
// validation (VALIDATE=false) still lets the workload run — a Skipped node fails
// an After gate but AfterAny is satisfied by create_indexes succeeding, while
// still ordering the workload after validation completes.
func buildSteps(o *options, iso driver.Isolation, file, extra *sqlfile.File) []*bench.StepDef {
	w := newWorld(tpccSeed, o.Warehouses)
	q := resolveTxQueries(file, extra)

	dropQs := file.Section("drop_schema")
	createQs := file.Section("create_schema")
	idxQs := file.Section("create_indexes")

	steps := []*bench.StepDef{
		bench.Step("drop_schema", multiExec(dropQs)).OnErr(bench.Silent).Uses("main"),
		bench.Step("create_schema", multiExec(createQs)).After("drop_schema").Uses("main"),
	}

	// One Pool load step per table (item once; the rest scale with W). Row
	// content is keyed by the global cycle, so the chunk count (sized by
	// LOAD_WORKERS) changes only parallelism, never the data.
	nChunks := o.LoadWorkers * 8
	var loadNames []string
	for _, tbl := range tables() {
		items := chunkRanges(tbl.cycles(w), nChunks)
		steps = append(steps,
			bench.Step(tbl.step(), &loadHandler{w: w, tbl: tbl}).
				Pool(o.LoadWorkers, items...).
				After("create_schema").Uses("main"))
		loadNames = append(loadNames, tbl.step())
	}

	steps = append(steps,
		bench.Step("create_indexes", multiExec(idxQs)).After(loadNames...).Uses("main"),

		bench.Step("validate_population", validatePopulation(w)).
			After("create_indexes").
			If(func(r *bench.Run) bool { return o.DoValidate }).
			Uses("main"),

		bench.Step("workload", &workloadHandler{w: w, q: q, iso: iso}).
			Closed(o.VUs, o.Duration).
			AfterAny("validate_population", "create_indexes").
			Retry(bench.RetryPolicy{MaxAttempts: 3, Retryable: driver.IsRetryable}).
			Uses("main"),

		bench.Step("check_consistency", checkConsistency()).
			After("workload").
			If(func(r *bench.Run) bool { return o.DoValidate }).
			Uses("main"),

		bench.Step("report", report(o)).After("workload").Uses("main"),
	)
	return steps
}

// multiExec returns a run-once handler that prepares and executes every query in
// order for side effect (DDL groups: drop, create, index).
func multiExec(qs []*sqlfile.Query) bench.Handler {
	return bench.FuncOnce(func(vu *bench.VU) error {
		conn := vu.Conn(vu.Slot())
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
