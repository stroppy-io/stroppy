package xk6

import (
	"context"

	"github.com/grafana/sobek"
	"github.com/stroppy-io/stroppy/pkg/common/logger"
	"github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"github.com/stroppy-io/stroppy/pkg/driver"
	"google.golang.org/protobuf/proto"

	"go.k6.io/k6/js/modules"
	"go.uber.org/zap"
)

// RootModule global object, runs with k6 process
type RootModule struct {
	lg *zap.Logger
}

func (r *RootModule) NewModuleInstance(vu modules.VU) modules.Instance { //nolint:ireturn
	vu.Runtime()
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

type XK6Instance struct {
	exports modules.Exports
	vu      modules.VU
	lg      *zap.Logger
	drv     driver.Driver
}

var rootModule *RootModule

func (i *XK6Instance) ParseConfig(configBin []byte) {
	var drvCfg stroppy.DriverConfig
	err := proto.Unmarshal(configBin, &drvCfg)
	if err != nil {
		i.lg.Panic("error unmarshall driver config", zap.Error(err))
	}
	processCtx := context.Background()
	i.lg.Sugar().Debugf(drvCfg.Url)
	drv, err := driver.Dispatch(processCtx, i.lg, &drvCfg)
	if err != nil {
		i.lg.Panic("can't get driver", zap.Error(err))
	}
	i.drv = drv
}

var _ modules.Module = new(RootModule)

func (i *XK6Instance) RunUnit(unitMsg []byte) (sobek.ArrayBuffer, error) {
	var unit stroppy.UnitDescriptor
	err := proto.Unmarshal(unitMsg, &unit)
	if err != nil {
		return sobek.ArrayBuffer{}, err
	}
	// TODO: keep global ctx
	stats, err := i.drv.RunTransaction(context.Background(), &unit)
	if err != nil {
		return sobek.ArrayBuffer{}, err
	}

	statsMsg, err := proto.Marshal(stats)
	if err != nil {
		return sobek.ArrayBuffer{}, err
	}
	return i.vu.Runtime().NewArrayBuffer(statsMsg), nil
}

func (i *XK6Instance) InsertValues(insertMsg []byte, count int64) (sobek.ArrayBuffer, error) {
	var descriptor stroppy.InsertDescriptor
	err := proto.Unmarshal(insertMsg, &descriptor)
	if err != nil {
		return sobek.ArrayBuffer{}, err
	}

	// TODO: keep global ctx
	stats, err := i.drv.InsertValues(context.Background(), &descriptor, count)
	if err != nil {
		return sobek.ArrayBuffer{}, err
	}

	statsMsg, err := proto.Marshal(stats)
	if err != nil {
		return sobek.ArrayBuffer{}, err
	}
	return i.vu.Runtime().NewArrayBuffer(statsMsg), nil
}

func init() { //nolint:gochecknoinits // allow for xk6
	pluginLoggerName := "xk6air"
	lg := logger.NewFromEnv().
		Named(pluginLoggerName).
		WithOptions(
			zap.AddCallerSkip(0),
			zap.AddStacktrace(zap.FatalLevel),
		)

	// var cc = stroppyconnect.NewCloudStatusServiceClient(
	// 	&http.Client{},
	// 	"",
	// )

	rootModule = &RootModule{
		lg: lg,
	}
	modules.Register("k6/x/stroppy", rootModule)
}
