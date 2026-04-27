package runtime

import (
	"fmt"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
)

// blockCache holds resolved BlockSlot values for the current outer
// entity of a Relationship. The semantic cache key is `(side_name,
// slot_name, outer_entity_idx)`; the cache itself is refreshed in place
// when the outer entity advances, so we only need to key by slot name
// within the cache and validate the entity via a checkpoint.
//
// Side-scoped isolation is provided by owning one blockCache per Side:
// the relationship runtime constructs the outer cache and, if the
// inner side also declares BlockSlots, a second cache for it. Both
// caches share the same evalContext at evaluation time but are
// addressed separately so slot-name collisions across sides do not
// cross-contaminate.
type blockCache struct {
	sideName      string
	slots         map[string]*dgproto.Expr
	values        map[string]any
	currentEntity int64
	hasEntity     bool
	evals         int
	// eval lets the cache compute a slot lazily. It is set to a closure
	// bound to the enclosing evalContext at relationship construction.
	eval func(name string, e *dgproto.Expr) (any, error)
}

// newBlockCache returns a cache populated with the given slots' exprs.
// eval is invoked the first time each slot is read for the current
// outer entity.
func newBlockCache(
	sideName string,
	slots []*dgproto.BlockSlot,
	eval func(name string, e *dgproto.Expr) (any, error),
) (*blockCache, error) {
	index := make(map[string]*dgproto.Expr, len(slots))

	for _, slot := range slots {
		name := slot.GetName()
		if name == "" {
			return nil, fmt.Errorf("%w: block slot with empty name on side %q",
				ErrUnknownBlockSlot, sideName)
		}

		if slot.GetExpr() == nil {
			return nil, fmt.Errorf("%w: block slot %q on side %q has no expr",
				ErrUnknownBlockSlot, name, sideName)
		}

		if _, dup := index[name]; dup {
			return nil, fmt.Errorf("%w: duplicate block slot %q on side %q",
				ErrUnknownBlockSlot, name, sideName)
		}

		index[name] = slot.GetExpr()
	}

	return &blockCache{
		sideName: sideName,
		slots:    index,
		values:   make(map[string]any, len(index)),
		eval:     eval,
	}, nil
}

// reset clears the memoized slot values and records the new outer
// entity index. It is called by the relationship runtime whenever it
// enters a new outer entity boundary.
func (b *blockCache) reset(entityIdx int64) {
	b.currentEntity = entityIdx

	b.hasEntity = true
	for key := range b.values {
		delete(b.values, key)
	}
}

// get returns the slot's value, evaluating it lazily on first read for
// the current entity. Returns ErrUnknownBlockSlot if the slot is not
// declared on this side.
func (b *blockCache) get(slot string) (any, error) {
	expression, ok := b.slots[slot]
	if !ok {
		return nil, fmt.Errorf("%w: %q not declared on side %q",
			ErrUnknownBlockSlot, slot, b.sideName)
	}

	if value, cached := b.values[slot]; cached {
		return value, nil
	}

	value, err := b.eval(slot, expression)
	if err != nil {
		return nil, fmt.Errorf("%w: slot %q on side %q: %w",
			ErrBlockSlotEval, slot, b.sideName, err)
	}

	b.values[slot] = value
	b.evals++

	return value, nil
}

// evalCount returns how many times the cache invoked its eval callback.
// Test-only, not part of the public API.
func (b *blockCache) evalCount() int {
	return b.evals
}
