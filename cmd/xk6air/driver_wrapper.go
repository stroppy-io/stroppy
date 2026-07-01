package xk6air

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
	"github.com/stroppy-io/stroppy/pkg/driver"
	"github.com/stroppy-io/stroppy/pkg/driver/insertprogress"
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
	rootModule.txMetrics.recordQuery(d.vu)

	return result, nil
}

// InsertSpecBin starts a relational bulk insert (InsertSpec) on the driver.
// The argument is a serialized dgproto.InsertSpec — the TS wrapper handles
// the marshal step so JS code never touches raw protobuf types.
func (d *DriverWrapper) InsertSpecBin(specBin []byte) (*stats.Query, error) {
	d.ensureReady()

	var spec dgproto.InsertSpec

	if err := proto.Unmarshal(specBin, &spec); err != nil {
		return nil, fmt.Errorf("error while unmarshalling InsertSpec: %w", err)
	}

	tracker, err := d.newInsertProgressTracker(&spec)
	if err != nil {
		return nil, err
	}

	ctx := d.vu.Context()
	if tracker.Enabled() {
		ctx = insertprogress.ContextWithTracker(ctx, tracker)
		tracker.Start(ctx)
	}

	result, err := d.drv.InsertSpec(ctx, &spec)
	if tracker.Enabled() {
		tracker.Finish(err)
	}
	if err != nil {
		return nil, fmt.Errorf("error while executing InsertSpec: %w", err)
	}
	rootModule.txMetrics.recordInsert(d.vu, spec.GetTable(), result.Rows)

	return result, nil
}

// InsertTpch loads one TPC-H table using the ported dbgen generator. The JS
// side passes only the table name and scale factor; the spec (with the tpch
// generator arm) is assembled here so workloads never model TpchSource in TS.
// Method is driver-native (COPY / bulk / CSV shard); workers <= 0 means 1.
func (d *DriverWrapper) InsertTpch(table string, scaleFactor float64, workers int) (*stats.Query, error) {
	d.ensureReady()

	if workers < 1 {
		workers = 1
	}

	spec := &dgproto.InsertSpec{
		Table:       table,
		Method:      dgproto.InsertMethod_NATIVE,
		Parallelism: &dgproto.Parallelism{Workers: int32(workers)},
		Generator: &dgproto.InsertSpec_Tpch{
			Tpch: &dgproto.TpchSource{Table: table, ScaleFactor: scaleFactor},
		},
	}

	tracker, err := d.newInsertProgressTracker(spec)
	if err != nil {
		return nil, err
	}

	ctx := d.vu.Context()
	if tracker.Enabled() {
		ctx = insertprogress.ContextWithTracker(ctx, tracker)
		tracker.Start(ctx)
	}

	result, err := d.drv.InsertSpec(ctx, spec)
	if tracker.Enabled() {
		tracker.Finish(err)
	}

	if err != nil {
		return nil, fmt.Errorf("error while executing InsertTpch %q: %w", table, err)
	}

	rootModule.txMetrics.recordInsert(d.vu, table, result.Rows)

	return result, nil
}

// InsertTpcds loads one TPC-DS table using the ported dsdgen generator. As with
// InsertTpch the JS side passes only the table name and scale factor; the spec
// (with the tpcds generator arm) is assembled here so workloads never model
// TpcdsSource in TS. Method is driver-native; workers <= 0 means 1.
func (d *DriverWrapper) InsertTpcds(table string, scaleFactor float64, workers int) (*stats.Query, error) {
	d.ensureReady()

	if workers < 1 {
		workers = 1
	}

	spec := &dgproto.InsertSpec{
		Table:       table,
		Method:      dgproto.InsertMethod_NATIVE,
		Parallelism: &dgproto.Parallelism{Workers: int32(workers)},
		Generator: &dgproto.InsertSpec_Tpcds{
			Tpcds: &dgproto.TpcdsSource{Table: table, ScaleFactor: scaleFactor},
		},
	}

	tracker, err := d.newInsertProgressTracker(spec)
	if err != nil {
		return nil, err
	}

	ctx := d.vu.Context()
	if tracker.Enabled() {
		ctx = insertprogress.ContextWithTracker(ctx, tracker)
		tracker.Start(ctx)
	}

	result, err := d.drv.InsertSpec(ctx, spec)
	if tracker.Enabled() {
		tracker.Finish(err)
	}

	if err != nil {
		return nil, fmt.Errorf("error while executing InsertTpcds %q: %w", table, err)
	}

	rootModule.txMetrics.recordInsert(d.vu, table, result.Rows)

	return result, nil
}

