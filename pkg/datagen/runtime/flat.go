package runtime

import (
	"fmt"
	"io"

	"github.com/stroppy-io/stroppy/pkg/datagen/compile"
	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
	"github.com/stroppy-io/stroppy/pkg/datagen/expr"
	"github.com/stroppy-io/stroppy/pkg/datagen/lookup"
)

// Runtime is a stateful row emitter for one InsertSpec. It advances
// through row indices `[0, size)`, evaluating the compiled attr DAG at
// each row and assembling a `[]any` in the configured column order.
// When the RelSource declares a Relationship, the Runtime iterates the
// nested (outer × inner) space instead; see relationship.go.
//
// A Runtime is not safe for concurrent use: the scratch map and row
// counter are mutated per call. Parallel workers own independent
// Runtimes built from the same InsertSpec.
type Runtime struct {
	dag     *compile.DAG
	columns []string
	emit    []int
	size    int64
	row     int64
	ctx     *evalContext

	// rel is non-nil when the RelSource declares a Relationship. In
	// that mode `size` is `outerSize × innerDegree` and Next advances
	// through the nested iteration.
	rel *relRuntime
}

// NewRuntime validates an InsertSpec and returns a Runtime ready to
// emit the first row. Validation checks that the RelSource exists, the
// Population size is positive, column_order is non-empty, every emitted
// column names a declared attr, and the attr graph is acyclic.
//
// When the RelSource declares a Relationship, NewRuntime additionally
// enforces the Stage-C scope limits (one relationship, two sides,
// Fixed degree, Sequential strategy) and compiles a LookupRegistry
// covering both declared LookupPops and the outer-side population.
func NewRuntime(spec *dgproto.InsertSpec) (*Runtime, error) {
	source, size, err := validateSpec(spec)
	if err != nil {
		return nil, err
	}

	dag, err := compile.Build(source.GetAttrs())
	if err != nil {
		return nil, fmt.Errorf("runtime: compile attrs: %w", err)
	}

	emit, err := resolveColumnOrder(source.GetColumnOrder(), dag)
	if err != nil {
		return nil, err
	}

	columns := make([]string, len(source.GetColumnOrder()))
	copy(columns, source.GetColumnOrder())

	registry, err := lookup.NewLookupRegistry(source.GetLookupPops(), spec.GetDicts(), 0)
	if err != nil {
		return nil, fmt.Errorf("runtime: compile LookupPops: %w", err)
	}

	registry.SetRootSeed(spec.GetSeed())

	ctx := &evalContext{
		scratch:  make(map[string]any, len(dag.Order)),
		dicts:    spec.GetDicts(),
		registry: registry,
		iterPop:  source.GetPopulation().GetName(),
		rootSeed: spec.GetSeed(),
	}

	runtime := &Runtime{
		dag:     dag,
		columns: columns,
		emit:    emit,
		size:    size,
		ctx:     ctx,
	}

	if len(source.GetRelationships()) > 0 {
		if err := runtime.installRelationship(source, registry); err != nil {
			return nil, err
		}
	}

	return runtime, nil
}

// installRelationship configures the runtime for relationship-driven
// iteration. It compiles the relRuntime, attaches block caches, and
// points the shared evalContext at the inner-/outer-side metadata.
func (r *Runtime) installRelationship(
	source *dgproto.RelSource,
	registry *lookup.LookupRegistry,
) error {
	plan, err := validateRelationship(source, r.dag, r.columns, r.emit, registry)
	if err != nil {
		return err
	}

	outer, inner := relSides(source.GetRelationships()[0], source.GetPopulation().GetName())

	if err := plan.rt.attachBlockCaches(outer, inner, r.ctx); err != nil {
		return err
	}

	r.rel = plan.rt
	r.size = plan.totalRows

	r.ctx.inRelationship = true
	r.ctx.outerPop = plan.outerPop
	r.ctx.blocks = plan.rt.outerBlocks

	return nil
}

// relSides re-extracts (outer, inner) for a validated Relationship.
// Safe to call here because validateRelationship already asserted
// exactly two sides with one matching iterPop.
func relSides(rel *dgproto.Relationship, iterPop string) (outer, inner *dgproto.Side) {
	sides := rel.GetSides()

	first, second := sides[0], sides[1]
	if first.GetPopulation() == iterPop {
		return second, first
	}

	return first, second
}

// Columns returns the emitted column order. The slice is owned by the
// Runtime; callers must not mutate it.
func (r *Runtime) Columns() []string {
	return r.columns
}

