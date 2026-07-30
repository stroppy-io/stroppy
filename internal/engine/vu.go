package engine

import (
	"context"

	js "github.com/grafana/sobek"
)

// VU is the engine's per-VU execution context. It replaces k6's modules.VU for
// the rewired module code: the methods the copied DriverWrapper/Instance/txMetrics
// call (Context, Runtime) keep their names so that logic is unchanged; k6-only
// accessors (State().Samples / Dialer / Tags, InitEnv().Registry) are replaced by
// fields reachable through root and VU itself.
type VU struct {
	root *RootState
	rt   *js.Runtime
	vuid uint64

	// initPhase mirrors k6's "vu.State() == nil" rule: a driver declared while
	// true (module-scope / setup run) is shared across VUs; one declared during
	// iterations is per-VU.
	initPhase bool

	// per-iteration mutable state
	ctx context.Context
	stepTag string

	// iteration counters, incremented before each exec call. Exposed via
	// exec.vu.* (k6 parity: idInTest, iterationInTest, iterationInScenario, ...).
	iterTest     uint64
	iterScenario uint64

	// execVu is the live `exec.vu` object; its fields are re-set each iteration.
	execVu *js.Object
}

func (v *VU) Context() context.Context { return v.ctx }
func (v *VU) Runtime() *js.Runtime     { return v.rt }
func (v *VU) VUID() uint64             { return v.vuid }
