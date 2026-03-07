package xk6air

import (
	"sync/atomic"

	"github.com/stroppy-io/stroppy/pkg/common/generate"
	"github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	_ "github.com/stroppy-io/stroppy/pkg/driver/postgres"
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
	driverCounter atomic.Uint64

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
			WithOptions(zap.AddStacktrace(zap.FatalLevel)),
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
		},
	}
}

// NewDriver creates an empty DriverWrapper shell.
// The driver is not connected — call Setup() to configure it.
//
// The driverCounter ensures deterministic ordering across VUs
// so shared drivers can be coordinated.
func (i *Instance) NewDriver() *DriverWrapper {
	idx := i.driverCounter.Add(1) - 1

	dw := &DriverWrapper{
		vu:          i.vu,
		lg:          i.lg,
		driverIndex: idx,
	}
	i.drivers = append(i.drivers, dw)
	return dw
}

// Teardown mirrors k6 "function teardown()".
func (i *Instance) Teardown() error {
	for _, dw := range i.drivers {
		if dw.drv != nil {
			// Only teardown per-VU drivers (non-shared).
			// Shared drivers are torn down by RootModule.Teardown.
			if !rootModule.isSharedDriver(dw.drv) {
				dw.drv.Teardown(i.vu.Context())
			}
		}
	}
	return nil
}
