package distribution

import (
	"testing"

	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
)

// TestNURandCLoadCRunDelta is the TPC-C §2.1.6.1 / §5.3 audit-grade check:
// for each spec A value, the |C_run − C_load| delta must fall within a
// mandated window. We verify across many seeds that the derived pair always
// lands in-range, and that both phase selectors return their intended C.
func TestNURandCLoadCRunDelta(t *testing.T) {
	type deltaSpec struct {
		a      int64
		loDiff int64
		hiDiff int64
	}

	specs := []deltaSpec{
		{a: 255, loDiff: 65, hiDiff: 119},     // C_LAST
		{a: 1023, loDiff: 259, hiDiff: 999},   // C_ID
		{a: 8191, loDiff: 2047, hiDiff: 7999}, // OL_I_ID
	}

	// Cover a broad range of seeds so a pathological seed can't slip through.
	// 10k iterations is fast (<50ms) and gives comfortable coverage.
	const seedCount = 10000

	for _, spec := range specs {
		for seed := uint64(1); seed <= seedCount; seed++ {
			loadDist := NewNURandDistribution[int64](
				seed,
				[2]int64{1, 3000},
				true,
				float64(spec.a),
				stroppy.Generation_Distribution_NURAND_PHASE_LOAD,
			)
			runDist := NewNURandDistribution[int64](
				seed,
				[2]int64{1, 3000},
				true,
				float64(spec.a),
				stroppy.Generation_Distribution_NURAND_PHASE_RUN,
			)

			// Both generators must derive the same (cLoad, cRun) pair from
			// the shared seed — phase only picks which one cVal uses.
			if loadDist.cLoad != runDist.cLoad || loadDist.cRun != runDist.cRun {
				t.Fatalf("A=%d seed=%d: (cLoad,cRun) mismatch across phases: load=(%d,%d) run=(%d,%d)",
					spec.a, seed,
					loadDist.cLoad, loadDist.cRun,
					runDist.cLoad, runDist.cRun)
			}

			// Phase selection must pick the intended C.
			if loadDist.cVal != loadDist.cLoad {
				t.Fatalf("A=%d seed=%d: LOAD phase used cVal=%d, want cLoad=%d",
					spec.a, seed, loadDist.cVal, loadDist.cLoad)
			}

			if runDist.cVal != runDist.cRun {
				t.Fatalf("A=%d seed=%d: RUN phase used cVal=%d, want cRun=%d",
					spec.a, seed, runDist.cVal, runDist.cRun)
			}

			// Both C values must remain within [0, A] per spec.
			if loadDist.cLoad < 0 || loadDist.cLoad > spec.a {
				t.Fatalf("A=%d seed=%d: cLoad=%d out of [0,%d]",
					spec.a, seed, loadDist.cLoad, spec.a)
			}

			if loadDist.cRun < 0 || loadDist.cRun > spec.a {
				t.Fatalf("A=%d seed=%d: cRun=%d out of [0,%d]",
					spec.a, seed, loadDist.cRun, spec.a)
			}

			// Delta must land in the audit window.
			delta := loadDist.cRun - loadDist.cLoad
			if delta < 0 {
				delta = -delta
			}

			if delta < spec.loDiff || delta > spec.hiDiff {
				t.Fatalf("A=%d seed=%d: |cRun-cLoad|=%d outside audit window [%d,%d] (cLoad=%d cRun=%d)",
					spec.a, seed, delta, spec.loDiff, spec.hiDiff,
					loadDist.cLoad, loadDist.cRun)
			}
		}
	}
}

// TestNURandPhaseUnspecifiedDefaultsToLoad verifies that an UNSPECIFIED
// phase (the proto zero-value, used by legacy callers) behaves identically
// to LOAD for back-compat.
func TestNURandPhaseUnspecifiedDefaultsToLoad(t *testing.T) {
	const (
		seed = uint64(42)
		a    = 1023.0
	)

	loadDist := NewNURandDistribution[int64](
		seed,
		[2]int64{1, 3000},
		true,
		a,
		stroppy.Generation_Distribution_NURAND_PHASE_LOAD,
	)
	unspecDist := NewNURandDistribution[int64](
		seed,
		[2]int64{1, 3000},
		true,
		a,
		stroppy.Generation_Distribution_NURAND_PHASE_UNSPECIFIED,
	)

	if loadDist.cVal != unspecDist.cVal {
		t.Fatalf("UNSPECIFIED should alias LOAD: load cVal=%d unspec cVal=%d",
			loadDist.cVal, unspecDist.cVal)
	}
}

// TestNURandUnknownAFallback checks that non-TPC-C A values fall back to
// a shared C across phases (no spec rule to enforce).
func TestNURandUnknownAFallback(t *testing.T) {
	const seed = uint64(7)

	const a = 500.0 // not 255/1023/8191

	loadDist := NewNURandDistribution[int64](
		seed,
		[2]int64{1, 1000},
		true,
		a,
		stroppy.Generation_Distribution_NURAND_PHASE_LOAD,
	)
	runDist := NewNURandDistribution[int64](
		seed,
		[2]int64{1, 1000},
		true,
		a,
		stroppy.Generation_Distribution_NURAND_PHASE_RUN,
	)

	if loadDist.cLoad != loadDist.cRun {
		t.Fatalf("unknown A=%v: cLoad=%d cRun=%d, want equal", a,
			loadDist.cLoad, loadDist.cRun)
	}

	if loadDist.cVal != runDist.cVal {
		t.Fatalf("unknown A=%v: phases should share C, got load=%d run=%d",
			a, loadDist.cVal, runDist.cVal)
	}
}
