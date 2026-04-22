package runtime

import (
	"fmt"
	"io"

	"github.com/stroppy-io/stroppy/pkg/datagen/compile"
	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
	"github.com/stroppy-io/stroppy/pkg/datagen/expr"
	"github.com/stroppy-io/stroppy/pkg/datagen/lookup"
)

// relRuntime wires the nested-loop iteration for a single Relationship
// with exactly two Sides, Fixed degree, and Sequential strategy. It is
// constructed by NewRuntime when the RelSource declares a relationship
// and is accessed through Runtime.nextRelationship.
//
// Iteration model:
//
//	for e := 0; e < outerSize; e++ {
//	    // enter outer entity: reset block caches
//	    for i := 0; i < innerDegree; i++ {
//	        // global row counter = e*innerDegree + i
//	        // evaluate inner-side attr DAG
//	        // emit row in column_order
//	    }
//	}
//
// Seek is O(1): given a global row index g, e = g/innerDegree and
// i = g%innerDegree. The runtime resets block caches on any non-inner
// transition.
type relRuntime struct {
	dag     *compile.DAG
	columns []string
	emit    []int

	outerName   string
	outerSize   int64
	innerName   string
	innerDegree int64

	outerBlocks *blockCache
	innerBlocks *blockCache
}

// expectedSideCount is the only relationship arity this stage supports
// (outer + inner). Higher arity is rejected with ErrUnsupportedArity.
const expectedSideCount = 2

// relPlan bundles the result of validateRelationship: the compiled
// relRuntime, the populated outer/inner pop names, and the total row
// count. Returned instead of a raw 5-tuple so new downstream fields
// slot in without churning every caller.
type relPlan struct {
	rt        *relRuntime
	outerPop  string
	innerPop  string
	totalRows int64
}

// validateRelationship picks the single Relationship the RelSource
// declares, resolves outer/inner sides, and enforces the Stage-C
// scope limits (one relationship, two sides, Fixed degree, Sequential
// strategy, outer side declared as LookupPop).
func validateRelationship(
	source *dgproto.RelSource,
	dag *compile.DAG,
	columns []string,
	emit []int,
	registry *lookup.LookupRegistry,
) (*relPlan, error) {
	rels := source.GetRelationships()
	if len(rels) > 1 {
		return nil, fmt.Errorf("%w: %d declared", ErrTooManyRelationships, len(rels))
	}

	rel := rels[0]

	if iter := source.GetIter(); iter != "" && iter != rel.GetName() {
		return nil, fmt.Errorf("%w: iter=%q, relationships=[%q]",
			ErrUnknownRelationship, iter, rel.GetName())
	}

	sides := rel.GetSides()
	if len(sides) != expectedSideCount {
		return nil, fmt.Errorf("%w: %d sides on relationship %q",
			ErrUnsupportedArity, len(sides), rel.GetName())
	}

	iterPop := source.GetPopulation().GetName()

	outer, inner, err := pairSides(sides, iterPop)
	if err != nil {
		return nil, err
	}

	if err := checkStrategy(outer); err != nil {
		return nil, err
	}

	if err := checkStrategy(inner); err != nil {
		return nil, err
	}

	innerDegree, err := extractFixedDegree(inner)
	if err != nil {
		return nil, err
	}

	// Outer degree is not consumed by the runtime (the outer side is
	// iterated once per entity). It is still validated so an invalid
	// spec fails fast rather than silently ignoring the field.
	if outer.GetDegree() != nil {
		if _, err := extractFixedDegree(outer); err != nil {
			return nil, err
		}
	}

	if registry == nil || !registry.Has(outer.GetPopulation()) {
		return nil, fmt.Errorf("%w: outer population %q",
			ErrMissingLookupPop, outer.GetPopulation())
	}

	outerSize, err := registry.Size(outer.GetPopulation())
	if err != nil {
		return nil, err
	}

	return &relPlan{
		rt: &relRuntime{
			dag:         dag,
			columns:     columns,
			emit:        emit,
			outerName:   outer.GetPopulation(),
			outerSize:   outerSize,
			innerName:   inner.GetPopulation(),
			innerDegree: innerDegree,
		},
		outerPop:  outer.GetPopulation(),
		innerPop:  inner.GetPopulation(),
		totalRows: outerSize * innerDegree,
	}, nil
}

// pairSides returns (outer, inner) from a 2-element Sides slice: the
// side whose population equals iterPop is the inner (the RelSource
// emits rows for it); the other side is the outer (driving the loop).
func pairSides(sides []*dgproto.Side, iterPop string) (outer, inner *dgproto.Side, err error) {
	for _, side := range sides {
		if side == nil || side.GetPopulation() == "" {
			return nil, nil, fmt.Errorf("%w: side has empty population", ErrOuterPopMismatch)
		}

		if side.GetPopulation() == iterPop {
			if inner != nil {
				return nil, nil, fmt.Errorf(
					"%w: both sides name the RelSource population %q", ErrOuterPopMismatch, iterPop)
			}

			inner = side

			continue
		}

		if outer != nil {
			return nil, nil, fmt.Errorf(
				"%w: neither side names the RelSource population %q", ErrOuterPopMismatch, iterPop)
		}

		outer = side
	}

	if inner == nil || outer == nil {
		return nil, nil, fmt.Errorf(
			"%w: iter population %q not referenced by a side", ErrOuterPopMismatch, iterPop)
	}

	return outer, inner, nil
}

