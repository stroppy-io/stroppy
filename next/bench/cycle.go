package bench

import "sync/atomic"

// CycleMode selects how Closed (and, structurally, Pool) hand cycle numbers to
// VUs. Open ignores it: its cycle is always the schedule index.
type CycleMode int

const (
	// CyclePartitioned gives each VU its own contiguous, non-overlapping cycle
	// range, so a run is deterministic and contention-free. Default.
	CyclePartitioned CycleMode = iota
	// CycleAtomic draws every cycle from one shared atomic counter. It balances
	// skewed work across VUs at the cost of reproducibility (which VU runs which
	// cycle depends on scheduling), so a run is no longer bit-reproducible.
	CycleAtomic
)

// String renders the CycleMode name.
func (m CycleMode) String() string {
	switch m {
	case CyclePartitioned:
		return "partitioned"
	case CycleAtomic:
		return "atomic"
	default:
		return "unknown"
	}
}

// cycler hands out the next cycle for a VU. Its Next is on the hot path and
// must not allocate.
type cycler interface {
	// Next returns the cycle for vu's next iteration.
	next(vuIndex int, local uint64) uint64
}

// partitionedCycler assigns VU k the contiguous range [k*span, (k+1)*span),
// where span = 2^64 / vus. VU k's j-th iteration is cycle k*span + j. Ranges
// never overlap (local counters stay far below span for any realistic run), so
// two runs with the same vus reproduce the identical cycle->VU assignment. When
// a per-VU iteration budget is set the range is used contiguously; the large
// span only guarantees non-overlap for the unbounded (duration) case.
type partitionedCycler struct{ span uint64 }

func newPartitionedCycler(vus int) partitionedCycler {
	span := ^uint64(0)
	if vus > 0 {
		span /= uint64(vus)
	}
	return partitionedCycler{span: span}
}

func (c partitionedCycler) next(vuIndex int, local uint64) uint64 {
	return uint64(vuIndex)*c.span + local
}

// atomicCycler draws cycles from a single shared counter; vuIndex and local are
// ignored.
type atomicCycler struct{ ctr *atomic.Uint64 }

func newAtomicCycler() atomicCycler { return atomicCycler{ctr: new(atomic.Uint64)} }

func (c atomicCycler) next(int, uint64) uint64 { return c.ctr.Add(1) - 1 }
