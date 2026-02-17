package xk6air

import (
	"fmt"
	"sync"

	"github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"github.com/stroppy-io/stroppy/pkg/driver"
	"go.k6.io/k6/js/modules"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

type DriverWrapper struct {
	vu  modules.VU
	lg  *zap.Logger
	drv driver.Driver

	configureOnce sync.Once
}

// This is a custom "VU setup" hook.
//
// NOTE: k6 have no option to make per VU setup code execution by itself.
// Check https://github.com/grafana/k6/issues/785
// https://github.com/grafana/k6/issues/1638
//
// Unfortunatly it's impossible to pass DialFunc at [Instance.NewDriverByConfigBin]
// because there is nil [modules.VU.State]. It may be fixed in the feature:
// https://github.com/grafana/k6/issues?q=is%3Aopen+is%3Aissue+label%3Anew-http
// https://github.com/grafana/k6/issues/2293
func (d *DriverWrapper) configure() {
	if rootModule.sharedDrv != nil {
		rootModule.once.Do(func() {
			rootModule.sharedDrv.Configure(rootModule.ctx, driver.Options{
				DialFunc: d.vu.State().Dialer.DialContext,
			})
		})
		return
	}
	d.configureOnce.Do(func() {
		d.drv.Configure(d.vu.Context(), driver.Options{
			DialFunc: d.vu.State().Dialer.DialContext,
		})
	})
}

func (d *DriverWrapper) RunQuery(sql string, args map[string]any) any {
	d.configure()
	stats, err := d.drv.RunQuery(d.vu.Context(), sql, args)
	if err != nil {
		return fmt.Errorf("error while executing sql query: %w", err)
	}
	return stats
}

// InsertValuesBin starts bulk insert blocking operation on driver.
func (d *DriverWrapper) InsertValuesBin(insertMsg []byte, count int64) any {
	d.configure()
	var descriptor stroppy.InsertDescriptor
	err := proto.Unmarshal(insertMsg, &descriptor)
	if err != nil {
		return err
	}

	stats, err := d.drv.InsertValues(d.vu.Context(), &descriptor)
	if err != nil {
		return err
	}

	return stats
}
