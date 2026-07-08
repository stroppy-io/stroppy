// Command tpch is the stroppy-next TPC-H port. It lifts the vendored dbgen
// generator (next/tests/tpch/dbgen) into the SDK's load shape, bulk-loads the 8
// relational tables via parallel columnar COPY, and runs the 22 business queries
// (§2.4) as a single Once pass with per-query servicetime metrics. SF=1 answer
// validation against the embedded reference answers is a skippable step.
//
//	STROPPY_DRIVER_URL=postgres://stroppy@127.0.0.1:5432/stroppy go run ./tests/tpch
//	SCALE_FACTOR=0.01 go run ./tests/tpch           # smoke load
//	SCALE_FACTOR=1 go run ./tests/tpch -plan        # the SF=1 DAG
//	go run ./tests/tpch -variant=query              # query pass over loaded data
//
// # Variants (D3b)
//
// load    — schema + 8-table bulk load + indexes + ANALYZE (prep only).
// query   — the q1..q22 workload pass (assumes data already loaded).
// full    — load → validate_answers → workload (the default).
//
// # Per-query metrics (D6/F5)
//
// servicetime (Histogram), query_runs and query_errors (Counters), each tagged
// q1..q22, are declared in Define and recorded through the workload's
// TxRecorder-free path (the workload is a Once pass, not a closed loop). The
// QuerySink renders the per-query breakdown from the final Report — the print
// side-channel this kind of port used to carry is gone.
//
// # Seed carve-out (F6)
//
// TPC-H data IS the spec: dbgen's RNG seeds are fixed by the TPC-H benchmark
// definition (initSeeds in dbgen/rand.go), and the dataset they produce is the
// reference the SF=1 answers are computed against. The stroppy --seed flag
// therefore does NOT reach tpch data generation — it cannot, without diverging
// from the canonical dataset and invalidating the answers. This is the same kind
// of carve-out as tpcc's loadTimestamp: a fixed, spec-pinned value that the run
// seed does not perturb. --seed still threads through stroppy's rng streams for
// any non-data draws; tpch simply has none.
package main

import (
	"fmt"
	"io"

	_ "embed"

	"github.com/stroppy-io/stroppy/next/bench"
	"github.com/stroppy-io/stroppy/next/metrics"
	"github.com/stroppy-io/stroppy/next/sqlfile"
	"github.com/stroppy-io/stroppy/next/tests/tpch/dbgen"
)

//go:embed tpch.sql
var tpchSQL []byte

// scaleFactor is the resolved SCALE_FACTOR param. It is set once in Define and
// read by the q11 fraction binding (the threshold scales by 1/SF). Package-level
// because the queryParams map builds its closures at init time and captures this
// variable by reference.
var scaleFactor float64

// options are the tpch tunables, copied from the resolved params so the DAG
// builder and predicates read them unchanged.
type options struct {
	ScaleFactor float64
	LoadWorkers int
	PGUnlogged  bool
	DoValidate  bool
}

func (o *options) validate() error {
	if o.ScaleFactor <= 0 {
		return fmt.Errorf("SCALE_FACTOR must be > 0, got %g", o.ScaleFactor)
	}
	if o.LoadWorkers < 1 {
		return fmt.Errorf("LOAD_WORKERS must be >= 1, got %d", o.LoadWorkers)
	}
	return nil
}

func main() {
	o := &options{}
	t := &bench.Test{
		Name: "tpch",
		// Seed is inert for tpch data: dbgen's spec seeds own the dataset (see
		// the package doc carve-out). "0" is a valid no-op root for stroppy's
		// own streams, which tpch's imperative generator does not consult.
		Seed: "0",
		Define: func(d *bench.Def) error {
			o.ScaleFactor = d.Param.Float64("scale_factor", 1, "TPC-H scale factor (0.01 for smoke, 1 for canonical)").Value()
			scaleFactor = o.ScaleFactor
			o.LoadWorkers = d.Param.Int("load_workers", 4, "parallel COPY workers per load step").Value()
			o.PGUnlogged = d.Param.Bool("pg_unlogged", true, "pg only: bulk-load with UNLOGGED tables, flip to LOGGED after").Value()
			o.DoValidate = d.Param.Bool("validate", true, "run SF=1 answer validation against the reference answers").Value()
			if err := o.validate(); err != nil {
				return err
			}

			// tpch.sql is the pg reference dialect, registered as the generic
			// fallback so every kind resolves without an override.
			qs := d.Queries("tpch", tpchSQL)
			file, err := qs.File()
			if err != nil {
				return fmt.Errorf("tpch: %w", err)
			}

			// Per-query instruments (D6/F5): servicetime, runs, errors, each
			// fanned out across q1..q22. The tag name "query" maps to the
			// Instrument.Tx field, so the QuerySink reads them back by Tx.
			queryTag := bench.Tag("query", queryNames...)
			lat := d.Histogram(servicetimeInst, queryTag)
			runs := d.Counter(queryRunsInst, queryTag)
			errs := d.Counter(queryErrorsInst, queryTag)

			d.Driver("main", "pg")
			return buildSteps(d, o, file, lat, runs, errs)
		},
		WrapSink: func(defaultSink metrics.Sink, stdout io.Writer) metrics.Sink {
			return metrics.MultiSink{defaultSink, newQuerySink(stdout)}
		},
	}
	bench.Main(t)
}

