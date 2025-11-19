package crossplaneservice

import (
	"context"
	"errors"
	"slices"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/crossplaneservice/k8s"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/crossplane"
)

type K8SActor interface {
	CreateResource(
		ctx context.Context,
		request *crossplane.Resource,
	) error
	UpdateResourceFromRemote(
		ctx context.Context,
		resource *crossplane.Resource,
	) (*crossplane.Resource, error)
	DeleteResource(
		ctx context.Context,
		ref *crossplane.ExtRef,
	) error
}

type CrossplaneService struct {
	k8sActor          K8SActor
	reconcileInterval time.Duration
}

func NewCrossplaneService(
	k8sActor K8SActor,
	reconcileInterval time.Duration,
) *CrossplaneService {
	return &CrossplaneService{
		k8sActor:          k8sActor,
		reconcileInterval: reconcileInterval,
	}
}

func (c *CrossplaneService) CreateDeployment(
	ctx context.Context,
	deployment *crossplane.Deployment,
) (*crossplane.Deployment, error) {
	nodes := deployment.GetResourceDag().GetNodes()
	for _, node := range nodes {
		node.Resource.Status = crossplane.Resource_STATUS_CREATING
		node.Resource.CreatedAt = timestamppb.Now()
		node.Resource.UpdatedAt = timestamppb.Now()
		err := c.k8sActor.CreateResource(ctx, node.GetResource())
		if err != nil {
			return nil, err
		}
	}
	return deployment, nil
}

func (c *CrossplaneService) ProcessDeploymentStatus(
	ctx context.Context,
	deployment *crossplane.Deployment,
) (*crossplane.Deployment, error) {
	for _, node := range deployment.GetResourceDag().GetNodes() {
		newResource, err := c.k8sActor.UpdateResourceFromRemote(ctx, node.GetResource())
		if err != nil {
			if errors.Is(err, k8s.ErrResourceNotFound) {
				if node.GetResource().GetStatus() == crossplane.Resource_STATUS_DESTROYING {
					node.Resource.Status = crossplane.Resource_STATUS_DESTROYED
					node.Resource.UpdatedAt = timestamppb.Now()
				}
				continue
			}
			return nil, err
		}
		node.Resource = newResource
		if IsResourceReady(newResource) {
			node.Resource.Status = crossplane.Resource_STATUS_READY
			node.Resource.UpdatedAt = timestamppb.Now()
		}
		if slices.Contains(
			[]crossplane.Resource_Status{
				crossplane.Resource_STATUS_CREATING,
				crossplane.Resource_STATUS_DESTROYING,
			},
			node.GetResource().GetStatus(),
		) &&
			node.GetResource().GetCreatedAt().AsTime().Add(c.reconcileInterval).Before(time.Now()) {
			node.Resource.Status = crossplane.Resource_STATUS_DEGRADED
			node.Resource.UpdatedAt = newResource.GetUpdatedAt()
			// delete resource if creating and degrading
			if node.GetResource().GetStatus() == crossplane.Resource_STATUS_CREATING {
				err := c.k8sActor.DeleteResource(ctx, node.GetResource().GetRef())
				if err != nil {
					return nil, err
				}
			}
		}
	}
	return deployment, nil
}

func (c *CrossplaneService) DestroyDeployment(
	ctx context.Context,
	deployment *crossplane.Deployment,
) error {
	for _, node := range deployment.GetResourceDag().GetNodes() {
		node.Resource.Status = crossplane.Resource_STATUS_DESTROYING
		err := c.k8sActor.DeleteResource(ctx, node.GetResource().GetRef())
		if err != nil {
			return err
		}
	}
	return nil
}
