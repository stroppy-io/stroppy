package expr

import "github.com/stroppy-io/stroppy/pkg/datagen/dgproto"

// evalCall evaluates each argument and delegates the dispatch to
// Context.Call.
func evalCall(ctx Context, node *dgproto.Call) (any, error) {
	args := make([]any, len(node.GetArgs()))
	for i, argExpr := range node.GetArgs() {
		value, err := Eval(ctx, argExpr)
		if err != nil {
			return nil, err
		}

		args[i] = value
	}

	return ctx.Call(node.GetFunc(), args)
}
