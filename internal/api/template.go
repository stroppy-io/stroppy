package api

import (
	"context"

	"github.com/samber/lo"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/entity/ids"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/entity/kv"
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
		orm.Template.SelectAll().Where(
			orm.Template.Or(
				orm.Template.Name.Like("%"+req.Name+"%"),
				orm.Template.Raw("tags @> ?", serializeTags(req.GetTagsList().GetTags())),
			)))
	if err != nil {
		return nil, err
	}
	return &panel.Template_List{Templates: templates}, nil
}

func (p *PanelService) GetTemplateKvs(ctx context.Context, ulid *panel.Ulid) (*panel.KV_Map, error) {
	template, err := p.templateRepo.GetBy(
		ctx,
		orm.Template.
			Select(orm.Template.Id).
			Where(orm.Template.Id.Eq(ulid.GetId())),
	)
	if err != nil {
		return nil, err
	}
	stroppyDeployment := template.GetStroppyDeployment()
	if stroppyDeployment == nil {
		return nil, nil
	}
	cmdKvs := kv.ExtractKvValues(stroppyDeployment.GetCmd())
	filesKvs := make([]string, len(stroppyDeployment.GetFiles()))
	for _, file := range stroppyDeployment.GetFiles() {
		filesKvs = append(filesKvs, kv.ExtractKvValues(file.GetContent()).GetKeys()...)
	}
	filesKvs = append(filesKvs, cmdKvs.GetKeys()...)
	kvsInfos, err := p.kvInfoRepo.ListBy(
		ctx,
		orm.KvTable.SelectAll().Where(orm.KvTable.Key.In(filesKvs...)),
	)
	if err != nil {
		return nil, err
	}
	ret := lo.Map(kvsInfos, func(i *panel.KvTable, _ int) *panel.KV {
		return &panel.KV{
			Key:   i.GetKey(),
			Value: i.GetInfo().GetDefaultValue(),
			Info:  i.GetInfo(),
		}
	})
	return &panel.KV_Map{Kvs: ret}, nil
}

func (p *PanelService) GetTemplate(ctx context.Context, ulid *panel.Ulid) (*panel.Template, error) {
	return p.templateRepo.GetBy(ctx, orm.Template.SelectAll().Where(orm.Template.Id.Eq(ulid.GetId())))
}

func (p *PanelService) CreateTemplate(ctx context.Context, template *panel.Template) (*panel.Template, error) {
	user, err := p.getUserFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	template.AuthorId = user.Id
	template.Id = ids.NewUlid()
	return p.templateRepo.InsertRet(ctx, template)
}

func (p *PanelService) UpdateTemplate(ctx context.Context, template *panel.Template) (*panel.Template, error) {
	return p.templateRepo.UpdateRet(ctx, template, orm.Template.Id.Eq(template.GetId().GetId()))
}

func (p *PanelService) DeleteTemplate(ctx context.Context, ulid *panel.Ulid) (*emptypb.Empty, error) {
	return emptyErr(p.templateRepo.Exec(ctx, orm.Template.Delete().Where(orm.Template.Id.Eq(ulid.GetId()))))
}
