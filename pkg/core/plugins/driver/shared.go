package driver

import (
	"context"

	stroppy "github.com/stroppy-io/stroppy/pkg/core/proto"
)

type Plugin interface {
	Initialize(ctx context.Context, runContext *stroppy.StepContext) error
	GenerateNext(context.Context, *stroppy.UnitDescriptor) (*stroppy.DriverTransaction, error)
	RunTransaction(ctx context.Context, transaction *stroppy.DriverTransaction) error
	Teardown(ctx context.Context) error
}
