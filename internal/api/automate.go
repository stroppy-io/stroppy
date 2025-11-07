package api

import (
	"context"
	"errors"
	"fmt"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/core/nodetree"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/core/uow"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/embed"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/entity/resource"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/entity/timestamps"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/crossplane"
	"go.uber.org/zap"

	"connectrpc.com/connect"

	"github.com/stroppy-io/stroppy-cloud-panel/internal/entity/ids"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/orm"
	postgres "github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/postgresql"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/panel"
)

func (p *PanelService) GetAutomation(ctx context.Context, ulid *panel.Ulid) (*panel.CloudAutomation, error) {
	automation, err := p.cloudAutomationRepo.GetBy(
		ctx,
		orm.CloudAutomation.SelectAll().Where(orm.CloudAutomation.Id.Eq(ulid.GetId())),
	)
	if err != nil {
		return nil, err
	}
	return automation, nil
}

func (p *PanelService) updateResourceStatus(ctx context.Context, root *panel.CloudResource_TreeNode) error {
	allNodes, err := nodetree.CollectNodes(root, func(node *panel.CloudResource_TreeNode) bool {
		return true // Вернуть все узлы
	})
	if err != nil {
		return err
	}
	descendants := allNodes[1:]
	for _, descendant := range descendants {
		err := p.updateResourceStatus(ctx, descendant)
		if err != nil {
			return err
		}
		resExtRef := resource.ExtRefFromResourceDef(
			descendant.GetResource().GetResource().GetRef(),
			descendant.GetResource().GetResource().GetResourceDef(),
		)
		resourceStatus, err := p.crossplaneService.GetResourceStatus(ctx, &crossplane.GetResourceStatusRequest{
			Ref: resExtRef,
		})
		if err != nil {
			return err
		}
		err = p.cloudResourceRepo.Exec(ctx, orm.CloudResource.Update().
			Set(
				orm.CloudResource.Synced.Set(resourceStatus.GetSynced()),
				orm.CloudResource.Ready.Set(resourceStatus.GetReady()),
				orm.CloudResource.ExternalId.Set(resourceStatus.GetExternalId()),
			).Where(orm.CloudResource.Id.Eq(descendant.GetId().GetId())))
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *PanelService) BackgroundCheckAutomationStatus(ctx context.Context) error {
	p.logger.Info("BackgroundCheckAutomationStatus started")
	automations, err := p.cloudAutomationRepo.ListBy(ctx, orm.CloudAutomation.SelectAll())
	if err != nil {
		return connect.NewError(connect.CodeInternal, err)
	}
	return postgres.WithSerializable(ctx, p.txManager, func(ctx context.Context) error {
		for _, automation := range automations {
			databaseRes, err := p.GetResource(ctx, automation.DatabaseRootResourceId)
			if err != nil {
				p.logger.Error("failed to get database resource", zap.Error(err), zap.String("automation_id", automation.GetId().GetId()))
				continue
			}
			err = p.updateResourceStatus(ctx, databaseRes)
			if err != nil {
				return fmt.Errorf("failed to update database resource status: %w", err)
			}
			allDatabaseNodes, _ := nodetree.CollectNodes(databaseRes, func(node *panel.CloudResource_TreeNode) bool { return true })

			workloadRes, err := p.GetResource(ctx, automation.WorkloadRootResourceId)
			if err != nil {
				p.logger.Error("failed to get workload resource", zap.Error(err), zap.String("automation_id", automation.GetId().GetId()))
				continue
			}
			err = p.updateResourceStatus(ctx, workloadRes)
			if err != nil {
				return fmt.Errorf("failed to update workload resource status: %w", err)
			}
			allWorkloadNodes, _ := nodetree.CollectNodes(workloadRes, func(node *panel.CloudResource_TreeNode) bool { return true })

			allNodesActive := true
			for _, node := range append(allDatabaseNodes, allWorkloadNodes...) {
				if !node.GetResource().GetResource().GetReady() {
					allNodesActive = false
					break
				}
			}
			autmationId := automation.GetId().GetId()
			if allNodesActive {
				err = p.runRecordRepo.Exec(ctx, orm.RunRecord.Update().
					Set(orm.RunRecord.Status.Set(int32(panel.Status_STATUS_RUNNING))).
					Where(orm.RunRecord.CloudAutomationId.Eq(&autmationId)))
				if err != nil {
					return fmt.Errorf("failed to update automation status: %w", err)
				}
			}

		}
		return nil
	})
}

func (p *PanelService) createCrossplaneResourcesTree(ctx context.Context, resources *panel.CloudResource_TreeNode) error {
	return uow.With(func(tx *uow.UnitOfWork) error {
		return nodetree.TraverseTreeBreadthFirst(resources,
			func(node *panel.CloudResource_TreeNode, depth int) (bool, error) {
				resExtRef := resource.ExtRefFromResourceDef(
					node.GetResource().GetResource().GetRef(),
					node.GetResource().GetResource().GetResourceDef(),
				)
				resourceWithStatus, err := p.crossplaneService.CreateResource(ctx,
					&crossplane.CreateResourceRequest{
						Resource:    node.GetResource().GetResource().GetResourceDef(),
						Ref:         resExtRef.GetRef(),
						WaitForSync: false,
					},
				)
				if err != nil {
					return false, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create resource:%s %w", resExtRef.GetKind(), err))
				}
				tx.Defer(func() {
					_, err := p.crossplaneService.DeleteResource(ctx, &crossplane.DeleteResourceRequest{
						Ref:         resExtRef,
						WaitForSync: false,
					})
					if err != nil {
						p.logger.Error("failed to delete resource", zap.Error(err), zap.String("kind", resExtRef.GetKind()))
					}
				})
				err = p.cloudResourceRepo.Insert(ctx, &panel.CloudResource{
					Id:               node.GetId(),
					Resource:         resourceWithStatus,
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

func (p *PanelService) RunAutomation(ctx context.Context, request *panel.RunAutomationRequest) (*panel.RunRecord, error) {
	user, err := p.getUserFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	var builder resource.Builder
	switch request.GetUsingCloudProvider() {
	case crossplane.SupportedCloud_SUPPORTED_CLOUD_UNSPECIFIED:
		builder = resource.NewYandexCloudBuilder(&p.k8sConfig.Crossplane.YandexCloudProviderConfig)
	}
	if builder == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("unsupported cloud provider"))
	}

	if request.GetDatabase().GetDatabaseType() != panel.Database_TYPE_POSTGRES_ORIOLE {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("unsupported database type"))
	}

	if request.GetWorkload().GetWorkloadType() != panel.Workload_TYPE_TPCC {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("unsupported workload type"))
	}

	newAutomationId := ids.NewUlid()
	dbDeployScript, err := embed.GetOrioleInstallScript()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get oriole install script: %w", err))
	}
	databaseResourcesTree, err := builder.NewDatabaseResources(newAutomationId, request.GetDatabase(), dbDeployScript)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	workloadDeployScript, err := embed.GetStroppyInstallScript()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get stroppy install script: %w", err))
	}
	workloadResourcesTree, err := builder.NewWorkloadResources(newAutomationId, request.GetWorkload(), workloadDeployScript)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	// TODO: Do not hardcode paths
	return postgres.WithSerializableRet(ctx, p.txManager,
		func(ctx context.Context) (*panel.RunRecord, error) {
			err = p.createCrossplaneResourcesTree(ctx, databaseResourcesTree)
			if err != nil {
				return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create database resources: %w", err))
			}
			err = p.createCrossplaneResourcesTree(ctx, workloadResourcesTree)
			if err != nil {
				return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create workload resources: %w", err))
			}
			err = p.cloudAutomationRepo.Insert(ctx, &panel.CloudAutomation{
				Id:                     newAutomationId,
				DatabaseRootResourceId: databaseResourcesTree.GetId(),
				WorkloadRootResourceId: workloadResourcesTree.GetId(),
				StroppyRunId:           nil,
			})
			if err != nil {
				return nil, connect.NewError(connect.CodeInternal, err)
			}
			newRunRecord := &panel.RunRecord{
				Id:                newAutomationId,
				AuthorId:          user.GetId(),
				Timing:            timestamps.NewTiming(),
				Status:            panel.Status_STATUS_IDLE,
				Database:          request.GetDatabase(),
				Workload:          request.GetWorkload(),
				CloudAutomationId: newAutomationId,
			}
			err = p.runRecordRepo.Insert(ctx, newRunRecord)
			if err != nil {
				return nil, connect.NewError(connect.CodeInternal, err)
			}
			return newRunRecord, nil
		},
	)
}
