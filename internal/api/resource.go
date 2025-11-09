package api

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"github.com/samber/lo"

	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/postgresql/sqlerr"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/sqlc"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/panel"
)

var ErrResourceNotFound = connect.NewError(connect.CodeNotFound, errors.New("resource not found"))

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
	treeNode := p.buildResourceTree(resources, root)
	return treeNode, nil
}
