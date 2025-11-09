package api

import (
	"context"
	"github.com/jackc/pgx/v5"

	"github.com/samber/lo"

	"github.com/stroppy-io/stroppy-cloud-panel/internal/entity/ids"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/sqlc"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/crossplane"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/panel"
	"github.com/yaroher/protoc-gen-pgx-orm/orm"
)

func (p *PanelService) sqlcResourceToProto(model *sqlc.GetResourceTreeRow) *panel.CloudResource {
	if model == nil {
		return nil
	}
	entity := &panel.CloudResource{
		Id:               ids.UlidFromString(model.ID),
		Status:           panel.CloudResource_Status(model.Status),
		ParentResourceId: ids.UlidFromStringPtr(model.ParentResourceID),
	}
	entity.Timing = &panel.Timing{
		CreatedAt: orm.TimestampFromTime(model.CreatedAt.Time),
		UpdatedAt: orm.TimestampFromTime(model.UpdatedAt.Time),
		DeletedAt: orm.TimestampFromPtrTime(timestamptzToTime(model.DeletedAt)),
	}
	entity.Resource = &crossplane.ResourceWithStatus{
		Ref:          orm.MessageFromSliceByte[*crossplane.Ref](model.Ref),
		ResourceDef:  orm.MessageFromSliceByte[*crossplane.ResourceDef](model.ResourceDef),
		ResourceYaml: model.ResourceYaml,
		Synced:       model.Synced,
		Ready:        model.Ready,
		ExternalId:   model.ExternalID,
	}

	return entity
}

// buildResourceTree строит дерево из плоского списка ресурсов
func (p *PanelService) buildResourceTree(resources []*sqlc.GetResourceTreeRow, root *sqlc.GetResourceTreeRow) *panel.CloudResource_TreeNode {
	// Создать карту детей
	childrenMap := make(map[string][]*sqlc.GetResourceTreeRow)
	for _, resource := range resources {
		if resource.ParentResourceID != nil {
			parentID := *resource.ParentResourceID
			childrenMap[parentID] = append(childrenMap[parentID], resource)
		}
	}

	// Рекурсивно создать узел дерева
	var buildNode func(resource *sqlc.GetResourceTreeRow) *panel.CloudResource_TreeNode
	buildNode = func(resource *sqlc.GetResourceTreeRow) *panel.CloudResource_TreeNode {
		children := childrenMap[resource.ID]
		treeChildren := make([]*panel.CloudResource_TreeNode, 0, len(children))

		for _, child := range children {
			treeChildren = append(treeChildren, buildNode(child))
		}

		return &panel.CloudResource_TreeNode{
			Id:       ids.UlidFromString(resource.ID),
			Resource: p.sqlcResourceToProto(resource),
			Children: treeChildren,
		}
	}

	return buildNode(root)
}

func convertResourceTreeByStatusesRowToGetResourceTreeRow(item *sqlc.GetResourceTreeByStatusesRow, _ int) *sqlc.GetResourceTreeRow {
	it := sqlc.GetResourceTreeRow(*item)
	return &it
}

func (p *PanelService) getResourceTreeByStatus(
	ctx context.Context,
	ulid *panel.Ulid,
	statuses []panel.CloudResource_Status,
) (*panel.CloudResource_TreeNode, error) {
	resources, err := p.sqlcRepo.GetResourceTreeByStatuses(ctx, &sqlc.GetResourceTreeByStatusesParams{
		ID: ulid.GetId(),
		Column2: lo.Map(statuses, func(status panel.CloudResource_Status, _ int) int32 {
			return int32(status)
		}),
	})
	if err != nil {
		return nil, err
	}
	if len(resources) == 0 {
		return nil, pgx.ErrNoRows
	}
	root, founded := lo.Find(resources, func(item *sqlc.GetResourceTreeByStatusesRow) bool {
		return item.ID == ulid.GetId()
	})
	if !founded {
		return nil, ErrResourceNotFound
	}
	treeNode := p.buildResourceTree(
		lo.Map(resources, convertResourceTreeByStatusesRowToGetResourceTreeRow),
		convertResourceTreeByStatusesRowToGetResourceTreeRow(root, 0),
	)
	return treeNode, nil
}
