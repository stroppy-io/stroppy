package expr

import (
	"fmt"
	"math"
	"time"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
)

// Slot is the evaluator's internal tagged value. Hot scalar arms keep
// values inline and avoid converting every intermediate result to any.
type Slot struct {
	kind SlotKind
	i64  int64
	f64  float64
	str  string
	ref  any
}

type SlotKind uint8

const (
	secondsPerDay = 86_400

	SlotUnset SlotKind = iota
	SlotNull
	SlotInt64
	SlotFloat64
	SlotBool
	SlotString
	SlotTime
	SlotAny
)

type SlotContext interface {
	Context
	SlotValue(index int) (Slot, bool)
}

type SlotEval func(ctx SlotContext) (Slot, error)

func SlotFromAny(value any) Slot {
	switch typed := value.(type) {
	case nil:
		return SlotNullValue()
	case int64:
		return SlotInt64Value(typed)
	case int:
		return SlotInt64Value(int64(typed))
	case int32:
		return SlotInt64Value(int64(typed))
	case int16:
		return SlotInt64Value(int64(typed))
	case int8:
		return SlotInt64Value(int64(typed))
	case uint8:
		return SlotInt64Value(int64(typed))
	case uint16:
		return SlotInt64Value(int64(typed))
	case uint32:
		return SlotInt64Value(int64(typed))
	case float64:
		return SlotFloat64Value(typed)
	case float32:
		return SlotFloat64Value(float64(typed))
	case bool:
		return SlotBoolValue(typed)
	case string:
		return SlotStringValue(typed)
	case time.Time:
		return SlotTimeValue(typed)
	default:
		return Slot{kind: SlotAny, ref: value}
	}
}

func SlotNullValue() Slot { return Slot{kind: SlotNull} }

func SlotInt64Value(value int64) Slot { return Slot{kind: SlotInt64, i64: value} }

func SlotFloat64Value(value float64) Slot { return Slot{kind: SlotFloat64, f64: value} }

func SlotBoolValue(value bool) Slot {
	if value {
		return Slot{kind: SlotBool, i64: 1}
	}

	return Slot{kind: SlotBool}
}

func SlotStringValue(value string) Slot { return Slot{kind: SlotString, str: value} }

func SlotTimeValue(value time.Time) Slot { return Slot{kind: SlotTime, i64: value.UnixNano()} }

func (slot Slot) Any() any {
	switch slot.kind {
	case SlotNull, SlotUnset:
		return nil
	case SlotInt64:
		return slot.i64
	case SlotFloat64:
		return slot.f64
	case SlotBool:
		return slot.i64 != 0
	case SlotString:
		return slot.str
	case SlotTime:
		return time.Unix(0, slot.i64).UTC()
	case SlotAny:
		return slot.ref
	default:
		return nil
	}
}

// CompileSlot builds a typed fast-path evaluator for node. Unsupported
// arms fall back to Eval, preserving existing behavior and errors.
func CompileSlot(node *dgproto.Expr, colIndex map[string]int, dicts map[string]*dgproto.Dict) SlotEval {
	if node == nil || node.GetKind() == nil {
		return fallbackSlotEval(node)
	}

	switch kind := node.GetKind().(type) {
	case *dgproto.Expr_Lit:
		return compileSlotLiteral(kind.Lit)
	case *dgproto.Expr_RowIndex:
		rowIndex := kind.RowIndex.GetKind()

		return func(ctx SlotContext) (Slot, error) {
			return SlotInt64Value(ctx.RowIndex(rowIndex)), nil
		}
	case *dgproto.Expr_Col:
		idx, ok := colIndex[kind.Col.GetName()]
		if !ok {
			return fallbackSlotEval(node)
		}

		return func(ctx SlotContext) (Slot, error) {
			value, set := ctx.SlotValue(idx)
			if !set {
				return Slot{}, ErrUnknownCol
			}

			return value, nil
		}
	case *dgproto.Expr_BinOp:
		return compileSlotBinOp(node, kind.BinOp, colIndex, dicts)
	case *dgproto.Expr_If_:
		return compileSlotIf(node, kind.If_, colIndex, dicts)
	case *dgproto.Expr_DictAt:
		return compileSlotDictAt(node, kind.DictAt, colIndex, dicts)
	case *dgproto.Expr_Call:
		return compileSlotCall(node, kind.Call, colIndex, dicts)
	default:
		return fallbackSlotEval(node)
	}
}

