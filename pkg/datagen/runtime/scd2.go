package runtime

import (
	"fmt"
	"math/rand/v2"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
	"github.com/stroppy-io/stroppy/pkg/datagen/expr"
	"github.com/stroppy-io/stroppy/pkg/datagen/stdlib"
)

// scd2State carries the precomputed SCD-2 values that the runtime
// injects into every emitted row. The five Exprs (boundary, historical
// start/end, current start/end) are evaluated once at NewRuntime against
// an empty-scratch context: runtime-varying SCD-2 values are not
// supported in v1.
//
// startFor / endFor dispatch on the row's global index: rows with
// rowIdx < boundary receive the historical pair; rows at or above
// receive the current pair (currentEnd may be nil, which emits as SQL
// NULL for that column).
type scd2State struct {
	boundary   int64
	histStart  any
	histEnd    any
	currStart  any
	currEnd    any
	hasCurrEnd bool
}

// installSCD2 evaluates the SCD2 Exprs once against an empty-scratch
// expr.Context and stores the results on the Runtime. Any evaluation
// failure (missing Expr, non-int64 boundary, per-row dependency) is
// reported as ErrInvalidSpec so a bad spec fails fast at construction.
func (r *Runtime) installSCD2(source *dgproto.RelSource) error {
	cfg := source.GetScd2()
	if cfg == nil {
		return nil
	}

	if cfg.GetBoundary() == nil {
		return fmt.Errorf("%w: scd2.boundary required", ErrInvalidSpec)
	}

	if cfg.GetHistoricalStart() == nil {
		return fmt.Errorf("%w: scd2.historical_start required", ErrInvalidSpec)
	}

	if cfg.GetHistoricalEnd() == nil {
		return fmt.Errorf("%w: scd2.historical_end required", ErrInvalidSpec)
	}

	if cfg.GetCurrentStart() == nil {
		return fmt.Errorf("%w: scd2.current_start required", ErrInvalidSpec)
	}

	// Constant-eval context: no row state, no dicts dependency. The
	// SCD2 Exprs must be constant-foldable; an Expr reaching for the
	// row scratch or stream draws will fail here.
	evalCtx := &scd2ConstContext{}

	boundaryVal, err := expr.Eval(evalCtx, cfg.GetBoundary())
	if err != nil {
		return fmt.Errorf("%w: scd2.boundary eval: %w", ErrInvalidSpec, err)
	}

	boundary, ok := boundaryVal.(int64)
	if !ok {
		return fmt.Errorf("%w: scd2.boundary must evaluate to int64, got %T",
			ErrInvalidSpec, boundaryVal)
	}

	if boundary < 0 {
		return fmt.Errorf("%w: scd2.boundary %d must be >= 0", ErrInvalidSpec, boundary)
	}

	histStart, err := expr.Eval(evalCtx, cfg.GetHistoricalStart())
	if err != nil {
		return fmt.Errorf("%w: scd2.historical_start eval: %w", ErrInvalidSpec, err)
	}

	histEnd, err := expr.Eval(evalCtx, cfg.GetHistoricalEnd())
	if err != nil {
		return fmt.Errorf("%w: scd2.historical_end eval: %w", ErrInvalidSpec, err)
	}

	currStart, err := expr.Eval(evalCtx, cfg.GetCurrentStart())
	if err != nil {
		return fmt.Errorf("%w: scd2.current_start eval: %w", ErrInvalidSpec, err)
	}

	state := &scd2State{
		boundary:  boundary,
		histStart: histStart,
		histEnd:   histEnd,
		currStart: currStart,
	}

	if cfg.GetCurrentEnd() != nil {
		currEnd, err := expr.Eval(evalCtx, cfg.GetCurrentEnd())
		if err != nil {
			return fmt.Errorf("%w: scd2.current_end eval: %w", ErrInvalidSpec, err)
		}

		state.currEnd = currEnd
		state.hasCurrEnd = true
	}

	r.scd2 = state

	return nil
}

// scd2ConstContext is the expr.Context used to evaluate SCD2 boundary
// and historical/current value expressions once at NewRuntime time.
// It supports the constant-foldable Expr arms (Lit, BinOp, If, Call
// via stdlib) and rejects every row-dependent arm: a SCD2 Expr that
// reaches for row_index, ColRef, BlockRef, Lookup, StreamDraw, Choose,
// or Cohort hooks fails at construction with an error wrapped into
// ErrInvalidSpec.
type scd2ConstContext struct{}

func (c *scd2ConstContext) LookupCol(string) (any, error) {
	return nil, fmt.Errorf("%w: scd2 Expr may not reference other columns", expr.ErrBadExpr)
}

func (c *scd2ConstContext) RowIndex(dgproto.RowIndex_Kind) int64 { return 0 }

func (c *scd2ConstContext) LookupDict(string) (*dgproto.Dict, error) {
	return nil, fmt.Errorf("%w: scd2 Expr may not read dicts", expr.ErrBadExpr)
}

func (c *scd2ConstContext) Call(name string, args []any) (any, error) {
	return stdlib.Call(name, args)
}

func (c *scd2ConstContext) BlockSlot(string) (any, error) {
	return nil, fmt.Errorf("%w: scd2 Expr may not read block slots", expr.ErrBadExpr)
}

func (c *scd2ConstContext) Lookup(string, string, int64) (any, error) {
	return nil, fmt.Errorf("%w: scd2 Expr may not perform lookups", expr.ErrBadExpr)
}

func (c *scd2ConstContext) Draw(uint32, string, int64) *rand.Rand {
	// Returning a PRNG here would invite non-constant SCD2 values; the
	// evaluator only calls Draw on StreamDraw / Choose arms, which we
	// reject at the StreamDraw path. A nil return would panic, so give
	// back a deterministic PRNG keyed on zero to keep downstream calls
	// well-typed — the result is still rejected by the boundary check
	// when the Expr happens to be a row-dependent draw.
	return rand.New(rand.NewPCG(0, 0)) //nolint:gosec // unreachable path
}

func (c *scd2ConstContext) AttrPath() string { return "" }

func (c *scd2ConstContext) CohortDraw(string, int64, int64) (int64, error) {
	return 0, fmt.Errorf("%w: scd2 Expr may not draw from cohorts", expr.ErrBadExpr)
}

func (c *scd2ConstContext) CohortLive(string, int64) (bool, error) {
	return false, fmt.Errorf("%w: scd2 Expr may not read cohort liveness", expr.ErrBadExpr)
}

func (c *scd2ConstContext) CohortBucketKey(string) *dgproto.Expr { return nil }

// startFor returns the start-column value for the row at rowIdx.
func (s *scd2State) startFor(rowIdx int64) any {
	if rowIdx < s.boundary {
		return s.histStart
	}

	return s.currStart
}

// endFor returns the end-column value for the row at rowIdx.
// Historical rows always carry histEnd; current rows return currEnd
// when declared, nil otherwise (SQL NULL).
func (s *scd2State) endFor(rowIdx int64) any {
	if rowIdx < s.boundary {
		return s.histEnd
	}

	if !s.hasCurrEnd {
		return nil
	}

	return s.currEnd
}
