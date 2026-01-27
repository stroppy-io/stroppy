/* Package xk6air is the K6 module 'k6/x/stroppy'.
 * TODO: stop to use 'protoMsg []byte' in module for arguments.
 *       Drop descriptors usage
 */
package xk6air

import (
	"context"

	"github.com/oklog/ulid/v2"
	"github.com/stroppy-io/stroppy/pkg/common/logger"
	"github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"github.com/stroppy-io/stroppy/pkg/common/proto/stroppy/stroppyconnect"

	"go.k6.io/k6/js/modules"
	"go.uber.org/zap"
)

var rootModule *RootModule
var _ modules.Module = new(RootModule)

// rootModule initialization.
func init() { //nolint:gochecknoinits // allow for xk6
	lg := logger.
		NewFromEnv().
		Named("k6-module").
		WithOptions(zap.AddStacktrace(zap.FatalLevel))

	rootModule = &RootModule{
		lg:  lg,
		ctx: context.Background(),
	}

	rootModule.runULID, rootModule.cloudClient = NewCloudClient(lg)

	modules.Register("k6/x/stroppy", rootModule)
}

// RootModule global object for all the VU instances.
type RootModule struct {
	lg          *zap.Logger
	cloudClient stroppyconnect.CloudStatusServiceClient
	runULID     ulid.ULID
	ctx         context.Context
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
