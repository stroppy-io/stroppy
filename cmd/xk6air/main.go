/* Package xk6air is the K6 module 'k6/x/stroppy'.
 * TODO: stop to use 'protoMsg []byte' in module for arguments. Add descriptors pre-allocation.
 */
package xk6air

import (
	"context"
	"net/http"
	"os"
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
	cloudClient *cloudClientWrapper
	runULID     ulid.ULID
	ctx         context.Context
}

// NewModuleInstance factory method for Instances.
// One instance creates per VU.
func (r *RootModule) NewModuleInstance(vu modules.VU) modules.Instance { //nolint:ireturn
	return NewXK6Instance(vu, vu.Runtime().NewObject())
}

func NewXK6Instance(vu modules.VU, object *sobek.Object) modules.Instance {
	instance := &XK6Instance{}
	instance.exports.Default = instance
	instance.vu = vu
	// Create per-VU logger to avoid log level conflicts
	instance.lg = logger.NewFromEnv().
		Named("xk6air").
		WithOptions(
			zap.AddCallerSkip(0),
			zap.AddStacktrace(zap.FatalLevel),
		)
	return instance
}

func (i *XK6Instance) Exports() modules.Exports {
	return i.exports
}

// rootModule initialization.
func init() { //nolint:gochecknoinits // allow for xk6
	pluginLoggerName := "xk6air"
	lg := logger.NewFromEnv().
		Named(pluginLoggerName).
		WithOptions(
			zap.AddCallerSkip(0),
			zap.AddStacktrace(zap.FatalLevel),
		)

	var cloudURL = os.Getenv("STROPPY_CLOUD_URL")
	var runULIDString = os.Getenv("STROPPY_CLOUD_RUN_ID")

	// Check if cloud integration is configured
	if cloudURL == "" || runULIDString == "" {
		lg.Warn("cloud integration disabled - missing STROPPY_CLOUD_URL or STROPPY_CLOUD_RUN_ID")
		rootModule = &RootModule{
			lg:          lg,
			cloudClient: &cloudClientWrapper{client: &noopCloudClient{}, lg: lg},
			runULID:     ulid.ULID{},
			ctx:         context.Background(),
		}
		modules.Register("k6/x/stroppy", rootModule)
		return
	}

	runULID, err := ulid.Parse(runULIDString)
	if err != nil {
		lg.Sugar().Fatalf("'%s' parse ulid error: %w", runULIDString, err)
	}

	var cc = stroppyconnect.NewCloudStatusServiceClient(
		&http.Client{},
		cloudURL,
	)

	wrappedClient := &cloudClientWrapper{
		client: cc,
		lg:     lg,
	}

	rootModule = &RootModule{
		lg:          lg,
		cloudClient: wrappedClient,
		runULID:     runULID,
		ctx:         context.Background(),
	}

	wrappedClient.NotifyRun(rootModule.ctx, &stroppy.StroppyRun{
		Id:     &stroppy.Ulid{Value: rootModule.runULID.String()},
		Status: stroppy.Status_STATUS_IDLE,
		Config: &stroppy.ConfigFile{},
		Cmd:    "",
	})
	modules.Register("k6/x/stroppy", rootModule)
}

type XK6Instance struct {
	exports modules.Exports
	vu      modules.VU
	lg      *zap.Logger
	drv     driver.Driver
}

var rootModule *RootModule
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

// DefineConfig initializes the driver from GlobalConfig.
// This is called by scripts using defineConfig(globalConfig) at the top level.
func (i *XK6Instance) DefineConfig(globalCfg stroppy.GlobalConfig) {

	drvCfg := globalCfg.GetDriver()
	if drvCfg == nil {
		i.lg.Fatal("GlobalConfig.driver is required")
	}
	var err error
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

// ParseConfig is deprecated. Use DefineConfig instead.
// Kept for backward compatibility with existing scripts.
func (i *XK6Instance) ParseConfig(configBin []byte) {
	var drvCfg stroppy.DriverConfig
	err := proto.Unmarshal(configBin, &drvCfg)
	if err != nil {
		i.lg.Fatal("error unmarshall driver config", zap.Error(err))
	}
	i.drv, err = driver.Dispatch(rootModule.ctx, i.lg, &drvCfg)
	if err != nil {
		i.lg.Fatal("can't get driver", zap.Error(err))
	}

	onceDefineConfig.Do(func() {
		rootModule.cloudClient.NotifyRun(rootModule.ctx, &stroppy.StroppyRun{
			Id:     &stroppy.Ulid{Value: rootModule.runULID.String()},
			Status: stroppy.Status_STATUS_RUNNING,
			Config: &stroppy.ConfigFile{Global: &stroppy.GlobalConfig{Driver: &drvCfg}},
			Cmd:    "",
		})
	})
}

var _ modules.Module = new(RootModule)

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
func (i *XK6Instance) NotifyStep(name string, status int32) {
	rootModule.cloudClient.NotifyStep(i.vu.Context(), &stroppy.StroppyStepRun{
		Id:           &stroppy.Ulid{Value: getStepId(name).String()},
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
