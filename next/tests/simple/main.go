// Command simple is the canonical stroppy-next example test: it loads one table
// and runs a point-select workload against it, exercising the whole SDK stack —
// options, driver slots, the SQL corpus parser, the step DAG, all four VU
// lifecycle phases and the metrics reporter.
//
//	STROPPY_DRIVER_URL=postgres://stroppy@127.0.0.1:5432/postgres go run ./tests/simple
//	go run ./tests/simple -plan          # print the step DAG
//	go run ./tests/simple -probe         # print the machine-readable description
//	STROPPY_DRIVER_KIND=noop go run ./tests/simple   # run with no database
//
// Under the noop driver every query is a canned empty result, so the row-count
// verification cannot hold; the check step is gated with If to skip it there,
// demonstrating conditional pruning.
package main

import (
	"fmt"
	"log"
	"time"

	_ "embed"

	"github.com/stroppy-io/stroppy/next/bench"
	"github.com/stroppy-io/stroppy/next/driver"
	"github.com/stroppy-io/stroppy/next/mem"
	"github.com/stroppy-io/stroppy/next/rng"
	"github.com/stroppy-io/stroppy/next/sqlfile"
)

//go:embed simple.sql
var sqlSrc []byte

// options are the test's tunables, filled from the environment by the SDK.
type options struct {
	Rows     int           `env:"ROWS" default:"10000"`
	VUs      int           `env:"VUS" default:"4"`
	Duration time.Duration `env:"DURATION" default:"5s"`
}

// Validate enforces sane bounds; the SDK calls it after parsing.
func (o *options) Validate() error {
	if o.Rows <= 0 {
		return fmt.Errorf("ROWS must be positive, got %d", o.Rows)
	}
	if o.VUs <= 0 {
		return fmt.Errorf("VUS must be positive, got %d", o.VUs)
	}
	return nil
}

// loadBatch is the columnar insert batch size.
const loadBatch = 1000

// valLen is the fixed length of a generated val string.
const valLen = 16

func main() {
	o := &options{}
	t := &bench.Test{
		Name:    "simple",
		Seed:    1,
		Opts:    o,
		Drivers: []bench.DriverSlot{{Name: "main", Kind: "pg"}},
		// Build runs after the SDK has parsed options, so the executor policies
		// below read the final o.VUs / o.Duration directly (no pre-parse).
		Build: buildSteps(o),
	}
	bench.Main(t)
}

// buildSteps returns the test's step-builder closure. It parses the embedded SQL
// and assembles the load → workload → check lifecycle, closing over the parsed
// options.
func buildSteps(o *options) func(*bench.Run) []*bench.StepDef {
	return func(*bench.Run) []*bench.StepDef {
		file, err := sqlfile.Parse(sqlSrc)
		if err != nil {
			log.Fatalf("simple: parse sql: %v", err)
		}
		dropQ := mustQuery(file, "schema", "drop")
		createQ := mustQuery(file, "schema", "create")
		selectQ := mustQuery(file, "workload", "point_select")
		countQ := mustQuery(file, "check", "count")

		return []*bench.StepDef{
			// DROP may fail on a fresh database (no table yet); Silent keeps the
			// step green and the run moving.
			bench.Step("drop_schema", exec(dropQ)).
				OnErr(bench.Silent).Uses("main"),

			bench.Step("create_schema", exec(createQ)).
				After("drop_schema").Uses("main"),

			bench.Step("load", &loadHandler{opts: o}).
				After("create_schema").Uses("main"),

			bench.Step("workload", &workloadHandler{opts: o, query: selectQ}).
				Closed(o.VUs, o.Duration).After("load").Uses("main"),

			// Row-count verification only holds against a real database.
			bench.Step("check", &checkHandler{opts: o, query: countQ}).
				After("workload").
				If(func(r *bench.Run) bool { return r.DriverKind(0) != "noop" }).
				Uses("main"),

			bench.Step("cleanup", exec(dropQ)).
				After("check").Uses("main"),
		}
	}
}

