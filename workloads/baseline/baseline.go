// Package baseline provides Stroppy's machine-benchmark workload. It loads a
// single wide table and runs fixed-shape DML transactions without ever
// validating result data, so it produces clean iterations against any driver —
// including noop and no-op wire servers, where stub rows would fail the result
// checks of ordinary workloads.
package baseline

import (
	"context"
	"runtime"

	"github.com/stroppy-io/stroppy/pkg/bench"
	"github.com/stroppy-io/stroppy/pkg/driver"
	"github.com/stroppy-io/stroppy/pkg/gen"
)

const (
	workloadName = "baseline"

	probeTable = "stroppy_baseline"
	rowFiller  = 84
	seed       = 0x0B45E11E

	defaultRows = 250_000
	maxValue    = 1_000_000
	batchRows   = 64
)

type workload struct {
	iso         bench.TxIsolationName
	rows        int64
	loadWorkers int
}

func init() {
	bench.Register(func() bench.Workload { return &workload{} })
}

func (*workload) Name() string { return workloadName }

func (w *workload) Define(d *bench.Def) error {
	w.rows = max(d.Param.Int64("rows", defaultRows, "Rows loaded into the probe table.").Value(), 1)
	w.loadWorkers = max(d.Param.Int(
		"load-workers", runtime.GOMAXPROCS(0), "Workers used to load the probe table.",
	).Value(), 1)
	w.iso = bench.TxIsolationName(d.Param.String(
		"tx-isolation", "", "Transaction isolation override.",
	).Value())

	return nil
}

func (w *workload) Setup(ctx context.Context, b *bench.Bench) error {
	w.iso = resolveIsolation(b.DriverTypeName(), w.iso)

	if err := b.Step("drop_schema", func() error {
		return b.Exec(ctx, "DROP TABLE IF EXISTS "+probeTable, nil)
	}); err != nil {
		return err
	}

	if err := b.Step("create_schema", func() error {
		return b.Exec(ctx,
			"CREATE TABLE "+probeTable+" (id BIGINT, v BIGINT, filler TEXT)", nil)
	}); err != nil {
		return err
	}

	return b.Step("load_data", func() error {
		_, err := b.Insert(ctx, probeInsertRequest(w.rows, w.loadWorkers))

		return err
	})
}

// Iterate runs one fixed-shape transaction: three updates and one insert with
// literal SQL. Statements carry no bind parameters and read no result values,
// so the same body runs unchanged against real databases, the noop driver,
// and no-op wire servers that discard I/O.
func (w *workload) Iterate(ctx context.Context, b *bench.Bench) error {
	return b.StepSilent("workload", func() error {
		return b.BeginTx(ctx, bench.BeginOpts{Isolation: w.iso, Name: "baseline"}, func(tx *bench.TxX) error {
			for _, stmt := range []string{
				"UPDATE " + probeTable + " SET v = v + 1 WHERE id = 1",
				"UPDATE " + probeTable + " SET v = v + 2 WHERE id = 2",
				"UPDATE " + probeTable + " SET filler = 'probe' WHERE id = 3",
				"INSERT INTO " + probeTable + " (id, v, filler) VALUES (0, 0, 'probe')",
			} {
				if err := tx.Exec(ctx, stmt, nil); err != nil {
					return err
				}
			}

			return nil
		})
	})
}

func (*workload) Teardown(ctx context.Context, b *bench.Bench) error {
	return b.Exec(ctx, "DROP TABLE IF EXISTS "+probeTable, nil)
}

func resolveIsolation(dt bench.DriverTypeName, override bench.TxIsolationName) bench.TxIsolationName {
	if override != "" {
		return override
	}

	switch dt {
	case bench.DriverPicodata:
		return bench.IsoNone
	case bench.DriverYDB:
		return bench.IsoSerializable
	default:
		return bench.IsoReadCommitted
	}
}

// probeInsertRequest builds the typed insert request for the probe table:
// id is the 1-based row counter, v a uniform int, filler an 84-character
// [A-Za-z] string.
func probeInsertRequest(totalRows int64, workers int) *driver.InsertRequest {
	root := gen.New(seed)

	return &driver.InsertRequest{
		Table: probeTable, Method: driver.InsertNative, Workers: workers,
		Source: probeSource(root, totalRows),
	}
}

func probeSource(root gen.Root, totalRows int64) *gen.IndexedSource {
	fillerField := root.Domain("baseline/probe@1").Field("filler")
	valueField := root.Domain("baseline/probe@1").Field("v")

	b := gen.NewSchemaBuilder()
	idCol := b.Int64("id")
	vCol := b.Int64("v")
	fillerCol := b.Bytes("filler", rowFiller)
	schema := b.Build()

	fn := func(r gen.Row, entity uint64) error {
		r.SetInt64(idCol, int64(entity)+1) //nolint:gosec // G115: bounded by totalRows
		r.SetInt64(vCol, valueField.Int64(entity, 0, maxValue))

		dst, err := r.Bytes(fillerCol, rowFiller)
		if err != nil {
			return err
		}

		draw := fillerField.At(entity)
		gen.Alpha.Fill(&draw, dst)

		return nil
	}

	return gen.NewIndexedSource(schema, root, "baseline/probe@1", totalRows, batchRows, fn)
}
