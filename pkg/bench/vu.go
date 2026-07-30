package bench

import (
	"context"
)

// VU is the per-VU execution context for a Go workload (no sobek). Carries
// per-iteration ctx + identity; sinks live on root. Step tag and iteration
// counters are read by the metrics layer and exposed to workloads via Bench.
type VU struct {
	root *RootState
	vuid uint64

	// initPhase mirrors k6's "vu.State() == nil" rule: a driver declared while
	// true (Setup run) is shared across VUs; one declared during Iterate is per-VU.
	initPhase bool

	// per-iteration mutable
	ctx          context.Context
	stepTag      string
	iterTest     uint64
	iterScenario uint64
}

func (v *VU) Context() context.Context { return v.ctx }
func (v *VU) VUID() uint64             { return v.vuid }
