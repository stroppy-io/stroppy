// Package simple is the Go-native port of workloads/simple/simple.ts — the
// minimal stroppy demo. Loads a small table via a relational InsertSpec, runs an
// aggregate count plus per-row lookups, and tears down. First Go workload; proves
// the Setup/Iterate/Teardown + Step + InsertSpec struct-literal path end to end.
package simple

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"

	"github.com/stroppy-io/stroppy/pkg/bench"
	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
)

var errRowCount = errors.New("row count mismatch")

const (
	demoRows = 100
	demoSeed = 0xC0FFEE
)

type workload struct {
	pick *rand.Rand
}

func init() { bench.Register(&workload{pick: rand.New(rand.NewPCG(demoSeed^1, 0))}) } //nolint:gosec // G404: data RNG

func (*workload) Name() string { return "simple" }

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
		_, err := b.InsertSpec(ctx, demoInsertSpec())

		return err
	}); err != nil {
		return err
	}

	b.StepBegin("workload")

	return nil
}

func (w *workload) Iterate(ctx context.Context, b *bench.Bench) error {
	count, err := b.QueryValue(ctx, "SELECT COUNT(*) FROM stroppy_demo", nil)
	if err != nil {
		return err
	}

	if toInt(count) != demoRows {
		return fmt.Errorf("%w: expected %d, got %v", errRowCount, demoRows, count)
	}

	b.Logger().Sugar().Infof("loaded %v rows into stroppy_demo", count)

	for range 3 {
		id := int64(1 + w.pick.IntN(demoRows))

		label, err := b.QueryValue(ctx, "SELECT label FROM stroppy_demo WHERE id = :id", map[string]any{"id": id})
		if err != nil {
			return err
		}

		b.Logger().Sugar().Infof("id=%d → label=%v", id, label)
	}

	return nil
}

func (w *workload) Teardown(ctx context.Context, b *bench.Bench) error {
	b.StepEnd("workload")

	return b.Exec(ctx, "DROP TABLE IF EXISTS stroppy_demo", nil)
}

// demoInsertSpec builds the relational InsertSpec for stroppy_demo directly as a
// struct literal — exactly what the TS Rel/Draw DSL compiles to. id is the 1-based
// row counter, label an 8-char ASCII string, value a uniform int in [0, 999].
// stream_id is left 0; the loader assigns unique ids at compile (compile.AssignStreamIDs).
func demoInsertSpec() *dgproto.InsertSpec {
	return &dgproto.InsertSpec{
		Table:  "stroppy_demo",
		Seed:   demoSeed,
		Method: dgproto.InsertMethod_PLAIN_BULK,
		Generator: &dgproto.InsertSpec_Source{Source: &dgproto.RelSource{
			Population:  &dgproto.Population{Name: "stroppy_demo", Size: demoRows},
			ColumnOrder: []string{"id", "label", "value"},
			Attrs: []*dgproto.Attr{
				{Name: "id", Expr: rowId()},
				{Name: "label", Expr: asciiDraw(8, 8)},
				{Name: "value", Expr: intUniformDraw(0, 999)},
			},
		}},
	}
}

func rowId() *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_BinOp{BinOp: &dgproto.BinOp{
		Op: dgproto.BinOp_ADD,
		A:  &dgproto.Expr{Kind: &dgproto.Expr_RowIndex{RowIndex: &dgproto.RowIndex{}}},
		B:  litInt(1),
	}}}
}

func asciiDraw(minLen, maxLen int64) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_StreamDraw{StreamDraw: &dgproto.StreamDraw{
		Draw: &dgproto.StreamDraw_Ascii{Ascii: &dgproto.DrawAscii{
			MinLen: litInt(minLen),
			MaxLen: litInt(maxLen),
			Alphabet: []*dgproto.AsciiRange{
				{Min: 65, Max: 90}, {Min: 97, Max: 122},
			},
		}},
	}}}
}

func intUniformDraw(min, max int64) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_StreamDraw{StreamDraw: &dgproto.StreamDraw{
		Draw: &dgproto.StreamDraw_IntUniform{IntUniform: &dgproto.DrawIntUniform{
			Min: litInt(min), Max: litInt(max),
		}},
	}}}
}

func litInt(n int64) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_Lit{Lit: &dgproto.Literal{
		Value: &dgproto.Literal_Int64{Int64: n},
	}}}
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
