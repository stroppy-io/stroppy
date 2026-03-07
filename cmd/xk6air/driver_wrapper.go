package xk6air

import (
	"fmt"
	"sync"

	"github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"github.com/stroppy-io/stroppy/pkg/driver"
	"github.com/stroppy-io/stroppy/pkg/driver/stats"
	"go.k6.io/k6/js/modules"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

// DriverWrapper is the per-VU driver handle exposed to JS.
// Created empty via NewDriver(), configured via Setup().
type DriverWrapper struct {
	vu modules.VU
	lg *zap.Logger

	drv       driver.Driver
	setupOnce sync.Once

	// driverIndex is the deterministic index of this driver within the VU's init.
	// Used to coordinate shared drivers across VUs.
	driverIndex uint64
}

// Setup configures the driver. Guarded by once.Do — safe to call every iteration.
//
// Sharing semantics are determined by the k6 lifecycle stage:
//   - vu.State() == nil (init phase): shared driver, stored on rootModule
//   - vu.State() != nil (iteration/setup phase): per-VU driver, created immediately
//
// The optional lambda runs after the driver is ready (useful for per-VU schema setup).
func (d *DriverWrapper) Setup(configBin []byte, lambda func()) {
	d.setupOnce.Do(func() {
		var cfg stroppy.DriverConfig
		if err := proto.Unmarshal(configBin, &cfg); err != nil {
			d.lg.Fatal("error unmarshalling DriverConfig", zap.Error(err))
		}

		if d.vu.State() == nil {
			// Init phase: shared driver
			d.drv = rootModule.getOrCreateSharedDriver(d.driverIndex, d.lg, &cfg)
		} else {
			// Iteration/setup phase: per-VU driver
			lg := d.lg.With(zap.Uint64("VUID", d.vu.State().VUID))
			drv, err := driver.Dispatch(d.vu.Context(), driver.Options{
				Config:   &cfg,
				Logger:   lg,
				DialFunc: d.vu.State().Dialer.DialContext,
			})
			if err != nil {
				d.lg.Fatal("can't initialize per-VU driver", zap.Error(err))
			}
			d.drv = drv
			d.lg = lg
		}

		if lambda != nil {
			lambda()
		}
	})
}

// ensureReady reconfigures a shared driver with DialFunc once VU state becomes available.
func (d *DriverWrapper) ensureReady() {
	if d.drv == nil {
		d.lg.Fatal("driver not configured: call setup() before using the driver")
	}
}

func (d *DriverWrapper) RunQuery(sql string, args map[string]any) (*driver.QueryResult, error) {
	d.ensureReady()
	result, err := d.drv.RunQuery(d.vu.Context(), sql, args)
	if err != nil {
		return nil, fmt.Errorf("error while executing sql query: %w", err)
	}
	return result, nil
}

// InsertValuesBin starts bulk insert blocking operation on driver.
func (d *DriverWrapper) InsertValuesBin(insertMsg []byte, count int64) (*stats.Query, error) {
	d.ensureReady()
	var descriptor stroppy.InsertDescriptor
	err := proto.Unmarshal(insertMsg, &descriptor)
	if err != nil {
		return nil, fmt.Errorf("error while unmarshalling insert descriptor: %w", err)
	}

	result, err := d.drv.InsertValues(d.vu.Context(), &descriptor)
	if err != nil {
		return nil, fmt.Errorf("error while executing insert: %w", err)
	}

	return result, nil
}
