package expr

import (
	"fmt"
	"reflect"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
)

// evalBinOp evaluates arithmetic, comparison, logical, concat, and NOT
// operators. Logical ops short-circuit; NOT is unary on operand a.
func evalBinOp(ctx Context, node *dgproto.BinOp) (any, error) {
	op := node.GetOp()
	switch op {
	case dgproto.BinOp_AND, dgproto.BinOp_OR:
		return evalLogical(ctx, node)
	case dgproto.BinOp_NOT:
		return evalNot(ctx, node)
	case dgproto.BinOp_OP_UNSPECIFIED,
		dgproto.BinOp_ADD, dgproto.BinOp_SUB, dgproto.BinOp_MUL,
		dgproto.BinOp_DIV, dgproto.BinOp_MOD,
		dgproto.BinOp_CONCAT,
		dgproto.BinOp_EQ, dgproto.BinOp_NE,
		dgproto.BinOp_LT, dgproto.BinOp_LE, dgproto.BinOp_GT, dgproto.BinOp_GE:
	}

	left, err := Eval(ctx, node.GetA())
	if err != nil {
		return nil, err
	}

	right, err := Eval(ctx, node.GetB())
	if err != nil {
		return nil, err
	}

	switch op {
	case dgproto.BinOp_ADD, dgproto.BinOp_SUB, dgproto.BinOp_MUL,
		dgproto.BinOp_DIV, dgproto.BinOp_MOD:
		return evalArith(op, left, right)
	case dgproto.BinOp_CONCAT:
		return fmt.Sprint(left) + fmt.Sprint(right), nil
	case dgproto.BinOp_EQ:
		return reflect.DeepEqual(left, right), nil
	case dgproto.BinOp_NE:
		return !reflect.DeepEqual(left, right), nil
	case dgproto.BinOp_LT, dgproto.BinOp_LE, dgproto.BinOp_GT, dgproto.BinOp_GE:
		return evalOrder(op, left, right)
	case dgproto.BinOp_OP_UNSPECIFIED,
		dgproto.BinOp_AND, dgproto.BinOp_OR, dgproto.BinOp_NOT:
		return nil, fmt.Errorf("%w: op %s", ErrBadExpr, op)
	default:
		return nil, fmt.Errorf("%w: op %s", ErrBadExpr, op)
	}
}

// evalLogical handles AND / OR with short-circuit semantics. Both
// operands must be bool; otherwise ErrTypeMismatch.
func evalLogical(ctx Context, node *dgproto.BinOp) (any, error) {
	left, err := Eval(ctx, node.GetA())
	if err != nil {
		return nil, err
	}

	lhs, ok := left.(bool)
	if !ok {
		return nil, fmt.Errorf("%w: logical on %T", ErrTypeMismatch, left)
	}

	if node.GetOp() == dgproto.BinOp_AND && !lhs {
		return false, nil
	}

	if node.GetOp() == dgproto.BinOp_OR && lhs {
		return true, nil
	}

	right, err := Eval(ctx, node.GetB())
	if err != nil {
		return nil, err
	}

	rhs, ok := right.(bool)
	if !ok {
		return nil, fmt.Errorf("%w: logical on %T", ErrTypeMismatch, right)
	}

	return rhs, nil
}

// evalNot evaluates the unary NOT on operand a only.
func evalNot(ctx Context, node *dgproto.BinOp) (any, error) {
	operand, err := Eval(ctx, node.GetA())
	if err != nil {
		return nil, err
	}

	value, ok := operand.(bool)
	if !ok {
		return nil, fmt.Errorf("%w: NOT on %T", ErrTypeMismatch, operand)
	}

	return !value, nil
}

// evalArith coerces both operands into a common numeric type (float64 if
// either side is float) and applies the operator. DIV / MOD on zero
// return ErrDivByZero / ErrModByZero.
func evalArith(op dgproto.BinOp_Op, left, right any) (any, error) {
	leftNum, leftIsFloat, err := toNumber(left)
	if err != nil {
		return nil, err
	}

	rightNum, rightIsFloat, err := toNumber(right)
	if err != nil {
		return nil, err
	}

	if leftIsFloat || rightIsFloat {
		return arithFloat(op, leftNum, rightNum)
	}

	return arithInt(op, int64(leftNum), int64(rightNum))
}

