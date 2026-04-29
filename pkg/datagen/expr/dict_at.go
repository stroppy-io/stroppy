package expr

import (
	"fmt"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
)

// evalDictAt looks up a row in a scalar Dict carried by InsertSpec.dicts.
// Multi-column dicts are rejected — joint draws go through DrawJoint
// (Stage D). The index is wrapped modulo the row count.
func evalDictAt(ctx Context, node *dgproto.DictAt) (any, error) {
	indexVal, err := Eval(ctx, node.GetIndex())
	if err != nil {
		return nil, err
	}

	index, ok := indexVal.(int64)
	if !ok {
		return nil, fmt.Errorf("%w: dict index %T", ErrTypeMismatch, indexVal)
	}

	dict, err := ctx.LookupDict(node.GetDictKey())
	if err != nil {
		return nil, err
	}

	if len(dict.GetColumns()) > 1 {
		return nil, fmt.Errorf("%w: multi-column dict %q", ErrTypeMismatch, node.GetDictKey())
	}

	rows := dict.GetRows()
	if len(rows) == 0 {
		return nil, fmt.Errorf("%w: empty dict %q", ErrBadExpr, node.GetDictKey())
	}

	count := int64(len(rows))
	position := ((index % count) + count) % count

	values := rows[position].GetValues()
	if len(values) == 0 {
		return nil, fmt.Errorf("%w: dict row empty in %q", ErrBadExpr, node.GetDictKey())
	}

	return values[0], nil
}
