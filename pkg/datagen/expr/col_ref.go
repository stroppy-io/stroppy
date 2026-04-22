package expr

import "github.com/stroppy-io/stroppy/pkg/datagen/dgproto"

// evalColRef resolves a ColRef through the Context's row scratch.
func evalColRef(ctx Context, c *dgproto.ColRef) (any, error) {
	return ctx.LookupCol(c.GetName())
}