// Clone returns an independent Runtime that shares the compiled DAG,
// column metadata, and dict map with the receiver but owns a fresh
// scratch buffer and row counter. The shared fields are read-only after
// NewRuntime, so clones are safe to run concurrently without locks.
//
// A cloned Runtime starts at row 0; call SeekRow to position it at a
// chunk boundary before iterating.
//
// Clone is only valid for flat runtimes; a relationship-bearing
// Runtime shares mutable caches (block caches, Lookup LRUs) that do
// not round-trip through Clone. Callers that need a fresh
// relationship Runtime should call NewRuntime again on the spec.
func (r *Runtime) Clone() *Runtime {
	if r.rel != nil {
		panic("runtime: Clone() unsupported on relationship runtime")
	}

	return &Runtime{
		dag:     r.dag,
		columns: r.columns,
		emit:    r.emit,
		size:    r.size,
		row:     0,
		ctx: &evalContext{
			scratch:  make(map[string]any, len(r.dag.Order)),
			dicts:    r.ctx.dicts,
			rootSeed: r.ctx.rootSeed,
			iterPop:  r.ctx.iterPop,
		},
	}
}

// SeekRow sets the next row index to emit. Valid inputs are in
// `[0, total]`; seeking to total leaves the Runtime at EOF. For
// relationship runtimes, total is `outerSize × innerDegree`. SeekRow
// is O(1) because every Expr is a pure function of the row index —
// there is no accumulated state to replay.
func (r *Runtime) SeekRow(row int64) error {
	if row < 0 || row > r.size {
		return fmt.Errorf("%w: %d not in [0, %d]", ErrSeekOutOfRange, row, r.size)
	}

	r.row = row

	// Invalidate block caches on any seek: the outer entity boundary
	// we are at after Seek is recomputed on the next Next() call.
	if r.rel != nil {
		r.rel.outerBlocks.hasEntity = false
	}

	return nil
}

// Next evaluates the DAG for the current row and returns its column
// values in Columns() order. At the end of iteration it returns
// (nil, io.EOF). Evaluation errors are wrapped with the attr name and
// row index so a loader log entry is sufficient to reproduce.
func (r *Runtime) Next() ([]any, error) {
	if r.rel != nil {
		return r.nextRelationship()
	}

	return r.nextFlat()
}

// nextFlat is the original Stage-B row emitter: linear over the
// RelSource's population, evaluating attrs once per row.
func (r *Runtime) nextFlat() ([]any, error) {
	if r.row >= r.size {
		return nil, io.EOF
	}

	r.ctx.rowIdx = r.row
	for key := range r.ctx.scratch {
		delete(r.ctx.scratch, key)
	}

	for _, attrNode := range r.dag.Order {
		name := attrNode.GetName()

		if null := attrNode.GetNull(); null != nil && nullProbabilityHit(null, name, r.row) {
			r.ctx.scratch[name] = nil

			continue
		}

		r.ctx.attrPath = name

		value, err := expr.Eval(r.ctx, attrNode.GetExpr())
		if err != nil {
			return nil, fmt.Errorf("runtime: attr %q at row %d: %w", name, r.row, err)
		}

		r.ctx.scratch[name] = value
	}

	out := make([]any, len(r.emit))
	for i, idx := range r.emit {
		out[i] = r.ctx.scratch[r.dag.Order[idx].GetName()]
	}

	r.row++

	return out, nil
}

// validateSpec enforces the minimal preconditions for the flat runtime:
// a non-nil RelSource, a positive population size, and a non-empty
// column_order. It returns the RelSource and size for downstream use.
func validateSpec(spec *dgproto.InsertSpec) (*dgproto.RelSource, int64, error) {
	if spec == nil {
		return nil, 0, fmt.Errorf("%w: nil spec", ErrInvalidSpec)
	}

	source := spec.GetSource()
	if source == nil {
		return nil, 0, fmt.Errorf("%w: nil source", ErrInvalidSpec)
	}

	population := source.GetPopulation()
	if population == nil {
		return nil, 0, fmt.Errorf("%w: nil population", ErrInvalidSpec)
	}

	size := population.GetSize()
	if size <= 0 {
		return nil, 0, fmt.Errorf("%w: population size %d", ErrInvalidSpec, size)
	}

	if len(source.GetColumnOrder()) == 0 {
		return nil, 0, ErrEmptyColumnOrder
	}

	return source, size, nil
}

// resolveColumnOrder returns the DAG positions of the attrs named in
// column_order, rejecting any name not declared in the RelSource.
func resolveColumnOrder(columnOrder []string, dag *compile.DAG) ([]int, error) {
	emit := make([]int, len(columnOrder))

	for i, name := range columnOrder {
		pos, ok := dag.Index[name]
		if !ok {
			return nil, fmt.Errorf("%w: %q", ErrMissingColumn, name)
		}

		emit[i] = pos
	}

	return emit, nil
}
