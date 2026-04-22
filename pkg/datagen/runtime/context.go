package runtime

import (
	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
	"github.com/stroppy-io/stroppy/pkg/datagen/expr"
	"github.com/stroppy-io/stroppy/pkg/datagen/stdlib"
)

// evalContext adapts a Runtime's per-row state to the expr.Context
// interface. A single evalContext is reused across rows: Runtime mutates
// scratch and rowIdx between evaluations rather than allocating a fresh
// context each row.
type evalContext struct {
	scratch map[string]any
	rowIdx  int64
	dicts   map[string]*dgproto.Dict
}

// LookupCol resolves a ColRef by consulting the current row's scratch
// map, returning expr.ErrUnknownCol when the referenced attr has not yet
// been evaluated (for example, a forward reference or a DAG bug).
func (c *evalContext) LookupCol(name string) (any, error) {
	value, ok := c.scratch[name]
	if !ok {
		return nil, expr.ErrUnknownCol
	}

	return value, nil
}

// RowIndex returns the current row counter. The flat runtime has a
// single iteration axis, so every RowIndex kind maps onto the same
// counter; relationship-aware runtimes in later stages will distinguish
// ENTITY, LINE, and GLOBAL.
func (c *evalContext) RowIndex(_ dgproto.RowIndex_Kind) int64 {
	return c.rowIdx
}

// LookupDict returns the Dict identified by key from the InsertSpec's
// dicts map, or expr.ErrDictMissing when the key is absent.
func (c *evalContext) LookupDict(key string) (*dgproto.Dict, error) {
	dict, ok := c.dicts[key]
	if !ok {
		return nil, expr.ErrDictMissing
	}

	return dict, nil
}

// Call forwards to the package-internal stdlib dispatch. The runtime
// does not own or shadow the registry; stdlib owns its catalog.
func (c *evalContext) Call(name string, args []any) (any, error) {
	return stdlib.Call(name, args)
}