// exec is a run-once handler that prepares q and executes it for side effect.
// As a trivial FuncOnce body it uses the panic-on-failure Conn/Prepare; the
// executor recovers any connect/prepare failure into the step's error.
func exec(q *sqlfile.Query) bench.Handler {
	return bench.FuncOnce(func(vu *bench.VU) error {
		return vu.Conn().Exec(vu.Ctx(), vu.Prepare(q))
	})
}

func mustQuery(f *sqlfile.File, section, name string) *sqlfile.Query {
	q, ok := f.Query(section, name)
	if !ok {
		log.Fatalf("simple: missing query %s/%s", section, name)
	}
	return q
}

// loadState is the load worker's per-VU state.
type loadState struct {
	conn driver.Conn
	buf  *mem.RowBuf
}

// loadHandler generates opts.Rows rows and bulk-inserts them in columnar batches.
type loadHandler struct{ opts *options }

func (h *loadHandler) Init(vu *bench.VU) error {
	st := bench.Local[loadState](vu)
	conn, err := vu.ConnE()
	if err != nil {
		return err
	}
	st.conn = conn
	st.buf = mem.NewRowBuf(loadBatch,
		mem.ColSpec{Name: "id", Type: mem.TypeInt64},
		mem.ColSpec{Name: "val", Type: mem.TypeBytes},
		mem.ColSpec{Name: "num", Type: mem.TypeFloat64},
	)
	return nil
}

func (h *loadHandler) Iter(vu *bench.VU) error {
	st := bench.Local[loadState](vu)
	valRnd := vu.Rand(1)
	numRnd := vu.Rand(2)
	var name [valLen]byte

	for base := 0; base < h.opts.Rows; base += loadBatch {
		st.buf.Reset()
		end := min(base+loadBatch, h.opts.Rows)
		for id := base; id < end; id++ {
			st.buf.AppendInt64(0, int64(id))
			rng.FillAlpha(name[:], valRnd, uint64(id))
			st.buf.AppendBytes(1, name[:])
			st.buf.AppendFloat64(2, rng.UniformFloat(numRnd, uint64(id)))
		}
		if _, err := st.conn.InsertColumns(vu.Ctx(), "simple_kv", st.buf); err != nil {
			return err
		}
	}
	return nil
}

func (h *loadHandler) Close(*bench.VU) error { return nil }

// wlState is the workload worker's per-VU state.
type wlState struct {
	conn driver.Conn
	stmt driver.Stmt
}

// workloadHandler point-selects a uniformly random row per iteration and discards
// the result.
type workloadHandler struct {
	opts  *options
	query *sqlfile.Query
}

func (h *workloadHandler) Init(vu *bench.VU) error {
	st := bench.Local[wlState](vu)
	conn, err := vu.ConnE()
	if err != nil {
		return err
	}
	st.conn = conn
	stmt, err := vu.PrepareE(h.query)
	if err != nil {
		return err
	}
	st.stmt = stmt
	return nil
}

func (h *workloadHandler) Iter(vu *bench.VU) error {
	st := bench.Local[wlState](vu)
	id := rng.UniformInt(vu.Rand(0), vu.Cycle(), 0, int64(h.opts.Rows)-1)

	args := st.stmt.Bind()
	args.Int64(id)
	row := st.conn.QueryRowWithArgs(vu.Ctx(), st.stmt, args)

	// Read and discard both columns; a missing row (noop driver) is not an error.
	if _, err := row.ScanString(0); err != nil && err != driver.ErrNoRows {
		return err
	}
	if _, err := row.ScanFloat64(1); err != nil && err != driver.ErrNoRows {
		return err
	}
	return nil
}

func (h *workloadHandler) Close(*bench.VU) error { return nil }

// checkHandler asserts the loaded row count equals opts.Rows.
type checkHandler struct {
	opts  *options
	query *sqlfile.Query
}

func (h *checkHandler) Init(*bench.VU) error { return nil }

func (h *checkHandler) Iter(vu *bench.VU) error {
	row := vu.Conn().QueryRow(vu.Ctx(), vu.Prepare(h.query))
	n, err := row.ScanInt64(0)
	if err != nil {
		return err
	}
	if int(n) != h.opts.Rows {
		return fmt.Errorf("row count = %d, want %d", n, h.opts.Rows)
	}
	return nil
}

func (h *checkHandler) Close(*bench.VU) error { return nil }
