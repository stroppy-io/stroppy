package expr

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"reflect"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
)

// secondsPerDayTest mirrors stdlib's std.daysToDate day length so the
// fakeCtx Call hook reproduces the real runtime semantics; CompileSlot's
// typed std.daysToDate arm must match exactly.
const secondsPerDayTest = 86_400

// fakeDaysToDate mirrors stdlib.daysToDate: UTC midnight at 1970-01-01 +
// days. Registered into the fakeCtx so Eval routes std.daysToDate to the
// same value CompileSlot's typed arm computes inline.
func fakeDaysToDate(args []any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("%w: std.daysToDate needs 1, got %d", ErrUnknownCall, len(args))
	}

	days, ok := args[0].(int64)
	if !ok {
		return nil, fmt.Errorf("%w: std.daysToDate arg 0: %T", ErrTypeMismatch, args[0])
	}

	return time.Unix(days*secondsPerDayTest, 0).UTC(), nil
}

// fakeFormat is a tiny deterministic stand-in for std.format. Both Eval
// and CompileSlot's fallback route through this same hook, so the value
// matches by construction; it only needs to be deterministic.
func fakeFormat(args []any) (any, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("%w: std.format needs >=1 arg", ErrUnknownCall)
	}

	format, ok := args[0].(string)
	if !ok {
		return nil, fmt.Errorf("%w: std.format arg 0: %T", ErrTypeMismatch, args[0])
	}

	return fmt.Sprintf(format, args[1:]...), nil
}

// slotFakeCtx adapts the package's fakeCtx into a SlotContext. Col reads
// resolve through both paths so CompileSlot (index-based) and Eval
// (name-based) observe identical column values:
//   - Eval calls LookupCol(name) -> fakeCtx.cols[name]
//   - CompileSlot calls SlotValue(index) -> slots[index]
//
// colIndex maps name -> index and slots[index] holds the SlotFromAny of
// the same value stored in cols[name], keeping the two views consistent.
type slotFakeCtx struct {
	*fakeCtx
	slots []Slot
	set   []bool
}

func (s *slotFakeCtx) SlotValue(index int) (Slot, bool) {
	if index < 0 || index >= len(s.slots) || !s.set[index] {
		return Slot{}, false
	}

	return s.slots[index], true
}

// newSlotFakeCtx builds a SlotContext whose columns carry the provided
// name->value bindings, exposing a colIndex usable by CompileSlot.
func newSlotFakeCtx(cols map[string]any, rootSeed uint64) (*slotFakeCtx, map[string]int) {
	base := newFakeCtx()
	base.rootSeed = rootSeed
	base.calls["std.daysToDate"] = fakeDaysToDate
	base.calls["std.format"] = fakeFormat

	colIndex := make(map[string]int, len(cols))
	slots := make([]Slot, 0, len(cols))
	set := make([]bool, 0, len(cols))

	for name, value := range cols {
		base.cols[name] = value
		colIndex[name] = len(slots)
		slots = append(slots, SlotFromAny(value))
		set = append(set, true)
	}

	return &slotFakeCtx{fakeCtx: base, slots: slots, set: set}, colIndex
}

// assertSlotEqualsEval is the core equivalence oracle: Eval is the source
// of truth. For every Expr shape, CompileSlot(e)(ctx).Any() must equal
// Eval(ctx, e) in both value and dynamic type, and the error behavior
// (success vs failure) must match.
func assertSlotEqualsEval(
	t *testing.T,
	label string,
	e *dgproto.Expr,
	cols map[string]any,
	dicts map[string]*dgproto.Dict,
	rootSeed uint64,
) {
	t.Helper()

	ctx, colIndex := newSlotFakeCtx(cols, rootSeed)
	// The runtime feeds one shared dict map to both the evalContext
	// (consulted by Eval via LookupDict) and CompileSlot. Mirror that so
	// the two paths resolve dicts identically.
	for key, dict := range dicts {
		ctx.dicts[key] = dict
	}

	want, wantErr := Eval(ctx, e)

	eval := CompileSlot(e, colIndex, dicts)

	gotSlot, gotErr := eval(ctx)
	got := gotSlot.Any()

	if (wantErr == nil) != (gotErr == nil) {
		t.Fatalf("%s: error mismatch: Eval err=%v, CompileSlot err=%v", label, wantErr, gotErr)
	}

	if wantErr != nil {
		// Both failed; require the same sentinel where Eval produced one.
		for _, sentinel := range []error{
			ErrBadExpr, ErrTypeMismatch, ErrUnknownCol, ErrDictMissing,
			ErrDivByZero, ErrModByZero, ErrUnknownCall,
		} {
			if errors.Is(wantErr, sentinel) && !errors.Is(gotErr, sentinel) {
				t.Fatalf("%s: sentinel mismatch: Eval=%v, CompileSlot=%v", label, wantErr, gotErr)
			}
		}

		return
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s: value/type mismatch:\n  Eval        = %#v (%T)\n  CompileSlot = %#v (%T)",
			label, want, want, got, got)
	}
}