func fallbackSlotEval(node *dgproto.Expr) SlotEval {
	return func(ctx SlotContext) (Slot, error) {
		value, err := Eval(ctx, node)
		if err != nil {
			return Slot{}, err
		}

		return SlotFromAny(value), nil
	}
}

func compileSlotLiteral(lit *dgproto.Literal) SlotEval {
	if lit == nil || lit.GetValue() == nil {
		return fallbackSlotEval(&dgproto.Expr{Kind: &dgproto.Expr_Lit{Lit: lit}})
	}

	switch value := lit.GetValue().(type) {
	case *dgproto.Literal_Int64:
		out := SlotInt64Value(value.Int64)

		return constSlotEval(out)
	case *dgproto.Literal_Double:
		out := SlotFloat64Value(value.Double)

		return constSlotEval(out)
	case *dgproto.Literal_String_:
		out := SlotStringValue(value.String_)

		return constSlotEval(out)
	case *dgproto.Literal_Bool:
		out := SlotBoolValue(value.Bool)

		return constSlotEval(out)
	case *dgproto.Literal_Timestamp:
		out := SlotTimeValue(value.Timestamp.AsTime())

		return constSlotEval(out)
	case *dgproto.Literal_Null:
		return constSlotEval(SlotNullValue())
	default:
		return func(_ SlotContext) (Slot, error) {
			value, err := evalLiteral(lit)
			if err != nil {
				return Slot{}, err
			}

			return SlotFromAny(value), nil
		}
	}
}

func constSlotEval(value Slot) SlotEval {
	return func(_ SlotContext) (Slot, error) { return value, nil }
}

func compileSlotBinOp(
	node *dgproto.Expr,
	binOp *dgproto.BinOp,
	colIndex map[string]int,
	dicts map[string]*dgproto.Dict,
) SlotEval {
	if binOp == nil {
		return fallbackSlotEval(node)
	}

	op := binOp.GetOp()
	leftEval := CompileSlot(binOp.GetA(), colIndex, dicts)
	rightEval := CompileSlot(binOp.GetB(), colIndex, dicts)

	switch op {
	case dgproto.BinOp_AND:
		return compileSlotAnd(leftEval, rightEval)
	case dgproto.BinOp_OR:
		return compileSlotOr(leftEval, rightEval)
	case dgproto.BinOp_NOT:
		return compileSlotNot(leftEval)
	case dgproto.BinOp_ADD, dgproto.BinOp_SUB, dgproto.BinOp_MUL, dgproto.BinOp_DIV, dgproto.BinOp_MOD,
		dgproto.BinOp_LT, dgproto.BinOp_LE, dgproto.BinOp_GT, dgproto.BinOp_GE:
		return compileSlotBinaryTyped(op, leftEval, rightEval)
	case dgproto.BinOp_CONCAT, dgproto.BinOp_EQ, dgproto.BinOp_NE, dgproto.BinOp_OP_UNSPECIFIED:
		return fallbackSlotEval(node)
	default:
		return fallbackSlotEval(node)
	}
}

func compileSlotAnd(leftEval, rightEval SlotEval) SlotEval {
	return func(ctx SlotContext) (Slot, error) {
		leftBool, err := evalSlotBool(ctx, leftEval, "logical")
		if err != nil {
			return Slot{}, err
		}

		if !leftBool {
			return SlotBoolValue(false), nil
		}

		rightBool, err := evalSlotBool(ctx, rightEval, "logical")
		if err != nil {
			return Slot{}, err
		}

		return SlotBoolValue(rightBool), nil
	}
}

func compileSlotOr(leftEval, rightEval SlotEval) SlotEval {
	return func(ctx SlotContext) (Slot, error) {
		leftBool, err := evalSlotBool(ctx, leftEval, "logical")
		if err != nil {
			return Slot{}, err
		}

		if leftBool {
			return SlotBoolValue(true), nil
		}

		rightBool, err := evalSlotBool(ctx, rightEval, "logical")
		if err != nil {
			return Slot{}, err
		}

		return SlotBoolValue(rightBool), nil
	}
}

func compileSlotNot(eval SlotEval) SlotEval {
	return func(ctx SlotContext) (Slot, error) {
		value, err := evalSlotBool(ctx, eval, "NOT")
		if err != nil {
			return Slot{}, err
		}

		return SlotBoolValue(!value), nil
	}
}

