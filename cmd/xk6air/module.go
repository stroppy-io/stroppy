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
		lg:         lg,
		ctx:        context.Background(),
		vuTeardown: make(map[*Instance]func() error),
	}

	rootModule.runULID, rootModule.cloudClient = NewCloudClient(lg)

	modules.Register("k6/x/stroppy", rootModule)

	subcommand.RegisterExtension("stroppy", commands.K6Subcommand)
}

// RootModule global object for all the VU instances.
type RootModule struct {
	lg          *zap.Logger
	cloudClient stroppyconnect.CloudStatusServiceClient
	runULID     ulid.ULID
	ctx         context.Context

	sharedDrv driver.Driver
	once      sync.Once

	vuMutex    sync.Mutex
	vuTeardown map[*Instance]func() error
}

// NewModuleInstance factory method for Instances.
// One instance creates per VU.
func (r *RootModule) NewModuleInstance(vu modules.VU) modules.Instance { //nolint:ireturn
	return NewInstance(vu)
}

// NotifyStep allows user to notify cloud-stroppy about test specific steps.
// Commonly to separate schema_init | insert | workload | cleanup stages.
func (r *RootModule) NotifyStep(name string, status int32) {
	r.cloudClient.NotifyStep(r.ctx, &stroppy.StroppyStepRun{
		Id:           &stroppy.Ulid{Value: getStepID(name).String()},
		StroppyRunId: &stroppy.Ulid{Value: r.runULID.String()},
		Status:       stroppy.Status(status),
		Name:         name,
	})
}

func (r *RootModule) addVuTeardown(instance *Instance) {
	r.vuMutex.Lock()
	r.vuTeardown[instance] = instance.Teardown
	r.vuMutex.Unlock()
}

func (r *RootModule) Teardown() error {

	var err error
	r.vuMutex.Lock()
	for _, teardown := range r.vuTeardown {
		err = errors.Join(err, teardown())
	}
	r.vuMutex.Unlock()

	r.sharedDrv.Teardown(r.ctx)

	_, errCloud := r.cloudClient.NotifyRun(rootModule.ctx, &stroppy.StroppyRun{
		Id:     &stroppy.Ulid{Value: rootModule.runULID.String()},
		Status: stroppy.Status_STATUS_COMPLETED,
		Cmd:    "",
	})
	return errors.Join(err, errCloud)
}
