package expr

import (
	"fmt"
	"math/rand/v2"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
)

// Context carries the runtime bindings that an Expr tree reaches for
// during evaluation. Implementations are supplied by the runtime (B6)
// and by tests; the evaluator never constructs one itself.
// One method per Expr-arm dispatch target; splitting loses the
// single-point substitution property the runtime relies on.
//
//nolint:interfacebloat // see doc comment above.
type Context interface {
	// LookupCol returns the value of a previously-evaluated column in the
	// current row scratch, or ErrUnknownCol if the column is not set.
	LookupCol(name string) (any, error)

	// RowIndex returns the row counter for the requested kind.
	RowIndex(kind dgproto.RowIndex_Kind) int64

	// LookupDict returns the Dict identified by the opaque key from the
	// enclosing InsertSpec.dicts map. Returns ErrDictMissing on an
	// unknown key.
	LookupDict(key string) (*dgproto.Dict, error)

	// Call dispatches a stdlib function by name with already-evaluated
	// arguments. Returns ErrUnknownCall if the name is unregistered.
	Call(name string, args []any) (any, error)

	// BlockSlot returns the cached value of the named BlockSlot on the
	// enclosing Side, resolved against the current outer-side entity.
	// The flat runtime, which has no Sides, returns ErrBadExpr.
	BlockSlot(slot string) (any, error)

	// Lookup resolves a cross-population read: the named attr of the
	// named population at the given entity index. Implementations route
	// to the iter-side scratch for same-population reads or to the
	// LookupPop registry for sibling reads.
	Lookup(popName, attrName string, entityIdx int64) (any, error)

	// Draw returns a fresh PRNG seeded deterministically from the
	// implementation's root seed combined with attrPath, streamID, and
	// rowIdx. The Expr evaluator calls this once per StreamDraw /
	// Choose node to obtain a local *rand.Rand.
	//
	// Derivation convention:
	//   seed.Derive(rootSeed, attrPath, "s"+strconv.FormatUint(streamID),
	//                strconv.FormatInt(rowIdx, 10))
	// Keeping streamID and rowIdx in the path (rather than XORing into
	// the root) lets two attrs with different attr_paths produce
	// independent streams even when streamIDs collide and makes the
	// seed composition visible in seed.Derive's single formula.
	Draw(streamID uint32, attrPath string, rowIdx int64) *rand.Rand

	// AttrPath returns the path string identifying the attr currently
	// being evaluated. Used by StreamDraw / Choose to mix attr identity
	// into the per-draw seed; implementations empty-string out when no
	// attr is active (e.g. a test harness).
	AttrPath() string

	// CohortDraw returns the entity ID at position `slot` in the named
	// cohort schedule's bucket identified by bucketKey. Implementations
	// that host no Cohort registry return ErrBadCohort.
	CohortDraw(name string, bucketKey, slot int64) (int64, error)

	// CohortLive reports whether the named cohort's bucket identified
	// by bucketKey is active. Implementations that host no Cohort
	// registry return ErrBadCohort.
	CohortLive(name string, bucketKey int64) (bool, error)

	// CohortBucketKey returns the default bucket_key Expr declared on
	// the named cohort schedule, or nil when either the schedule does
	// not exist or no default bucket_key is configured. Callers use the
	// default only when the per-arm bucket_key override is absent.
	CohortBucketKey(name string) *dgproto.Expr
}

// evalLookup resolves a Lookup arm: it evaluates the entity-index
// subexpression, type-checks it to int64, and forwards the triple to
// the Context. Contexts that host no cross-population mechanism (the
// flat runtime) return ErrBadExpr from their Lookup hook.
func evalLookup(ctx Context, node *dgproto.Lookup) (any, error) {
	if node == nil {
		return nil, ErrBadExpr
	}

	indexVal, err := Eval(ctx, node.GetEntityIndex())
	if err != nil {
		return nil, err
	}

	index, ok := indexVal.(int64)
	if !ok {
		return nil, fmt.Errorf("%w: lookup entity_index %T", ErrTypeMismatch, indexVal)
	}

	return ctx.Lookup(node.GetTargetPop(), node.GetAttrName(), index)
}

// Eval evaluates expr against ctx and returns its Go-typed value.
func Eval(ctx Context, expr *dgproto.Expr) (any, error) {
	if expr == nil || expr.GetKind() == nil {
		return nil, ErrBadExpr
	}

	switch kind := expr.GetKind().(type) {
	case *dgproto.Expr_Col:
		return evalColRef(ctx, expr.GetCol())
	case *dgproto.Expr_RowIndex:
		return evalRowIndex(ctx, expr.GetRowIndex()), nil
	case *dgproto.Expr_Lit:
		return evalLiteral(expr.GetLit())
	case *dgproto.Expr_BinOp:
		return evalBinOp(ctx, expr.GetBinOp())
	case *dgproto.Expr_Call:
		return evalCall(ctx, expr.GetCall())
	case *dgproto.Expr_If_:
		return evalIf(ctx, expr.GetIf_())
	case *dgproto.Expr_DictAt:
		return evalDictAt(ctx, expr.GetDictAt())
	case *dgproto.Expr_BlockRef:
		return ctx.BlockSlot(expr.GetBlockRef().GetSlot())
	case *dgproto.Expr_Lookup:
		return evalLookup(ctx, expr.GetLookup())
	case *dgproto.Expr_StreamDraw:
		return evalStreamDraw(ctx, expr.GetStreamDraw())
	case *dgproto.Expr_Choose:
		return evalChoose(ctx, expr.GetChoose())
	case *dgproto.Expr_CohortDraw:
		return evalCohortDraw(ctx, expr.GetCohortDraw())
	case *dgproto.Expr_CohortLive:
		return evalCohortLive(ctx, expr.GetCohortLive())
	default:
		return nil, fmt.Errorf("%w: %T", ErrBadExpr, kind)
	}
}
