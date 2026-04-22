package runtime

import (
	"fmt"
	"io"
	"sort"
	"strconv"

	"github.com/stroppy-io/stroppy/pkg/datagen/compile"
	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
	"github.com/stroppy-io/stroppy/pkg/datagen/expr"
	"github.com/stroppy-io/stroppy/pkg/datagen/lookup"
	"github.com/stroppy-io/stroppy/pkg/datagen/seed"
)

// relRuntime wires the nested-loop iteration for a single Relationship
// with exactly two Sides. It is constructed by NewRuntime when the
// RelSource declares a relationship and is accessed through
// Runtime.nextRelationship.
//
// Iteration model (Fixed degree):
//
//	for e := 0; e < outerSize; e++ {
//	    for i := 0; i < innerDegree; i++ {
//	        // global row counter = e*innerDegree + i
//	    }
//	}
//
// For Uniform degree the inner-line count varies per outer entity. The
// runtime precomputes a cumulativeRows slice so Seek(row) reduces to a
// binary search that locates (entity, lineWithinEntity) in O(log N).
type relRuntime struct {
	dag     *compile.DAG
	columns []string

	outerName string
	outerSize int64
	innerName string

	// degree resolves the inner-row count for a given outer-entity
	// index. For Fixed it is a constant; for Uniform it is a
	// deterministic PRNG draw keyed by (rootSeed, rel-name, entityIdx).
	degree degreeResolver

	// cumulativeRows[e] is Σ_{i<=e} degree(i). Populated at construction
	// so Seek(row) can map a global row back to (entity, line) with a
	// binary search. Non-nil for both Fixed and Uniform degrees; Fixed
	// uses it for consistency with the Seek path.
	cumulativeRows []int64

	// total is cumulativeRows[outerSize-1] when outerSize > 0, else 0.
	total int64

	outerBlocks *blockCache
	innerBlocks *blockCache
}

