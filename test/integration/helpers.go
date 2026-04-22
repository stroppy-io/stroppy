//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
	"github.com/stroppy-io/stroppy/pkg/datagen/runtime"
)

// Expr / Attr proto-builder shortcuts shared across integration tests. Keep
// the surface small; bespoke builders belong in the test file that owns them.

// litOf wraps a Go scalar in a Literal-kind Expr. Supports the scalar types
// every integration test has needed so far: string, bool, int (widened to
// int64), int64, and float64. Callers should use the narrowest Go type that
// matches the expected wire type.
func litOf(value any) *dgproto.Expr {
	lit := &dgproto.Literal{}
	switch v := value.(type) {
	case int:
		lit.Value = &dgproto.Literal_Int64{Int64: int64(v)}
	case int64:
		lit.Value = &dgproto.Literal_Int64{Int64: v}
	case float64:
		lit.Value = &dgproto.Literal_Double{Double: v}
	case string:
		lit.Value = &dgproto.Literal_String_{String_: v}
	case bool:
		lit.Value = &dgproto.Literal_Bool{Bool: v}
	default:
		panic(fmt.Sprintf("litOf: unsupported type %T", value))
	}
	return &dgproto.Expr{Kind: &dgproto.Expr_Lit{Lit: lit}}
}

// rowIndexOf returns a RowIndex Expr pinned to the GLOBAL kind, the common
// case for flat-population attrs.
func rowIndexOf() *dgproto.Expr {
	return rowIndexKind(dgproto.RowIndex_GLOBAL)
}

// rowIndexKind returns a RowIndex Expr of the given kind (ENTITY, LINE, or
// GLOBAL).
func rowIndexKind(kind dgproto.RowIndex_Kind) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_RowIndex{RowIndex: &dgproto.RowIndex{Kind: kind}}}
}

// colOf returns a ColRef Expr naming another attr in the current scope.
func colOf(name string) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_Col{Col: &dgproto.ColRef{Name: name}}}
}

// binOpOf wraps (a, b) in a BinOp Expr with the given operator.
func binOpOf(op dgproto.BinOp_Op, a, b *dgproto.Expr) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_BinOp{BinOp: &dgproto.BinOp{Op: op, A: a, B: b}}}
}

// callOf wraps a stdlib function call with positional args.
func callOf(name string, args ...*dgproto.Expr) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_Call{Call: &dgproto.Call{Func: name, Args: args}}}
}

// ifOf wraps a conditional Expr.
func ifOf(cond, thenExpr, elseExpr *dgproto.Expr) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_If_{If_: &dgproto.If{Cond: cond, Then: thenExpr, Else_: elseExpr}}}
}

// dictAtOf wraps a DictAt Expr keyed by dict and row index.
func dictAtOf(key string, index *dgproto.Expr) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_DictAt{DictAt: &dgproto.DictAt{DictKey: key, Index: index}}}
}

// lookupOf wraps a Lookup Expr targeting (pop, attr) at the given
// entity-index Expr.
func lookupOf(pop, attrName string, idx *dgproto.Expr) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_Lookup{Lookup: &dgproto.Lookup{
		TargetPop: pop, AttrName: attrName, EntityIndex: idx,
	}}}
}

// streamDrawExpr wraps a StreamDraw oneof arm into an Expr. `stream_id` is
// left zero — compile.AssignStreamIDs fills it during Runtime construction.
// Typed via `any` because the `isStreamDraw_Draw` interface is unexported
// from the dgproto package; the type switch enforces arm membership at
// runtime.
func streamDrawExpr(arm any) *dgproto.Expr {
	sd := &dgproto.StreamDraw{}
	switch v := arm.(type) {
	case *dgproto.StreamDraw_IntUniform:
		sd.Draw = v
	case *dgproto.StreamDraw_FloatUniform:
		sd.Draw = v
	case *dgproto.StreamDraw_Normal:
		sd.Draw = v
	case *dgproto.StreamDraw_Zipf:
		sd.Draw = v
	case *dgproto.StreamDraw_Nurand:
		sd.Draw = v
	case *dgproto.StreamDraw_Bernoulli:
		sd.Draw = v
	case *dgproto.StreamDraw_Dict:
		sd.Draw = v
	case *dgproto.StreamDraw_Joint:
		sd.Draw = v
	case *dgproto.StreamDraw_Date:
		sd.Draw = v
	case *dgproto.StreamDraw_Decimal:
		sd.Draw = v
	case *dgproto.StreamDraw_Ascii:
		sd.Draw = v
	case *dgproto.StreamDraw_Phrase:
		sd.Draw = v
	default:
		panic(fmt.Sprintf("streamDrawExpr: unknown arm %T", v))
	}
	return &dgproto.Expr{Kind: &dgproto.Expr_StreamDraw{StreamDraw: sd}}
}