// checkStrategy rejects Hash/Equitable and treats a missing Strategy
// message as Sequential (the only implemented variant).
func checkStrategy(side *dgproto.Side) error {
	strategy := side.GetStrategy()
	if strategy == nil {
		return nil
	}

	switch strategy.GetKind().(type) {
	case *dgproto.Strategy_Sequential, nil:
		return nil
	case *dgproto.Strategy_Hash:
		return fmt.Errorf("%w: hash on side %q", ErrUnsupportedStrategy, side.GetPopulation())
	case *dgproto.Strategy_Equitable:
		return fmt.Errorf("%w: equitable on side %q", ErrUnsupportedStrategy, side.GetPopulation())
	default:
		return fmt.Errorf("%w: unknown strategy on side %q",
			ErrUnsupportedStrategy, side.GetPopulation())
	}
}

// extractFixedDegree returns the Fixed count, or ErrUnsupportedDegree
// for Uniform / missing kinds.
func extractFixedDegree(side *dgproto.Side) (int64, error) {
	degree := side.GetDegree()
	if degree == nil {
		return 0, fmt.Errorf("%w: missing degree on side %q",
			ErrUnsupportedDegree, side.GetPopulation())
	}

	switch kind := degree.GetKind().(type) {
	case *dgproto.Degree_Fixed:
		count := kind.Fixed.GetCount()
		if count <= 0 {
			return 0, fmt.Errorf("%w: fixed count %d on side %q",
				ErrUnsupportedDegree, count, side.GetPopulation())
		}

		return count, nil
	case *dgproto.Degree_Uniform:
		return 0, fmt.Errorf("%w: uniform on side %q (lands in Stage D5)",
			ErrUnsupportedDegree, side.GetPopulation())
	default:
		return 0, fmt.Errorf("%w: unknown degree on side %q",
			ErrUnsupportedDegree, side.GetPopulation())
	}
}

// attachBlockCaches wires blockCaches for both sides. Each cache's
// eval closure defers to expr.Eval against the shared evalContext.
// The outer cache is populated from outer.block_slots; the inner cache
// from inner.block_slots (degenerate — one eval per inner row).
func (r *relRuntime) attachBlockCaches(
	outer, inner *dgproto.Side,
	ctx *evalContext,
) error {
	evaluator := func(_ string, e *dgproto.Expr) (any, error) {
		return expr.Eval(ctx, e)
	}

	outerCache, err := newBlockCache(outer.GetPopulation(), outer.GetBlockSlots(), evaluator)
	if err != nil {
		return err
	}

	innerCache, err := newBlockCache(inner.GetPopulation(), inner.GetBlockSlots(), evaluator)
	if err != nil {
		return err
	}

	r.outerBlocks = outerCache
	r.innerBlocks = innerCache

	return nil
}

// totalRows returns `outerSize × innerDegree`, the number of rows the
// relationship will emit from SeekRow(0).
func (r *relRuntime) totalRows() int64 {
	return r.outerSize * r.innerDegree
}

// nextRelationship advances the Runtime by one inner row. It refreshes
// the outer block cache on every new outer entity, evaluates the
// RelSource attr DAG into scratch, and assembles the emit slice.
func (rt *Runtime) nextRelationship() ([]any, error) {
	rel := rt.rel

	if rt.row >= rel.totalRows() {
		return nil, io.EOF
	}

	entityIdx := rt.row / rel.innerDegree
	lineIdx := rt.row % rel.innerDegree

	// Refresh outer-side block cache when entering a new outer entity.
	// The inner-side cache resets every row (degenerate by spec).
	if !rt.ctx.blocks.hasEntity || rt.ctx.blocks.currentEntity != entityIdx {
		rel.outerBlocks.reset(entityIdx)
	}

	rel.innerBlocks.reset(entityIdx)

	rt.ctx.entityIdx = entityIdx
	rt.ctx.lineIdx = lineIdx
	rt.ctx.rowIdx = rt.row

	for key := range rt.ctx.scratch {
		delete(rt.ctx.scratch, key)
	}

	for _, attr := range rel.dag.Order {
		name := attr.GetName()

		if null := attr.GetNull(); null != nil && nullProbabilityHit(null, name, rt.row) {
			rt.ctx.scratch[name] = nil

			continue
		}

		rt.ctx.attrPath = name

		value, err := expr.Eval(rt.ctx, attr.GetExpr())
		if err != nil {
			return nil, fmt.Errorf("runtime: attr %q at (e=%d,i=%d): %w",
				name, entityIdx, lineIdx, err)
		}

		rt.ctx.scratch[name] = value
	}

	out := make([]any, len(rel.emit))
	for idx, pos := range rel.emit {
		out[idx] = rt.ctx.scratch[rel.dag.Order[pos].GetName()]
	}

	rt.row++

	return out, nil
}
