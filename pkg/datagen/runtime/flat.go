package runtime

import (
	"fmt"
	"io"

	"github.com/stroppy-io/stroppy/pkg/datagen/cohort"
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
	dag                 *compile.DAG
	columns             []string
	emit                []emitSlot
	size                int64
	row                 int64
	ctx                 *evalContext
	discardNeedsScratch bool

	// rel is non-nil when the RelSource declares a Relationship. In
	// that mode `size` is the per-entity count summed over all outer
	// entities and Next advances through the nested iteration.
	rel *relRuntime

	// scd2 is non-nil when RelSource.scd2 is set. It carries the
	// precomputed start/end pairs and the boundary row index.
	scd2 *scd2State
}

// emitKind distinguishes a regular DAG-attr column from a column whose
// value is injected by a runtime mechanism (currently only SCD-2).
type emitKind uint8

const (
	// emitAttr sources the column value from the scratch map at the
	// position recorded in emitSlot.ref.
	emitAttr emitKind = iota
	// emitSCD2Start sources the column value from scd2State.startValue,
	// chosen by the current row's boundary test.
	emitSCD2Start
	// emitSCD2End sources the column value from scd2State.endValue.
	emitSCD2End
)

// emitSlot pairs a column position with the source that supplies its
// value for each emitted row. Regular attrs reference the DAG position;
// SCD-2 columns reference the runtime's scd2State.
type emitSlot struct {
	kind emitKind
	// ref is the DAG index when kind == emitAttr; unused otherwise.
	ref int
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

	emit, err := resolveColumnOrder(source.GetColumnOrder(), dag, source.GetScd2())
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

	cohorts, err := cohort.New(source.GetCohorts(), spec.GetSeed(), 0)
	if err != nil {
		return nil, fmt.Errorf("runtime: compile cohorts: %w", err)
	}

	ctx := &evalContext{
		scratch:          make(map[string]any, len(dag.Order)),
		scratchKeys:      make([]string, 0, len(dag.Order)),
		dicts:            spec.GetDicts(),
		registry:         registry,
		cohorts:          cohorts,
		cohortBucketKeys: cohortDefaultKeys(source.GetCohorts()),
		iterPop:          source.GetPopulation().GetName(),
		rootSeed:         spec.GetSeed(),
	}

	runtime := &Runtime{
		dag:                 dag,
		columns:             columns,
		emit:                emit,
		size:                size,
		row:                 0,
		ctx:                 ctx,
		discardNeedsScratch: discardNeedsScratch(source),
	}

	if len(source.GetRelationships()) > 0 {
		if err := runtime.installRelationship(source, registry, spec.GetSeed()); err != nil {
			return nil, err
		}
	}

	if source.GetScd2() != nil {
		if err := runtime.installSCD2(source); err != nil {
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
	rootSeed uint64,
) error {
	plan, err := validateRelationship(source, r.dag, r.columns, registry, rootSeed)
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

// TotalRows returns the number of rows this runtime will emit. For flat
// populations it equals RelSource.population.size; for relationship-backed
// sources it is the computed relationship row total.
func (r *Runtime) TotalRows() int64 {
	return r.size
}

// Clone returns an independent Runtime that shares the compiled DAG,
// column metadata, dict map, and (for relationship runtimes) the
// immutable cumulativeRows profile with the receiver, but owns a fresh
// scratch buffer, row counter, block caches, lookup registry, and
// cohort registry. The shared fields are read-only after NewRuntime,
// so clones are safe to run concurrently without locks; the lookup
// and cohort registries are cloned so each worker writes into its own
// LRU state rather than racing on a shared map.
//
// A cloned Runtime starts at row 0; call SeekRow to position it at a
// chunk boundary before iterating.
func (r *Runtime) Clone() *Runtime {
	clone := &Runtime{
		dag:     r.dag,
		columns: r.columns,
		emit:    r.emit,
		size:    r.size,
		row:     0,
		scd2:    r.scd2,
		ctx: &evalContext{
			scratch:          make(map[string]any, len(r.dag.Order)),
			scratchKeys:      make([]string, 0, len(r.dag.Order)),
			dicts:            r.ctx.dicts,
			registry:         r.ctx.registry.CloneRegistry(),
			rootSeed:         r.ctx.rootSeed,
			iterPop:          r.ctx.iterPop,
			cohorts:          r.ctx.cohorts.CloneRegistry(),
			cohortBucketKeys: r.ctx.cohortBucketKeys,
			inRelationship:   r.ctx.inRelationship,
			outerPop:         r.ctx.outerPop,
		},
		discardNeedsScratch: r.discardNeedsScratch,
	}

	if r.rel != nil {
		// Share the immutable relRuntime fields (compile DAG, degree
		// resolver, cumulativeRows) but mint fresh, per-worker block
		// caches so the outer/inner entity checkpoints stay independent.
		relClone := *r.rel

		outerEval := func(_ string, e *dgproto.Expr) (any, error) {
			return expr.Eval(clone.ctx, e)
		}

		relClone.outerBlocks = &blockCache{
			sideName: r.rel.outerBlocks.sideName,
			slots:    r.rel.outerBlocks.slots,
			values:   make(map[string]any, len(r.rel.outerBlocks.slots)),
			eval:     outerEval,
		}
		relClone.innerBlocks = &blockCache{
			sideName: r.rel.innerBlocks.sideName,
			slots:    r.rel.innerBlocks.slots,
			values:   make(map[string]any, len(r.rel.innerBlocks.slots)),
			eval:     outerEval,
		}

		clone.rel = &relClone
		clone.ctx.blocks = relClone.outerBlocks
	}

	return clone
}

// cohortDefaultKeys builds the schedule-name → default-bucket_key map
// consulted by evalContext.CohortBucketKey. Schedules with a nil
// bucket_key are omitted; the per-arm override is required for those.
func cohortDefaultKeys(cohorts []*dgproto.Cohort) map[string]*dgproto.Expr {
	if len(cohorts) == 0 {
		return nil
	}

	out := make(map[string]*dgproto.Expr, len(cohorts))

	for _, c := range cohorts {
		if c == nil || c.GetBucketKey() == nil {
			continue
		}

		out[c.GetName()] = c.GetBucketKey()
	}

	return out
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
	return r.NextInto(nil)
}

// NextInto evaluates the next row into dst and returns the row slice. If dst is
// nil or too small, a new slice is allocated; otherwise dst[:len(Columns())] is
// reused. The returned slice is owned by the caller until the next mutation of
// the supplied dst.
func (r *Runtime) NextInto(dst []any) ([]any, error) {
	if r.rel != nil {
		return r.nextRelationshipInto(dst)
	}

	return r.nextFlatInto(dst)
}

// NextDiscard advances the runtime after evaluating the current row without
// assembling or returning the row values. It is intended for sinks that truly
// discard rows, such as the noop driver benchmark path.
func (r *Runtime) NextDiscard() error {
	if r.rel != nil {
		return r.nextRelationshipDiscard()
	}

	return r.nextFlatDiscard()
}

// nextFlat is the original Stage-B row emitter: linear over the
// RelSource's population, evaluating attrs once per row.
func (r *Runtime) nextFlat() ([]any, error) {
	return r.nextFlatInto(nil)
}

func (r *Runtime) nextFlatInto(dst []any) ([]any, error) {
	if r.row >= r.size {
		return nil, io.EOF
	}

	r.ctx.rowIdx = r.row
	// Fast clear: only delete keys we wrote last row (O(written) vs O(cap)).
	for _, k := range r.ctx.scratchKeys {
		delete(r.ctx.scratch, k)
	}
	r.ctx.scratchKeys = r.ctx.scratchKeys[:0]

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
		r.ctx.scratchKeys = append(r.ctx.scratchKeys, name)
	}

	out := r.assembleRowInto(dst, r.row)

	r.row++

	return out, nil
}

func (r *Runtime) nextFlatDiscard() error {
	if r.row >= r.size {
		return io.EOF
	}

	r.ctx.rowIdx = r.row
	if r.discardNeedsScratch {
		for _, k := range r.ctx.scratchKeys {
			delete(r.ctx.scratch, k)
		}
		r.ctx.scratchKeys = r.ctx.scratchKeys[:0]
	}

	for _, attrNode := range r.dag.Order {
		name := attrNode.GetName()

		if null := attrNode.GetNull(); null != nil && nullProbabilityHit(null, name, r.row) {
			if r.discardNeedsScratch {
				r.ctx.scratch[name] = nil
				r.ctx.scratchKeys = append(r.ctx.scratchKeys, name)
			}

			continue
		}

		r.ctx.attrPath = name

		value, err := expr.Eval(r.ctx, attrNode.GetExpr())
		if err != nil {
			return fmt.Errorf("runtime: attr %q at row %d: %w", name, r.row, err)
		}

		if r.discardNeedsScratch {
			r.ctx.scratch[name] = value
			r.ctx.scratchKeys = append(r.ctx.scratchKeys, name)
		}
	}

	r.row++

	return nil
}

// assembleRow builds the output row for the given global row index,
// consulting the DAG scratch for emitAttr slots and the SCD2 state for
// emitSCD2Start / emitSCD2End slots.
func (r *Runtime) assembleRow(rowIdx int64) []any {
	return r.assembleRowInto(nil, rowIdx)
}

func (r *Runtime) assembleRowInto(dst []any, rowIdx int64) []any {
	if cap(dst) < len(r.emit) {
		dst = make([]any, len(r.emit))
	} else {
		dst = dst[:len(r.emit)]
		clear(dst)
	}

	for i, slot := range r.emit {
		switch slot.kind {
		case emitAttr:
			dst[i] = r.ctx.scratch[r.dag.Order[slot.ref].GetName()]
		case emitSCD2Start:
			dst[i] = r.scd2.startFor(rowIdx)
		case emitSCD2End:
			dst[i] = r.scd2.endFor(rowIdx)
		}
	}

	return dst
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

func discardNeedsScratch(source *dgproto.RelSource) bool {
	iterPop := source.GetPopulation().GetName()

	for _, attr := range source.GetAttrs() {
		if len(compile.CollectColRefs(attr.GetExpr())) > 0 || hasSelfLookup(attr.GetExpr(), iterPop) {
			return true
		}
	}

	return false
}

func hasSelfLookup(e *dgproto.Expr, iterPop string) bool {
	if e == nil {
		return false
	}

	switch kind := e.GetKind().(type) {
	case *dgproto.Expr_BinOp:
		return hasSelfLookup(kind.BinOp.GetA(), iterPop) || hasSelfLookup(kind.BinOp.GetB(), iterPop)
	case *dgproto.Expr_Call:
		for _, arg := range kind.Call.GetArgs() {
			if hasSelfLookup(arg, iterPop) {
				return true
			}
		}
	case *dgproto.Expr_If_:
		ifExpr := kind.If_
		return hasSelfLookup(ifExpr.GetCond(), iterPop) ||
			hasSelfLookup(ifExpr.GetThen(), iterPop) ||
			hasSelfLookup(ifExpr.GetElse_(), iterPop)
	case *dgproto.Expr_DictAt:
		return hasSelfLookup(kind.DictAt.GetIndex(), iterPop)
	case *dgproto.Expr_Lookup:
		return kind.Lookup.GetTargetPop() == iterPop || hasSelfLookup(kind.Lookup.GetEntityIndex(), iterPop)
	case *dgproto.Expr_StreamDraw:
		return streamDrawHasSelfLookup(kind.StreamDraw, iterPop)
	case *dgproto.Expr_Choose:
		for _, branch := range kind.Choose.GetBranches() {
			if hasSelfLookup(branch.GetExpr(), iterPop) {
				return true
			}
		}
	case *dgproto.Expr_CohortDraw:
		return hasSelfLookup(kind.CohortDraw.GetSlot(), iterPop) ||
			hasSelfLookup(kind.CohortDraw.GetBucketKey(), iterPop)
	case *dgproto.Expr_CohortLive:
		return hasSelfLookup(kind.CohortLive.GetBucketKey(), iterPop)
	case *dgproto.Expr_Col:
		return true
	case *dgproto.Expr_RowIndex, *dgproto.Expr_Lit, *dgproto.Expr_BlockRef, nil:
		return false
	}

	return false
}

func streamDrawHasSelfLookup(node *dgproto.StreamDraw, iterPop string) bool {
	if node == nil {
		return false
	}

	switch arm := node.GetDraw().(type) {
	case *dgproto.StreamDraw_IntUniform:
		return hasSelfLookup(arm.IntUniform.GetMin(), iterPop) || hasSelfLookup(arm.IntUniform.GetMax(), iterPop)
	case *dgproto.StreamDraw_FloatUniform:
		return hasSelfLookup(arm.FloatUniform.GetMin(), iterPop) || hasSelfLookup(arm.FloatUniform.GetMax(), iterPop)
	case *dgproto.StreamDraw_Normal:
		return hasSelfLookup(arm.Normal.GetMin(), iterPop) || hasSelfLookup(arm.Normal.GetMax(), iterPop)
	case *dgproto.StreamDraw_Zipf:
		return hasSelfLookup(arm.Zipf.GetMin(), iterPop) || hasSelfLookup(arm.Zipf.GetMax(), iterPop)
	case *dgproto.StreamDraw_Decimal:
		return hasSelfLookup(arm.Decimal.GetMin(), iterPop) || hasSelfLookup(arm.Decimal.GetMax(), iterPop)
	case *dgproto.StreamDraw_Ascii:
		return hasSelfLookup(arm.Ascii.GetMinLen(), iterPop) || hasSelfLookup(arm.Ascii.GetMaxLen(), iterPop)
	case *dgproto.StreamDraw_Phrase:
		return hasSelfLookup(arm.Phrase.GetMinWords(), iterPop) || hasSelfLookup(arm.Phrase.GetMaxWords(), iterPop)
	case *dgproto.StreamDraw_Grammar:
		return hasSelfLookup(arm.Grammar.GetMinLen(), iterPop) || hasSelfLookup(arm.Grammar.GetMaxLen(), iterPop)
	default:
		return false
	}
}

// resolveColumnOrder returns an emitSlot per column in column_order.
// Regular columns resolve to DAG indices; when scd2 is non-nil, the
// start_col and end_col entries resolve to SCD-2-injected slots and
// must not also be declared as attrs.
func resolveColumnOrder(
	columnOrder []string,
	dag *compile.DAG,
	scd2 *dgproto.SCD2,
) ([]emitSlot, error) {
	startCol, endCol, err := validateSCD2Columns(dag, scd2)
	if err != nil {
		return nil, err
	}

	emit := make([]emitSlot, len(columnOrder))

	var sawStart, sawEnd bool

	for i, name := range columnOrder {
		slot, isStart, isEnd, err := resolveEmitSlot(name, dag, startCol, endCol)
		if err != nil {
			return nil, err
		}

		emit[i] = slot
		sawStart = sawStart || isStart
		sawEnd = sawEnd || isEnd
	}

	if scd2 != nil && !sawStart {
		return nil, fmt.Errorf("%w: scd2 start_col %q not in column_order",
			ErrMissingColumn, startCol)
	}

	if scd2 != nil && !sawEnd {
		return nil, fmt.Errorf("%w: scd2 end_col %q not in column_order",
			ErrMissingColumn, endCol)
	}

	return emit, nil
}

// validateSCD2Columns returns (start_col, end_col) for the supplied
// SCD2 config, or ("", "") when scd2 is nil. It rejects empty names,
// start_col == end_col, and SCD2 columns that are also declared attrs.
func validateSCD2Columns(dag *compile.DAG, scd2 *dgproto.SCD2) (startCol, endCol string, err error) {
	if scd2 == nil {
		return "", "", nil
	}

	startCol = scd2.GetStartCol()
	endCol = scd2.GetEndCol()

	if startCol == "" || endCol == "" {
		return "", "", fmt.Errorf("%w: scd2 start_col/end_col required", ErrInvalidSpec)
	}

	if startCol == endCol {
		return "", "", fmt.Errorf("%w: scd2 start_col and end_col must differ (%q)",
			ErrInvalidSpec, startCol)
	}

	if _, declared := dag.Index[startCol]; declared {
		return "", "", fmt.Errorf("%w: scd2 start_col %q must not be declared as an attr",
			ErrInvalidSpec, startCol)
	}

	if _, declared := dag.Index[endCol]; declared {
		return "", "", fmt.Errorf("%w: scd2 end_col %q must not be declared as an attr",
			ErrInvalidSpec, endCol)
	}

	return startCol, endCol, nil
}

// resolveEmitSlot resolves one column name to its emitSlot, returning
// (slot, isSCD2Start, isSCD2End) so the caller can track whether the
// start/end columns were observed in column_order. Names matching
// startCol/endCol route to SCD2 slots; anything else must be a known
// attr in the DAG.
func resolveEmitSlot(
	name string,
	dag *compile.DAG,
	startCol, endCol string,
) (slot emitSlot, isStart, isEnd bool, err error) {
	if startCol != "" && name == startCol {
		return emitSlot{kind: emitSCD2Start}, true, false, nil
	}

	if endCol != "" && name == endCol {
		return emitSlot{kind: emitSCD2End}, false, true, nil
	}

	pos, ok := dag.Index[name]
	if !ok {
		return emitSlot{}, false, false, fmt.Errorf("%w: %q", ErrMissingColumn, name)
	}

	return emitSlot{kind: emitAttr, ref: pos}, false, false, nil
}
