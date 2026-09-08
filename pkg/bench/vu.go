package bench

import (
	"context"
)

// VU is the per-VU execution context for a Go workload. It carries
// per-iteration ctx + identity; sinks live on root. Step tag and iteration
// counters are read by the metrics layer and exposed to workloads via Bench.
type VU struct {
	root *RootState
	vuid uint64

	// initPhase distinguishes shared Setup drivers from per-VU Iterate drivers.
	initPhase bool

	// per-iteration mutable
	ctx          context.Context //nolint:containedctx // per-VU request/cancel lifecycle ctx, intentional
	stepTag      string
	iterTest     uint64
	iterScenario uint64
}

func (v *VU) Context() context.Context { return v.ctx }
func (v *VU) VUID() uint64             { return v.vuid }