func evalSlotBool(ctx SlotContext, eval SlotEval, opName string) (bool, error) {
	value, err := eval(ctx)
	if err != nil {
		return false, err
	}

	boolValue, ok := value.Bool()
	if !ok {
		return false, fmt.Errorf("%w: %s on %T", ErrTypeMismatch, opName, value.Any())
	}

	return boolValue, nil
}

func compileSlotBinaryTyped(op dgproto.BinOp_Op, leftEval, rightEval SlotEval) SlotEval {
	return func(ctx SlotContext) (Slot, error) {
		left, err := leftEval(ctx)
		if err != nil {
			return Slot{}, err
		}

		right, err := rightEval(ctx)
		if err != nil {
			return Slot{}, err
		}

		switch op {
		case dgproto.BinOp_ADD, dgproto.BinOp_SUB, dgproto.BinOp_MUL, dgproto.BinOp_DIV, dgproto.BinOp_MOD:
			return slotArith(op, left, right)
		case dgproto.BinOp_LT, dgproto.BinOp_LE, dgproto.BinOp_GT, dgproto.BinOp_GE:
			return slotOrder(op, left, right)
		default:
			return Slot{}, fmt.Errorf("%w: op %s", ErrBadExpr, op)
		}
	}
}

func slotArith(op dgproto.BinOp_Op, left, right Slot) (Slot, error) {
	leftNum, leftFloat, ok := left.Number()
	if !ok {
		return Slot{}, fmt.Errorf("%w: number %T", ErrTypeMismatch, left.Any())
	}

	rightNum, rightFloat, ok := right.Number()
	if !ok {
		return Slot{}, fmt.Errorf("%w: number %T", ErrTypeMismatch, right.Any())
	}

	if leftFloat || rightFloat {
		return slotFloatArith(op, leftNum, rightNum)
	}

	return slotIntArith(op, int64(leftNum), int64(rightNum))
}

func slotFloatArith(op dgproto.BinOp_Op, left, right float64) (Slot, error) {
	switch op {
	case dgproto.BinOp_ADD:
		return SlotFloat64Value(left + right), nil
	case dgproto.BinOp_SUB:
		return SlotFloat64Value(left - right), nil
	case dgproto.BinOp_MUL:
		return SlotFloat64Value(left * right), nil
	case dgproto.BinOp_DIV:
		if right == 0 {
			return Slot{}, ErrDivByZero
		}

		return SlotFloat64Value(left / right), nil
	case dgproto.BinOp_MOD:
		if right == 0 {
			return Slot{}, ErrModByZero
		}

		return SlotFloat64Value(float64(int64(left) % int64(right))), nil
	default:
		return Slot{}, fmt.Errorf("%w: arith op %s", ErrBadExpr, op)
	}
}

func slotIntArith(op dgproto.BinOp_Op, left, right int64) (Slot, error) {
	switch op {
	case dgproto.BinOp_ADD:
		return SlotInt64Value(left + right), nil
	case dgproto.BinOp_SUB:
		return SlotInt64Value(left - right), nil
	case dgproto.BinOp_MUL:
		return SlotInt64Value(left * right), nil
	case dgproto.BinOp_DIV:
		if right == 0 {
			return Slot{}, ErrDivByZero
		}

		return SlotInt64Value(left / right), nil
	case dgproto.BinOp_MOD:
		if right == 0 {
			return Slot{}, ErrModByZero
		}

		return SlotInt64Value(left % right), nil
	default:
		return Slot{}, fmt.Errorf("%w: arith op %s", ErrBadExpr, op)
	}
}

func slotOrder(op dgproto.BinOp_Op, left, right Slot) (Slot, error) {
	if left.kind == SlotString && right.kind == SlotString {
		return SlotBoolValue(cmpOrder(op, stringCmp(left.str, right.str))), nil
	}

	leftNum, _, leftOK := left.Number()

	rightNum, _, rightOK := right.Number()
	if !leftOK || !rightOK {
		return Slot{}, fmt.Errorf("%w: order %T vs %T", ErrTypeMismatch, left.Any(), right.Any())
	}

	switch {
	case leftNum < rightNum:
		return SlotBoolValue(cmpOrder(op, -1)), nil
	case leftNum > rightNum:
		return SlotBoolValue(cmpOrder(op, 1)), nil
	default:
		return SlotBoolValue(cmpOrder(op, 0)), nil
	}
}

