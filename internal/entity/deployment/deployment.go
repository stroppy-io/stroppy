package deployment

import (
	"context"
	"fmt"

	"github.com/stroppy-io/stroppy-cloud-panel/internal/entity/ids"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/crossplane"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/panel"
)

type VmDeploymentDagWithParams struct {
	Dag                *crossplane.ResourceDag
	Quotas             []*crossplane.Quota
	AssignedInternalIp *crossplane.Ip
}

type DagBuilder interface {
	BuildVmResourceDag(
		namespace string,
		commonId *panel.Ulid,
		vm *crossplane.Deployment_Vm,
	) (*VmDeploymentDagWithParams, error)
}

type Builder struct {
	dispatchMap map[crossplane.SupportedCloud]DagBuilder
}

func NewBuilder(dispatchMap map[crossplane.SupportedCloud]DagBuilder) *Builder {
	return &Builder{
		dispatchMap: dispatchMap,
	}
}

const DefaultCrossplaneNamespace = "crossplane-system"

var ErrUnsupportedCloud = fmt.Errorf("unsupported cloud")

func (b Builder) BuildVmDeployment(
	_ context.Context,
	cloud crossplane.SupportedCloud,
	commonId *panel.Ulid,
	vm *crossplane.Deployment_Vm,
) (*crossplane.Deployment, error) {
	builder, ok := b.dispatchMap[cloud]
	if !ok {
		return nil, ErrUnsupportedCloud
	}
	dagWithQuotas, err := builder.BuildVmResourceDag(DefaultCrossplaneNamespace, commonId, vm)
	if err != nil {
		return nil, err
	}
	vm.InternalIp = dagWithQuotas.AssignedInternalIp
	return &crossplane.Deployment{
		Id:             ids.NewUlid().String(),
		SupportedCloud: cloud,
		ResourceDag:    dagWithQuotas.Dag,
		UsingQuotas: &crossplane.Quota_List{
			Quotas: dagWithQuotas.Quotas,
		},
		Deployment: &crossplane.Deployment_Vm_{
			Vm: vm,
		},
	}, nil
}