// binOp is a small constructor for BinOp expressions.
func binOp(op dgproto.BinOp_Op, a, b *dgproto.Expr) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_BinOp{BinOp: &dgproto.BinOp{Op: op, A: a, B: b}}}
}

func unOp(op dgproto.BinOp_Op, a *dgproto.Expr) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_BinOp{BinOp: &dgproto.BinOp{Op: op, A: a}}}
}

func ifExpr(cond, then, els *dgproto.Expr) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_If_{If_: &dgproto.If{Cond: cond, Then: then, Else_: els}}}
}

func colExpr(name string) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_Col{Col: &dgproto.ColRef{Name: name}}}
}

func rowIndexExpr(kind dgproto.RowIndex_Kind) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_RowIndex{RowIndex: &dgproto.RowIndex{Kind: kind}}}
}

// dictAtExpr is provided by dict_at_test.go in this package.

// callExprArgs builds a Call Expr with arguments (the bare callExpr helper
// in if_test.go takes only a name).
func callExprArgs(fn string, args ...*dgproto.Expr) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_Call{Call: &dgproto.Call{Func: fn, Args: args}}}
}

// TestCompileSlotEquivTable drives a broad fixed table of Expr shapes
// through the Eval==CompileSlot oracle.
func TestCompileSlotEquivTable(t *testing.T) {
	dicts := map[string]*dgproto.Dict{
		"colors": {
			Columns: []string{"name"},
			Rows: []*dgproto.DictRow{
				{Values: []string{"red"}},
				{Values: []string{"green"}},
				{Values: []string{"blue"}},
			},
		},
	}

	cols := map[string]any{
		"i":    int64(7),
		"j":    int64(3),
		"f":    float64(2.5),
		"g":    float64(4.0),
		"flag": true,
		"name": "alpha",
	}

	ts := timestamppb.New(time.Unix(1_700_000_000, 0).UTC())

	cases := []struct {
		name string
		e    *dgproto.Expr
	}{
		// Literals across every scalar type.
		{"lit_int", litInt(42)},
		{"lit_int_neg", litInt(-9)},
		{"lit_float", litFloat(3.14)},
		{"lit_float_whole", litFloat(8.0)},
		{"lit_bool_true", litBool(true)},
		{"lit_bool_false", litBool(false)},
		{"lit_str", litStr("hello")},
		{"lit_str_empty", litStr("")},
		{
			"lit_timestamp",
			&dgproto.Expr{Kind: &dgproto.Expr_Lit{Lit: &dgproto.Literal{
				Value: &dgproto.Literal_Timestamp{Timestamp: ts},
			}}},
		},
		{
			"lit_null",
			&dgproto.Expr{Kind: &dgproto.Expr_Lit{Lit: &dgproto.Literal{
				Value: &dgproto.Literal_Null{Null: &dgproto.NullMarker{}},
			}}},
		},

		// RowIndex (all kinds collapse onto the same axis in fakeCtx).
		{"row_index_global", rowIndexExpr(dgproto.RowIndex_GLOBAL)},
		{"row_index_unspecified", rowIndexExpr(dgproto.RowIndex_UNSPECIFIED)},
		{"row_index_entity", rowIndexExpr(dgproto.RowIndex_ENTITY)},
		{"row_index_line", rowIndexExpr(dgproto.RowIndex_LINE)},

		// Column refs of every value type.
		{"col_int", colExpr("i")},
		{"col_float", colExpr("f")},
		{"col_bool", colExpr("flag")},
		{"col_str", colExpr("name")},

		// Integer arithmetic.
		{"add_ii", binOp(dgproto.BinOp_ADD, litInt(2), litInt(3))},
		{"sub_ii", binOp(dgproto.BinOp_SUB, litInt(10), litInt(4))},
		{"mul_ii", binOp(dgproto.BinOp_MUL, litInt(6), litInt(7))},
		{"div_ii", binOp(dgproto.BinOp_DIV, litInt(20), litInt(6))},
		{"mod_ii", binOp(dgproto.BinOp_MOD, litInt(20), litInt(6))},
		{"add_cols_ii", binOp(dgproto.BinOp_ADD, colExpr("i"), colExpr("j"))},

		// Float arithmetic.
		{"add_ff", binOp(dgproto.BinOp_ADD, litFloat(1.5), litFloat(2.25))},
		{"sub_ff", binOp(dgproto.BinOp_SUB, litFloat(5.5), litFloat(1.25))},
		{"mul_ff", binOp(dgproto.BinOp_MUL, litFloat(2.0), litFloat(3.5))},
		{"div_ff", binOp(dgproto.BinOp_DIV, litFloat(7.0), litFloat(2.0))},
		{"mod_ff", binOp(dgproto.BinOp_MOD, litFloat(7.0), litFloat(3.0))},

		// Mixed int x float (promotes to float in both paths).
		{"add_if", binOp(dgproto.BinOp_ADD, litInt(3), litFloat(0.5))},
		{"add_fi", binOp(dgproto.BinOp_ADD, litFloat(0.5), litInt(3))},
		{"mul_if", binOp(dgproto.BinOp_MUL, litInt(4), litFloat(2.5))},
		{"div_if", binOp(dgproto.BinOp_DIV, litInt(9), litFloat(2.0))},
		{"add_col_if", binOp(dgproto.BinOp_ADD, colExpr("i"), colExpr("f"))},

		// Numeric comparisons.
		{"lt_true", binOp(dgproto.BinOp_LT, litInt(2), litInt(3))},
		{"lt_false", binOp(dgproto.BinOp_LT, litInt(3), litInt(2))},
		{"le_eq", binOp(dgproto.BinOp_LE, litInt(3), litInt(3))},
		{"gt_true", binOp(dgproto.BinOp_GT, litFloat(3.5), litFloat(2.5))},
		{"ge_true", binOp(dgproto.BinOp_GE, litInt(5), litInt(5))},
		{"lt_mixed", binOp(dgproto.BinOp_LT, litInt(2), litFloat(2.5))},
		{"gt_cols", binOp(dgproto.BinOp_GT, colExpr("i"), colExpr("j"))},

		// String ordering comparisons.
		{"lt_str", binOp(dgproto.BinOp_LT, litStr("apple"), litStr("banana"))},
		{"gt_str", binOp(dgproto.BinOp_GT, litStr("zebra"), litStr("ant"))},

		// Logical AND/OR/NOT, including short-circuit shapes.
		{"and_tt", binOp(dgproto.BinOp_AND, litBool(true), litBool(true))},
		{"and_tf", binOp(dgproto.BinOp_AND, litBool(true), litBool(false))},
		{"and_ff", binOp(dgproto.BinOp_AND, litBool(false), litBool(true))},
		{"or_ff", binOp(dgproto.BinOp_OR, litBool(false), litBool(false))},
		{"or_tf", binOp(dgproto.BinOp_OR, litBool(true), litBool(false))},
		{"not_t", unOp(dgproto.BinOp_NOT, litBool(true))},
		{"not_f", unOp(dgproto.BinOp_NOT, litBool(false))},
		{"and_cmp", binOp(dgproto.BinOp_AND,
			binOp(dgproto.BinOp_LT, litInt(1), litInt(2)),
			binOp(dgproto.BinOp_GT, litInt(5), litInt(4)))},

		// If across branch types.
		{"if_true_int", ifExpr(litBool(true), litInt(1), litInt(2))},
		{"if_false_int", ifExpr(litBool(false), litInt(1), litInt(2))},
		{"if_str", ifExpr(litBool(false), litStr("yes"), litStr("no"))},
		{"if_cond_expr", ifExpr(binOp(dgproto.BinOp_LT, colExpr("j"), colExpr("i")),
			litStr("j<i"), litStr("j>=i"))},
		{"if_float_branch", ifExpr(litBool(true), litFloat(1.5), litFloat(2.5))},

		// DictAt, including modular wraparound (positive and negative).
		{"dict_at_0", dictAtExpr("colors", litInt(0))},
		{"dict_at_1", dictAtExpr("colors", litInt(1))},
		{"dict_at_wrap", dictAtExpr("colors", litInt(4))},
		{"dict_at_neg", dictAtExpr("colors", litInt(-1))},
		{"dict_at_expr_index", dictAtExpr("colors", binOp(dgproto.BinOp_ADD, colExpr("i"), litInt(1)))},

		// Call: typed fast-path (std.daysToDate) and fallback (std.format).
		{"call_days_to_date", callExprArgs("std.daysToDate", litInt(100))},
		{"call_days_to_date_expr", callExprArgs("std.daysToDate", binOp(dgproto.BinOp_ADD, litInt(50), litInt(50)))},

		// Deeply nested arithmetic + comparison + branch.
		{
			"nested",
			ifExpr(
				binOp(dgproto.BinOp_GT,
					binOp(dgproto.BinOp_ADD, colExpr("i"), binOp(dgproto.BinOp_MUL, colExpr("j"), litInt(2))),
					litInt(10)),
				binOp(dgproto.BinOp_SUB, colExpr("f"), litFloat(0.5)),
				binOp(dgproto.BinOp_ADD, colExpr("f"), litFloat(0.5)),
			),
		},
	}

	rootSeeds := []uint64{0, 1, 0xDEADBEEF}

	for _, seed := range rootSeeds {
		for _, tc := range cases {
			assertSlotEqualsEval(t, tc.name, tc.e, cols, dicts, seed)
		}
	}
}

