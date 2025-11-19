package deployment

import (
	"context"
	"fmt"

	"github.com/stroppy-io/stroppy-cloud-panel/internal/entity/ids"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/crossplane"
)

type dagBuilder interface {
	BuildVmResourceDag(namespace string, strategy *crossplane.Deployment_Vm) (*crossplane.ResourceDag, error)
}

type Builder struct {
	dispatchMap map[crossplane.SupportedCloud]dagBuilder
}

func NewBuilder(dispatchMap map[crossplane.SupportedCloud]dagBuilder) *Builder {
	return &Builder{
		dispatchMap: dispatchMap,
	}
}

const DefaultCrossplaneNamespace = "crossplane-system"

var ErrUnsupportedCloud = fmt.Errorf("unsupported cloud")

func (b Builder) BuildVmDeployment(
	ctx context.Context,
	cloud crossplane.SupportedCloud,
	vm *crossplane.Deployment_Vm,
) (*crossplane.Deployment, error) {
	builder, ok := b.dispatchMap[cloud]
	if !ok {
		return nil, ErrUnsupportedCloud
	}
	dag, err := builder.BuildVmResourceDag(
		DefaultCrossplaneNamespace,
		vm,
	)
	if err != nil {
		return nil, err
	}
	return &crossplane.Deployment{
		Id:             ids.NewUlid().String(),
		SupportedCloud: cloud,
		ResourceDag:    dag,
		Deployment: &crossplane.Deployment_Vm_{
			Vm: vm,
		},
	}, nil
}
