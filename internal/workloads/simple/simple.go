// Package simple is the Go-native port of workloads/simple/simple.ts — the
// minimal stroppy demo. Loads a small table via a typed InsertRequest, runs an
// aggregate count plus per-row lookups, and tears down. First Go workload on
// the typed insert path; proves Setup/Iterate/Teardown + Step + Driver.Insert
// with a plain-Go row formula end to end.
package simple

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"

	"github.com/stroppy-io/stroppy/pkg/bench"
	"github.com/stroppy-io/stroppy/pkg/driver"
	"github.com/stroppy-io/stroppy/pkg/gen"
)

var errRowCount = errors.New("row count mismatch")

const (
	demoRows = 100
	demoSeed = 0xC0FFEE
	// demoDomain is the versioned namespace simple's fields derive under;
	// bump the suffix to change the dataset intentionally.
	demoDomain = "simple/stroppy_demo@1"
)

type workload struct {
	pick *rand.Rand
}

func init() {
	bench.Register(func() bench.Workload {
		return &workload{pick: rand.New(rand.NewPCG(demoSeed^1, 0))} //nolint:gosec // G404: data RNG
	})
}

func (*workload) Name() string { return "simple" }

func (*workload) Define(*bench.Def) error { return nil }

func (w *workload) Setup(ctx context.Context, b *bench.Bench) error {
	if err := b.Step("drop_schema", func() error {
		return b.Exec(ctx, "DROP TABLE IF EXISTS stroppy_demo", nil)
	}); err != nil {
		return err
	}

	if err := b.Step("create_schema", func() error {
		return b.Exec(ctx, "CREATE TABLE stroppy_demo (id INT PRIMARY KEY, label TEXT, value INT)", nil)
	}); err != nil {
		return err
	}

	if err := b.Step("load_data", func() error {
		_, err := b.Insert(ctx, demoInsertRequest())

		return err
	}); err != nil {
		return err
	}

	return nil
}

func (w *workload) Iterate(ctx context.Context, b *bench.Bench) error {
	return b.StepSilent("workload", func() error {
		count, err := b.QueryValue(ctx, "SELECT COUNT(*) FROM stroppy_demo", nil)
		if err != nil {
			return err
		}

		if toInt(count) != demoRows {
			return fmt.Errorf("%w: expected %d, got %v", errRowCount, demoRows, count)
		}

		b.Logger().Sugar().Debugf("loaded %v rows into stroppy_demo", count)

		for range 3 {
			id := int64(1 + w.pick.IntN(demoRows))

			label, err := b.QueryValue(ctx, "SELECT label FROM stroppy_demo WHERE id = :id", map[string]any{"id": id})
			if err != nil {
				return err
			}

			b.Logger().Sugar().Debugf("id=%d → label=%v", id, label)
		}

		return nil
	})
}

func (w *workload) Teardown(ctx context.Context, b *bench.Bench) error {
	return b.Exec(ctx, "DROP TABLE IF EXISTS stroppy_demo", nil)
}

// demoInsertRequest builds the typed insert request for stroppy_demo from a
// plain Go row formula. id is the 1-based row counter, label an 8-character
// [A-Za-z] string, value a uniform int in [0, 999]. The fields derive from
// the demo seed under a versioned domain, so the dataset is deterministic and
// seekable; no protobuf spec, expression AST, or scratch buffer appears.
func demoInsertRequest() *driver.InsertRequest {
	root := gen.New(demoSeed)

	return &driver.InsertRequest{
		Table:   "stroppy_demo",
		Method:  driver.InsertPlainBulk,
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
	case float64:
		return int(n)
	default:
		return -1
	}
}
