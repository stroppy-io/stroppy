package engine

import (
	"context"
	"errors"
	"net"
	"sync"

	"github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"github.com/stroppy-io/stroppy/pkg/driver"
	k6metrics "go.k6.io/k6/metrics"
	"go.uber.org/zap"
)

// RootState is the engine-wide singleton, replacing xk6air.RootModule.
// Cloud status notification is a no-op at the floor.
type RootState struct {
	lg  *zap.Logger
	ctx context.Context

	// dialer backs both shared and per-VU drivers (k6 sourced it from each VU's
	// state; stroppy never customized it, so one process-wide dialer is enough).
	dialer *net.Dialer

	// metrics sink (k6/metrics substrate): txMetrics pushes Samples here, the
	// runner drains them into the console summary.
	registry *k6metrics.Registry
	samples  chan k6metrics.SampleContainer

	txMetrics *txMetrics

	sharedMu    sync.Mutex
	sharedSlots map[uint64]*sharedDriverSlot

	globalOnceMu    sync.Mutex
	globalOnceSlots map[string]*globalOnceSlot

	instanceMu       sync.Mutex
	instanceTeardown map[*Instance]func() error
}

// root is the process-wide engine state, set once by Run. The copied xk6air
// module code references it by this name (it was `rootModule` in xk6air).
var root *RootState

type sharedDriverSlot struct {
	once sync.Once
	drv  driver.Driver
}

type globalOnceSlot struct {
	once sync.Once
	err  error
}

func newRootState(lg *zap.Logger, ctx context.Context) *RootState {
	return &RootState{
		lg:               lg,
		ctx:              ctx,
		dialer:           &net.Dialer{},
		registry:         k6metrics.NewRegistry(),
		samples:          make(chan k6metrics.SampleContainer, 4096),
		txMetrics:        &txMetrics{},
		sharedSlots:      make(map[uint64]*sharedDriverSlot),
		globalOnceSlots:  make(map[string]*globalOnceSlot),
		instanceTeardown: make(map[*Instance]func() error),
	}
}

func (r *RootState) globalOnceSlot(name string) *globalOnceSlot {
	r.globalOnceMu.Lock()
	defer r.globalOnceMu.Unlock()

	slot, ok := r.globalOnceSlots[name]
	if !ok {
		slot = &globalOnceSlot{}
		r.globalOnceSlots[name] = slot
	}
	return slot
}

func (r *RootState) addVuTeardown(instance *Instance) {
	r.instanceMu.Lock()
	r.instanceTeardown[instance] = instance.Teardown
	r.instanceMu.Unlock()
}

// initSharedDriver lazily creates a shared driver on the first VU to call it.
func (r *RootState) initSharedDriver(index uint64, vu *VU, cfg *stroppy.DriverConfig) driver.Driver {
	r.sharedMu.Lock()
	slot, ok := r.sharedSlots[index]
	if !ok {
		slot = &sharedDriverSlot{}
		r.sharedSlots[index] = slot
	}
	r.sharedMu.Unlock()

	slot.once.Do(func() {
		drv, err := driver.Dispatch(vu.Context(), driver.Options{
			Config:   cfg,
			Logger:   r.lg,
			DialFunc: r.dialer.DialContext,
		})
		if err != nil {
			r.lg.Fatal("can't initialize shared driver", zap.Error(err))
		}
		slot.drv = drv
	})
	return slot.drv
}

// NotifyStep is a no-op at the floor (cloud notification deferred).
func (r *RootState) NotifyStep(name string, status int32) {}

// Teardown stops samplers, tears down all per-VU and shared drivers.
func (r *RootState) Teardown() error {
	r.txMetrics.stop()

	var err error
	r.instanceMu.Lock()
	for _, teardown := range r.instanceTeardown {
		err = errors.Join(err, teardown())
	}
	r.instanceMu.Unlock()

	r.sharedMu.Lock()
	for _, slot := range r.sharedSlots {
		if slot.drv != nil {
			slot.drv.Teardown(r.ctx)
		}
	}
	r.sharedMu.Unlock()

	return err
}
