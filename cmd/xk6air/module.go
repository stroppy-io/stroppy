/* Package xk6air is the K6 module 'k6/x/stroppy'.
 * TODO: stop to use 'protoMsg []byte' in module for arguments.
 *       Drop descriptors usage
 */
package xk6air

import (
	"context"
	"sync"

	"github.com/grafana/sobek"
	"github.com/oklog/ulid/v2"
	"github.com/stroppy-io/stroppy/pkg/common/logger"
	"github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"github.com/stroppy-io/stroppy/pkg/common/proto/stroppy/stroppyconnect"
	"github.com/stroppy-io/stroppy/pkg/driver"
	"google.golang.org/protobuf/proto"

	"go.k6.io/k6/js/modules"
	"go.uber.org/zap"
)

// RootModule global object for all the VU instances.
type RootModule struct {
	lg          *zap.Logger
	cloudClient stroppyconnect.CloudStatusServiceClient
	runULID     ulid.ULID
	ctx         context.Context
}

// NewModuleInstance factory method for Instances.
// One instance creates per VU.
func (r *RootModule) NewModuleInstance(vu modules.VU) modules.Instance { //nolint:ireturn
	return NewXK6Instance(vu, vu.Runtime().NewObject())
}

// rootModule initialization.
func init() { //nolint:gochecknoinits // allow for xk6
	lg := logger.
		NewFromEnv().
		Named("k6-module").
		WithOptions(zap.AddStacktrace(zap.FatalLevel))

	rootModule = &RootModule{
		lg:  lg,
		ctx: context.Background(),
	}

	rootModule.runULID, rootModule.cloudClient = NewCloudClient(lg)

	modules.Register("k6/x/stroppy", rootModule)
}

var rootModule *RootModule
var _ modules.Module = new(RootModule)

type XK6Instance struct {
	vu  modules.VU
	lg  *zap.Logger
	drv driver.Driver
}

func NewXK6Instance(vu modules.VU, object *sobek.Object) modules.Instance {
	instance := &XK6Instance{
		vu:  vu,
		lg:  &zap.Logger{},
		drv: nil,
	}
	// Create per-VU logger to avoid log level conflicts
	VUID := uint64(0)
	if state := vu.State(); state != nil {
		VUID = state.VUID
	}
	instance.lg = logger.
		NewFromEnv().
		Named("k6-vu").
		With(zap.Uint64("VUID", uint64(VUID))).
		WithOptions(zap.AddStacktrace(zap.FatalLevel))

	return instance
}

func (i *XK6Instance) Exports() modules.Exports {
	return modules.Exports{
		Default: i,
		Named:   map[string]any{},
	}
}

var onceDefineConfig sync.Once

// DefineConfigBin initializes the driver from GlobalConfig.
// This is called by scripts using defineConfig(globalConfig) at the top level.
func (i *XK6Instance) DefineConfigBin(configBin []byte) {
	var globalCfg stroppy.GlobalConfig
	err := proto.Unmarshal(configBin, &globalCfg)
	if err != nil {
		i.lg.Fatal("error unmarshalling GlobalConfig", zap.Error(err))
	}

	drvCfg := globalCfg.GetDriver()
	if drvCfg == nil {
		i.lg.Fatal("GlobalConfig.driver is required")
	}

	i.drv, err = driver.Dispatch(rootModule.ctx, i.lg, drvCfg)
	if err != nil {
		i.lg.Fatal("can't initialize driver", zap.Error(err))
	}

	onceDefineConfig.Do(func() {
		rootModule.cloudClient.NotifyRun(rootModule.ctx, &stroppy.StroppyRun{
			Id:     &stroppy.Ulid{Value: rootModule.runULID.String()},
			Status: stroppy.Status_STATUS_RUNNING,
			Config: &stroppy.ConfigFile{Global: &globalCfg},
			Cmd:    "",
		})
	})
}

func (i *XK6Instance) RunQuery(sql string, args map[string]any) {
	i.drv.RunQuery(i.vu.Context(), sql, args)
}

// RunUnit runs a single driver unit: query | transaction | create_table | insert
func (i *XK6Instance) RunUnit(unitMsg []byte) (sobek.ArrayBuffer, error) {
	var unit stroppy.UnitDescriptor
	err := proto.Unmarshal(unitMsg, &unit)
	if err != nil {
		return sobek.ArrayBuffer{}, err
	}

	stats, err := i.drv.RunTransaction(i.vu.Context(), &unit)
	if err != nil {
		return sobek.ArrayBuffer{}, err
	}

	statsMsg, err := proto.Marshal(stats)
	if err != nil {
		return sobek.ArrayBuffer{}, err
	}
	return i.vu.Runtime().NewArrayBuffer(statsMsg), nil
}

// NotifyStep allows user to notify cloud-stroppy about test specific steps.
// Commonly to separate schema_init | insert | workload | cleanup stages.
// TODO: separate. should be standalone function
func (i *XK6Instance) NotifyStep(name string, status int32) {
	rootModule.cloudClient.NotifyStep(i.vu.Context(), &stroppy.StroppyStepRun{
		Id:           &stroppy.Ulid{Value: getStepID(name).String()},
		StroppyRunId: &stroppy.Ulid{Value: rootModule.runULID.String()},
		Context: &stroppy.StepContext{
			Step: &stroppy.Step{
				Name: name,
			},
		},
		Status: stroppy.Status(status),
	})
}

// InsertValues starts bulk insert blocking operation on driver.
func (i *XK6Instance) InsertValues(insertMsg []byte, count int64) (sobek.ArrayBuffer, error) {
	var descriptor stroppy.InsertDescriptor
	err := proto.Unmarshal(insertMsg, &descriptor)
	if err != nil {
		return sobek.ArrayBuffer{}, err
	}

	stats, err := i.drv.InsertValues(i.vu.Context(), &descriptor, count)
	if err != nil {
		return sobek.ArrayBuffer{}, err
	}

	statsMsg, err := proto.Marshal(stats)
	if err != nil {
		return sobek.ArrayBuffer{}, err
	}
	return i.vu.Runtime().NewArrayBuffer(statsMsg), nil
}

// Teardown mirrors k6 "function teardown()".
func (i *XK6Instance) Teardown() error {
	rootModule.cloudClient.NotifyRun(rootModule.ctx, &stroppy.StroppyRun{
		Id:     &stroppy.Ulid{Value: rootModule.runULID.String()},
		Status: stroppy.Status_STATUS_COMPLETED,
		Config: &stroppy.ConfigFile{},
		Cmd:    "",
	})
	return nil
}