// degreeResolver returns the inner-row count for the outer entity at
// index entityIdx. It is pure: equal inputs produce equal outputs.
type degreeResolver func(entityIdx int64) int64

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
// declares, resolves outer/inner sides, and enforces the scope limits
// (one relationship, two sides, Sequential strategy, outer side declared
// as LookupPop). It accepts Fixed and Uniform degrees.
func validateRelationship(
	source *dgproto.RelSource,
	dag *compile.DAG,
	columns []string,
	registry *lookup.LookupRegistry,
	rootSeed uint64,
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

	// Outer degree is not consumed by the runtime (the outer side is
	// iterated once per entity). It is still validated so an invalid
	// spec fails fast rather than silently ignoring the field.
	if outer.GetDegree() != nil {
		if _, err := extractDegreeResolver(outer, rel.GetName(), rootSeed); err != nil {
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

	innerDegree, err := extractDegreeResolver(inner, rel.GetName(), rootSeed)
	if err != nil {
		return nil, err
	}

	cumulative, total := precomputeCumulative(outerSize, innerDegree)

	return &relPlan{
		rt: &relRuntime{
			dag:            dag,
			columns:        columns,
			outerName:      outer.GetPopulation(),
			outerSize:      outerSize,
			innerName:      inner.GetPopulation(),
			degree:         innerDegree,
			cumulativeRows: cumulative,
			total:          total,
		},
		outerPop:  outer.GetPopulation(),
		innerPop:  inner.GetPopulation(),
		totalRows: total,
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

// extractDegreeResolver returns a degreeResolver for the Side. Fixed
// degrees produce a constant-count resolver; Uniform degrees produce a
// PRNG-keyed resolver that draws deterministically per outer entity.
func extractDegreeResolver(side *dgproto.Side, relName string, rootSeed uint64) (degreeResolver, error) {
	degree := side.GetDegree()
	if degree == nil {
		return nil, fmt.Errorf("%w: missing degree on side %q",
			ErrUnsupportedDegree, side.GetPopulation())
	}

	switch kind := degree.GetKind().(type) {
	case *dgproto.Degree_Fixed:
		count := kind.Fixed.GetCount()
		if count <= 0 {
			return nil, fmt.Errorf("%w: fixed count %d on side %q",
				ErrUnsupportedDegree, count, side.GetPopulation())
		}

		return func(_ int64) int64 { return count }, nil
	case *dgproto.Degree_Uniform:
		minV := kind.Uniform.GetMin()

		maxV := kind.Uniform.GetMax()
		if maxV < minV {
			return nil, fmt.Errorf("%w: uniform max %d < min %d on side %q",
				ErrUnsupportedDegree, maxV, minV, side.GetPopulation())
		}

		if minV < 0 {
			return nil, fmt.Errorf("%w: uniform min %d < 0 on side %q",
				ErrUnsupportedDegree, minV, side.GetPopulation())
		}

		// Uniform min==max is equivalent to Fixed; keep the PRNG call
		// out of the hot path in that case.
		if minV == maxV {
			return func(_ int64) int64 { return minV }, nil
		}

		span := maxV - minV + 1

		return func(entityIdx int64) int64 {
			return uniformDegreeFor(entityIdx, minV, span, rootSeed, relName)
		}, nil
	default:
		return nil, fmt.Errorf("%w: unknown degree on side %q",
			ErrUnsupportedDegree, side.GetPopulation())
	}
}

// uniformDegreeFor returns the Uniform draw for one outer entity. The
// per-entity PRNG is keyed by (rootSeed, "degree", relName, "u",
// entityIdx) so two spec authors that reuse entity indices across
// relationships still get independent streams.
func uniformDegreeFor(entityIdx, minV, span int64, rootSeed uint64, relName string) int64 {
	key := seed.Derive(
		rootSeed,
		"degree",
		relName,
		"u",
		strconv.FormatInt(entityIdx, 10),
	)
	prng := seed.PRNG(key)

	return minV + prng.Int64N(span)
}

// precomputeCumulative walks every outer entity, invoking degree(i),
// and returns the cumulative-sum slice plus the grand total. The slice
// is indexed by outer entity: cumulative[e] is Σ_{i<=e} degree(i).
// Callers use it both for size reporting (total == cumulative[size-1])
// and for Seek (binary search locates the entity containing a given
// global row index).
//
// Cost is O(outerSize). For very large outer populations this is
// non-trivial but is paid once at NewRuntime and amortizes across every
// row emitted thereafter.
func precomputeCumulative(outerSize int64, degree degreeResolver) (cumulative []int64, total int64) {
	if outerSize <= 0 {
		return nil, 0
	}

	cumulative = make([]int64, outerSize)

	for i := range outerSize {
		total += degree(i)
		cumulative[i] = total
	}

	return cumulative, total
}

// locateRow maps a global row index to (entityIdx, lineIdx) by binary
// searching cumulativeRows. Pre: 0 <= row < total.
func (r *relRuntime) locateRow(row int64) (entityIdx, lineIdx int64) {
	// sort.Search finds the smallest index i such that cumulativeRows[i]
	// > row; that index is the outer entity hosting row. The line within
	// the entity is row - (cumulativeRows[i] - degree(i)).
	idx := sort.Search(len(r.cumulativeRows), func(i int) bool {
		return r.cumulativeRows[i] > row
	})

	entityIdx = int64(idx)

	var entityStart int64
	if entityIdx > 0 {
		entityStart = r.cumulativeRows[entityIdx-1]
	}

	lineIdx = row - entityStart

	return entityIdx, lineIdx
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

// totalRows returns the precomputed grand total: Σ degree(e) over every
// outer entity. Equals outerSize × innerDegree for Fixed degrees.
func (r *relRuntime) totalRows() int64 {
	return r.total
}

// nextRelationship advances the Runtime by one inner row. It refreshes
// the outer block cache on every new outer entity, evaluates the
// RelSource attr DAG into scratch, and assembles the emit slice.
func (rt *Runtime) nextRelationship() ([]any, error) {
	rel := rt.rel

	if rt.row >= rel.totalRows() {
		return nil, io.EOF
	}

	entityIdx, lineIdx := rel.locateRow(rt.row)

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

	out := rt.assembleRow(rt.row)

	rt.row++

	return out, nil
}
