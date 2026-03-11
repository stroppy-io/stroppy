/* Package xk6air is the K6 module 'k6/x/stroppy'.
 * TODO: stop to use 'protoMsg []byte' in module for arguments.
 *       Drop descriptors usage
 */
package xk6air

import (
	"context"
	"errors"
	"sync"

	"github.com/oklog/ulid/v2"
	"github.com/stroppy-io/stroppy/cmd/stroppy/commands"
	"github.com/stroppy-io/stroppy/pkg/common/logger"
	"github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"github.com/stroppy-io/stroppy/pkg/common/proto/stroppy/stroppyconnect"
	"github.com/stroppy-io/stroppy/pkg/driver"

	"go.k6.io/k6/js/modules"
	"go.k6.io/k6/subcommand"
	"go.uber.org/zap"
)

var rootModule *RootModule
var _ modules.Module = new(RootModule)

// rootModule initialization.
func init() {
	lg := logger.
		NewFromEnv().
		Named("k6-module").
		WithOptions(zap.AddStacktrace(zap.FatalLevel))

	rootModule = &RootModule{
		lg:               lg,
		ctx:              context.Background(),
		instanceTeardown: make(map[*Instance]func() error),
		sharedSlots:      make(map[uint64]*sharedDriverSlot),
		steps:            make(map[string]stroppy.StroppyRun_Status),
	}

	rootModule.runULID, rootModule.cloudClient = NewCloudClient(lg)

	modules.Register("k6/x/stroppy", rootModule)

	subcommand.RegisterExtension("stroppy", commands.K6Subcommand)
}

// sharedDriverSlot holds lazy-init state for a shared driver.
// The sync.Once ensures only the first VU to reach iteration phase creates the driver.
type sharedDriverSlot struct {
	once sync.Once
	drv  driver.Driver
}

// RootModule global object for all the VU instances.
type RootModule struct {
	lg          *zap.Logger
	cloudClient stroppyconnect.CloudStatusServiceClient
	runULID     ulid.ULID
	ctx         context.Context

	sharedMu    sync.Mutex
	sharedSlots map[uint64]*sharedDriverSlot

	instanceMu       sync.Mutex
	instanceTeardown map[*Instance]func() error

	stepsMu sync.Mutex
	steps   map[string]stroppy.StroppyRun_Status
}

// NewModuleInstance factory method for Instances.
// One instance creates per VU.
func (r *RootModule) NewModuleInstance(vu modules.VU) modules.Instance { //nolint:ireturn
	return NewInstance(vu)
}

// NotifyStep allows user to notify cloud-stroppy about test specific steps.
// Commonly to separate schema_init | insert | workload | cleanup stages.
// Accumulates step statuses and always sends the full snapshot.
func (r *RootModule) NotifyStep(name string, status int32) {
	r.stepsMu.Lock()
	r.steps[name] = stroppy.StroppyRun_Status(status)
	snapshot := make(map[string]stroppy.StroppyRun_Status, len(r.steps))
	for k, v := range r.steps {
		snapshot[k] = v
	}
	r.stepsMu.Unlock()

	r.cloudClient.NotifyRun(r.ctx, &stroppy.StroppyRun{
		Id:     r.runULID.String(),
		Status: stroppy.StroppyRun_STATUS_RUNNING,
		Cmd:    "",
		Steps:  snapshot,
	})
}

func (r *RootModule) addVuTeardown(instance *Instance) {
	r.instanceMu.Lock()
	r.instanceTeardown[instance] = instance.Teardown
	r.instanceMu.Unlock()
}

// initSharedDriver lazily creates a shared driver on the first VU to call it.
// The VU provides DialFunc from its State(), ensuring the shared driver has network access.
func (r *RootModule) initSharedDriver(
	index uint64,
	vu modules.VU,
	cfg *stroppy.DriverConfig,
) driver.Driver {
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
			DialFunc: vu.State().Dialer.DialContext,
		})
		if err != nil {
			r.lg.Fatal("can't initialize shared driver", zap.Error(err))
		}
		slot.drv = drv
	})

	return slot.drv
}

func (r *RootModule) Teardown() error {
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

	r.stepsMu.Lock()
	snapshot := make(map[string]stroppy.StroppyRun_Status, len(r.steps))
	for k, v := range r.steps {
		snapshot[k] = v
	}
	r.stepsMu.Unlock()

	_, errCloud := r.cloudClient.NotifyRun(rootModule.ctx, &stroppy.StroppyRun{
		Id:     rootModule.runULID.String(),
		Status: stroppy.StroppyRun_STATUS_COMPLETED,
		Cmd:    "",
		Steps:  snapshot,
	})
	return errors.Join(err, errCloud)
}
