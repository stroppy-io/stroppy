package engine

import (
	"fmt"
	"sync"

	"github.com/grafana/sobek"
	_ "github.com/stroppy-io/stroppy/pkg/driver/csv"
	_ "github.com/stroppy-io/stroppy/pkg/driver/mysql"
	_ "github.com/stroppy-io/stroppy/pkg/driver/noop"
	_ "github.com/stroppy-io/stroppy/pkg/driver/picodata"
	_ "github.com/stroppy-io/stroppy/pkg/driver/postgres"
	_ "github.com/stroppy-io/stroppy/pkg/driver/ydb"
	"go.uber.org/zap"
)

// Instance is the per-VU module instance (engine rewire of xk6air.Instance).
type Instance struct {
	vu *VU
	lg *zap.Logger

	// driverCounter tracks the Nth NewDriver() call within this VU's init,
	// used to coordinate shared drivers across VUs (deterministic ordering).
	driverCounter uint64

	// drivers tracks all DriverWrappers created by this instance for teardown.
	drivers []*DriverWrapper

	// activeStep is the logical Stroppy step this VU is currently in.
	activeStep string

	// exports memoizes the named exports table.
	exports map[string]any
}

// NewInstance creates a new module instance bound to the engine VU.
func NewInstance(vu *VU) *Instance {
	i := &Instance{
		vu: vu,
		lg: root.lg.Named("engine-vu").WithOptions(),
	}
	root.addVuTeardown(i)
	return i
}

// Exports returns the JS-visible named exports (set on the sobek VM as globals).
func (i *Instance) Exports() map[string]any {
	if i.exports != nil {
		return i.exports
	}
	i.exports = map[string]any{
		"NotifyStep":   root.NotifyStep,
		"SetStepTag":   i.SetStepTag,
		"ClearStepTag": i.ClearStepTag,
		"CurrentStep":  i.CurrentStep,
		"NewDriver":    i.NewDriver,
		"Teardown":     root.Teardown,
		"NewPicker":    NewPicker,
		"DeclareEnv":   func([]string, string, string) {},
		"Once":         i.Once,
		"GlobalOnce":   i.GlobalOnce,

		// In-process TPC-DS query-stream generation (no offline step).
		"GenerateTpcdsQueries": GenerateTpcdsQueries,

		// Draw iter 2 — sobek-bound Go structs, one per StreamDraw arm.
		"RegisterDict":       RegisterDict,
		"RegisterAlphabet":   RegisterAlphabet,
		"RegisterGrammar":    RegisterGrammar,
		"NewDrawIntUniform":  NewDrawIntUniform,
		"NewDrawFloatUniform": NewDrawFloatUniform,
		"NewDrawNormal":      NewDrawNormal,
		"NewDrawZipf":        NewDrawZipf,
		"NewDrawNURand":      NewDrawNURand,
		"NewDrawBernoulli":   NewDrawBernoulli,
		"NewDrawDate":        NewDrawDate,
		"NewDrawDecimal":     NewDrawDecimal,
		"NewDrawASCII":       NewDrawASCII,
		"NewDrawDict":        NewDrawDict,
		"NewDrawJoint":       NewDrawJoint,
		"NewDrawPhrase":      NewDrawPhrase,
		"NewDrawGrammar":     NewDrawGrammar,
	}
	return i.exports
}

// SetStepTag marks subsequent samples with the logical Stroppy step.
func (i *Instance) SetStepTag(name string) {
	i.activeStep = name
	i.vu.stepTag = name
}

// ClearStepTag removes the logical Stroppy step tag if it is still active.
func (i *Instance) ClearStepTag(name string) {
	if i.activeStep == name {
		i.activeStep = ""
		i.vu.stepTag = ""
	}
}

// CurrentStep returns the logical Stroppy step this VU is currently in.
func (i *Instance) CurrentStep() string { return i.activeStep }

// NewDriver creates an empty DriverWrapper shell, lazily dispatched on first use.
func (i *Instance) NewDriver() *DriverWrapper {
	idx := i.driverCounter
	i.driverCounter++

	dw := &DriverWrapper{
		vu:          i.vu,
		lg:          i.lg,
		driverIndex: idx,
	}
	i.drivers = append(i.drivers, dw)
	return dw
}

// Once wraps a function so it executes only once per VU.
func (i *Instance) Once(call sobek.FunctionCall) sobek.Value {
	rt := i.vu.Runtime()
	fn, ok := sobek.AssertFunction(call.Argument(0))
	if !ok {
		panic(rt.NewTypeError("Once() requires a function argument"))
	}

	var once sync.Once
	var result sobek.Value
	var callErr error

	return rt.ToValue(func(innerCall sobek.FunctionCall) sobek.Value {
		once.Do(func() {
			result, callErr = fn(sobek.Undefined(), innerCall.Arguments...)
		})
		if callErr != nil {
			panic(callErr)
		}
		return result
	})
}

// GlobalOnce executes a callback once across all VUs in this process.
func (i *Instance) GlobalOnce(call sobek.FunctionCall) sobek.Value {
	rt := i.vu.Runtime()
	const globalOnceArgs = 2
	if len(call.Arguments) < globalOnceArgs {
		panic(rt.NewTypeError("GlobalOnce() requires a name and function argument"))
	}

	name := call.Argument(0).String()
	if name == "" {
		panic(rt.NewTypeError("GlobalOnce() requires a non-empty name"))
	}

	fn, ok := sobek.AssertFunction(call.Argument(1))
	if !ok {
		panic(rt.NewTypeError("GlobalOnce() requires a function argument"))
	}

	slot := root.globalOnceSlot(name)
	slot.once.Do(func() {
		defer func() {
			if r := recover(); r != nil {
				slot.err = recoveredToError(r)
			}
		}()

		_, slot.err = fn(sobek.Undefined())
	})
	if slot.err != nil {
		panic(rt.NewGoError(fmt.Errorf("global once %q failed: %w", name, slot.err)))
	}

	return sobek.Undefined()
}

func recoveredToError(value any) error {
	if err, ok := value.(error); ok {
		return err
	}

	return fmt.Errorf("%v", value)
}

// Teardown mirrors k6 "function teardown()".
func (i *Instance) Teardown() error {
	for _, dw := range i.drivers {
		if dw.drv != nil && !dw.shared {
			dw.drv.Teardown(i.vu.Context())
		}
	}
	return nil
}
