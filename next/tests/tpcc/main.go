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
// # Per-transaction metrics (D6)
//
// The workload declares per-tx instruments in Define — tx_latency (Histogram)
// and tx_count (Counter), each tagged with the five tx names — and records
// whole-tx wall-clock latency through bench.Transaction's TxRecorder plus one
// counter Inc per completed tx. The MixSink reads those tx-tagged instruments
// from the final Report and formats the transaction-mix table + tpmC, replacing
// the count-only print side-channel this port used to carry.
package main

import (
	"fmt"
	"io"
	"time"

	_ "embed"

	"github.com/stroppy-io/stroppy/next/bench"
	"github.com/stroppy-io/stroppy/next/driver"
	"github.com/stroppy-io/stroppy/next/metrics"
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
		Name:  "tpcc",
		Seed:  canonicalSeed,
		Retry: txRetry,
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

			// Per-tx instruments (D6/F5): one latency histogram and one counter,
			// each fanned out across the five tx names. The handles are forward
			// references here; phase-3 registration resolves them before the
			// workload's Init runs. Declared unconditionally at the top of
			// Define so discovery (probe/plan) sees the full instrument catalog.
			txLat := d.Histogram(txLatencyInst, bench.Tag("tx", txNames[:]...))
			txCnt := d.Counter(txCountInst, bench.Tag("tx", txNames[:]...))

			iso, _ := driver.ParseIsolation(o.TxIsolation)
			d.Driver("main", "pg")
			return buildSteps(d, o, d.Seed(), iso, file, txLat, txCnt)
		},
		// WrapSink composes the generic ConsoleSink with tpcc's MixSink so both
		// the per-instrument table and the domain mix/tpmC view render from one
		// Report — the print side-channel is gone.
		WrapSink: func(defaultSink metrics.Sink, stdout io.Writer) metrics.Sink {
			return metrics.MultiSink{defaultSink, newMixSink(stdout, o)}
		},
	}
	bench.Main(t)
}

// buildSteps assembles the tpcc step DAG against d:
//
//	drop_schema(Silent) -> create_schema -> load_* (Pool, per table, concurrent)
//	  -> create_indexes -> validate_population(If VALIDATE)
//	  -> workload(Closed) -> check_consistency(If VALIDATE)
//
// workload gates on After(validate_population, create_indexes): with F3 a
// Skipped validate_population (VALIDATE=false) unblocks its dependents just
// like a Succeeded one, so plain After orders the workload after both without
// the AfterAny workaround the block-on-skip cascade used to require. The
// transaction mix and tpmC are reported by the MixSink from the workload's
// per-tx instruments, not by a report step (D6).
func buildSteps(d *bench.Def, o *options, seed uint64, iso driver.Isolation, file *sqlfile.File, txLat *bench.Histogram, txCnt *bench.Counter) error {
	w := newWorld(seed, o.Warehouses)
	q := resolveTxQueries(file)

	dropQs := file.Section("drop_schema")
	createQs := file.Section("create_schema")
	idxQs := file.Section("create_indexes")

	d.Step("drop_schema", multiExec(dropQs)).OnErr(bench.ModeSilent).Uses("main")
	d.Step("create_schema", multiExec(createQs)).After("drop_schema").Uses("main")

	// One Pool load step per table (item once; the rest scale with W). Row
	// content is keyed by the global cycle, so the chunk count (sized by
	// LOAD_WORKERS) changes only parallelism, never the data. bench.Loader owns
	// the fill-batch-flush COPY loop and the named-stream namespace; the table
	// struct supplies only schema, cycle count and generator.
	loadNames := make([]string, 0, len(tables()))
	for _, tbl := range tables() {
		bench.Loader(d, o.LoadWorkers, 8, tbl.spec(w)).
			After("create_schema").Uses("main")
		loadNames = append(loadNames, tbl.step())
	}

	d.Step("create_indexes", multiExec(idxQs)).After(loadNames...).Uses("main")

	d.Step("validate_population", validatePopulation(w)).
		After("create_indexes").
		If(func(r *bench.Run) bool { return o.DoValidate }).
		Uses("main")

	d.Step("workload", &workloadHandler{w: w, q: q, iso: iso, lat: txLat, cnt: txCnt}).
		Closed(o.VUs, o.Duration).
		After("validate_population", "create_indexes").
		Uses("main")

	d.Step("check_consistency", checkConsistency()).
		After("workload").
		If(func(r *bench.Run) bool { return o.DoValidate }).
		Uses("main")

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
