// Command simple is the canonical stroppy-next example test: it loads one table
// and runs a point-select workload against it, exercising the whole SDK stack —
// the Define declarative pass, driver slots, the SQL corpus parser, the step
// DAG, all four VU lifecycle phases and the metrics reporter.
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

// loadBatch is the columnar insert batch size.
const loadBatch = 1000

// valLen is the fixed length of a generated val string.
const valLen = 16

func main() {
	t := &bench.Test{
		Name: "simple",
		Seed: "1",
		Define: func(d *bench.Def) error {
			// One declarative pass: params resolve immediately, so the values
			// feed straight into executor policies and handler config below.
			rows := d.Param.Int("rows", 10000, "number of rows to generate")
			vus := d.Param.Int("vus", 4, "closed-loop virtual users")
			dur := d.Param.Duration("duration", 5*time.Second, "workload duration")
			if rows.Value() <= 0 {
				return fmt.Errorf("rows must be positive, got %d", rows.Value())
			}
			if vus.Value() <= 0 {
				return fmt.Errorf("vus must be positive, got %d", vus.Value())
			}

			d.Driver("main", "pg")

			// Eager query-set resolution (F1): File is usable inside Define.
			qs := d.Queries("simple", sqlSrc)
			file, err := qs.File()
			if err != nil {
				return fmt.Errorf("simple: parse sql: %w", err)
			}
			dropQ, err := mustQuery(file, "schema", "drop")
			if err != nil {
				return err
			}
			createQ, err := mustQuery(file, "schema", "create")
			if err != nil {
				return err
			}
			selectQ, err := mustQuery(file, "workload", "point_select")
			if err != nil {
				return err
			}
			countQ, err := mustQuery(file, "check", "count")
			if err != nil {
				return err
			}

			// Readiness gate: ping the database until it answers (or timeout),
			// before any step opens a connection. Explicit and skippable via
			// --steps, rather than buried in driver construction.
			bench.ReadyStep(d, 30*time.Second, time.Second).Uses("main")

			// DROP may fail on a fresh database (no table yet); Silent keeps
			// the step green and the run moving.
			d.Step("drop_schema", exec(dropQ)).OnErr(bench.ModeSilent).After("ready").Uses("main")
			d.Step("create_schema", exec(createQ)).After("drop_schema").Uses("main")

			// Load is a single-table columnar COPY of `rows` rows; bench.Loader
			// owns the fill-batch-flush loop and the named-stream namespace, so
			// only the generator is authored here. One worker (the load is
			// small and not the measured path); 8 chunks per worker for the
			// same skew-tolerance split the Pool executor expects.
			loadSpec := bench.Spec{
				Step:  "load",
				Table: "simple_kv",
				Cols: []mem.ColSpec{
					{Name: "id", Type: mem.TypeInt64},
					{Name: "val", Type: mem.TypeBytes},
					{Name: "num", Type: mem.TypeFloat64},
				},
				Batch:  loadBatch,
				Cycles: func() int64 { return int64(rows.Value()) },
				Gen: func(b *mem.RowBuf, cycle int64, s *bench.Streams) {
					valRnd := s.Stream("val")
					numRnd := s.Stream("num")
					var name [valLen]byte
					b.AppendInt64(0, cycle)
					rng.FillAlpha(name[:], valRnd, uint64(cycle))
					b.AppendBytes(1, name[:])
					b.AppendFloat64(2, rng.UniformFloat(numRnd, uint64(cycle)))
				},
			}
			bench.Loader(d, 1, 8, loadSpec).After("create_schema").Uses("main")

			d.Step("workload", &workloadHandler{rows: rows.Value(), query: selectQ}).
				Closed(vus.Value(), dur.Value()).After("load").Uses("main")
			// Row-count verification only holds against a real database.
			notNoop := func(r *bench.Run) bool { return r.DriverKind(0) != "noop" }
			d.Step("check", &checkHandler{rows: rows.Value(), query: countQ}).
				After("workload").
				If(notNoop).
				Uses("main")
			// cleanup carries the same If as check: under F3 a Skipped step
			// unblocks its dependents, so a gate on check alone would let
			// cleanup run on noop — there's nothing to drop there. The
			// explicit If keeps cleanup skipped on noop rather than relying
			// on the old block-on-skip cascade.
			d.Step("cleanup", exec(dropQ)).
				After("check").
				If(notNoop).
				Uses("main")

			d.Variant("full")
			return nil
		},
	}
	bench.Main(t)
}

// exec is a run-once handler that prepares q and executes it for side effect.
// Conn/Prepare return errors (D10: native errors, no panics); a FuncOnce body
// surfaces them directly and the executor counts the failed Iter.
func exec(q *sqlfile.Query) bench.Handler {
	return bench.FuncOnce(func(vu *bench.VU) error {
		conn, err := vu.Conn()
		if err != nil {
			return err
		}
		st, err := vu.Prepare(q)
		if err != nil {
			return err
		}
		return conn.Exec(vu.Ctx(), st)
	})
}

func mustQuery(f *sqlfile.File, section, name string) (*sqlfile.Query, error) {
	q, ok := f.Query(section, name)
	if !ok {
		return nil, fmt.Errorf("simple: missing query %s/%s", section, name)
	}
	return q, nil
}

// wlState is the workload worker's per-VU state.
type wlState struct {
	conn driver.Conn
	stmt driver.Stmt
}

// workloadHandler point-selects a uniformly random row per iteration and
// discards the result.
type workloadHandler struct {
	rows  int
	query *sqlfile.Query
}

func (h *workloadHandler) Init(vu *bench.VU) error {
	st := bench.Local[wlState](vu)
	conn, err := vu.Conn()
	if err != nil {
		return err
	}
	st.conn = conn
	stmt, err := vu.Prepare(h.query)
	if err != nil {
		return err
	}
	st.stmt = stmt
	return nil
}

func (h *workloadHandler) Iter(vu *bench.VU) error {
	st := bench.Local[wlState](vu)
	id := rng.UniformInt(vu.Rand(bench.StreamID("row_pick")), vu.Cycle(), 0, int64(h.rows)-1)

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

// checkHandler asserts the loaded row count equals rows.
type checkHandler struct {
	rows  int
	query *sqlfile.Query
}

func (h *checkHandler) Init(*bench.VU) error { return nil }

func (h *checkHandler) Iter(vu *bench.VU) error {
	conn, err := vu.Conn()
	if err != nil {
		return err
	}
	st, err := vu.Prepare(h.query)
	if err != nil {
		return err
	}
	row := conn.QueryRow(vu.Ctx(), st)
	n, err := row.ScanInt64(0)
	if err != nil {
		return err
	}
	if int(n) != h.rows {
		return fmt.Errorf("row count = %d, want %d", n, h.rows)
	}
	return nil
}

func (h *checkHandler) Close(*bench.VU) error { return nil }
