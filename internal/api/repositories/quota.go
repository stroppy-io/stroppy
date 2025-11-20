package repositories

import (
	"context"

	"github.com/samber/lo"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/orm"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/postgres/sqlexec"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/crossplane"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/panel"
)

type QuotaRepository struct {
	repository orm.QuotaTableRepository
}

func NewQuotaRepository(executor sqlexec.Executor) QuotaRepository {
	getter := func(ctx context.Context, operation orm.SqlOpType) orm.DB {
		return executor
	}
	return QuotaRepository{
		repository: orm.NewQuotaTableRepository(getter),
	}
}

func (q QuotaRepository) GetQuota(
	ctx context.Context,
	cloud crossplane.SupportedCloud,
	kind crossplane.Quota_Kind,
) (*crossplane.Quota, error) {
	quotaTable, err := q.repository.GetBy(ctx, orm.QuotaTable.SelectAll().Where(
		orm.QuotaTable.Cloud.Eq(int32(cloud)),
		orm.QuotaTable.Kind.Eq(int32(kind)),
	))
	if err != nil {
		return nil, err
	}
	return quotaTable.GetQuota(), nil
}

func (q QuotaRepository) FindQuotas(
	ctx context.Context,
	cloud crossplane.SupportedCloud,
	kinds []crossplane.Quota_Kind,
) ([]*crossplane.Quota, error) {
	quotaTables, err := q.repository.ListBy(ctx,
		orm.QuotaTable.SelectAll().Where(orm.QuotaTable.Cloud.Eq(int32(cloud)),
			orm.QuotaTable.Kind.Any(lo.Map(kinds, func(k crossplane.Quota_Kind, _ int) int32 { return int32(k) })...),
		))
	if err != nil {
		return nil, err
	}
	return lo.Map(quotaTables, func(item *panel.QuotaTable, index int) *crossplane.Quota {
		return item.GetQuota()
	}), nil
}

func (q QuotaRepository) IncrementQuota(
	ctx context.Context,
	cloud crossplane.SupportedCloud,
	kind crossplane.Quota_Kind,
	added uint32,
) error {
	return q.repository.Exec(ctx, orm.QuotaTable.Update().Set(
		orm.QuotaTable.Current.SetRaw("current + $1", added),
	).Where(
		orm.QuotaTable.Cloud.Eq(int32(cloud)),
		orm.QuotaTable.Kind.Eq(int32(kind)),
	))

}

func (q QuotaRepository) DecrementQuota(
	ctx context.Context,
	cloud crossplane.SupportedCloud,
	kind crossplane.Quota_Kind,
	subtracted uint32,
) error {
	return q.repository.Exec(ctx, orm.QuotaTable.Update().Set(
		orm.QuotaTable.Current.SetRaw("current - $1", subtracted),
	).Where(
		orm.QuotaTable.Cloud.Eq(int32(cloud)),
		orm.QuotaTable.Kind.Eq(int32(kind)),
	))
}