// buildSteps assembles the tpch step DAG and the load/query/full variants.
//
//	drop_schema(Silent) -> create_schema -> set_unlogged(If pg, Skippable)
//	  -> load_region..load_lineitem (Pool, per table, concurrent)
//	  -> create_indexes -> set_logged(If pg, Skippable) -> analyze(Skippable)
//	  -> validate_answers(If SF=1 && pg && VALIDATE, Skippable)
//	  -> workload(Once, q1..q22)
//
// The UNLOGGED flip is two If-gated Once steps around the load (pg only): the
// step kind predicate reads Run.DriverKindByName("main"). A skipped
// set_unlogged/set_logged/validate_answers unblocks its After-dependents (F3), so
// plain After orders the chain without an AfterAny workaround.
func buildSteps(d *bench.Def, o *options, file *sqlfile.File, lat *bench.Histogram, runs, errs *bench.Counter) error {
	// dbgen's scale-dependent globals (tDefs, distributions, text pool, per-table
	// ranges) are lazy-initialized by EnsureInit; declareLoad reads BaseRowCount
	// (tDefs), so init must precede it. Idempotent — re-inits if SF changed.
	dbgen.EnsureInit(o.ScaleFactor)
	dropQs := file.Section("drop_schema")
	createQs := file.Section("create_schema")
	unloggedQs := file.Section("set_unlogged")
	loggedQs := file.Section("set_logged")
	idxQs := file.Section("create_indexes")
	analyzeQs := file.Section("analyze")

	isPG := func(r *bench.Run) bool { return r.DriverKindByName("main") == "pg" }
	doUnlogged := func(r *bench.Run) bool { return isPG(r) && o.PGUnlogged }
	doValidate := func(r *bench.Run) bool { return isPG(r) && o.DoValidate && o.ScaleFactor == 1 }

	dropSD := d.Step("drop_schema", multiExec(dropQs)).OnErr(bench.ModeSilent).Uses("main")
	createSD := d.Step("create_schema", multiExec(createQs)).After("drop_schema").Uses("main")
	setUnloggedSD := d.Step("set_unlogged", multiExec(unloggedQs)).
		After("create_schema").If(doUnlogged).Skippable().Uses("main")

	// One Pool load step per table; row content is keyed by the global entity
	// index, so the chunk count (LOAD_WORKERS) changes only parallelism.
	loadSDs := make([]*bench.StepDef, 0, len(tpchTables()))
	loadNames := make([]string, 0, len(tpchTables()))
	for _, tbl := range tpchTables() {
		sd := declareLoad(d, tbl, o.ScaleFactor, o.LoadWorkers, 8).
			After("set_unlogged").Uses("main")
		loadSDs = append(loadSDs, sd)
		loadNames = append(loadNames, tbl.step())
	}

	createIdxSD := d.Step("create_indexes", multiExec(idxQs)).
		After(loadNames...).Uses("main")
	setLoggedSD := d.Step("set_logged", multiExec(loggedQs)).
		After("create_indexes").If(doUnlogged).Skippable().Uses("main")
	analyzeSD := d.Step("analyze", multiExec(analyzeQs)).
		After("create_indexes").Skippable().Uses("main")
	d.Step("validate_answers", validateAnswers(file)).
		After("analyze").If(doValidate).Skippable().Uses("main")

	workloadSD := d.Step("workload", &workloadHandler{file: file, lat: lat, runs: runs, errs: errs}).
		After("validate_answers").Uses("main")

	// Variants (D3b): load = prep only; query = workload over loaded data; full
	// = all steps (the default, empty step set means all).
	d.Variant("load", append([]*bench.StepDef{dropSD, createSD, setUnloggedSD},
		append(loadSDs, createIdxSD, setLoggedSD, analyzeSD)...)...)
	d.Variant("query", workloadSD)
	d.Variant("full")
	return nil
}

// multiExec returns a run-once handler that prepares and executes every query in
// a section for side effect (DDL groups: drop, create, index, analyze, unlogged
// flip). A missing/empty section is a no-op (D3: missing section → skip).
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