// streamDrawAttr wraps a StreamDraw arm in a named Attr.
func streamDrawAttr(name string, arm any) *dgproto.Attr {
	return &dgproto.Attr{Name: name, Expr: streamDrawExpr(arm)}
}

// chooseAttr wraps a named attr whose Expr is a Choose over the given
// branches. stream_id is filled during compile.
func chooseAttr(name string, branches ...*dgproto.ChooseBranch) *dgproto.Attr {
	return &dgproto.Attr{Name: name, Expr: &dgproto.Expr{
		Kind: &dgproto.Expr_Choose{Choose: &dgproto.Choose{Branches: branches}},
	}}
}

// attrOf is the common Attr builder for tests that don't need Null injection.
func attrOf(name string, e *dgproto.Expr) *dgproto.Attr {
	return &dgproto.Attr{Name: name, Expr: e}
}

// attrWithNullOf attaches a Null policy to an otherwise-vanilla Attr.
func attrWithNullOf(name string, e *dgproto.Expr, rate float32, salt uint64) *dgproto.Attr {
	return &dgproto.Attr{Name: name, Expr: e, Null: &dgproto.Null{Rate: rate, SeedSalt: salt}}
}

// litFloat builds a double-typed Literal Expr. Distinct from litOf so
// callers can pass integer constants (`litFloat(10)`) without accidentally
// emitting an int64 literal.
func litFloat(f float64) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_Lit{Lit: &dgproto.Literal{
		Value: &dgproto.Literal_Double{Double: f},
	}}}
}

// daysEpoch returns the number of whole days between 1970-01-01 UTC and t's
// midnight-UTC day. Matches the semantics of DrawDate's min/max fields.
func daysEpoch(t time.Time) int64 {
	utc := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	return utc.Unix() / 86400
}

// drainRuntime runs a Runtime to EOF and returns the rows in emit order.
func drainRuntime(t *testing.T, rt *runtime.Runtime) [][]any {
	t.Helper()

	var rows [][]any
	for {
		row, err := rt.Next()
		if errors.Is(err, io.EOF) {
			return rows
		}
		if err != nil {
			t.Fatalf("runtime.Next: %v", err)
		}
		out := make([]any, len(row))
		copy(out, row)
		rows = append(rows, out)
	}
}

// drainSpec builds a Runtime from spec and drains it to [][]any. Fatal on
// any construction or iteration error.
func drainSpec(t *testing.T, spec *dgproto.InsertSpec) [][]any {
	t.Helper()

	rt, err := runtime.NewRuntime(spec)
	if err != nil {
		t.Fatalf("NewRuntime(%s): %v", spec.GetTable(), err)
	}
	return drainRuntime(t, rt)
}

// copyRowsTo bulk-inserts rows into the given table via the Postgres COPY
// protocol. Returns the number of rows inserted. Fatal on COPY error.
func copyRowsTo(
	t *testing.T,
	pool *pgxpool.Pool,
	table string,
	columns []string,
	rows [][]any,
) int64 {
	t.Helper()

	n, err := pool.CopyFrom(
		context.Background(),
		pgx.Identifier{table},
		columns,
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		t.Fatalf("CopyFrom(%s): %v", table, err)
	}
	return n
}

// loadSpec drains spec and bulk-loads the rows into table via COPY. Returns
// the row count inserted.
func loadSpec(
	t *testing.T,
	pool *pgxpool.Pool,
	spec *dgproto.InsertSpec,
	table string,
	columns []string,
) int64 {
	t.Helper()

	return copyRowsTo(t, pool, table, columns, drainSpec(t, spec))
}
