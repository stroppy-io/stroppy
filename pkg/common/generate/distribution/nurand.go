package distribution

import (
	r "math/rand/v2"

	"github.com/stroppy-io/stroppy/pkg/common/generate/constraint"
	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
)

// TPC-C §2.1.6 / §2.1.6.1 / §5.3 spec-mandated constants. The outer key is
// the NURand A value (255 C_LAST, 1023 C_ID, 8191 OL_I_ID); the inner pair
// is the [min, max] inclusive window for |C_run − C_load|.
//
// Hoisted into this table so that (a) the numeric literals appear once and
// the switch below can iterate a typed map, and (b) golangci-lint's mnd
// checker sees them as named constants rather than magic switch cases.
const (
	nuRandACLast  = int64(255)  // A for C_LAST
	nuRandACID    = int64(1023) // A for C_ID
	nuRandAOLIID  = int64(8191) // A for OL_I_ID
	nuRandLoCLast = int64(65)
	nuRandHiCLast = int64(119)
	nuRandLoCID   = int64(259)
	nuRandHiCID   = int64(999)
	nuRandLoOLIID = int64(2047)
	nuRandHiOLIID = int64(7999)
)

// nuRandDeltaWindow returns the [lo, hi] inclusive delta window required by
// TPC-C §2.1.6.1 for the given A. ok=false means A is not a spec value and
// there is no audit rule — callers should fall back to a shared C.
func nuRandDeltaWindow(a int64) (lo, hi int64, ok bool) {
	switch a {
	case nuRandACLast:
		return nuRandLoCLast, nuRandHiCLast, true
	case nuRandACID:
		return nuRandLoCID, nuRandHiCID, true
	case nuRandAOLIID:
		return nuRandLoOLIID, nuRandHiOLIID, true
	default:
		return 0, 0, false
	}
}

// NURandDistribution implements the TPC-C non-uniform random function per
// TPC-C spec §2.1.6:
//
//	NURand(A, x, y) = (((rand(0,A) | rand(x,y)) + C) % (y - x + 1)) + x
//
// where `|` is a bitwise OR of two independent uniform samples and `C` is a
// per-generator constant chosen once from seed. Typical `A` values used by
// TPC-C are 255 (C_LAST), 1023 (C_ID), and 8191 (OL_I_ID).
//
// Per §2.1.6.1 / §5.3, the C constant used during C-Load (data population)
// must differ from the C used during C-Run (measurement) by a delta that
// falls into an A-specific window:
//
//	A = 255  (C_LAST)  → |C_run − C_load| ∈ [65, 119]
//	A = 1023 (C_ID)    → |C_run − C_load| ∈ [259, 999]
//	A = 8191 (OL_I_ID) → |C_run − C_load| ∈ [2047, 7999]
//
// We derive BOTH C_load and C_run from the same PRNG in the same order, so
// that two generators constructed with the same seed but different phases
// produce reproducible, matching (C_load, C_run) pairs. The phase field then
// selects which of the two to use for Next(). For non-TPC-C A values (or
// A = 0) we fall back to a single derived C shared across both phases —
// there's no spec rule to satisfy.
//
// Only integers make sense for this distribution; construct with `round=true`.
type NURandDistribution[T constraint.Number] struct {
	prng  *r.Rand
	aVal  int64 // A parameter (the mask upper bound for the OR term)
	cVal  int64 // C constant actually used by Next(), picked by phase
	cLoad int64 // C derived for the C-Load phase (stored for audit/debug)
	cRun  int64 // C derived for the C-Run  phase (stored for audit/debug)
	xVal  int64 // low bound (inclusive)
	mod   int64 // y - x + 1
}

// NewNURandDistribution constructs a NURand distribution over [ranges[0], ranges[1]]
// using `aParam` as TPC-C's `A`. The `round` flag is ignored (output is always
// integer). `C` is derived deterministically from seed so two generators with
// the same seed (and the same phase) produce matching sequences. Use `phase`
// to select C-Load vs C-Run per TPC-C §2.1.6.1 / §5.3.
func NewNURandDistribution[T constraint.Number](
	seed uint64,
	ranges [2]T,
	_ bool,
	aParam float64,
	phase stroppy.Generation_Distribution_NURandPhase,
) *NURandDistribution[T] {
	prng := r.New(r.NewPCG(seed, seed)) //nolint: gosec // benchmark PRNG

	aInt := max(int64(aParam), 0)

	// Derive C_load and C_run from the same PRNG in a fixed order so that
	// both phases end up with consistent, reproducible values from a shared
	// seed. For TPC-C's known A values we enforce the §2.1.6.1 delta window;
	// for unknown A we share a single derived C.
	var cLoad, cRun int64

	if aInt > 0 {
		if lo, hi, known := nuRandDeltaWindow(aInt); known {
			// Pick delta ∈ [lo, hi] and C_load ∈ [0, A-hi] so that
			// C_run = C_load + delta stays in [0, A]. Both values are
			// deterministic from the same seed because we always advance
			// the PRNG in the same order regardless of the requested phase.
			delta := lo + prng.Int64N(hi-lo+1)
			cLoad = prng.Int64N(aInt - hi + 1)
			cRun = cLoad + delta
		} else {
			// Non-TPC-C A: no spec rule; use a single derived C for both phases.
			cLoad = prng.Int64N(aInt + 1)
			cRun = cLoad
		}
	}

	var cInt int64

	switch phase {
	case stroppy.Generation_Distribution_NURAND_PHASE_RUN:
		cInt = cRun
	default: // UNSPECIFIED or LOAD
		cInt = cLoad
	}

	xInt := int64(ranges[0])
	yInt := int64(ranges[1])
	mod := max(yInt-xInt+1, 1)

	return &NURandDistribution[T]{
		prng:  prng,
		aVal:  aInt,
		cVal:  cInt,
		cLoad: cLoad,
		cRun:  cRun,
		xVal:  xInt,
		mod:   mod,
	}
}

// Next returns the next NURand value in [x, y]. See the type comment for the
// formula.
func (nd *NURandDistribution[T]) Next() T {
	var aSample int64
	if nd.aVal > 0 {
		aSample = nd.prng.Int64N(nd.aVal + 1)
	}

	bSample := nd.xVal + nd.prng.Int64N(nd.mod)

	// ((a | b) + C) % (y - x + 1) + x
	v := (((aSample | bSample) + nd.cVal) % nd.mod) + nd.xVal

	return T(v)
}
