package api

import (
	"connectrpc.com/connect"
	"context"
	"errors"
	"github.com/samber/lo"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/entity/ids"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/orm"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/sqlc"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/crossplane"

	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/postgresql/sqlerr"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/panel"
)

var ErrResourceNotFound = connect.NewError(connect.CodeNotFound, errors.New("resource not found"))

func (p *PanelService) sqlcResourceToProto(model *sqlc.GetResourceTreeRow) *panel.CloudResource {
	if model == nil {
		return nil
	}
	entity := &panel.CloudResource{
		Id:               ids.UlidFromString(model.ID),
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

func (p *PanelService) GetResource(ctx context.Context, ulid *panel.Ulid) (*panel.CloudResource_TreeNode, error) {
	resources, err := p.sqlcRepo.GetResourceTree(ctx, ulid.GetId())
	if err != nil {
		if sqlerr.IsNotFound(err) {
			return nil, ErrResourceNotFound
		}
		return nil, err
	}
	root, founded := lo.Find(resources, func(item *sqlc.GetResourceTreeRow) bool {
		return item.ID == ulid.GetId()
	})
	if !founded {
		return nil, ErrResourceNotFound
	}
	// Построить дерево из плоского списка ресурсов
	treeNode := p.buildResourceTree(resources, root)
	return treeNode, nil
}
