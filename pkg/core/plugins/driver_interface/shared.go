package driver_interface

import (
	"context"

	stroppy "github.com/stroppy-io/stroppy/pkg/core/proto"
)

type Driver interface {
	Initialize(ctx context.Context, runContext *stroppy.StepContext) error
	GenerateNextUnit(
		ctx context.Context,
		unit *stroppy.UnitDescriptor,
	) (*stroppy.DriverTransaction, error)
	RunTransaction(ctx context.Context, transaction *stroppy.DriverTransaction) error
	Teardown(ctx context.Context) error
}
