// Package simple provides Stroppy's minimal workload example. It owns a small
// typed load, aggregate count, per-row lookups, and teardown lifecycle.
package simple

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"time"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/pkg/gen"
)

var errRowCount = errors.New("row count mismatch")

const (
	demoRows = 100
	demoSeed = 0xC0FFEE
	// demoDomain is the versioned namespace simple's fields derive under;
	// bump the suffix to change the dataset intentionally.
	demoDomain = "simple/stroppy_demo@1"
)

type workload struct{ workers []*rand.Rand }

var Test = bench.Test{Name: "simple", Define: define, Source: publication("Test")}

func init() { bench.Register(Test) }
func define(d *bench.Def) error {
	settings := bench.RunParameters(&d.Param, bench.RunDefaults{})

	w := &workload{workers: make([]*rand.Rand, settings.Workers)}
	for i := range w.workers {
		//nolint:gosec // benchmark data generator, not security randomness.
		w.workers[i] = rand.New(rand.NewPCG(demoSeed^uint64(i+1), 0))
	}

	d.Execution.Step("drop_schema", w.drop)
	d.Execution.Step("create_schema", w.create)
	d.Execution.Step("load_data", w.load)
	d.Execution.Step("workload", w.work, settings.Policy())
	d.Execution.Step("cleanup", w.drop, bench.Always(30*time.Second))

	return d.Execution.Err()
}

func (w *workload) drop(ctx context.Context, b *bench.Bench) error {
	return b.Exec(ctx, "DROP TABLE IF EXISTS stroppy_demo", nil)
}

func (w *workload) create(ctx context.Context, b *bench.Bench) error {
	sql := "CREATE TABLE stroppy_demo (id INT PRIMARY KEY, label TEXT, value INT)"
	if b.DriverTypeName() == bench.DriverYDB {
		sql = "CREATE TABLE stroppy_demo (id Int64 NOT NULL, label String, value Int64, PRIMARY KEY (id))"
	}

	return b.Exec(ctx, sql, nil)
}

func (w *workload) load(ctx context.Context, b *bench.Bench) error {
	request := demoInsertRequest()
	_, err := b.Insert(
		ctx,
		request.Table,
		request.Source,
		bench.InsertMethod(bench.InsertPlainBulk),
	)

	return err
}

func (w *workload) work(ctx context.Context, b *bench.Bench) error {
	count, err := b.QueryValue[int64](ctx, "SELECT COUNT(*) FROM stroppy_demo", nil)
	if err != nil {
		return err
	}

	if count != demoRows {
		return fmt.Errorf("%w: expected %d, got %v", errRowCount, demoRows, count)
	}

	for range 3 {
		id := int64(1 + w.workers[b.Worker()].IntN(demoRows))

		label, err := b.QueryValue[string](
			ctx,
			"SELECT label FROM stroppy_demo WHERE id = :id",
			map[string]any{"id": id},
		)
		if err != nil {
			return err
		}

		b.Log.Debug("row selected", "id", id, "label", label)
	}

	return nil
}

// demoInsertRequest builds the typed insert request for stroppy_demo from a
// plain Go row formula. id is the 1-based row counter, label an 8-character
// [A-Za-z] string, value a uniform int in [0, 999]. The fields derive from
// the demo seed under a versioned domain, so the dataset is deterministic and
// seekable; no protobuf spec, expression AST, or scratch buffer appears.
func demoInsertRequest() *insertRequest {
	root := gen.New(demoSeed)

	return &insertRequest{
		Table:   "stroppy_demo",
		Method:  bench.InsertPlainBulk,
		Workers: 1,
		Source:  demoSource(root, demoRows, 64),
	}
}

// demoSource returns the indexed source for stroppy_demo under root. Split
// out so tests can build a large variant for allocation measurements without
// re-stating the row formula.
func demoSource(root gen.Root, totalRows int64, batchRows int) *gen.IndexedSource {
	domain := root.Domain(demoDomain)

	labelField := domain.Field("label")
	valueField := domain.Field("value")

	b := gen.NewSchemaBuilder()
	idCol := b.Int64("id")
	labelCol := b.Bytes("label", 8)
	valueCol := b.Int64("value")
	schema := b.Build()

	fn := func(r gen.Row, entity uint64) error {
		r.SetInt64(idCol, int64(entity)+1) //nolint:gosec // G115: entity < totalRows, in int64 range for any real workload

		dst, err := r.Bytes(labelCol, 8)
		if err != nil {
			return err
		}

		draw := labelField.At(entity)
		gen.Alpha.Fill(&draw, dst)

		r.SetInt64(valueCol, valueField.Int64(entity, 0, 999))

		return nil
	}

	return gen.NewIndexedSource(schema, root, demoDomain, totalRows, batchRows, fn)
}

func toInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case uint64:
		if n > math.MaxInt {
			return -1
		}

		return int(n)
	case float64:
		return int(n)
	default:
		return -1
	}
}

type insertRequest struct {
	Table   string
	Method  bench.InsertStrategy
	Workers int
	Source  gen.BatchSource
}
