// Package baseline provides Stroppy's machine-benchmark workload. It loads a
// single wide table and runs fixed-shape DML transactions without ever
// validating result data, so it produces clean iterations against any driver —
// including noop and no-op wire servers, where stub rows would fail the result
// checks of ordinary workloads.
package baseline

import (
	"context"
	"runtime"
	"time"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/pkg/gen"
)

const (
	workloadName   = "baseline"
	cleanupTimeout = 30 * time.Second

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

var Test = bench.Test{Name: workloadName, Define: define}

func init() { bench.Register(Test) }
func define(d *bench.Def) error {
	settings := bench.RunParameters(&d.Param, bench.RunDefaults{})
	w := &workload{}
	w.rows, _ = d.Param.Int64(
		"rows",
		defaultRows,
		"Rows loaded into probe table.",
		bench.Min(int64(1)),
	)
	w.loadWorkers, _ = d.Param.Int(
		"load-workers",
		runtime.GOMAXPROCS(0),
		"Load workers.",
		bench.Min(1),
	)
	iso, _ := d.Param.String("tx-isolation", "", "Transaction isolation override.")
	db := d.Drivers.Declare("default", bench.DriverConfig{})
	w.iso = resolveIsolation(db.Kind(), bench.TxIsolationName(iso))
	d.Execution.Step("drop_schema", w.drop)
	d.Execution.Step("create_schema", w.create)
	d.Execution.Step("load_data", w.load)
	d.Execution.Step("workload", w.work, settings.Policy())
	d.Execution.Step("cleanup", w.drop, bench.Always(cleanupTimeout))

	return d.Execution.Err()
}

func (w *workload) drop(ctx context.Context, b *bench.Bench) error {
	return b.Exec(ctx, "DROP TABLE IF EXISTS "+probeTable, nil)
}

func (w *workload) create(ctx context.Context, b *bench.Bench) error {
	return b.Exec(
		ctx,
		"CREATE TABLE "+probeTable+" (id BIGINT, v BIGINT, filler TEXT)",
		nil,
	)
}

func (w *workload) load(ctx context.Context, b *bench.Bench) error {
	request := probeInsertRequest(w.rows, w.loadWorkers)
	_, err := b.Insert(
		ctx,
		request.Table,
		request.Source,
		bench.InsertMethod(bench.InsertNative),
		bench.LoadWorkers(w.loadWorkers),
	)

	return err
}

func (w *workload) work(ctx context.Context, b *bench.Bench) error {
	return b.Transaction(
		ctx,
		bench.TransactionOptions{Name: "baseline", Isolation: w.iso},
		w.transaction,
	)
}

func (w *workload) transaction(ctx context.Context, tx *bench.Tx) error {
	for _, sql := range []string{
		"UPDATE " + probeTable + " SET v = v + 1 WHERE id = 1",
		"UPDATE " + probeTable + " SET v = v + 2 WHERE id = 2",
		"UPDATE " + probeTable + " SET filler = 'probe' WHERE id = 3",
		"INSERT INTO " + probeTable + " (id, v, filler) VALUES (0, 0, 'probe')",
	} {
		if err := tx.Exec(ctx, sql, nil); err != nil {
			return err
		}
	}

	return nil
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
func probeInsertRequest(totalRows int64, workers int) *insertRequest {
	root := gen.New(seed)

	return &insertRequest{
		Table: probeTable, Method: bench.InsertNative, Workers: workers,
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

	return gen.NewIndexedSource(
		schema,
		root,
		"baseline/probe@1",
		totalRows,
		batchRows,
		fn,
	)
}

type insertRequest struct {
	Table   string
	Method  bench.InsertStrategy
	Workers int
	Source  gen.BatchSource
}
