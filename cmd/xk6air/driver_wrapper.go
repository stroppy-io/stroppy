package xk6air

import (
	"fmt"
	"sync"

	"github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
	"github.com/stroppy-io/stroppy/pkg/driver"
	"github.com/stroppy-io/stroppy/pkg/driver/stats"
	"go.k6.io/k6/js/modules"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

// DriverWrapper is the per-VU driver handle exposed to JS.
// Created empty via NewDriver(), configured via Setup(), dispatched lazily on first use.
type DriverWrapper struct {
	vu modules.VU
	lg *zap.Logger

	// driverIndex is the deterministic index of this driver within the VU's init.
	// Used to coordinate shared drivers across VUs.
	driverIndex uint64

	cfg       *stroppy.DriverConfig
	shared    bool
	setupOnce sync.Once
	readyOnce sync.Once
	drv       driver.Driver
}

// Setup stores the driver configuration. Guarded by once.Do — safe to call every iteration.
//
// Sharing semantics are determined by the k6 lifecycle stage:
//   - vu.State() == nil (init phase): shared driver, dispatched lazily via rootModule slot
//   - vu.State() != nil (iteration/setup phase): per-VU driver, dispatched lazily on first use
func (d *DriverWrapper) Setup(configBin []byte) {
	d.setupOnce.Do(func() {
		var cfg stroppy.DriverConfig
		if err := proto.Unmarshal(configBin, &cfg); err != nil {
			d.lg.Fatal("error unmarshalling DriverConfig", zap.Error(err))
		}
		d.cfg = &cfg
		d.shared = d.vu.State() == nil
	})
}

// ensureReady lazily dispatches the driver on first use.
// At this point vu.State() is always available, so DialFunc is provided.
func (d *DriverWrapper) ensureReady() {
	d.readyOnce.Do(func() {
		if d.cfg == nil {
			d.lg.Fatal("driver not configured: call setup() before using the driver")
		}

		if d.shared {
			d.drv = rootModule.initSharedDriver(d.driverIndex, d.vu, d.cfg)
		} else {
			d.lg = d.lg.With(zap.Uint64("VUID", d.vu.State().VUID))
			var err error
			d.drv, err = driver.Dispatch(d.vu.Context(), driver.Options{
				Config:   d.cfg,
				Logger:   d.lg,
				DialFunc: d.vu.State().Dialer.DialContext,
			})
			if err != nil {
				d.lg.Fatal("can't initialize per-VU driver", zap.Error(err))
			}
		}
	})
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

// InsertSpecBin starts a relational bulk insert (InsertSpec) on the driver.
// The argument is a serialised dgproto.InsertSpec — the TS wrapper handles
// the marshal step so JS code never touches raw protobuf types.
func (d *DriverWrapper) InsertSpecBin(specBin []byte) (*stats.Query, error) {
	d.ensureReady()

	var spec dgproto.InsertSpec

	if err := proto.Unmarshal(specBin, &spec); err != nil {
		return nil, fmt.Errorf("error while unmarshalling InsertSpec: %w", err)
	}

	result, err := d.drv.InsertSpec(d.vu.Context(), &spec)
	if err != nil {
		return nil, fmt.Errorf("error while executing InsertSpec: %w", err)
	}

	return result, nil
}

// Begin starts a new transaction with the given isolation level.
// isolationLevel maps to proto TxIsolationLevel int32 values.
func (d *DriverWrapper) Begin(isolationLevel int32) (*TxWrapper, error) {
	d.ensureReady()

	level := stroppy.TxIsolationLevel(isolationLevel)

	// NONE mode: no actual transaction, delegate to driver.RunQuery
	if level == stroppy.TxIsolationLevel_NONE {
		return &TxWrapper{tx: nil, drv: d, vu: d.vu}, nil
	}

	tx, err := d.drv.Begin(d.vu.Context(), level)
	if err != nil {
		return nil, fmt.Errorf("error starting transaction: %w", err)
	}

	return &TxWrapper{tx: tx, drv: d, vu: d.vu}, nil
}

// TxWrapper wraps a driver.Tx for JS exposure.
// For NONE mode, tx is nil and queries delegate to the parent driver.
type TxWrapper struct {
	tx  driver.Tx
	drv *DriverWrapper
	vu  modules.VU
}

func (t *TxWrapper) RunQuery(sql string, args map[string]any) (*driver.QueryResult, error) {
	if t.tx == nil {
		// NONE mode: delegate to driver
		return t.drv.RunQuery(sql, args)
	}

	result, err := t.tx.RunQuery(t.vu.Context(), sql, args)
	if err != nil {
		return nil, fmt.Errorf("error executing query in transaction: %w", err)
	}

	return result, nil
}

func (t *TxWrapper) Commit() error {
	if t.tx == nil {
		return nil
	}

	return t.tx.Commit(t.vu.Context())
}

func (t *TxWrapper) Rollback() error {
	if t.tx == nil {
		return nil
	}

	return t.tx.Rollback(t.vu.Context())
}