// TestCompileSlotEquivFallbackArms covers arms that MUST route through
// fallbackSlotEval: BinOp_CONCAT, BinOp_EQ, BinOp_NE, and a non-typed
// Call. Their values must still round-trip identically to Eval.
func TestCompileSlotEquivFallbackArms(t *testing.T) {
	cols := map[string]any{
		"a": "foo",
		"b": "bar",
		"i": int64(5),
	}

	// std.format is registered by stdlib; concat/eq/ne route through the
	// expr concat/compare kernels. CompileSlot routes all of these to
	// fallbackSlotEval, so equivalence must hold by construction.
	cases := []struct {
		name string
		e    *dgproto.Expr
	}{
		{"concat", binOp(dgproto.BinOp_CONCAT, litStr("a-"), litStr("b"))},
		{"concat_cols", binOp(dgproto.BinOp_CONCAT, colExpr("a"), colExpr("b"))},
		{"eq_int", binOp(dgproto.BinOp_EQ, litInt(3), litInt(3))},
		{"ne_int", binOp(dgproto.BinOp_NE, litInt(3), litInt(4))},
		{"eq_str", binOp(dgproto.BinOp_EQ, colExpr("a"), litStr("foo"))},
		{"call_format", callExprArgs("std.format", litStr("%d!"), colExpr("i"))},
	}

	dicts := map[string]*dgproto.Dict{}

	for _, tc := range cases {
		// newSlotFakeCtx registers std.format/std.daysToDate so Eval and
		// CompileSlot's fallback route through the same Call hook.
		ctx, colIndex := newSlotFakeCtx(cols, 0)

		want, wantErr := Eval(ctx, tc.e)

		gotSlot, gotErr := CompileSlot(tc.e, colIndex, dicts)(ctx)
		got := gotSlot.Any()

		if (wantErr == nil) != (gotErr == nil) {
			t.Fatalf("%s: error mismatch: Eval err=%v, CompileSlot err=%v", tc.name, wantErr, gotErr)
		}

		if wantErr != nil {
			continue
		}

		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: fallback value/type mismatch:\n  Eval        = %#v (%T)\n  CompileSlot = %#v (%T)",
				tc.name, want, want, got, got)
		}
	}
}

