package runtime

import (
	"fmt"
	"io"

	"github.com/stroppy-io/stroppy/pkg/datagen/compile"
	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
	"github.com/stroppy-io/stroppy/pkg/datagen/expr"
)

// Runtime is a stateful row emitter for one InsertSpec. It advances
// through row indices `[0, size)`, evaluating the compiled attr DAG at
// each row and assembling a `[]any` in the configured column order.
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
}

// NewRuntime validates an InsertSpec and returns a Runtime ready to
// emit the first row. Validation checks that the RelSource exists, the
// Population size is positive, column_order is non-empty, every emitted
// column names a declared attr, and the attr graph is acyclic.
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

	return &Runtime{
		dag:     dag,
		columns: columns,
		emit:    emit,
		size:    size,
		ctx: &evalContext{
			scratch: make(map[string]any, len(dag.Order)),
			dicts:   spec.GetDicts(),
		},
	}, nil
}

// Columns returns the emitted column order. The slice is owned by the
// Runtime; callers must not mutate it.
func (r *Runtime) Columns() []string {
	return r.columns
}

// SeekRow sets the next row index to emit. Valid inputs are in
// `[0, Population.Size]`; seeking to Size leaves the Runtime at EOF.
// SeekRow is O(1) because every Expr is a pure function of the row index —
// there is no accumulated state to replay.
func (r *Runtime) SeekRow(row int64) error {
	if row < 0 || row > r.size {
		return fmt.Errorf("%w: %d not in [0, %d]", ErrSeekOutOfRange, row, r.size)
	}

	r.row = row

	return nil
}

// Next evaluates the DAG for the current row and returns its column
// values in Columns() order. At the end of the population it returns
// (nil, io.EOF). Evaluation errors are wrapped with the attr name and
// row index so a loader log entry is sufficient to reproduce.
func (r *Runtime) Next() ([]any, error) {
	if r.row >= r.size {
		return nil, io.EOF
	}

	r.ctx.rowIdx = r.row
	for key := range r.ctx.scratch {
		delete(r.ctx.scratch, key)
	}

	for _, attr := range r.dag.Order {
		name := attr.GetName()

		if null := attr.GetNull(); null != nil && nullProbabilityHit(null, name, r.row) {
			r.ctx.scratch[name] = nil

			continue
		}

		value, err := expr.Eval(r.ctx, attr.GetExpr())
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
