package bench

import (
	"context"
	"errors"
	"net"
	"sync"

	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"github.com/stroppy-io/stroppy/pkg/driver"
)

// root is the process-wide engine state for the Go-native runner. Set once by Run.
var root *RootState

// RootState is the engine-wide singleton for a Go-workload run: owns the metrics
// sink, dialer, shared-driver slots, and cross-VU once barriers. Cloud notify is
// a no-op at the floor.
type RootState struct {
	lg  *zap.Logger
	ctx context.Context

	// dialer backs shared and per-VU drivers.
	dialer *net.Dialer

	// metrics sink (k6/metrics substrate).
	registry *Registry
	samples  chan SampleContainer

	txMetrics *txMetrics

	sharedMu    sync.Mutex
	sharedSlots map[uint64]*sharedDriverSlot

	globalOnceMu    sync.Mutex
	globalOnceSlots map[string]*globalOnceSlot

	// env is the script env (-e overrides, config) passed to Run; consulted by
	// Env after the real process environment (real env takes precedence).
	env map[string]string

	// stepFilter is built at Run start (after STROPPY_STEPS/NO_STEPS env is set).
	stepFilter *stepFilterState
}

type sharedDriverSlot struct {
	once sync.Once
	drv  driver.Driver
}

type globalOnceSlot struct {
	once sync.Once
	err  error
}

func newRootState(lg *zap.Logger, ctx context.Context, env map[string]string) *RootState {
	return &RootState{
		lg:              lg,
		ctx:             ctx,
		dialer:          &net.Dialer{},
		registry:        NewRegistry(),
		samples:         make(chan SampleContainer, 4096),
		txMetrics:       &txMetrics{},
		sharedSlots:     make(map[uint64]*sharedDriverSlot),
		globalOnceSlots: make(map[string]*globalOnceSlot),
		env:             env,
		stepFilter:      newStepFilter(),
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

// Teardown closes all shared drivers. (Workload.Teardown is invoked separately by Run.)
func (r *RootState) Teardown() error {
	r.txMetrics.stop()

	var err error

	r.sharedMu.Lock()
	for _, slot := range r.sharedSlots {
		if slot.drv != nil {
			err = errors.Join(err, slot.drv.Teardown(r.ctx))
		}
	}
	r.sharedMu.Unlock()

	return err
}