// arithFloat applies op to two float64 values.
func arithFloat(op dgproto.BinOp_Op, left, right float64) (any, error) {
	switch op {
	case dgproto.BinOp_ADD:
		return left + right, nil
	case dgproto.BinOp_SUB:
		return left - right, nil
	case dgproto.BinOp_MUL:
		return left * right, nil
	case dgproto.BinOp_DIV:
		if right == 0 {
			return nil, ErrDivByZero
		}

		return left / right, nil
	case dgproto.BinOp_MOD:
		if right == 0 {
			return nil, ErrModByZero
		}

		return float64(int64(left) % int64(right)), nil
	case dgproto.BinOp_OP_UNSPECIFIED,
		dgproto.BinOp_CONCAT,
		dgproto.BinOp_EQ, dgproto.BinOp_NE,
		dgproto.BinOp_LT, dgproto.BinOp_LE, dgproto.BinOp_GT, dgproto.BinOp_GE,
		dgproto.BinOp_AND, dgproto.BinOp_OR, dgproto.BinOp_NOT:
		return nil, fmt.Errorf("%w: arith op %s", ErrBadExpr, op)
	default:
		return nil, fmt.Errorf("%w: arith op %s", ErrBadExpr, op)
	}
}

// arithInt applies op to two int64 values.
func arithInt(op dgproto.BinOp_Op, left, right int64) (any, error) {
	switch op {
	case dgproto.BinOp_ADD:
		return left + right, nil
	case dgproto.BinOp_SUB:
		return left - right, nil
	case dgproto.BinOp_MUL:
		return left * right, nil
	case dgproto.BinOp_DIV:
		if right == 0 {
			return nil, ErrDivByZero
		}

		return left / right, nil
	case dgproto.BinOp_MOD:
		if right == 0 {
			return nil, ErrModByZero
		}

		return left % right, nil
	case dgproto.BinOp_OP_UNSPECIFIED,
		dgproto.BinOp_CONCAT,
		dgproto.BinOp_EQ, dgproto.BinOp_NE,
		dgproto.BinOp_LT, dgproto.BinOp_LE, dgproto.BinOp_GT, dgproto.BinOp_GE,
		dgproto.BinOp_AND, dgproto.BinOp_OR, dgproto.BinOp_NOT:
		return nil, fmt.Errorf("%w: arith op %s", ErrBadExpr, op)
	default:
		return nil, fmt.Errorf("%w: arith op %s", ErrBadExpr, op)
	}
}

// evalOrder applies LT/LE/GT/GE to ordered operand types (numeric or
// string). Mixed or unordered types return ErrTypeMismatch.
func evalOrder(op dgproto.BinOp_Op, left, right any) (any, error) {
	if leftStr, leftOk := left.(string); leftOk {
		rightStr, rightOk := right.(string)
		if !rightOk {
			return nil, fmt.Errorf("%w: order %T vs %T", ErrTypeMismatch, left, right)
		}

		return cmpOrder(op, stringCmp(leftStr, rightStr)), nil
	}

	leftNum, _, errLeft := toNumber(left)
	if errLeft != nil {
		return nil, fmt.Errorf("%w: order %T", ErrTypeMismatch, left)
	}

	rightNum, _, errRight := toNumber(right)
	if errRight != nil {
		return nil, fmt.Errorf("%w: order %T", ErrTypeMismatch, right)
	}

	switch {
	case leftNum < rightNum:
		return cmpOrder(op, -1), nil
	case leftNum > rightNum:
		return cmpOrder(op, 1), nil
	default:
		return cmpOrder(op, 0), nil
	}
}

// cmpOrder maps a sign (-1, 0, 1) through the requested comparison op.
func cmpOrder(op dgproto.BinOp_Op, sign int) bool {
	switch op {
	case dgproto.BinOp_LT:
		return sign < 0
	case dgproto.BinOp_LE:
		return sign <= 0
	case dgproto.BinOp_GT:
		return sign > 0
	case dgproto.BinOp_GE:
		return sign >= 0
	case dgproto.BinOp_OP_UNSPECIFIED,
		dgproto.BinOp_ADD, dgproto.BinOp_SUB, dgproto.BinOp_MUL,
		dgproto.BinOp_DIV, dgproto.BinOp_MOD,
		dgproto.BinOp_CONCAT,
		dgproto.BinOp_EQ, dgproto.BinOp_NE,
		dgproto.BinOp_AND, dgproto.BinOp_OR, dgproto.BinOp_NOT:
		return false
	}

	return false
}

// stringCmp returns -1, 0, or 1.
func stringCmp(left, right string) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}

// toNumber reduces any integer or float type to float64. The isFloat
// flag reports whether the source value was a floating-point type.
// Non-numeric values return ErrTypeMismatch.
func toNumber(value any) (num float64, isFloat bool, err error) {
	switch typed := value.(type) {
	case int:
		return float64(typed), false, nil
	case int32:
		return float64(typed), false, nil
	case int64:
		return float64(typed), false, nil
	case uint:
		return float64(typed), false, nil
	case uint32:
		return float64(typed), false, nil
	case uint64:
		return float64(typed), false, nil
	case float32:
		return float64(typed), true, nil
	case float64:
		return typed, true, nil
	default:
		return 0, false, fmt.Errorf("%w: not a number: %T", ErrTypeMismatch, value)
	}
}
