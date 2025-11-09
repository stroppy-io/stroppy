package api

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy-cloud-panel/internal/core/nodetree"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/core/uow"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/orm"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/crossplane"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/panel"
)

func (p *PanelService) createCrossplaneResourcesTree(ctx context.Context, resources *panel.CloudResource_TreeNode) error {
	return uow.With(func(tx *uow.UnitOfWork) error {
		return nodetree.TraverseTreeBreadthFirst(resources,
			func(node *panel.CloudResource_TreeNode, depth int) (bool, error) {
				resExtRef := nodetree.GetExtNodeRef(node)
				resourceWithStatus, err := p.crossplaneService.CreateResource(ctx,
					&crossplane.CreateResourceRequest{
						Resource:    node.GetResource().GetResource().GetResourceDef(),
						Ref:         resExtRef.GetRef(),
						WaitForSync: false,
					},
				)
				if err != nil {
					return false, connect.NewError(
						connect.CodeInternal,
						fmt.Errorf("failed to create resource [%s]: %w", resExtRef.GetKind(), err),
					)
				}
				tx.Defer(func() {
					_, err := p.crossplaneService.DeleteResource(ctx, &crossplane.DeleteResourceRequest{
						Ref:         resExtRef,
						WaitForSync: false,
					})
					if err != nil {
						p.logger.Error(
							"failed to delete resource",
							zap.Error(err),
							zap.String("kind", resExtRef.GetKind()),
						)
					}
				})
				err = p.cloudResourceRepo.Insert(ctx, &panel.CloudResource{
					Id:               node.GetId(),
					Resource:         resourceWithStatus,
					Status:           panel.CloudResource_STATUS_CREATING,
					Timing:           node.GetResource().GetTiming(),
					ParentResourceId: node.GetResource().GetParentResourceId(),
				})
				if err != nil {
					return false, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to insert resource: %w", err))
				}
				return true, nil
			})
	})
}

func (p *PanelService) updateResourceStatus(ctx context.Context, root *panel.CloudResource_TreeNode) error {
	descendants, err := nodetree.GetDescendants(root)
	if err != nil {
		return err
	}
	for _, descendant := range descendants {
		err := p.updateResourceStatus(ctx, descendant)
		if err != nil {
			return err
		}
	}
	resourceStatus, err := p.crossplaneService.GetResourceStatus(ctx,
		&crossplane.GetResourceStatusRequest{Ref: nodetree.GetExtNodeRef(root)},
	)
	if err != nil {
		if connect.CodeOf(err) == connect.CodeNotFound {
			return p.cloudResourceRepo.Exec(ctx, orm.CloudResource.Update().
				Set(orm.CloudResource.Status.Set(int32(panel.CloudResource_STATUS_DESTROYED))).
				Where(orm.CloudResource.Id.Eq(root.GetId().GetId())))
		}
		return err
	}
	root.Resource.Resource.Ready = resourceStatus.GetReady()
	root.Resource.Resource.Synced = resourceStatus.GetSynced()
	root.Resource.Resource.ExternalId = resourceStatus.GetExternalId()
	ready, err := nodetree.IsNodeAndDescendantsReady(root)
	if err != nil {
		return err
	}
	if ready {
		root.Resource.Status = panel.CloudResource_STATUS_WORKING
	}
	return p.cloudResourceRepo.Update(ctx, root.GetResource(), orm.CloudResource.Id.Eq(root.GetId().GetId()))
}

func (p *PanelService) updateCrossplaneAutomation(ctx context.Context, automation *panel.CloudAutomation) error {
	databaseRootRes, err := p.GetResource(ctx, automation.DatabaseRootResourceId)
	if err != nil {
		return fmt.Errorf("failed to get database resource: %w", err)
	}
	err = p.updateResourceStatus(ctx, databaseRootRes)
	if err != nil {
		return fmt.Errorf("failed to update database resource status: %w", err)
	}
	workloadRootRes, err := p.GetResource(ctx, automation.WorkloadRootResourceId)
	if err != nil {
		return fmt.Errorf("failed to get workload resource: %w", err)
	}
	err = p.updateResourceStatus(ctx, workloadRootRes)
	if err != nil {
		return fmt.Errorf("failed to update workload resource status: %w", err)
	}
	if databaseRootRes.GetResource().GetStatus() == panel.CloudResource_STATUS_WORKING &&
		workloadRootRes.GetResource().GetStatus() == panel.CloudResource_STATUS_WORKING {
		return p.cloudAutomationRepo.Exec(ctx, orm.CloudAutomation.Update().
			Set(orm.CloudAutomation.Status.Set(int32(panel.Status_STATUS_RUNNING))).
			Where(
				orm.CloudAutomation.Id.Eq(automation.GetId().GetId()),
				orm.CloudAutomation.Status.Eq(int32(panel.Status_STATUS_IDLE)),
			))
	} else {
		if automation.GetTiming().GetCreatedAt().AsTime().Add(p.automateConfig.CreationTimeout).Before(time.Now()) {
			p.logger.Debug(
				"automation is timed out, stopping it",
				zap.String("automation_id", automation.GetId().GetId()),
				zap.Time("creation_time", automation.GetTiming().GetCreatedAt().AsTime()),
				zap.Duration("creation_timeout", p.automateConfig.CreationTimeout),
			)
			return p.stopCrossplaneAutomation(ctx, automation, panel.Status_STATUS_FAILED)
		}
	}
	return nil
}

func (p *PanelService) stopResource(
	ctx context.Context,
	root *panel.CloudResource_TreeNode,
) error {
	return nodetree.TraverseTreeBreadthFirst(root,
		func(node *panel.CloudResource_TreeNode, depth int) (bool, error) {
			resExtRef := nodetree.GetExtNodeRef(node)
			_, err := p.crossplaneService.DeleteResource(ctx, &crossplane.DeleteResourceRequest{
				Ref:         resExtRef,
				WaitForSync: false,
			})
			if err != nil {
				return false, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete resource:%s %w", resExtRef.GetKind(), err))
			}
			err = p.cloudResourceRepo.Exec(ctx, orm.CloudResource.Update().
				Set(orm.CloudResource.Status.Set(int32(panel.CloudResource_STATUS_DESTROYING))).
				Where(orm.CloudResource.Id.Eq(node.GetId().GetId())),
			)
			if err != nil {
				return false, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete resource: %w", err))
			}
			return true, nil
		})
}

func (p *PanelService) stopCrossplaneAutomation(
	ctx context.Context,
	automation *panel.CloudAutomation,
	targetAutomationStatus panel.Status,
) error {
	databaseRootRes, err := p.GetResource(ctx, automation.DatabaseRootResourceId)
	if err != nil {
		return fmt.Errorf("failed to get database resource: %w", err)
	}
	err = p.stopResource(ctx, databaseRootRes)
	if err != nil {
		return fmt.Errorf("failed to stop database resource: %w", err)
	}
	workloadRootRes, err := p.GetResource(ctx, automation.WorkloadRootResourceId)
	if err != nil {
		return fmt.Errorf("failed to get workload resource: %w", err)
	}
	err = p.stopResource(ctx, workloadRootRes)
	if err != nil {
		return fmt.Errorf("failed to stop workload resource: %w", err)
	}

	return p.cloudResourceRepo.Exec(ctx, orm.CloudAutomation.Update().
		Set(orm.CloudAutomation.Status.Set(int32(targetAutomationStatus))).
		Where(orm.CloudAutomation.Id.Eq(automation.GetId().GetId())),
	)

}