func (d *DriverWrapper) newInsertProgressTracker(
	spec *dgproto.InsertSpec,
) (*insertprogress.Tracker, error) {
	config := insertprogress.DefaultConfig()
	config.Table = spec.GetTable()
	config.Method = insertMethodName(spec.GetMethod())
	config.Workers = int(spec.GetParallelism().GetWorkers())
	config.Logger = d.lg.Named("insert-progress")
	config.OnSample = func(snapshot insertprogress.Snapshot) {
		rootModule.txMetrics.recordInsertProgress(d.vu, snapshot)
	}

	if err := applyInsertProgressConfig(&config, d.cfg.GetInsertProgress()); err != nil {
		return nil, err
	}

	return insertprogress.NewTracker(&config), nil
}

func applyInsertProgressConfig(
	config *insertprogress.Config,
	raw *stroppy.DriverConfig_InsertProgressConfig,
) error {
	if raw == nil {
		return nil
	}

	if raw.Enabled != nil {
		config.Enabled = raw.GetEnabled()
	}

	if raw.Interval != nil {
		interval, err := parseInsertProgressDuration("insertProgress.interval", raw.GetInterval())
		if err != nil {
			return err
		}

		config.Interval = interval
	}

	if raw.StallAfter != nil {
		stallAfter, err := parseInsertProgressDuration("insertProgress.stallAfter", raw.GetStallAfter())
		if err != nil {
			return err
		}

		config.StallAfter = stallAfter
	}

	if raw.Mode != nil {
		mode, err := insertprogress.ParseMode(raw.GetMode())
		if err != nil {
			return err
		}

		config.Mode = mode
	}

	return nil
}

func parseInsertProgressDuration(name, raw string) (time.Duration, error) {
	duration, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s: parse duration %q: %w", name, raw, err)
	}

	if duration <= 0 {
		return 0, fmt.Errorf("%s must be positive, got %q", name, raw)
	}

	return duration, nil
}

func insertMethodName(method dgproto.InsertMethod) string {
	switch method {
	case dgproto.InsertMethod_PLAIN_QUERY:
		return "plain_query"
	case dgproto.InsertMethod_PLAIN_BULK:
		return "plain_bulk"
	case dgproto.InsertMethod_COLUMNAR:
		return "columnar"
	case dgproto.InsertMethod_NATIVE:
		return "native"
	default:
		return strings.ToLower(method.String())
	}
}

// Begin starts a new transaction with the given isolation level.
// isolationLevel maps to proto TxIsolationLevel int32 values.
func (d *DriverWrapper) Begin(isolationLevel int32, txName ...string) (*TxWrapper, error) {
	d.ensureReady()

	level := stroppy.TxIsolationLevel(isolationLevel)
	name := ""
	if len(txName) > 0 {
		name = txName[0]
	}

	// NONE mode: no actual transaction, delegate to driver.RunQuery
	if level == stroppy.TxIsolationLevel_NONE {
		return &TxWrapper{tx: nil, drv: d, vu: d.vu, isolation: level, name: name}, nil
	}

	tx, err := d.drv.Begin(d.vu.Context(), level)
	if err != nil {
		return nil, fmt.Errorf("error starting transaction: %w", err)
	}

	return &TxWrapper{tx: tx, drv: d, vu: d.vu, isolation: level, name: name}, nil
}

// TxWrapper wraps a driver.Tx for JS exposure.
// For NONE mode, tx is nil and queries delegate to the parent driver.
type TxWrapper struct {
	tx        driver.Tx
	drv       *DriverWrapper
	vu        modules.VU
	isolation stroppy.TxIsolationLevel
	name      string
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
	rootModule.txMetrics.recordQuery(t.vu)

	return result, nil
}

func (t *TxWrapper) Commit() error {
	if t.tx != nil {
		if err := t.tx.Commit(t.vu.Context()); err != nil {
			return err
		}
	}

	rootModule.txMetrics.record(t.vu, "commit", t.name, t.isolation)
	return nil
}

func (t *TxWrapper) Rollback() error {
	if t.tx != nil {
		if err := t.tx.Rollback(t.vu.Context()); err != nil {
			return err
		}
	}

	rootModule.txMetrics.record(t.vu, "rollback", t.name, t.isolation)
	return nil
}