func compileSlotIf(
	node *dgproto.Expr,
	ifNode *dgproto.If,
	colIndex map[string]int,
	dicts map[string]*dgproto.Dict,
) SlotEval {
	if ifNode == nil {
		return fallbackSlotEval(node)
	}

	condEval := CompileSlot(ifNode.GetCond(), colIndex, dicts)
	thenEval := CompileSlot(ifNode.GetThen(), colIndex, dicts)
	elseEval := CompileSlot(ifNode.GetElse_(), colIndex, dicts)

	return func(ctx SlotContext) (Slot, error) {
		cond, err := condEval(ctx)
		if err != nil {
			return Slot{}, err
		}

		value, ok := cond.Bool()
		if !ok {
			return Slot{}, fmt.Errorf("%w: if cond %T", ErrTypeMismatch, cond.Any())
		}

		if value {
			return thenEval(ctx)
		}

		return elseEval(ctx)
	}
}

func compileSlotDictAt(
	node *dgproto.Expr,
	dictAt *dgproto.DictAt,
	colIndex map[string]int,
	dicts map[string]*dgproto.Dict,
) SlotEval {
	if dictAt == nil {
		return fallbackSlotEval(node)
	}

	indexEval := CompileSlot(dictAt.GetIndex(), colIndex, dicts)
	dictKey := dictAt.GetDictKey()
	compiledDict := dicts[dictKey]

	return func(ctx SlotContext) (Slot, error) {
		indexSlot, err := indexEval(ctx)
		if err != nil {
			return Slot{}, err
		}

		index, ok := indexSlot.Int64()
		if !ok {
			return Slot{}, fmt.Errorf("%w: dict index %T", ErrTypeMismatch, indexSlot.Any())
		}

		dict := compiledDict
		if dict == nil {
			dict, err = ctx.LookupDict(dictKey)
			if err != nil {
				return Slot{}, err
			}
		}

		value, err := dictAtValue(dict, dictKey, index)
		if err != nil {
			return Slot{}, err
		}

		return SlotStringValue(value), nil
	}
}

func dictAtValue(dict *dgproto.Dict, dictKey string, index int64) (string, error) {
	if len(dict.GetColumns()) > 1 {
		return "", fmt.Errorf("%w: multi-column dict %q", ErrTypeMismatch, dictKey)
	}

	rows := dict.GetRows()
	if len(rows) == 0 {
		return "", fmt.Errorf("%w: empty dict %q", ErrBadExpr, dictKey)
	}

	count := int64(len(rows))
	position := ((index % count) + count) % count

	values := rows[position].GetValues()
	if len(values) == 0 {
		return "", fmt.Errorf("%w: dict row empty in %q", ErrBadExpr, dictKey)
	}

	return values[0], nil
}

func compileSlotCall(
	node *dgproto.Expr,
	call *dgproto.Call,
	colIndex map[string]int,
	dicts map[string]*dgproto.Dict,
) SlotEval {
	if call == nil || call.GetFunc() != "std.daysToDate" || len(call.GetArgs()) != 1 {
		return fallbackSlotEval(node)
	}

	argEval := CompileSlot(call.GetArgs()[0], colIndex, dicts)
	fallback := fallbackSlotEval(node)

	return func(ctx SlotContext) (Slot, error) {
		arg, err := argEval(ctx)
		if err != nil {
			return Slot{}, err
		}

		days, ok := arg.Int64()
		if !ok {
			return fallback(ctx)
		}

		return SlotTimeValue(time.Unix(days*secondsPerDay, 0).UTC()), nil
	}
}

func (slot Slot) Int64() (int64, bool) {
	if slot.kind != SlotInt64 {
		return 0, false
	}

	return slot.i64, true
}

func (slot Slot) Bool() (value, ok bool) {
	if slot.kind != SlotBool {
		return false, false
	}

	return slot.i64 != 0, true
}

func (slot Slot) Number() (value float64, isFloat, ok bool) {
	switch slot.kind {
	case SlotInt64:
		return float64(slot.i64), false, true
	case SlotFloat64:
		return slot.f64, true, true
	case SlotAny:
		num, anyIsFloat, err := toNumber(slot.ref)
		if err != nil {
			return math.NaN(), false, false
		}

		return num, anyIsFloat, true
	default:
		return math.NaN(), false, false
	}
}
