package api

import (
	"context"
	"time"

	"github.com/samber/lo"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/entity/timestamps"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/orm"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/panel"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func (p *PanelService) ListKvs(ctx context.Context, e *emptypb.Empty) (*panel.KV_Map, error) {
	kvs, err := p.kvInfoRepo.ListBy(ctx, orm.KvTable.SelectAll().Where(orm.KvTable.DeletedAt.IsNull()))
	if err != nil {
		return nil, err
	}
	return &panel.KV_Map{Kvs: lo.Map(kvs, func(item *panel.KvTable, index int) *panel.KV {
		return &panel.KV{
			Key:   item.Key,
			Value: item.Info.GetDefaultValue(),
			Info:  item.Info,
		}
	})}, nil
}

func (p *PanelService) PutKv(ctx context.Context, info *panel.KV) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, p.kvInfoRepo.Insert(ctx, &panel.KvTable{
		Timing: timestamps.NewTiming(),
		Key:    info.Key,
		Info:   info.Info,
	})
}

func (p *PanelService) UpdateKv(ctx context.Context, info *panel.KV) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, p.kvInfoRepo.Exec(
		ctx,
		p.kvInfoRepo.Table().
			Update().
			Set(orm.KvTable.UpdatedAt.Set(time.Now())).
			Set(orm.KvTable.Info.Set(orm.MessageToSliceByte(info.GetInfo()))).
			Where(orm.KvTable.Key.Eq(info.Key)),
	)
}

func (p *PanelService) DeleteKv(ctx context.Context, key *wrapperspb.StringValue) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, p.kvInfoRepo.Exec(ctx, p.kvInfoRepo.Table().Delete().Where(orm.KvTable.Key.Eq(key.GetValue())))
}
