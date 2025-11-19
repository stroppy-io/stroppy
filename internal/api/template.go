package api

import (
	"context"

	"github.com/samber/lo"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/entity/ids"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/orm"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/panel"
)

func (p *PanelService) ListTemplatesTags(ctx context.Context, _ *emptypb.Empty) (*panel.Tag_List, error) {
	templates, err := p.templateRepo.ListBy(ctx, orm.Template.Select(orm.Template.Tags).
		Where(orm.Template.DeletedAt.IsNull()),
	)
	if err != nil {
		return nil, err
	}
	foundedTags := lo.Reduce(templates,
		func(acc []*panel.Tag, t *panel.Template, _ int) []*panel.Tag {
			return append(acc, t.GetTags()...)
		},
		[]*panel.Tag{},
	)
	return &panel.Tag_List{Tags: foundedTags}, nil
}

func (p *PanelService) SearchTemplates(ctx context.Context, req *panel.SearchTemplatesRequest) (*panel.Template_List, error) {
	templates, err := p.templateRepo.ListBy(ctx,
		orm.Template.Or(
			orm.Template.SelectAll().Where(orm.Template.Name.Like("%"+req.Name+"%")),
			orm.Template.SelectAll().Where(orm.Template.Raw("tags @> ?", serializeTags(req.GetTagsList().GetTags()))),
		),
	)
	if err != nil {
		return nil, err
	}
	return &panel.Template_List{Templates: templates}, nil
}

func (p *PanelService) CreateTemplate(ctx context.Context, template *panel.Template) (*panel.Template, error) {
	template.Id = ids.NewUlid()
	return p.templateRepo.InsertRet(ctx, template)
}

func (p *PanelService) UpdateTemplate(ctx context.Context, template *panel.Template) (*panel.Template, error) {
	return p.templateRepo.UpdateRet(ctx, template, orm.Template.Id.Eq(template.GetId().GetId()))
}

func (p *PanelService) DeleteTemplate(ctx context.Context, ulid *panel.Ulid) (*emptypb.Empty, error) {
	return emptyErr(p.templateRepo.Exec(ctx, orm.Template.Delete().Where(orm.Template.Id.Eq(ulid.GetId()))))
}
