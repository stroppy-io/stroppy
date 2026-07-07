package bench

// partitionedCycler assigns VU k the contiguous cycle range
// [k*span, (k+1)*span), where span = 2^64 / vus. VU k's j-th iteration is cycle
// k*span + j. Ranges never overlap (local counters stay far below span for any
// realistic run), so the cycle->VU assignment is a pure function of vus. When a
// per-VU iteration budget is set the range is used contiguously; the large span
// only guarantees non-overlap for the unbounded (duration) case.
//
// This is the sole cycle-allocation policy. Run-repro is a non-goal at the
// concurrent layer — worker scheduling is not bit-reproducible across runs — so
// there is no shared-counter alternative. Partitioning carries the
// data-reproducibility contract: the generated dataset is bit-identical given
// (seed, WAREHOUSES) regardless of worker count, because a row's content is
// keyed by the global cycle, never by which worker produced it.
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