// TestCompileSlotEquivRandomized fuzzes random arithmetic/comparison/logic
// trees against the Eval oracle across many row indices and a couple of
// root seeds. Any divergence is a CompileSlot bug, never a test weakness.
func TestCompileSlotEquivRandomized(t *testing.T) {
	dicts := map[string]*dgproto.Dict{
		"d": {
			Columns: []string{"v"},
			Rows: []*dgproto.DictRow{
				{Values: []string{"zero"}},
				{Values: []string{"one"}},
				{Values: []string{"two"}},
				{Values: []string{"three"}},
			},
		},
	}

	for _, rootSeed := range []uint64{0x1234, 0xABCDEF, 99} {
		rng := rand.New(rand.NewPCG(rootSeed, rootSeed^0x9E3779B97F4A7C15))

		for iter := 0; iter < 4000; iter++ {
			rowIdx := int64(rng.Uint64() % 1_000_003)

			cols := map[string]any{
				"i": int64(rng.Int64N(2001) - 1000),
				"j": int64(rng.Int64N(2001) - 1000),
				"f": float64(rng.Int64N(20001)-10000) / 100.0,
				"g": float64(rng.Int64N(20001)-10000) / 100.0,
				"b": rng.Uint64()&1 == 0,
				"c": rng.Uint64()&1 == 0,
			}

			e := randExpr(rng, 4)

			ctx, colIndex := newSlotFakeCtxRow(cols, rootSeed, rowIdx)
			for key, dict := range dicts {
				ctx.dicts[key] = dict
			}

			want, wantErr := Eval(ctx, e)

			gotSlot, gotErr := CompileSlot(e, colIndex, dicts)(ctx)
			got := gotSlot.Any()

			if (wantErr == nil) != (gotErr == nil) {
				t.Fatalf("seed %d iter %d: error mismatch: Eval=%v CompileSlot=%v\nexpr=%v",
					rootSeed, iter, wantErr, gotErr, e)
			}

			if wantErr != nil {
				continue
			}

			if !reflect.DeepEqual(got, want) {
				t.Fatalf("seed %d iter %d row %d: mismatch:\n  Eval        = %#v (%T)\n  CompileSlot = %#v (%T)\n  expr        = %v",
					rootSeed, iter, rowIdx, want, want, got, got, e)
			}
		}
	}
}

