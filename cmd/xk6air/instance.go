package xk6air

import (
	"sync"

	"github.com/stroppy-io/stroppy/pkg/common/generate"
	"github.com/stroppy-io/stroppy/pkg/common/logger"
	"github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"github.com/stroppy-io/stroppy/pkg/driver"
	_ "github.com/stroppy-io/stroppy/pkg/driver/postgres"
	"go.k6.io/k6/js/modules"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

// Instance - module instance.
// K6 creates it once before test run to get options and execute 'function setup()'.
// And K6 creates one instance per VU to execute 'function default' or 'exec: method'
type Instance struct {
	vu modules.VU
	lg *zap.Logger
	dw *DriverWrapper
}

// NewInstance creates new instance of module.
//
// NOTE: at this time vu.State() is nil.
// It's init phase, and only preInitEnv is accessible.
func NewInstance(vu modules.VU) modules.Instance {
	// Create per-VU logger to avoid log level conflicts
	VUID := uint64(0)
	if state := vu.State(); state != nil {
		VUID = state.VUID
	}
	i := &Instance{
		vu: vu,
		lg: logger.
			NewFromEnv().
			Named("k6-vu").
			With(zap.Uint64("VUID", uint64(VUID))).
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
			"NewDriverByConfigBin":        i.NewDriverByConfigBin,
			"Teardown":                    rootModule.Teardown,
			"NewGeneratorByRuleBin":       NewGeneratorByRuleBin,
			"NewGroupGeneratorByRulesBin": NewGroupGeneratorByRulesBin,
		},
	}
}

var onceGetConfig sync.Once

// NewDriverByConfigBin initializes the driver from GlobalConfig.
// This is called by scripts using defineConfig(globalConfig) at the top level.
//
// NOTE: this function commonly called at init phase of k6 lifecycle.
// i.vu.State() is nil
func (i *Instance) NewDriverByConfigBin(configBin []byte) *DriverWrapper {
	var globalCfg stroppy.GlobalConfig
	if err := proto.Unmarshal(configBin, &globalCfg); err != nil {
		i.lg.Fatal("error unmarshalling GlobalConfig", zap.Error(err))
	}
	drvCfg := globalCfg.GetDriver()
	if drvCfg == nil {
		i.lg.Fatal("GlobalConfig.driver is required")
	}

	onceGetConfig.Do(func() {
		rootModule.cloudClient.NotifyRun(rootModule.ctx, &stroppy.StroppyRun{
			Id:     &stroppy.Ulid{Value: rootModule.runULID.String()},
			Status: stroppy.Status_STATUS_RUNNING,
			Cmd:    "",
		})
	})

	drv := i.getOrCreateDriver(&globalCfg)

	i.dw = &DriverWrapper{
		vu:  i.vu,
		lg:  i.lg,
		drv: drv,
	}
	return i.dw
}

func (i *Instance) getOrCreateDriver(cfg *stroppy.GlobalConfig) (drv driver.Driver) {
	var err error
	if cfg.GetDriver().GetConnectionType().GetSingleConnPerVu() != nil {
		if drv, err = driver.Dispatch(rootModule.ctx, i.lg, cfg.GetDriver()); err != nil {
			i.lg.Fatal("can't initialize driver", zap.Error(err))
		}
		return drv
	}

	if rootModule.sharedDrv != nil {
		return rootModule.sharedDrv
	}

	if cfg.GetDriver().GetConnectionType() == nil {
		// NOTE: unfortunately we have no good suggestion on which amount of connections we may use.
		// Nice idea to use i.State().Options.VUs, but it's not available at pre-init state.
		cfg.GetDriver().ConnectionType = &stroppy.DriverConfig_ConnectionType{
			Is: &stroppy.DriverConfig_ConnectionType_SharedPool{},
		}
	}

	rootModule.sharedDrv, err = driver.Dispatch(rootModule.ctx, i.lg, cfg.GetDriver())
	if err != nil {
		i.lg.Fatal("can't initialize shared driver", zap.Error(err))
	}
	return rootModule.sharedDrv
}

// Teardown mirrors k6 "function teardown()".
func (i *Instance) Teardown() error {
	if rootModule.sharedDrv == nil {
		i.dw.drv.Teardown(i.vu.Context())
	}

	return nil
}
