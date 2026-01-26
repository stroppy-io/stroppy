package xk6air

import (
	"sync"

	"github.com/stroppy-io/stroppy/pkg/common/generate"
	"github.com/stroppy-io/stroppy/pkg/common/logger"
	"github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"github.com/stroppy-io/stroppy/pkg/driver"
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

func NewInstance(vu modules.VU) modules.Instance {
	// Create per-VU logger to avoid log level conflicts
	VUID := uint64(0)
	if state := vu.State(); state != nil {
		VUID = state.VUID
	}
	return &Instance{
		vu: vu,
		lg: logger.
			NewFromEnv().
			Named("k6-vu").
			With(zap.Uint64("VUID", uint64(VUID))).
			WithOptions(zap.AddStacktrace(zap.FatalLevel)),
	}
}

func (i *Instance) Exports() modules.Exports {
	generate.NewValueGenerator(0, &stroppy.QueryParamDescriptor{})
	return modules.Exports{
		Default: i,
		Named: map[string]any{
			"NotifyStep":                  rootModule.NotifyStep,
			"NewDriverByConfigBin":        i.NewDriverByConfigBin,
			"Teardown":                    i.Teardown,
			"NewGeneratorByRuleBin":       NewGeneratorByRuleBin,
			"NewGroupGeneratorByRulesBin": NewGroupGeneratorByRulesBin,
		},
	}
}

var onceDefineConfig sync.Once

// NewDriverByConfig initializes the driver from GlobalConfig.
// This is called by scripts using defineConfig(globalConfig) at the top level.
func (i *Instance) NewDriverByConfigBin(configBin []byte) *DriverWrapper {
	var globalCfg stroppy.GlobalConfig
	err := proto.Unmarshal(configBin, &globalCfg)
	if err != nil {
		i.lg.Fatal("error unmarshalling GlobalConfig", zap.Error(err))
	}
	drvCfg := globalCfg.GetDriver()
	if drvCfg == nil {
		i.lg.Fatal("GlobalConfig.driver is required")
	}

	drv, err := driver.Dispatch(rootModule.ctx, i.lg, drvCfg)
	if err != nil {
		i.lg.Fatal("can't initialize driver", zap.Error(err))
	}

	onceDefineConfig.Do(func() {
		rootModule.cloudClient.NotifyRun(rootModule.ctx, &stroppy.StroppyRun{
			Id:     &stroppy.Ulid{Value: rootModule.runULID.String()},
			Status: stroppy.Status_STATUS_RUNNING,
			Cmd:    "",
		})
	})

	i.dw = &DriverWrapper{
		vu:  i.vu,
		lg:  i.lg,
		drv: drv,
	}
	return i.dw
}

// Teardown mirrors k6 "function teardown()".
func (i *Instance) Teardown() error {
	i.dw.drv.Teardown(i.vu.Context())

	rootModule.cloudClient.NotifyRun(rootModule.ctx, &stroppy.StroppyRun{
		Id:     &stroppy.Ulid{Value: rootModule.runULID.String()},
		Status: stroppy.Status_STATUS_COMPLETED,
		Cmd:    "",
	})
	return nil
}