// newSlotFakeCtxRow is like newSlotFakeCtx but also pins the row counter
// so RowIndex arms in fuzzed trees resolve deterministically.
func newSlotFakeCtxRow(cols map[string]any, rootSeed uint64, rowIdx int64) (*slotFakeCtx, map[string]int) {
	ctx, colIndex := newSlotFakeCtx(cols, rootSeed)
	for _, kind := range []dgproto.RowIndex_Kind{
		dgproto.RowIndex_UNSPECIFIED,
		dgproto.RowIndex_ENTITY,
		dgproto.RowIndex_LINE,
		dgproto.RowIndex_GLOBAL,
	} {
		ctx.rowIndex[kind] = rowIdx
	}

	return ctx, colIndex
}

// randExpr builds a random scalar Expr tree of bounded depth using only
// arms whose Eval result type survives the SlotFromAny round-trip
// (int64/float64/bool/string/time.Time). It deliberately includes both
// CompileSlot-typed arms and at least the DictAt arm.
func randExpr(rng *rand.Rand, depth int) *dgproto.Expr {
	if depth <= 0 {
		return randLeaf(rng)
	}

	switch rng.IntN(10) {
	case 0: // ADD/SUB/MUL on arbitrary numeric operands
		ops := []dgproto.BinOp_Op{
			dgproto.BinOp_ADD, dgproto.BinOp_SUB, dgproto.BinOp_MUL,
		}
		op := ops[rng.IntN(len(ops))]

		return binOp(op, randNumExpr(rng, depth-1), randNumExpr(rng, depth-1))
	case 1: // DIV/MOD: divisor is a nonzero int literal.
		// Both Eval and CompileSlot compute float MOD as
		// float64(int64(left) % int64(right)); a divisor whose int64
		// truncation is zero panics in BOTH (pre-existing, identical),
		// so keep |divisor| >= 1 to stay in the defined domain.
		op := []dgproto.BinOp_Op{dgproto.BinOp_DIV, dgproto.BinOp_MOD}[rng.IntN(2)]
		divisor := rng.Int64N(1000) + 1
		if rng.IntN(2) == 0 {
			divisor = -divisor
		}

		return binOp(op, randNumExpr(rng, depth-1), litInt(divisor))
	case 2, 3: // comparison
		ops := []dgproto.BinOp_Op{
			dgproto.BinOp_LT, dgproto.BinOp_LE, dgproto.BinOp_GT, dgproto.BinOp_GE,
		}
		op := ops[rng.IntN(len(ops))]

		return binOp(op, randNumExpr(rng, depth-1), randNumExpr(rng, depth-1))
	case 4: // AND
		return binOp(dgproto.BinOp_AND, randBoolExpr(rng, depth-1), randBoolExpr(rng, depth-1))
	case 5: // OR
		return binOp(dgproto.BinOp_OR, randBoolExpr(rng, depth-1), randBoolExpr(rng, depth-1))
	case 6: // NOT
		return unOp(dgproto.BinOp_NOT, randBoolExpr(rng, depth-1))
	case 7: // If
		return ifExpr(randBoolExpr(rng, depth-1), randExpr(rng, depth-1), randExpr(rng, depth-1))
	case 8: // DictAt
		return dictAtExpr("d", randNumExpr(rng, depth-1))
	default: // std.daysToDate (typed Call arm)
		// Bound the day count to the range where time.Time has a valid
		// UnixNano representation. SlotTime stores UnixNano, so out-of-range
		// dates round-trip lossily through Slot.Any(); that is an inherent
		// property of the ported Slot design, not an equivalence violation,
		// and the runtime never produces such dates. Keep inputs in-domain.
		return callExprArgs("std.daysToDate", litInt(rng.Int64N(80_000)-30_000))
	}
}

