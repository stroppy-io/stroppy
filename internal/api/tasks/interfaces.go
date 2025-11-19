package tasks

import (
	"context"

	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/crossplane"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/panel"
)

type RunRecordRepository interface {
	FindRunRecord(ctx context.Context, id string) (*panel.RunRecord, error)
}

type DeploymentActor interface {
	CreateDeployment(
		ctx context.Context,
		deployment *crossplane.Deployment,
	) (*crossplane.Deployment, error)
	ProcessDeploymentStatus(
		ctx context.Context,
		deployment *crossplane.Deployment,
	) (*crossplane.Deployment, error)
	DestroyDeployment(
		ctx context.Context,
		deployment *crossplane.Deployment,
	) error
}

type DeploymentBuilder interface {
	BuildVmDeployment(
		ctx context.Context,
		cloud crossplane.SupportedCloud,
		vm *crossplane.Deployment_Vm,
	) (*crossplane.Deployment, error)
}
