package expr

import (
	"fmt"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
)

// evalIf evaluates the condition and exactly one of the branches.
// A non-boolean condition returns ErrTypeMismatch.
func evalIf(ctx Context, node *dgproto.If) (any, error) {
	condVal, err := Eval(ctx, node.GetCond())
	if err != nil {
		return nil, err
	}

	cond, ok := condVal.(bool)
	if !ok {
		return nil, fmt.Errorf("%w: if cond %T", ErrTypeMismatch, condVal)
	}

	if cond {
		return Eval(ctx, node.GetThen())
	}

	return Eval(ctx, node.GetElse_())
}