func randNumExpr(rng *rand.Rand, depth int) *dgproto.Expr {
	if depth <= 0 || rng.IntN(2) == 0 {
		switch rng.IntN(4) {
		case 0:
			return litInt(rng.Int64N(2001) - 1000)
		case 1:
			return litFloat(float64(rng.Int64N(20001)-10000) / 100.0)
		case 2:
			return colExpr([]string{"i", "j"}[rng.IntN(2)])
		default:
			return colExpr([]string{"f", "g"}[rng.IntN(2)])
		}
	}

	ops := []dgproto.BinOp_Op{
		dgproto.BinOp_ADD, dgproto.BinOp_SUB, dgproto.BinOp_MUL,
	}

	return binOp(ops[rng.IntN(len(ops))], randNumExpr(rng, depth-1), randNumExpr(rng, depth-1))
}

func randBoolExpr(rng *rand.Rand, depth int) *dgproto.Expr {
	if depth <= 0 || rng.IntN(2) == 0 {
		switch rng.IntN(3) {
		case 0:
			return litBool(rng.IntN(2) == 0)
		case 1:
			return colExpr([]string{"b", "c"}[rng.IntN(2)])
		default:
			ops := []dgproto.BinOp_Op{
				dgproto.BinOp_LT, dgproto.BinOp_LE, dgproto.BinOp_GT, dgproto.BinOp_GE,
			}

			return binOp(ops[rng.IntN(len(ops))], randNumExpr(rng, 1), randNumExpr(rng, 1))
		}
	}

	switch rng.IntN(3) {
	case 0:
		return binOp(dgproto.BinOp_AND, randBoolExpr(rng, depth-1), randBoolExpr(rng, depth-1))
	case 1:
		return binOp(dgproto.BinOp_OR, randBoolExpr(rng, depth-1), randBoolExpr(rng, depth-1))
	default:
		return unOp(dgproto.BinOp_NOT, randBoolExpr(rng, depth-1))
	}
}

func randLeaf(rng *rand.Rand) *dgproto.Expr {
	switch rng.IntN(6) {
	case 0:
		return litInt(rng.Int64N(2001) - 1000)
	case 1:
		return litFloat(float64(rng.Int64N(20001)-10000) / 100.0)
	case 2:
		return litBool(rng.IntN(2) == 0)
	case 3:
		return colExpr([]string{"i", "j", "f", "g"}[rng.IntN(4)])
	case 4:
		return colExpr([]string{"b", "c"}[rng.IntN(2)])
	default:
		return rowIndexExpr(dgproto.RowIndex_GLOBAL)
	}
}
