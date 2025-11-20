package tasks

import (
	"context"
	"net"

	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/crossplane"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/panel"
)

type RunRecordRepository interface {
	FindRunRecord(ctx context.Context, id string) (*panel.RunRecord, error)
}

type CidrProvider interface {
	UsingCidr(ctx context.Context) *net.IPNet
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
		commonId *panel.Ulid,
		vm *crossplane.Deployment_Vm,
	) (*crossplane.Deployment, error)
}

type QuotaRepository interface {
	GetQuota(
		ctx context.Context,
		cloud crossplane.SupportedCloud,
		kind crossplane.Quota_Kind,
	) (*crossplane.Quota, error)
	FindQuotas(
		ctx context.Context,
		cloud crossplane.SupportedCloud,
		kinds []crossplane.Quota_Kind,
	) ([]*crossplane.Quota, error)
	IncrementQuota(
		ctx context.Context,
		cloud crossplane.SupportedCloud,
		kind crossplane.Quota_Kind,
		added uint32,
	) error
	DecrementQuota(
		ctx context.Context,
		cloud crossplane.SupportedCloud,
		kind crossplane.Quota_Kind,
		subtracted uint32,
	) error
}
