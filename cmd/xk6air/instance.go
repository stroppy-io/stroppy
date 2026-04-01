package xk6air

import (
	"sync"

	"github.com/grafana/sobek"
	"github.com/stroppy-io/stroppy/pkg/common/generate"
	"github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	_ "github.com/stroppy-io/stroppy/pkg/driver/mysql"
	_ "github.com/stroppy-io/stroppy/pkg/driver/noop"
	_ "github.com/stroppy-io/stroppy/pkg/driver/picodata"
	_ "github.com/stroppy-io/stroppy/pkg/driver/postgres"
	_ "github.com/stroppy-io/stroppy/pkg/driver/ydb"
	"go.k6.io/k6/js/modules"
	"go.uber.org/zap"
)

// Instance - module instance.
// K6 creates it once before test run to get options and execute 'function setup()'.
// And K6 creates one instance per VU to execute 'function default' or 'exec: method'
type Instance struct {
	vu modules.VU
	lg *zap.Logger

	// driverCounter tracks the Nth NewDriver() call within this VU's init.
	// Used to coordinate shared drivers across VUs (deterministic ordering).
	driverCounter uint64

	// drivers tracks all DriverWrappers created by this instance for teardown.
	drivers []*DriverWrapper
}

// NewInstance creates new instance of module.
//
// NOTE: at this time vu.State() is nil.
// It's init phase, and only preInitEnv is accessible.
func NewInstance(vu modules.VU) modules.Instance {
	i := &Instance{
		vu: vu,
		lg: rootModule.lg.Named("k6-vu").
			WithOptions(),
	}
	rootModule.addVuTeardown(i)
	return i
}

func (i *Instance) Exports() modules.Exports {
	generate.NewValueGenerator(0, &stroppy.QueryParamDescriptor{})
	return modules.Exports{
		Default: i,
		Named: map[string]any{
			"NotifyStep":                  rootModule.NotifyStep,
			"NewDriver":                   i.NewDriver,
			"Teardown":                    rootModule.Teardown,
			"NewGeneratorByRuleBin":       NewGeneratorByRuleBin,
			"NewGroupGeneratorByRulesBin": NewGroupGeneratorByRulesBin,
			"NewPicker":                   NewPicker,
			"DeclareEnv":                  func([]string, string, string) {},
			"Once":                        i.Once,
		},
	}
}

// NewDriver creates an empty DriverWrapper shell.
// The driver is not connected — call Setup() to store config,
// then the driver is lazily dispatched on first use.
//
// The driverCounter ensures deterministic ordering across VUs
// so shared drivers can be coordinated.
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
// Call Once() during init to capture the sync.Once, then call the
// returned function during iterations — it will only fire on the first call.
// Accepts any function signature, returns a function with the same signature
// that caches and returns the result of the first invocation.
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

// Teardown mirrors k6 "function teardown()".
func (i *Instance) Teardown() error {
	for _, dw := range i.drivers {
		if dw.drv != nil && !dw.shared {
			dw.drv.Teardown(i.vu.Context())
		}
	}
	return nil
}
